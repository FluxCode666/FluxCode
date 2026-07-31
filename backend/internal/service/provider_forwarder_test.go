package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type providerForwardUpstreamStub struct {
	request *http.Request
	body    string
	policy  ProviderUpstreamPolicy
	status  int
	result  string
	header  http.Header
	calls   int
}

func (s *providerForwardUpstreamStub) Do(*http.Request, string, int64, int) (*http.Response, error) {
	panic("provider forwarding must use the destination-pinned transport")
}

func (s *providerForwardUpstreamStub) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	panic("not used")
}

func (s *providerForwardUpstreamStub) DoProvider(req *http.Request, policy ProviderUpstreamPolicy) (*http.Response, error) {
	s.calls++
	s.request = req
	s.policy = policy
	body, _ := io.ReadAll(req.Body)
	s.body = string(body)
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	header := s.header
	if header == nil {
		header = http.Header{"Content-Type": []string{"application/json"}}
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(s.result)),
	}, nil
}

func providerForwardCandidate(protocol ProtocolFamily) RouteCandidate {
	capability := routeCapability(44, 4400+int64(protocol[0]), protocol, false)
	capability.Profile.Connection = ProviderConnectionConfig{
		BaseURL:  "https://upstream.example/api",
		AuthType: "bearer",
		Headers:  map[string]string{"X-Tenant": "tenant-a"},
	}
	capability.Account.Credentials = map[string]any{"api_key": "upstream-secret"}
	capability.Endpoint.Path = protocol.DefaultPath()
	capability.Capability.UpstreamModel = "vendor-model"
	return NewNativeRouteCandidate(capability, protocol)
}

func TestProviderForwarderSendsChatDirectlyToNativeChatEndpoint(t *testing.T) {
	upstream := &providerForwardUpstreamStub{result: `{
		"id":"chatcmpl_1","object":"chat.completion","model":"vendor-model",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
	}`}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.10"))
	defer restore()

	result, err := forwarder.ForwardChat(context.Background(), ProviderForwardInput{
		Candidate: providerForwardCandidate(ProtocolChatCompletions),
		Body:      []byte(`{"model":"logical-model","messages":[{"role":"user","content":"hi"}]}`),
		Headers:   http.Header{"Authorization": []string{"Bearer client-secret"}},
	})

	require.NoError(t, err)
	require.Equal(t, "https://upstream.example/api/v1/chat/completions", upstream.request.URL.String())
	require.JSONEq(t, `{"model":"vendor-model","messages":[{"role":"user","content":"hi"}]}`, upstream.body)
	require.Equal(t, "Bearer upstream-secret", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "tenant-a", upstream.request.Header.Get("X-Tenant"))
	require.NotContains(t, upstream.request.Header.Values("Authorization"), "client-secret")
	require.JSONEq(t, `{
		"id":"chatcmpl_1","object":"chat.completion","model":"model-a",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
	}`, string(result.Body))
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.True(t, result.Usage.Complete)
}

func TestProviderForwarderCollectsInjectedChatStreamUsageWithoutExposingExtraChunk(t *testing.T) {
	upstream := &providerForwardUpstreamStub{result: strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"vendor-model","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		"",
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"vendor-model","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.16"))
	defer restore()
	candidate := providerForwardCandidate(ProtocolChatCompletions)
	candidate.Capability.FeatureProfile = FeatureProfileStreamText

	result, err := forwarder.ForwardChatStream(context.Background(), ProviderForwardInput{
		Candidate: candidate,
		Body:      []byte(`{"model":"logical-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`),
	})
	require.NoError(t, err)
	defer func() { _ = result.Body.Close() }()

	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.True(t, gjson.Get(upstream.body, "stream_options.include_usage").Bool())
	require.Contains(t, string(body), `"content":"hello"`)
	require.NotContains(t, string(body), `"prompt_tokens":3`)
	require.Equal(t, 3, result.Usage().InputTokens)
	require.Equal(t, 2, result.Usage().OutputTokens)
	require.True(t, result.Usage().Complete)
}

