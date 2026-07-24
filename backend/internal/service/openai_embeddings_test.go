//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type embeddingUpstreamStep struct {
	status int
	body   string
	err    error
}

type embeddingUpstreamCall struct {
	body   []byte
	header http.Header
	ip     net.IP
}

type embeddingUpstreamStub struct {
	steps []embeddingUpstreamStep
	calls []embeddingUpstreamCall
}

func (s *embeddingUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, errors.New("unexpected generic upstream call")
}

func (s *embeddingUpstreamStub) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return nil, errors.New("unexpected TLS upstream call")
}

func (s *embeddingUpstreamStub) DoEmbedding(req *http.Request, policy EmbeddingUpstreamPolicy) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	headers := req.Header.Clone()
	s.calls = append(s.calls, embeddingUpstreamCall{
		body:   body,
		header: headers,
		ip:     append(net.IP(nil), policy.ValidatedIP...),
	})
	step := embeddingUpstreamStep{status: http.StatusOK, body: `{"data":[{"embedding":[0.1],"index":0}],"usage":{"prompt_tokens":1}}`}
	if len(s.steps) > 0 {
		step = s.steps[0]
		s.steps = s.steps[1:]
	}
	if step.err != nil {
		return nil, step.err
	}
	return &http.Response{
		StatusCode: step.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(step.body)),
	}, nil
}

func newEmbeddingForwardTestService(t *testing.T, accounts []Account, upstream *embeddingUpstreamStub) *OpenAIGatewayService {
	t.Helper()
	price := 2e-6
	svc := newEmbeddingEligibilityTestService(t, accounts, Channel{ModelPricing: []ChannelModelPricing{{
		Platform:   PlatformEmbedding,
		Models:     []string{"embed-public"},
		InputPrice: &price,
	}}}, nil)
	svc.cfg.Gateway.Embedding = config.EmbeddingGatewayConfig{
		AllowedHosts:             []string{"embedding.example.test"},
		RequestMaxBytes:          4096,
		ResponseMaxBytes:         4096,
		MaxJSONDepth:             16,
		MaxInputItems:            16,
		MaxInputItemBytes:        256,
		MaxTokenValue:            2147483647,
		UpstreamTimeoutSeconds:   5,
		ResponseHeaderTimeoutSec: 2,
		MaxConcurrentRequests:    2,
	}
	svc.httpUpstream = upstream
	return svc
}

func withEmbeddingDNS(t *testing.T, ips ...net.IP) {
	t.Helper()
	previous := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) { return ips, nil }
	t.Cleanup(func() { lookupEmbeddingHostIP = previous })
}

func TestForwardEmbeddingsMapsOnlyOutboundModelAndRestoresPublicModel(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{{
		status: http.StatusOK,
		body:   `{"object":"list","data":[{"object":"embedding","embedding":"AQID","index":0}],"model":"upstream-embed","usage":{"prompt_tokens":7,"total_tokens":7}}`,
	}}}
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-embed"}),
	}, upstream)
	groupID := embeddingEligibilityTestGroupID

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{
		GroupID: &groupID,
		Body:    []byte(`{"model":"embed-public","input":["first","second"],"encoding_format":"base64","dimensions":3,"user":"client"}`),
	})
	require.NoError(t, err)
	require.Len(t, upstream.calls, 1)
	require.Equal(t, "upstream-embed", gjson.GetBytes(upstream.calls[0].body, "model").String())
	require.Equal(t, "base64", gjson.GetBytes(upstream.calls[0].body, "encoding_format").String())
	require.Equal(t, "Bearer upstream-key", upstream.calls[0].header.Get("Authorization"))
	require.Equal(t, net.ParseIP("8.8.8.8").String(), upstream.calls[0].ip.String())
	require.Equal(t, "embed-public", gjson.GetBytes(result.Body, "model").String())
	require.Equal(t, "AQID", gjson.GetBytes(result.Body, "data.0.embedding").String())
	require.Equal(t, 7, result.PromptTokens)
	require.Equal(t, "application/json", result.Headers.Get("Content-Type"))
}

