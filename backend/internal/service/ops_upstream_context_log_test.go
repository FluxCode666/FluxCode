package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func (s *inMemoryLogSink) latestFieldString(t *testing.T, field string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.events) - 1; i >= 0; i-- {
		if event := s.events[i]; event != nil && event.Fields != nil {
			if value, ok := event.Fields[field]; ok {
				return fmt.Sprint(value)
			}
		}
	}
	t.Fatalf("日志中缺少字段 %q", field)
	return ""
}

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

func TestAppendOpsUpstreamErrorLogTruncatesOnlyUserAndSystemPrompts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logSink, cleanup := captureStructuredLog(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))

	longSystemPrompt := "system-prefix-" + strings.Repeat("系", opsUpstreamRequestBodyEarlyCap) + "-system-suffix"
	longUserPrompt := "user-prefix-" + strings.Repeat("用", opsUpstreamRequestBodyEarlyCap) + "-user-suffix"
	longAssistantContent := "assistant-prefix-" + strings.Repeat("答", opsUpstreamRequestBodyEarlyCap) + "-assistant-suffix"
	longToolOutput := "tool-prefix-" + strings.Repeat("数", opsUpstreamRequestBodyEarlyCap) + "-tool-suffix"
	longToolDescription := "description-prefix-" + strings.Repeat("明", opsUpstreamRequestBodyEarlyCap) + "-description-suffix"
	requestBody := map[string]any{
		"model":        "gpt-5.2",
		"instructions": longSystemPrompt,
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": longUserPrompt},
					map[string]any{"type": "input_image", "image_url": "https://example.com/image.png"},
				},
			},
			map[string]any{"type": "message", "role": "assistant", "content": longAssistantContent},
			map[string]any{"type": "function_call_output", "call_id": "call-1", "output": longToolOutput},
		},
		"tools": []any{
			map[string]any{
				"type":        "function",
				"name":        "lookup_weather",
				"description": longToolDescription,
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
		},
		"metadata": map[string]any{"trace_id": "trace-at-json-tail"},
	}
	raw, err := json.Marshal(requestBody)
	require.NoError(t, err)
	require.Greater(t, len(raw), opsUpstreamRequestBodyEarlyCap)

	setOpsUpstreamRequestBody(c, raw)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           "openai",
		UpstreamStatusCode: http.StatusBadRequest,
		Kind:               "http_error",
		Message:            "bad request",
	})

	loggedBody := logSink.latestFieldString(t, "upstream_request_body")
	var logged map[string]any
	require.NoError(t, json.Unmarshal([]byte(loggedBody), &logged), loggedBody)
	require.Equal(t, "gpt-5.2", logged["model"])
	require.Equal(t, map[string]any{"trace_id": "trace-at-json-tail"}, logged["metadata"])
	require.Equal(t, requestBody["tools"], logged["tools"])

	loggedInstructions, ok := logged["instructions"].(string)
	require.True(t, ok)
	require.Less(t, len(loggedInstructions), len(longSystemPrompt))
	require.LessOrEqual(t, len(loggedInstructions), opsUpstreamDebugLogPromptMaxBytes)
	require.Contains(t, loggedInstructions, "[truncated]")

	input, ok := logged["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)
	userMessage, ok := input[0].(map[string]any)
	require.True(t, ok)
	content, ok := userMessage["content"].([]any)
	require.True(t, ok)
	userText, ok := content[0].(map[string]any)["text"].(string)
	require.True(t, ok)
	require.Less(t, len(userText), len(longUserPrompt))
	require.LessOrEqual(t, len(userText), opsUpstreamDebugLogPromptMaxBytes)
	require.Contains(t, userText, "[truncated]")
	require.Equal(t, "https://example.com/image.png", content[1].(map[string]any)["image_url"])
	require.Equal(t, longAssistantContent, input[1].(map[string]any)["content"])
	require.Equal(t, longToolOutput, input[2].(map[string]any)["output"])
	require.True(t, logSink.ContainsFieldValue("upstream_request_body_truncated", "true"))

	stored, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	storedEvents, ok := stored.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, storedEvents, 1)
	require.LessOrEqual(t, len(storedEvents[0].UpstreamRequestBody), opsUpstreamRequestBodyEarlyCap)
}

