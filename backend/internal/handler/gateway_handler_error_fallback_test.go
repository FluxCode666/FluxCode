package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGatewayEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "error", parsed["type"])
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestGatewayEnsureForwardErrorResponse_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &GatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestGatewayHandleStreamingAwareError_IncludesTraceIDInSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), ctxkey.TraceID, "trace-claude-stream-1")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "request-claude-stream-1")
	c.Request = req.WithContext(ctx)

	h := &GatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "stream failed", true)

	jsonStr := strings.TrimPrefix(strings.TrimSuffix(w.Body.String(), "\n\n"), "data: ")
	var parsed map[string]any
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "trace-claude-stream-1", parsed["trace_id"])
	assert.Equal(t, "request-claude-stream-1", parsed["request_id"])
}

func TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}
	apiKey := &service.APIKey{
		Group: &service.Group{ID: 1, Platform: service.PlatformAnthropic},
	}

	got, result := h.trySwitchToClaudeFallbackGroup(c, zap.NewNop(), apiKey, false)

	require.Equal(t, claudeFallbackUnavailable, result)
	require.Nil(t, got)
}

func TestGatewayHandleClaudeFallbackBillingFailure_ReturnsHandledTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h := &GatewayHandler{}

	result := h.handleClaudeFallbackBillingFailure(c, service.ErrInsufficientBalance, false)

	require.Equal(t, claudeFallbackHandled, result)
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "insufficient balance")
}

func TestGatewayShouldAttemptClaudeRuntimeFallback_NonRetryableFailoverExhausted(t *testing.T) {
	shouldFallback := shouldAttemptClaudeRuntimeFallback(false, 0, 0, &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: []byte(`{"error":{"message":"bad request"}}`),
	})

	require.False(t, shouldFallback)
}

func TestGatewayShouldAttemptClaudeRuntimeFallback_StreamAlreadyWritten(t *testing.T) {
	shouldFallback := shouldAttemptClaudeRuntimeFallback(false, 0, 32, &service.UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"error":{"message":"rate limited"}}`),
	})

	require.False(t, shouldFallback)
}

func TestGatewayShouldRetryClaudeRuntimeFallback_AllowsTransportLikeErrors(t *testing.T) {
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: 0}))
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusRequestTimeout}))
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
	require.True(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}))
	require.False(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusUnauthorized}))
	require.False(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusBadRequest}))
	require.False(t, shouldRetryClaudeRuntimeFallback(&service.UpstreamFailoverError{StatusCode: http.StatusForbidden}))
	require.False(t, shouldRetryClaudeRuntimeFallback(nil))
}
