package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardAsChatCompletions_APIKeyWithoutResponsesUsesRawChatEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"compat-model","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Originator", "codex-cli")
	c.Request.Header.Set("User-Agent", "compat-client")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": {"application/json"},
			"X-Request-Id": {"compat-request"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"chatcmpl_1","object":"chat.completion","model":"compat-model","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://compat.example/v1"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "compat-model")

	require.NoError(t, err)
	require.Equal(t, "https://compat.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "compat-client", upstream.lastReq.Header.Get("User-Agent"))
	require.Empty(t, upstream.lastReq.Header.Get("Originator"))
	require.JSONEq(t, string(body), string(upstream.lastBody))
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), `"object":"chat.completion"`)
}

func TestForwardAsChatCompletions_APIKeyWithoutResponsesStreamsRawChatSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"compat-model","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":true}}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3}}\n\n" +
			"data: [DONE]\n\n")),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          8,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://compat.example"},
		Extra:       map[string]any{openai_compat.ExtraKeyResponsesSupported: false},
	}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "compat-model")

	require.NoError(t, err)
	require.Equal(t, "https://compat.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, result.Stream)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Contains(t, recorder.Body.String(), "data: [DONE]")
}

func TestForwardAsChatCompletions_APIKeyPropagatesPromptCacheKeyAndStripsSamplingAfterMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"custom-gpt56","messages":[{"role":"user","content":"hello"}],"temperature":0.7,"top_p":0.8,"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 99})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "cache-key-123", "gpt-5.6-sol")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "cache-key-123", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "top_p").Exists())
}

func TestForwardAsChatCompletions_APIKeyPreservesExistingPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-4.1","messages":[{"role":"user","content":"hello"}],"prompt_cache_key":"client-key","stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "gateway-key", "gpt-4.1")

	require.Error(t, err)
	require.Equal(t, "client-key", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
}

func TestForwardAsChatCompletions_OAuthDoesNotInjectDefaultInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.6","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth", "chatgpt_account_id": "acc"}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.6-sol")
	require.Error(t, err)
	require.Nil(t, result)
	require.True(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
	require.Equal(t, "", gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.NotContains(t, string(upstream.lastBody), "You are a helpful coding assistant.")
}

func TestForwardAsChatCompletions_APIKeyResponsesShapePreservesOAuthOnlyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"custom-model","input":"hello","prompt_cache_retention":"24h","safety_identifier":"safe","metadata":{"a":1},"stream_options":{"include_usage":true}}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}}
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "custom-model")
	require.Error(t, err)
	for _, path := range []string{"prompt_cache_retention", "safety_identifier", "metadata.a", "stream_options.include_usage"} {
		require.True(t, gjson.GetBytes(upstream.lastBody, path).Exists(), path)
	}
}

func TestHandleChatBufferedStreamingResponse_CacheWriteUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid-buffered-cache-write"}},
		Body: io.NopCloser(strings.NewReader("data: " +
			`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cache_write_tokens":6}}}}` + "\n\n")),
	}

	result, err := (&OpenAIGatewayService{}).handleChatBufferedStreamingResponse(
		resp, c, "gpt-5.6", "gpt-5.6-sol", "gpt-5.6-sol", time.Now(),
	)

	require.NoError(t, err)
	require.Equal(t, 6, result.Usage.CacheCreationInputTokens)
	require.Contains(t, recorder.Body.String(), `"cache_creation_input_tokens":6`)
	require.Contains(t, recorder.Body.String(), `"cache_write_input_tokens":6`)
}

func TestHandleChatStreamingResponse_CacheCreationUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid-stream-cache-write"}},
		Body: io.NopCloser(strings.NewReader("data: " +
			`{"type":"response.completed","response":{"id":"resp_2","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":5}}}` + "\n\n")),
	}

	result, err := (&OpenAIGatewayService{}).handleChatStreamingResponse(
		resp, c, "gpt-5.6", "gpt-5.6-sol", "gpt-5.6-sol", true, time.Now(),
	)

	require.NoError(t, err)
	require.Equal(t, 5, result.Usage.CacheCreationInputTokens)
	require.Contains(t, recorder.Body.String(), `"cache_creation_input_tokens":5`)
	require.Contains(t, recorder.Body.String(), `"cache_write_input_tokens":5`)
}