func TestForwardEmbeddingsFailsClosedForInvalidUsageWithoutFailover(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{{
		status: http.StatusOK,
		body:   `{"data":[{"embedding":[0.1]}],"model":"upstream","usage":{"prompt_tokens":0}}`,
	}}}
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-a"}),
		embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-b"}),
	}, upstream)
	groupID := embeddingEligibilityTestGroupID

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)})
	require.Nil(t, result)
	require.Error(t, err)
	var upstreamErr *EmbeddingForwardError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, "invalid_usage", upstreamErr.Category)
	require.False(t, upstreamErr.Retryable)
	require.Len(t, upstream.calls, 1)
}

func TestForwardEmbeddingsRejectsMalformedEmbeddingDataWithoutFailover(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	tests := []struct {
		name string
		body string
	}{
		{name: "missing data", body: `{"usage":{"prompt_tokens":1}}`},
		{name: "empty data", body: `{"data":[],"usage":{"prompt_tokens":1}}`},
		{name: "missing embedding", body: `{"data":[{"index":0}],"usage":{"prompt_tokens":1}}`},
		{name: "empty vector", body: `{"data":[{"embedding":[]}],"usage":{"prompt_tokens":1}}`},
		{name: "invalid vector", body: `{"data":[{"embedding":[0.1,"bad"]}],"usage":{"prompt_tokens":1}}`},
		{name: "empty base64", body: `{"data":[{"embedding":""}],"usage":{"prompt_tokens":1}}`},
		{name: "invalid base64", body: `{"data":[{"embedding":"***"}],"usage":{"prompt_tokens":1}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{{status: http.StatusOK, body: tt.body}}}
			svc := newEmbeddingForwardTestService(t, []Account{
				embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-a"}),
				embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-b"}),
			}, upstream)
			groupID := embeddingEligibilityTestGroupID

			result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)})
			require.Nil(t, result)
			var upstreamErr *EmbeddingForwardError
			require.ErrorAs(t, err, &upstreamErr)
			require.Equal(t, "invalid_response", upstreamErr.Category)
			require.False(t, upstreamErr.Retryable)
			require.Len(t, upstream.calls, 1)
		})
	}
}

func TestForwardEmbeddingsSkipsIneligibleAccountWithoutConsumingAttempt(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	ineligible := embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-ineligible"})
	ineligible.Type = AccountTypeOAuth
	ineligible.Priority = 0
	eligible := embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-eligible"})
	eligible.Priority = 100
	upstream := &embeddingUpstreamStub{}
	svc := newEmbeddingForwardTestService(t, []Account{ineligible, eligible}, upstream)
	groupID := embeddingEligibilityTestGroupID

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.calls, 1)
	require.Equal(t, "upstream-eligible", gjson.GetBytes(upstream.calls[0].body, "model").String())
}

func TestForwardEmbeddingsFailsOverOnlyForExplicitRetryableStatus(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{
		{status: http.StatusTooManyRequests, body: `{"error":"rate limited"}`},
		{status: http.StatusOK, body: `{"data":[{"embedding":[0.1]}],"model":"upstream-b","usage":{"prompt_tokens":3}}`},
	}}
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-a"}),
		embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-b"}),
	}, upstream)
	groupID := embeddingEligibilityTestGroupID

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":[1,2,3]}`)})
	require.NoError(t, err)
	require.Len(t, upstream.calls, 2)
	require.ElementsMatch(t, []string{"upstream-a", "upstream-b"}, []string{
		gjson.GetBytes(upstream.calls[0].body, "model").String(),
		gjson.GetBytes(upstream.calls[1].body, "model").String(),
	})
	require.Equal(t, "embed-public", gjson.GetBytes(result.Body, "model").String())
}

func TestForwardEmbeddingsNeverFailsOverAfterUnknownTransportWrite(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{{err: errors.New("connection reset")}}}
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-a"}),
		embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-b"}),
	}, upstream)
	groupID := embeddingEligibilityTestGroupID

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)})
	require.Nil(t, result)
	var upstreamErr *EmbeddingForwardError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, "transport_unknown", upstreamErr.Category)
	require.Len(t, upstream.calls, 1)
}

func TestForwardEmbeddingsFailsOverWhenTransportProvesNoRequestWrite(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{
		{err: &EmbeddingTransportError{RequestNotWritten: true}},
		{status: http.StatusOK, body: `{"data":[{"embedding":[0.1]}],"usage":{"prompt_tokens":2}}`},
	}}
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-a"}),
		embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-b"}),
	}, upstream)
	groupID := embeddingEligibilityTestGroupID

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.calls, 2)
}