func TestSanitizeRequestBodyForDebugLogSupportsChatAndGeminiPromptShapes(t *testing.T) {
	longPrompt := strings.Repeat("提示词", opsUpstreamDebugLogPromptMaxBytes)
	longNonPrompt := strings.Repeat("保留数据", opsUpstreamDebugLogPromptMaxBytes)

	tests := []struct {
		name  string
		body  map[string]any
		check func(t *testing.T, logged map[string]any)
	}{
		{
			name: "OpenAI 和 Anthropic messages",
			body: map[string]any{
				"messages": []any{
					map[string]any{"role": "system", "content": longPrompt},
					map[string]any{"role": "user", "content": []any{
						map[string]any{"type": "text", "text": longPrompt},
						map[string]any{"type": "tool_result", "content": longNonPrompt},
						map[string]any{"type": "tool_use_result", "content": longNonPrompt},
					}},
					map[string]any{"role": "assistant", "content": longNonPrompt},
				},
			},
			check: func(t *testing.T, logged map[string]any) {
				messages := logged["messages"].([]any)
				require.Contains(t, messages[0].(map[string]any)["content"], "[truncated]")
				userContent := messages[1].(map[string]any)["content"].([]any)
				require.Contains(t, userContent[0].(map[string]any)["text"], "[truncated]")
				require.Equal(t, longNonPrompt, userContent[1].(map[string]any)["content"])
				require.Equal(t, longNonPrompt, userContent[2].(map[string]any)["content"])
				require.Equal(t, longNonPrompt, messages[2].(map[string]any)["content"])
			},
		},
		{
			name: "Antigravity 包装的 Gemini systemInstruction 和 contents",
			body: map[string]any{
				"model": "gemini-3.1-pro-preview",
				"request": map[string]any{
					"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": longPrompt}}},
					"contents": []any{
						map[string]any{"role": "user", "parts": []any{map[string]any{"text": longPrompt}}},
						map[string]any{"role": "model", "parts": []any{map[string]any{"text": longNonPrompt}}},
					},
					"generationConfig": map[string]any{"maxOutputTokens": 8192},
				},
			},
			check: func(t *testing.T, logged map[string]any) {
				request := logged["request"].(map[string]any)
				require.Equal(t, map[string]any{"maxOutputTokens": float64(8192)}, request["generationConfig"])
				systemInstruction := request["systemInstruction"].(map[string]any)
				systemText := systemInstruction["parts"].([]any)[0].(map[string]any)["text"]
				require.Contains(t, systemText, "[truncated]")
				contents := request["contents"].([]any)
				userText := contents[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"]
				require.Contains(t, userText, "[truncated]")
				modelText := contents[1].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"]
				require.Equal(t, longNonPrompt, modelText)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.body)
			require.NoError(t, err)

			loggedBody, truncated := sanitizeRequestBodyForDebugLog(string(raw), opsUpstreamDebugLogPromptMaxBytes)
			require.True(t, truncated)
			var logged map[string]any
			require.NoError(t, json.Unmarshal([]byte(loggedBody), &logged))
			test.check(t, logged)
		})
	}
}

func TestSanitizeRequestBodyForDebugLogRejectsTrailingJSONData(t *testing.T) {
	for _, raw := range []string{
		`{"model":"gpt-5"}garbage`,
		`{"model":"gpt-5"}{"model":"gpt-4"}`,
	} {
		loggedBody, truncated := sanitizeRequestBodyForDebugLog(raw, opsUpstreamDebugLogPromptMaxBytes)
		require.Empty(t, loggedBody)
		require.False(t, truncated)
	}
}
