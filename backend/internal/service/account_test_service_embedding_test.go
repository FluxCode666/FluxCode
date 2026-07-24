//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type embeddingAccountTestUpstream struct {
	queuedHTTPUpstream
	request *http.Request
	policy  EmbeddingUpstreamPolicy
}

func (u *embeddingAccountTestUpstream) DoEmbedding(req *http.Request, _ int64, _ int, policy EmbeddingUpstreamPolicy) (*http.Response, error) {
	u.request, u.policy = req, policy
	if len(u.responses) == 0 {
		return nil, &EmbeddingTransportError{}
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func TestAccountTestService_EmbeddingUsesBearerMappedModelAndDiscardsVector(t *testing.T) {
	originalLookup := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("203.0.113.10")}, nil }
	t.Cleanup(func() { lookupEmbeddingHostIP = originalLookup })

	account := &Account{ID: 71, Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"base_url": "https://embedding.example.com/v1", "api_key": "upstream-canary",
		"model_mapping": map[string]any{"public-embed": "upstream-embed"},
	}}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{71: account}}
	upstream := &embeddingAccountTestUpstream{queuedHTTPUpstream: queuedHTTPUpstream{responses: []*http.Response{{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"object":"list","data":[{"embedding":[0.1,0.2],"index":0}],"usage":{"prompt_tokens":1}}`)),
	}}}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{Gateway: config.GatewayConfig{Embedding: config.EmbeddingGatewayConfig{AllowedHosts: []string{"embedding.example.com"}}}}}
	c, recorder := newTestContext()
	require.NoError(t, svc.TestAccountConnection(c, 71, "public-embed", "user-content-canary"))
	require.Equal(t, "Bearer upstream-canary", upstream.request.Header.Get("Authorization"))
	var body map[string]any
	require.NoError(t, json.NewDecoder(upstream.request.Body).Decode(&body))
	require.Equal(t, "upstream-embed", body["model"])
	require.NotEqual(t, "user-content-canary", body["input"])
	require.Contains(t, recorder.Body.String(), "test_complete")
	require.NotContains(t, recorder.Body.String(), "0.1")
	require.NotContains(t, recorder.Body.String(), "upstream-canary")
}

func TestAccountTestService_EmbeddingAcceptsBase64AndRejectsInvalidUsageWithoutBodyLeak(t *testing.T) {
	originalLookup := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("203.0.113.10")}, nil }
	t.Cleanup(func() { lookupEmbeddingHostIP = originalLookup })
	account := &Account{ID: 72, Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"base_url": "https://embedding.example.com", "api_key": "secret", "model_whitelist": []any{"embed"},
	}}
	for _, tc := range []struct {
		name, body string
		success    bool
	}{
		{"base64", `{"data":[{"embedding":"AQID","index":0}],"usage":{"prompt_tokens":2}}`, true},
		{"zero-usage", `{"data":[{"embedding":"vector-canary","index":0}],"usage":{"prompt_tokens":0},"echo":"body-canary"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{72: account}}
			upstream := &embeddingAccountTestUpstream{queuedHTTPUpstream: queuedHTTPUpstream{responses: []*http.Response{{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}}}}
			svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{Gateway: config.GatewayConfig{Embedding: config.EmbeddingGatewayConfig{AllowedHosts: []string{"embedding.example.com"}}}}}
			c, recorder := newTestContext()
			err := svc.TestAccountConnection(c, 72, "embed", "")
			if tc.success {
				require.NoError(t, err)
				require.Contains(t, recorder.Body.String(), "test_complete")
			} else {
				require.Error(t, err)
				require.Contains(t, recorder.Body.String(), "embedding_test_invalid_usage")
			}
			require.NotContains(t, recorder.Body.String(), "vector-canary")
			require.NotContains(t, recorder.Body.String(), "body-canary")
		})
	}
}

func TestAccountTestService_EmbeddingRejectsUnsafeURLBeforeSendingBearer(t *testing.T) {
	account := &Account{ID: 73, Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://127.0.0.1/v1", "api_key": "bearer-canary", "model_whitelist": []any{"embed"},
	}}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{73: account}}
	upstream := &embeddingAccountTestUpstream{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{Gateway: config.GatewayConfig{Embedding: config.EmbeddingGatewayConfig{AllowedHosts: []string{"127.0.0.1"}}}}}
	c, recorder := newTestContext()
	require.Error(t, svc.TestAccountConnection(c, 73, "embed", ""))
	require.Nil(t, upstream.request)
	require.Contains(t, recorder.Body.String(), "embedding_test_unsafe_upstream")
	require.NotContains(t, recorder.Body.String(), "bearer-canary")
}

func TestAccountTestService_EmbeddingBackgroundResultStoresOnlyFixedCategory(t *testing.T) {
	originalLookup := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10")}, nil
	}
	t.Cleanup(func() { lookupEmbeddingHostIP = originalLookup })

	account := &Account{ID: 74, Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"base_url": "https://embedding.example.com/v1", "api_key": "scheduled-key-canary", "model_whitelist": []any{"embed"},
	}}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{74: account}}
	upstream := &embeddingAccountTestUpstream{queuedHTTPUpstream: queuedHTTPUpstream{responses: []*http.Response{{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Canary": []string{"scheduled-header-canary"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":0},"echo":"scheduled-body-canary"}`)),
	}}}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg: &config.Config{Gateway: config.GatewayConfig{Embedding: config.EmbeddingGatewayConfig{
			AllowedHosts: []string{"embedding.example.com"},
		}}},
	}

	result, err := svc.RunTestBackground(context.Background(), 74, "embed")
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "embedding_test_invalid_usage", result.ErrorMessage)
	require.Empty(t, result.ResponseText)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	for _, canary := range []string{"scheduled-key-canary", "scheduled-header-canary", "scheduled-body-canary", "0.1"} {
		require.NotContains(t, string(encoded), canary)
	}
}