func TestEmbeddingInputValidationAcceptsOpenAIShapesAndRejectsUnsafeValues(t *testing.T) {
	limits := embeddingForwardLimits{requestMaxBytes: 4096, maxJSONDepth: 8, maxInputItems: 4, maxInputItemBytes: 16, maxTokenValue: 2147483647}
	for _, body := range [][]byte{
		[]byte(`{"model":"embed-public","input":"text"}`),
		[]byte(`{"model":"embed-public","input":["one","two"]}`),
		[]byte(`{"model":"embed-public","input":[1,2,3]}`),
		[]byte(`{"model":"embed-public","input":[[1,2],[3]]}`),
	} {
		_, err := validateEmbeddingRequest(body, limits)
		require.NoError(t, err)
	}
	for _, body := range [][]byte{
		[]byte(`{"model":"embed-public","input":[1,"two"]}`),
		[]byte(`{"model":"embed-public","input":[1.5]}`),
		[]byte(`{"model":"embed-public","input":[-1]}`),
		[]byte(`{"model":"embed-public","input":[2147483648]}`),
		[]byte(`{"model":"embed-public","input":{"nested":"unsupported"}}`),
	} {
		_, err := validateEmbeddingRequest(body, limits)
		require.Error(t, err)
	}
}

func TestEmbeddingTargetValidationRejectsPrivateDNSAndBuildsEndpointOnce(t *testing.T) {
	limits := embeddingForwardLimits{allowedHosts: []string{"embedding.example.test"}}
	previous := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.1")}, nil }
	t.Cleanup(func() { lookupEmbeddingHostIP = previous })
	_, _, err := resolveEmbeddingUpstreamTarget(context.Background(), "https://embedding.example.test/v1", limits)
	require.Error(t, err)

	require.Equal(t, "https://embedding.example.test/v1/embeddings", buildOpenAIEmbeddingsURL("https://embedding.example.test"))
	require.Equal(t, "https://embedding.example.test/v1/embeddings", buildOpenAIEmbeddingsURL("https://embedding.example.test/v1/"))
	require.Equal(t, "https://embedding.example.test/v1/embeddings", buildOpenAIEmbeddingsURL("https://embedding.example.test/v1/embeddings"))
}

func TestEmbeddingTargetValidationPrivateCIDRAndMixedDNS(t *testing.T) {
	limits := embeddingForwardLimits{
		allowedHosts:       []string{"embedding.example.test"},
		allowedPrivateCIDR: []string{"10.10.0.0/16"},
	}
	previous := lookupEmbeddingHostIP
	t.Cleanup(func() { lookupEmbeddingHostIP = previous })

	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.10.1.2")}, nil }
	_, pinned, err := resolveEmbeddingUpstreamTarget(context.Background(), "https://embedding.example.test", limits)
	require.NoError(t, err)
	require.Equal(t, "10.10.1.2", pinned.String())

	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.11.1.2")}, nil }
	_, _, err = resolveEmbeddingUpstreamTarget(context.Background(), "https://embedding.example.test", limits)
	require.Error(t, err)

	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("127.0.0.1")}, nil
	}
	_, _, err = resolveEmbeddingUpstreamTarget(context.Background(), "https://embedding.example.test", limits)
	require.Error(t, err)
}

func TestEmbeddingIPPolicyRejectsReservedAndMetadataRanges(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1",
		"169.254.169.254",
		"100.64.0.1",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"2001:db8::1",
	} {
		require.Falsef(t, isAllowedEmbeddingIP(net.ParseIP(raw), nil), "address %s must be blocked", raw)
	}
	require.True(t, isAllowedEmbeddingIP(net.ParseIP("8.8.8.8"), nil))
}

func TestForwardEmbeddingsBoundsDNSResolutionByAttemptTimeout(t *testing.T) {
	previous := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(ctx context.Context, _ string) ([]net.IP, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() { lookupEmbeddingHostIP = previous })
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream"}),
	}, &embeddingUpstreamStub{})
	svc.cfg.Gateway.Embedding.UpstreamTimeoutSeconds = 1
	groupID := embeddingEligibilityTestGroupID
	started := time.Now()

	result, err := svc.ForwardEmbeddings(context.Background(), EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)})
	require.Nil(t, result)
	var upstreamErr *EmbeddingForwardError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, "unsafe_upstream", upstreamErr.Category)
	require.Less(t, time.Since(started), 2*time.Second)
}