func TestProviderForwarderSendsMessagesWithConfiguredBearerDialect(t *testing.T) {
	upstream := &providerForwardUpstreamStub{result: `{
		"id":"msg_1","type":"message","role":"assistant","model":"vendor-model",
		"content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn",
		"usage":{"input_tokens":4,"output_tokens":3}
	}`}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	restore := stubProviderDNS(t, net.ParseIP("2001:db8::10"))
	defer restore()
	candidate := providerForwardCandidate(ProtocolAnthropicMessages)
	candidate.Endpoint.WireProfile = WireProfileSiliconFlowMessages

	result, err := forwarder.ForwardMessages(context.Background(), ProviderForwardInput{
		Candidate: candidate,
		Body:      []byte(`{"model":"model-a","max_tokens":128,"messages":[{"role":"user","content":"hi"}]}`),
		Headers: http.Header{
			"Anthropic-Version": []string{"2023-06-01"},
			"X-Api-Key":         []string{"client-secret"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "/api/v1/messages", upstream.request.URL.Path)
	require.Equal(t, "Bearer upstream-secret", upstream.request.Header.Get("Authorization"))
	require.Empty(t, upstream.request.Header.Get("X-Api-Key"))
	require.Equal(t, "2023-06-01", upstream.request.Header.Get("Anthropic-Version"))
	require.Contains(t, upstream.body, `"model":"vendor-model"`)
	require.Contains(t, string(result.Body), `"model":"model-a"`)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}

func TestProviderForwarderPreservesNativeResponsesItems(t *testing.T) {
	upstreamBody := `{
		"id":"resp_1","object":"response","status":"completed","model":"vendor-model",
		"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],
		"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12}
	}`
	upstream := &providerForwardUpstreamStub{result: upstreamBody}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.11"))
	defer restore()

	result, err := forwarder.ForwardResponses(context.Background(), ProviderForwardInput{
		Candidate: providerForwardCandidate(ProtocolResponses),
		Body:      []byte(`{"model":"model-a","input":"hello"}`),
	})

	require.NoError(t, err)
	require.Contains(t, string(result.Body), `"type":"message"`)
	require.Contains(t, string(result.Body), `"type":"output_text"`)
	require.Contains(t, string(result.Body), `"model":"model-a"`)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
}

func TestProviderForwarderRejectsNonNativeProtocolAndUnsafeDestination(t *testing.T) {
	upstream := &providerForwardUpstreamStub{}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	candidate := providerForwardCandidate(ProtocolChatCompletions)
	candidate.Tier = RouteTierConversion

	_, err := forwarder.ForwardChat(context.Background(), ProviderForwardInput{
		Candidate: candidate,
		Body:      []byte(`{"model":"model-a","messages":[]}`),
	})
	require.ErrorIs(t, err, ErrProviderProtocolMismatch)

	candidate.Tier = RouteTierNative
	restore := stubProviderDNS(t, net.ParseIP("127.0.0.1"))
	defer restore()
	_, err = forwarder.ForwardChat(context.Background(), ProviderForwardInput{
		Candidate: candidate,
		Body:      []byte(`{"model":"model-a","messages":[]}`),
	})
	require.ErrorIs(t, err, ErrUnsafeProviderDestination)
	require.Zero(t, upstream.calls)
}

func TestProviderForwarderBoundsBufferedResponse(t *testing.T) {
	upstream := &providerForwardUpstreamStub{result: strings.Repeat("x", 65)}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{
		ResponseBodyMaxBytes: 64,
		RequestTimeout:       time.Second,
	})
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.12"))
	defer restore()

	_, err := forwarder.ForwardChat(context.Background(), ProviderForwardInput{
		Candidate: providerForwardCandidate(ProtocolChatCompletions),
		Body:      []byte(`{"model":"model-a","messages":[]}`),
	})

	require.ErrorIs(t, err, ErrProviderResponseTooLarge)
}

func TestProviderForwarderValidatesEmbeddingResponses(t *testing.T) {
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.15"))
	defer restore()

	validUpstream := &providerForwardUpstreamStub{result: `{
		"object":"list","model":"vendor-model",
		"data":[{"object":"embedding","index":0,"embedding":[0.1,-0.2,0.3]}],
		"usage":{"prompt_tokens":4,"total_tokens":4}
	}`}
	result, err := NewProviderForwarder(validUpstream, ProviderForwarderOptions{}).ForwardEmbeddings(
		context.Background(), ProviderForwardInput{
			Candidate: providerForwardCandidate(ProtocolEmbeddings),
			Body:      []byte(`{"model":"model-a","input":"hello"}`),
		},
	)
	require.NoError(t, err)
	require.Equal(t, 4, result.Usage.InputTokens)
	require.True(t, result.Usage.Complete)
	require.Contains(t, string(result.Body), `"model":"model-a"`)

	invalidBodies := []string{
		`not-json`,
		`{"data":[],"usage":{"prompt_tokens":4}}`,
		`{"data":[{"embedding":[]}],"usage":{"prompt_tokens":4}}`,
		`{"data":[{"embedding":[0.1,"bad"]}],"usage":{"prompt_tokens":4}}`,
		`{"data":[{"embedding":[0.1]}]}`,
	}
	for _, body := range invalidBodies {
		upstream := &providerForwardUpstreamStub{result: body}
		_, err := NewProviderForwarder(upstream, ProviderForwarderOptions{}).ForwardEmbeddings(
			context.Background(), ProviderForwardInput{
				Candidate: providerForwardCandidate(ProtocolEmbeddings),
				Body:      []byte(`{"model":"model-a","input":"hello"}`),
			},
		)
		require.ErrorIs(t, err, ErrProviderProtocolMismatch)
	}
}

func TestProviderForwarderStreamsNativeChatSSEAndCapturesUsage(t *testing.T) {
	upstream := &providerForwardUpstreamStub{
		result: "data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"vendor-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n" +
			"data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"vendor-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
			"data: [DONE]\n\n",
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{})
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.13"))
	defer restore()
	candidate := providerForwardCandidate(ProtocolChatCompletions)
	candidate.Capability.FeatureProfile = FeatureProfileStreamText

	result, err := forwarder.ForwardChatStream(context.Background(), ProviderForwardInput{
		Candidate: candidate,
		Body:      []byte(`{"model":"model-a","messages":[{"role":"user","content":"hi"}],"stream":true}`),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = result.Body.Close() })

	body, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"model":"model-a"`)
	require.NotContains(t, string(body), `"model":"vendor-model"`)
	require.Contains(t, string(body), "data: [DONE]")
	usage := result.Usage()
	require.Equal(t, 3, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
	require.True(t, usage.Complete)
}

func TestProviderForwarderRejectsOversizedSSEEvent(t *testing.T) {
	upstream := &providerForwardUpstreamStub{
		result: "data: {\"value\":\"" + strings.Repeat("x", 80) + "\"}\n\n",
		header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	forwarder := NewProviderForwarder(upstream, ProviderForwarderOptions{StreamEventMaxBytes: 32})
	restore := stubProviderDNS(t, net.ParseIP("203.0.113.14"))
	defer restore()
	candidate := providerForwardCandidate(ProtocolChatCompletions)
	candidate.Capability.FeatureProfile = FeatureProfileStreamText

	result, err := forwarder.ForwardChatStream(context.Background(), ProviderForwardInput{
		Candidate: candidate,
		Body:      []byte(`{"model":"model-a","messages":[],"stream":true}`),
	})
	require.NoError(t, err)
	defer func() { _ = result.Body.Close() }()
	_, err = io.ReadAll(result.Body)
	require.ErrorIs(t, err, ErrProviderStreamEventTooLarge)
}

func stubProviderDNS(t *testing.T, ips ...net.IP) func() {
	t.Helper()
	previous := lookupProviderHostIPs
	lookupProviderHostIPs = func(context.Context, string) ([]net.IP, error) {
		return ips, nil
	}
	return func() { lookupProviderHostIPs = previous }
}
