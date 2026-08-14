//go:build unit

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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVisibleOutputClassification(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		eventType string
		want      bool
	}{
		{name: "keepalive", data: `{"type":"keepalive"}`, want: false},
		{name: "created", data: `{"type":"response.created"}`, want: false},
		{name: "empty item", data: `{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`, want: false},
		{name: "empty delta", data: `{"type":"response.output_text.delta","delta":""}`, want: false},
		{name: "text delta", data: `{"type":"response.output_text.delta","delta":"hello"}`, want: true},
		{name: "tool arguments done", data: `{"type":"response.function_call_arguments.done","arguments":"{}"}`, want: true},
		{name: "partial image", data: `{"type":"response.image_generation_call.partial_image","partial_image_b64":"dGVzdA=="}`, want: true},
		{name: "completed image item", data: `{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"dGVzdA=="}}`, want: true},
		{name: "usage-only terminal", data: `{"type":"response.completed","response":{"usage":{"input_tokens":1}}}`, want: false},
		{name: "completed text", data: `{"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}}`, want: true},
		{name: "done marker", data: `[DONE]`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIStreamDataStartsVisibleOutput(tt.data, tt.eventType))
		})
	}
}

func TestOpenAIResponsesEmptyCompletedFailsOverBeforeClientOutput(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := strings.NewReader(
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"r-empty\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r-empty\",\"output\":[]}}\n\n",
			)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"x-request-id": []string{"req-empty"}}, Body: io.NopCloser(body)}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			service := &OpenAIGatewayService{}
			account := &Account{ID: 1, Platform: PlatformOpenAI}

			var err error
			if passthrough {
				_, err = service.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now())
			} else {
				_, err = service.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
			}
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr), "got %v", err)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestOpenAIResponsesCompletedEventIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		usage *OpenAIUsage
		want  bool
	}{
		{name: "bare completed", data: `{"type":"response.completed"}`, want: true},
		{name: "empty output", data: `{"type":"response.completed","response":{"output":[]}}`, want: true},
		{name: "usage", data: `{"type":"response.completed","response":{"usage":{"input_tokens":1}}}`, want: false},
		{name: "error", data: `{"type":"response.completed","response":{"error":{"code":"x"}}}`, want: false},
		{name: "output item", data: `{"type":"response.completed","response":{"output":[{"type":"message"}]}}`, want: false},
		{name: "accumulated usage", data: `{"type":"response.completed"}`, usage: &OpenAIUsage{InputTokens: 1}, want: false},
		{name: "invalid json", data: `{"type":`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIResponsesCompletedEventIsEmpty([]byte(tt.data), tt.usage))
		})
	}
}

func TestOpenAIResponsesTTFTStartsAtVisibleOutput(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			result := runSyntheticVisibleTTFTStream(t, passthrough, 120*time.Millisecond,
				`{"type":"response.output_text.delta","delta":"test output"}`)
			require.NotNil(t, result)
			require.GreaterOrEqual(t, *result, 100)
		})
	}
}

func runSyntheticVisibleTTFTStream(t *testing.T, passthrough bool, visibleDelay time.Duration, visibleEvent string) *int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	reader, writer := io.Pipe()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer func() { _ = writer.Close() }()
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item_test\",\"type\":\"reasoning\",\"summary\":[]}}\n\n")
		time.Sleep(visibleDelay)
		_, _ = io.WriteString(writer, "data: "+visibleEvent+"\n\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}
	account := &Account{ID: 1, Name: "account_test", Platform: PlatformOpenAI}
	started := time.Now()
	var firstTokenMs *int
	if passthrough {
		result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, started)
		require.NoError(t, err)
		firstTokenMs = result.firstTokenMs
	} else {
		result, err := svc.handleStreamingResponse(context.Background(), resp, c, account, started, "test-model", "test-model")
		require.NoError(t, err)
		firstTokenMs = result.firstTokenMs
	}
	require.Contains(t, recorder.Body.String(), visibleEvent)
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("synthetic upstream writer did not exit")
	}
	return firstTokenMs
}

func TestOpenAIStreamingUsageOnlyTerminalDoesNotStartTTFT(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "native"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := strings.NewReader(
				"data: {\"type\":\"response.created\",\"response\":{\"id\":\"r1\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r1\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n",
			)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(body)}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			service := &OpenAIGatewayService{}
			account := &Account{ID: 1, Platform: PlatformOpenAI}

			if passthrough {
				result, err := service.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now())
				require.NoError(t, err)
				require.Nil(t, result.firstTokenMs)
			} else {
				result, err := service.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
				require.NoError(t, err)
				require.Nil(t, result.firstTokenMs)
			}
		})
	}
}
