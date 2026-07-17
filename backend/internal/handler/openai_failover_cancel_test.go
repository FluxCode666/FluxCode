package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFailoverClientGoneMarksUnwrittenResponseAsClientClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)

	require.True(t, failoverClientGone(c))
	require.Equal(t, statusClientClosedRequest, c.Writer.Status())
}

func TestFailoverClientGoneLeavesConnectedRequestAlone(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	require.False(t, failoverClientGone(c))
	require.False(t, c.Writer.Written())
}

func TestOpenAIHandleFailoverExhaustedPreservesBodyLimitStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	h := &OpenAIGatewayHandler{}

	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusRequestEntityTooLarge,
		ResponseBody: []byte(`{"error":{"message":"request body is too large"}}`),
	}, false)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Request payload is too large")
}