func TestEmbeddingLimitsClampValuesWhenConfigValidationIsBypassed(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{Embedding: config.EmbeddingGatewayConfig{
		RequestMaxBytes:          config.EmbeddingRequestMaxBytesHardLimit + 1,
		ResponseMaxBytes:         config.EmbeddingResponseMaxBytesHardLimit + 1,
		MaxJSONDepth:             config.EmbeddingMaxJSONDepthHardLimit + 1,
		MaxInputItems:            config.EmbeddingMaxInputItemsHardLimit + 1,
		MaxInputItemBytes:        config.EmbeddingMaxInputItemBytesHardLimit + 1,
		MaxTokenValue:            config.EmbeddingMaxTokenValueHardLimit + 1,
		UpstreamTimeoutSeconds:   config.EmbeddingUpstreamTimeoutSecondsHardLimit + 1,
		ResponseHeaderTimeoutSec: config.EmbeddingResponseHeaderTimeoutSecHardLimit + 1,
		MaxConcurrentRequests:    config.EmbeddingMaxConcurrentRequestsHardLimit + 1,
	}}}}
	limits := svc.embeddingLimits()
	require.Equal(t, config.EmbeddingRequestMaxBytesHardLimit, limits.requestMaxBytes)
	require.Equal(t, config.EmbeddingResponseMaxBytesHardLimit, limits.responseMaxBytes)
	require.Equal(t, config.EmbeddingMaxJSONDepthHardLimit, limits.maxJSONDepth)
	require.Equal(t, config.EmbeddingMaxInputItemsHardLimit, limits.maxInputItems)
	require.Equal(t, config.EmbeddingMaxInputItemBytesHardLimit, limits.maxInputItemBytes)
	require.Equal(t, config.EmbeddingMaxTokenValueHardLimit, limits.maxTokenValue)
	require.Equal(t, time.Duration(config.EmbeddingUpstreamTimeoutSecondsHardLimit)*time.Second, limits.timeout)
	require.Equal(t, time.Duration(config.EmbeddingResponseHeaderTimeoutSecHardLimit)*time.Second, limits.headerTimeout)
	require.Equal(t, config.EmbeddingMaxConcurrentRequestsHardLimit, limits.maxConcurrent)
}

func TestForwardEmbeddingsReleasesGlobalSlotAfterResponseLimitFailure(t *testing.T) {
	withEmbeddingDNS(t, net.ParseIP("8.8.8.8"))
	upstream := &embeddingUpstreamStub{steps: []embeddingUpstreamStep{
		{status: http.StatusOK, body: strings.Repeat("x", 5000)},
		{status: http.StatusOK, body: `{"data":[{"embedding":[0.1]}],"usage":{"prompt_tokens":1}}`},
	}}
	svc := newEmbeddingForwardTestService(t, []Account{
		embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream"}),
	}, upstream)
	svc.cfg.Gateway.Embedding.MaxConcurrentRequests = 1
	groupID := embeddingEligibilityTestGroupID
	input := EmbeddingForwardInput{GroupID: &groupID, Body: []byte(`{"model":"embed-public","input":"safe"}`)}

	first, err := svc.ForwardEmbeddings(context.Background(), input)
	require.Nil(t, first)
	var forwardErr *EmbeddingForwardError
	require.ErrorAs(t, err, &forwardErr)
	require.Equal(t, "response_read", forwardErr.Category)

	second, err := svc.ForwardEmbeddings(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, second)
}

func TestParseEmbeddingPromptTokensRequiresPositiveInteger(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"usage":{"prompt_tokens":1}}`),
	} {
		tokens, err := parseEmbeddingPromptTokens(body, 8)
		require.NoError(t, err)
		require.Equal(t, 1, tokens)
	}
	for _, body := range [][]byte{
		[]byte(`{"usage":{}}`),
		[]byte(`{"usage":{"prompt_tokens":null}}`),
		[]byte(`{"usage":{"prompt_tokens":"1"}}`),
		[]byte(`{"usage":{"prompt_tokens":1.5}}`),
		[]byte(`{"usage":{"prompt_tokens":0}}`),
		[]byte(`{"usage":{"prompt_tokens":-1}}`),
	} {
		_, err := parseEmbeddingPromptTokens(body, 8)
		require.Error(t, err)
	}
}
