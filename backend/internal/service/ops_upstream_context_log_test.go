package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAppendOpsUpstreamErrorLogsSanitizedRequestAndRawResponseBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, cleanup := captureStructuredLog(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	ctx := context.WithValue(c.Request.Context(), ctxkey.UserEmail, "alice@example.com")
	ctx = logger.IntoContext(ctx, logger.L().With(zap.String("user_email", "alice@example.com")))
	c.Request = c.Request.WithContext(ctx)

	setOpsUpstreamRequestBody(c, []byte(`{"model":"gpt-5","access_token":"secret-token","messages":[{"role":"user","content":"hello"}]}`))
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             "openai",
		AccountID:            7,
		AccountName:          "acct",
		UpstreamStatusCode:   http.StatusBadRequest,
		UpstreamRequestID:    "req-upstream-1",
		Kind:                 "http_error",
		Message:              "bad request",
		UpstreamResponseBody: `{"error":{"message":"upstream says no","access_token":"response-token"}}`,
	})

	require.True(t, logSink.ContainsMessage("upstream model request failed"))
	require.True(t, logSink.ContainsFieldValue("user_email", "alice@example.com"))
	require.True(t, logSink.ContainsFieldValue("upstream_request_body", `"access_token":"[REDACTED]"`))
	require.False(t, logSink.ContainsFieldValue("upstream_request_body", "secret-token"))
	require.True(t, logSink.ContainsFieldValue("upstream_response_body", "response-token"))
}
