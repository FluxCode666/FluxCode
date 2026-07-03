package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newImageControlService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	return &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
}

func newImageControlContext(path string, allowImages bool, userAgent string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Request.Header.Set("User-Agent", userAgent)
	c.Set("api_key", &APIKey{Group: &Group{AllowImageGeneration: allowImages}})
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return c, rec
}

func newImageControlAccount() *Account {
	return &Account{
		ID:          91,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com/v1"},
	}
}

func newImageControlResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp","usage":{"input_tokens":1,"output_tokens":1}}`)),
	}
}

func TestOpenAIGatewayForward_CodexImageBridgeDefaultDoesNotInject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: newImageControlResponse()}
	svc := newImageControlService(upstream)
	c, _ := newImageControlContext("/v1/responses", true, "codex_cli_rs/0.98.0")

	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"write code","stream":false}`))

	require.NoError(t, err)
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
}

func TestOpenAIGatewayForward_CodexImageBridgeInjectsWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: newImageControlResponse()}
	svc := newImageControlService(upstream)
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	c, _ := newImageControlContext("/v1/responses", true, "codex_cli_rs/0.98.0")

	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"write code","stream":false}`))

	require.NoError(t, err)
	require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.Equal(t, "auto", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "instructions").String(), codexImageGenerationBridgeMarker)
}

func TestOpenAIGatewayForward_ExplicitImageToolDeniedWhenGroupDisallows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{}
	svc := newImageControlService(upstream)
	c, rec := newImageControlContext("/v1/responses", false, "codex_cli_rs/0.98.0")

	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"draw","tools":[{"type":"image_generation"}]}`))

	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), ImageGenerationPermissionMessage())
	require.Nil(t, upstream.lastReq)
}

func TestOpenAIGatewayForward_ExplicitImageToolStillForwardsWhenBridgeDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: newImageControlResponse()}
	svc := newImageControlService(upstream)
	c, _ := newImageControlContext("/v1/responses", true, "codex_cli_rs/0.98.0")

	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"draw","tools":[{"type":"image_generation","output_format":"png"}]}`))

	require.NoError(t, err)
	require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
}

func TestOpenAIGatewayForward_CompactSkipsCodexImageBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: newImageControlResponse()}
	svc := newImageControlService(upstream)
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	c, _ := newImageControlContext("/v1/responses/compact", true, "codex_cli_rs/0.98.0")

	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"summarize","stream":false}`))

	require.NoError(t, err)
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
}

func TestOpenAIGatewayForward_SparkStripsImageToolBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: newImageControlResponse()}
	svc := newImageControlService(upstream)
	c, _ := newImageControlContext("/v1/responses", true, "codex_cli_rs/0.98.0")

	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.3-codex-spark","input":"write code","tools":[{"type":"image_generation"},{"type":"web_search"}]}`))

	require.NoError(t, err)
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
}

func TestOpenAIGatewayForward_SparkStripsImageToolBeforeUpstream_RemovesImageToolChoice(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		toolChoice string
	}{
		{name: "string", toolChoice: `"image_generation"`},
		{name: "type object", toolChoice: `{"type":"image_generation"}`},
		{name: "nested tool object", toolChoice: `{"tool":{"type":"image_generation"}}`},
		{name: "function object", toolChoice: `{"function":{"name":"image_generation"}}`},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: newImageControlResponse()}
			svc := newImageControlService(upstream)
			c, _ := newImageControlContext("/v1/responses", true, "codex_cli_rs/0.98.0")

			body := `{"model":"gpt-5.3-codex-spark","input":"write code","tools":[{"type":"image_generation"},{"type":"web_search"}],"tool_choice":` + tt.toolChoice + `}`
			_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(body))

			require.NoError(t, err)
			require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
			require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
		})
	}
}
