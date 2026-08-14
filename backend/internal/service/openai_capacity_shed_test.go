package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamErrorFrameDoesNotStartClientOutput(t *testing.T) {
	tests := []struct {
		data      string
		eventType string
		want      bool
	}{
		{`{"type":"error","error":{"code":"server_is_overloaded","message":"overloaded"}}`, "error", false},
		{`{"type":"error","error":{"code":"slow_down","message":"slow down"}}`, "error", false},
		{`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"limited"}}`, "error", false},
		{`{"type":"error","error":{"type":"invalid_request_error","code":"content_policy_violation","message":"blocked"}}`, "error", true},
		{`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`, "response.failed", false},
		{`{"type":"response.created","response":{"id":"resp_1"}}`, "response.created", false},
		{`{"type":"response.in_progress","response":{"id":"resp_1"}}`, "response.in_progress", false},
		{`{"type":"response.output_text.delta","delta":"hi"}`, "response.output_text.delta", true},
		{`[DONE]`, "", true},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, openAIStreamDataStartsClientOutput(tt.data, tt.eventType), "data=%s type=%s", tt.data, tt.eventType)
	}
}

func TestOpenAIStreamCapacityShedErrorFramePrecedingFailedStillFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-shed-error-then-failed"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"},"sequence_number":0}`,
			"",
			"event: response.in_progress",
			`data: {"type":"response.in_progress","response":{"id":"resp_1"},"sequence_number":1}`,
			"",
			"event: error",
			`data: {"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."},"sequence_number":2}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}},"sequence_number":3}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.RequestScopedTransient)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

func TestOpenAIStreamCapacityShedAfterOutputRewritesCodeForClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		toolCorrector: NewCodexToolCorrector(),
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-shed-after-output"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.created",
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			"",
			"event: response.output_text.delta",
			`data: {"type":"response.output_text.delta","delta":"partial"}`,
			"",
			"event: error",
			`data: {"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
			"",
			"event: response.failed",
			`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Name: "acc"}, time.Now(), "model", "model")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	body := rec.Body.String()
	require.Contains(t, body, "partial")
	require.Contains(t, body, "event: response.failed")
	require.Contains(t, body, `"code":"server_error"`)
	require.NotContains(t, body, "server_is_overloaded")
	require.Contains(t, body, "Our servers are currently overloaded")
}

func TestSanitizeOpenAICapacityShedErrorCodeForClient(t *testing.T) {
	tests := []struct {
		payload     string
		wantChanged bool
		want        string
	}{
		{`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"overloaded"}}}`, true, `"code":"server_error"`},
		{`{"type":"error","error":{"code":"slow_down","message":"slow down"}}`, true, `"code":"server_error"`},
		{`{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"retry"}}}`, false, `"code":"rate_limit_exceeded"`},
		{`not-json`, false, `not-json`},
	}
	for _, tt := range tests {
		out, changed := sanitizeOpenAICapacityShedErrorCodeForClient([]byte(tt.payload))
		require.Equal(t, tt.wantChanged, changed)
		require.Contains(t, string(out), tt.want)
	}
}
