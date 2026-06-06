package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func testGinContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func TestOpenAIGatewayHandlerFailoverExhausted_OpenAIOAuthSessionTerminated(t *testing.T) {
	c, rec := testGinContext()
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, service.NewOpenAIOAuthSessionTerminatedFailoverError(), false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIOAuthSessionTerminatedGatewayMessage)
}

func TestOpenAIGatewayHandlerAnthropicFailoverExhausted_OpenAIOAuthSessionTerminated(t *testing.T) {
	c, rec := testGinContext()
	h := &OpenAIGatewayHandler{}

	h.handleAnthropicFailoverExhausted(c, service.NewOpenAIOAuthSessionTerminatedFailoverError(), false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIOAuthSessionTerminatedGatewayMessage)
}

func TestGatewayHandlerFailoverExhausted_OpenAIOAuthSessionTerminated(t *testing.T) {
	c, rec := testGinContext()
	h := &GatewayHandler{}

	h.handleFailoverExhausted(c, service.NewOpenAIOAuthSessionTerminatedFailoverError(), service.PlatformOpenAI, false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIOAuthSessionTerminatedGatewayMessage)
}

func TestGatewayHandlerResponsesFailoverExhausted_OpenAIOAuthSessionTerminated(t *testing.T) {
	c, rec := testGinContext()
	h := &GatewayHandler{}

	h.handleResponsesFailoverExhausted(c, service.NewOpenAIOAuthSessionTerminatedFailoverError(), false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIOAuthSessionTerminatedGatewayMessage)
}

func TestGatewayHandlerChatCompletionsFailoverExhausted_OpenAIOAuthSessionTerminated(t *testing.T) {
	c, rec := testGinContext()
	h := &GatewayHandler{}

	h.handleCCFailoverExhausted(c, service.NewOpenAIOAuthSessionTerminatedFailoverError(), false)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), service.OpenAIOAuthSessionTerminatedGatewayMessage)
}
