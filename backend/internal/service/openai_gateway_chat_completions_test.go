package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
