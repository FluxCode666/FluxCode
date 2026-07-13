package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ChatCompletionsToResponses tests
// ---------------------------------------------------------------------------

func TestChatCompletionsToResponses_BasicText(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", resp.Model)
	assert.True(t, resp.Stream) // always forced true
	assert.False(t, *resp.Store)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "user", items[0].Role)
}

func TestChatCompletionsToResponses_GPT56StripsSampling(t *testing.T) {
	v := 0.7
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt5.6", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Temperature: &v, TopP: &v,
	})
	require.NoError(t, err)
	require.Nil(t, resp.Temperature)
	require.Nil(t, resp.TopP)
}

func TestChatCompletionsToResponses_NonReasoningModelPreservesSampling(t *testing.T) {
	v := 0.7
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Temperature: &v, TopP: &v,
	})
	require.NoError(t, err)
	require.Same(t, &v, resp.Temperature)
	require.Same(t, &v, resp.TopP)
}

func TestChatCompletionsToResponses_ResponseFormatAndParallelTools(t *testing.T) {
	parallel := false
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt-5.6", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		ResponseFormat:    json.RawMessage(`{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object"},"strict":true}}`),
		ParallelToolCalls: &parallel,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Text)
	require.JSONEq(t, `{"type":"json_schema","name":"answer","schema":{"type":"object"},"strict":true}`, string(resp.Text.Format))
	require.NotNil(t, resp.ParallelToolCalls)
	require.False(t, *resp.ParallelToolCalls)
}

func TestChatCompletionsToResponses_EmptyContentNeverNull(t *testing.T) {
	for _, content := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`[{"type":"text","text":""}]`)} {
		resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{Model: "gpt-5.6", Messages: []ChatMessage{{Role: "user", Content: content}}})
		require.NoError(t, err)
		require.NotContains(t, string(resp.Input), `"content":null`)
	}
}

func TestChatCompletionsToResponses_InvalidResponseFormatReturnsError(t *testing.T) {
	_, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model:          "gpt-5.6",
		Messages:       []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		ResponseFormat: json.RawMessage(`{"type":"json_schema","json_schema":`),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "response_format")
}

func TestChatCompletionsToResponses_SystemMessage(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "system", Content: json.RawMessage(`"You are helpful."`)},
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 2)
	assert.Equal(t, "system", items[0].Role)
	assert.Equal(t, "user", items[1].Role)
}

func TestChatCompletionsToResponses_ToolCalls(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Call the function"`)},
			{
				Role: "assistant",
				ToolCalls: []ChatToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: ChatFunctionCall{
							Name:      "ping",
							Arguments: `{"host":"example.com"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_1",
				Content:    json.RawMessage(`"pong"`),
			},
		},
		Tools: []ChatTool{
			{
				Type: "function",
				Function: &ChatFunction{
					Name:        "ping",
					Description: "Ping a host",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + function_call + function_call_output = 3
	// (assistant message with empty content + tool_calls → only function_call items emitted)
	require.Len(t, items, 3)

	// Check function_call item
	assert.Equal(t, "function_call", items[1].Type)
	assert.Equal(t, "call_1", items[1].CallID)
	assert.Empty(t, items[1].ID)
	assert.Equal(t, "ping", items[1].Name)

	// Check function_call_output item
	assert.Equal(t, "function_call_output", items[2].Type)
	assert.Equal(t, "call_1", items[2].CallID)
	assert.Equal(t, "pong", items[2].Output)

	// Check tools
	require.Len(t, resp.Tools, 1)
	assert.Equal(t, "function", resp.Tools[0].Type)
	assert.Equal(t, "ping", resp.Tools[0].Name)
}

func TestChatCompletionsToResponses_MaxTokens(t *testing.T) {
	t.Run("max_tokens", func(t *testing.T) {
		maxTokens := 100
		req := &ChatCompletionsRequest{
			Model:     "gpt-4o",
			MaxTokens: &maxTokens,
			Messages:  []ChatMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
		}
		resp, err := ChatCompletionsToResponses(req)
		require.NoError(t, err)
		require.NotNil(t, resp.MaxOutputTokens)
		// Below minMaxOutputTokens (128), should be clamped
		assert.Equal(t, minMaxOutputTokens, *resp.MaxOutputTokens)
	})

	t.Run("max_completion_tokens_preferred", func(t *testing.T) {
		maxTokens := 100
		maxCompletion := 500
		req := &ChatCompletionsRequest{
			Model:               "gpt-4o",
			MaxTokens:           &maxTokens,
			MaxCompletionTokens: &maxCompletion,
			Messages:            []ChatMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
		}
		resp, err := ChatCompletionsToResponses(req)
		require.NoError(t, err)
		require.NotNil(t, resp.MaxOutputTokens)
		assert.Equal(t, 500, *resp.MaxOutputTokens)
	})
}

func TestChatCompletionsToResponses_ReasoningEffort(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model:           "gpt-4o",
		ReasoningEffort: "high",
		Messages:        []ChatMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
	}
	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	require.NotNil(t, resp.Reasoning)
	assert.Equal(t, "high", resp.Reasoning.Effort)
	assert.Equal(t, "auto", resp.Reasoning.Summary)
}

func TestChatCompletionsToResponses_ImageURL(t *testing.T) {
	content := `[{"type":"text","text":"Describe this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc123"}}]`
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(content)},
		},
	}
	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 2)
	assert.Equal(t, "input_text", parts[0].Type)
	assert.Equal(t, "Describe this", parts[0].Text)
	assert.Equal(t, "input_image", parts[1].Type)
	assert.Equal(t, "data:image/png;base64,abc123", parts[1].ImageURL)
}

func TestChatCompletionsToResponses_EmptyBase64ImageURLSkipped(t *testing.T) {
	content := `[{"type":"text","text":"Describe this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,"}}]`
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(content)},
		},
	}
	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_text", parts[0].Type)
	assert.Equal(t, "Describe this", parts[0].Text)
}

func TestChatCompletionsToResponses_WhitespaceOnlyBase64ImageURLSkipped(t *testing.T) {
	content := `[{"type":"text","text":"Describe this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,   "}}]`
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(content)},
		},
	}
	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 1)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "input_text", parts[0].Type)
	assert.Equal(t, "Describe this", parts[0].Text)
}

func TestChatCompletionsToResponses_SystemArrayContent(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"You are a careful visual assistant."}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"Describe this image"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc123"}}]`)},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 2)

	var systemParts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[0].Content, &systemParts))
	require.Len(t, systemParts, 1)
	assert.Equal(t, "input_text", systemParts[0].Type)
	assert.Equal(t, "You are a careful visual assistant.", systemParts[0].Text)

	var userParts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[1].Content, &userParts))
	require.Len(t, userParts, 2)
	assert.Equal(t, "input_image", userParts[1].Type)
	assert.Equal(t, "data:image/png;base64,abc123", userParts[1].ImageURL)
}

func TestChatCompletionsToResponses_LegacyFunctions(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
		Functions: []ChatFunction{
			{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		},
		FunctionCall: json.RawMessage(`{"name":"get_weather"}`),
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	require.Len(t, resp.Tools, 1)
	assert.Equal(t, "function", resp.Tools[0].Type)
	assert.Equal(t, "get_weather", resp.Tools[0].Name)

	// tool_choice should be converted
	require.NotNil(t, resp.ToolChoice)
	var tc map[string]any
	require.NoError(t, json.Unmarshal(resp.ToolChoice, &tc))
	assert.Equal(t, "function", tc["type"])
	assert.Equal(t, "get_weather", tc["name"])
}

func TestChatCompletionsToResponses_ToolStrictDefaultsFalse(t *testing.T) {
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model:    "gpt-5.6",
		Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools:    []ChatTool{{Type: "function", Function: &ChatFunction{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Tools, 1)
	require.NotNil(t, resp.Tools[0].Strict)
	require.False(t, *resp.Tools[0].Strict)
}

func TestChatCompletionsToResponses_ServiceTier(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model:       "gpt-4o",
		ServiceTier: "flex",
		Messages:    []ChatMessage{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
	}
	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)
	assert.Equal(t, "flex", resp.ServiceTier)
}

func TestChatCompletionsToResponses_AssistantWithTextAndToolCalls(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Do something"`)},
			{
				Role:    "assistant",
				Content: json.RawMessage(`"Let me call a function."`),
				ToolCalls: []ChatToolCall{
					{
						ID:   "call_abc",
						Type: "function",
						Function: ChatFunctionCall{
							Name:      "do_thing",
							Arguments: `{}`,
						},
					},
				},
			},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	// user + assistant message (with text) + function_call
	require.Len(t, items, 3)
	assert.Equal(t, "user", items[0].Role)
	assert.Equal(t, "assistant", items[1].Role)
	assert.Equal(t, "function_call", items[2].Type)
	assert.Empty(t, items[2].ID)
}

func TestChatCompletionsToResponses_AssistantArrayContentPreserved(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"A"},{"type":"text","text":"B"}]`)},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 2)
	assert.Equal(t, "assistant", items[1].Role)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[1].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "output_text", parts[0].Type)
	assert.Equal(t, "AB", parts[0].Text)
}

func TestChatCompletionsToResponses_AssistantThinkingTagPreserved(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"internal plan"},{"type":"text","text":"final answer"}]`)},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 2)

	var parts []ResponsesContentPart
	require.NoError(t, json.Unmarshal(items[1].Content, &parts))
	require.Len(t, parts, 1)
	assert.Equal(t, "output_text", parts[0].Type)
	assert.Contains(t, parts[0].Text, "<thinking>internal plan</thinking>")
	assert.Contains(t, parts[0].Text, "final answer")
}

func TestChatCompletionsToResponses_AssistantReasoningContentPreserved(t *testing.T) {
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model:    "gpt-5.6",
		Messages: []ChatMessage{{Role: "assistant", Content: json.RawMessage(`"answer"`), ReasoningContent: "internal plan"}},
	})
	require.NoError(t, err)
	require.Contains(t, string(resp.Input), "<thinking>internal plan</thinking>")
	require.Contains(t, string(resp.Input), "answer")
}

// ---------------------------------------------------------------------------
// ResponsesToChatCompletions tests
// ---------------------------------------------------------------------------

func TestResponsesToChatCompletions_BasicText(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_123",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "Hello, world!"},
				},
			},
		},
		Usage: &ResponsesUsage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-4o")
	assert.Equal(t, "chat.completion", chat.Object)
	assert.Equal(t, "gpt-4o", chat.Model)
	require.Len(t, chat.Choices, 1)
	assert.Equal(t, "stop", chat.Choices[0].FinishReason)

	var content string
	require.NoError(t, json.Unmarshal(chat.Choices[0].Message.Content, &content))
	assert.Equal(t, "Hello, world!", content)

	require.NotNil(t, chat.Usage)
	assert.Equal(t, 10, chat.Usage.PromptTokens)
	assert.Equal(t, 5, chat.Usage.CompletionTokens)
	assert.Equal(t, 15, chat.Usage.TotalTokens)
}

func TestResponsesToChatCompletions_ToolCalls(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_456",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type:      "function_call",
				CallID:    "call_xyz",
				Name:      "get_weather",
				Arguments: `{"city":"NYC"}`,
			},
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-4o")
	require.Len(t, chat.Choices, 1)
	assert.Equal(t, "tool_calls", chat.Choices[0].FinishReason)

	msg := chat.Choices[0].Message
	require.Len(t, msg.ToolCalls, 1)
	assert.Equal(t, "call_xyz", msg.ToolCalls[0].ID)
	assert.Equal(t, "function", msg.ToolCalls[0].Type)
	assert.Equal(t, "get_weather", msg.ToolCalls[0].Function.Name)
	assert.Equal(t, `{"city":"NYC"}`, msg.ToolCalls[0].Function.Arguments)
}

func TestResponsesToChatCompletions_Reasoning(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_789",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "reasoning",
				Summary: []ResponsesSummary{
					{Type: "summary_text", Text: "I thought about it."},
				},
			},
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "The answer is 42."},
				},
			},
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-4o")
	require.Len(t, chat.Choices, 1)

	var content string
	require.NoError(t, json.Unmarshal(chat.Choices[0].Message.Content, &content))
	assert.Equal(t, "The answer is 42.", content)
	assert.Equal(t, "I thought about it.", chat.Choices[0].Message.ReasoningContent)
}

func TestChatCompletionsToResponses_ToolArrayContent(t *testing.T) {
	req := &ChatCompletionsRequest{
		Model: "gpt-4o",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"Use the tool"`)},
			{
				Role: "assistant",
				ToolCalls: []ChatToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: ChatFunctionCall{
							Name:      "inspect_image",
							Arguments: `{}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_1",
				Content: json.RawMessage(
					`[{"type":"text","text":"image width: 100"},{"type":"image_url","image_url":{"url":"data:image/png;base64,ignored"}},{"type":"text","text":"; image height: 200"}]`,
				),
			},
		},
	}

	resp, err := ChatCompletionsToResponses(req)
	require.NoError(t, err)

	var items []ResponsesInputItem
	require.NoError(t, json.Unmarshal(resp.Input, &items))
	require.Len(t, items, 3)
	assert.Equal(t, "function_call_output", items[2].Type)
	assert.Equal(t, "call_1", items[2].CallID)
	assert.Equal(t, "image width: 100; image height: 200", items[2].Output)
}

func TestResponsesToChatCompletions_Incomplete(t *testing.T) {
	resp := &ResponsesResponse{
		ID:                "resp_inc",
		Status:            "incomplete",
		IncompleteDetails: &ResponsesIncompleteDetails{Reason: "max_output_tokens"},
		Output: []ResponsesOutput{
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "partial..."},
				},
			},
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-4o")
	require.Len(t, chat.Choices, 1)
	assert.Equal(t, "length", chat.Choices[0].FinishReason)
}

func TestResponsesToChatCompletions_CachedTokens(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_cache",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type:    "message",
				Content: []ResponsesContentPart{{Type: "output_text", Text: "cached"}},
			},
		},
		Usage: &ResponsesUsage{
			InputTokens:  100,
			OutputTokens: 10,
			TotalTokens:  110,
			InputTokensDetails: &ResponsesInputTokensDetails{
				CachedTokens: 80,
			},
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-4o")
	require.NotNil(t, chat.Usage)
	require.NotNil(t, chat.Usage.PromptTokensDetails)
	assert.Equal(t, 80, chat.Usage.PromptTokensDetails.CachedTokens)
}

func TestResponsesToChatCompletions_CacheWriteUsage(t *testing.T) {
	resp := &ResponsesResponse{
		Status: "completed",
		Usage: &ResponsesUsage{
			InputTokens:  100,
			OutputTokens: 10,
			InputTokensDetails: &ResponsesInputTokensDetails{
				CachedTokens:     80,
				CacheWriteTokens: 12,
			},
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-5.6")
	require.NotNil(t, chat.Usage)
	assert.Equal(t, 12, chat.Usage.CacheCreationInputTokens)
	assert.Equal(t, 12, chat.Usage.CacheWriteInputTokens)
	require.NotNil(t, chat.Usage.PromptTokensDetails)
	assert.Equal(t, 12, chat.Usage.PromptTokensDetails.CacheCreationTokens)
	assert.Equal(t, 12, chat.Usage.PromptTokensDetails.CacheWriteTokens)
}

func TestResponsesUsage_CacheWriteTokensWinsOverZeroTopLevelCreation(t *testing.T) {
	var usage ResponsesUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"input_tokens": 10,
		"output_tokens": 1,
		"cache_creation_input_tokens": 0,
		"cache_write_tokens": 11
	}`), &usage))

	assert.Equal(t, 11, usage.CacheCreationInputTokenCount())
}

func TestResponsesUsage_UnmarshalChatStyleUsagePreservesCacheWrite(t *testing.T) {
	var usage ResponsesUsage
	require.NoError(t, json.Unmarshal([]byte(`{
		"prompt_tokens": 43769,
		"completion_tokens": 16,
		"total_tokens": 43785,
		"prompt_tokens_details": {
			"cached_tokens": 43392,
			"cache_write_tokens": 200
		}
	}`), &usage))

	assert.Equal(t, 43769, usage.InputTokens)
	assert.Equal(t, 16, usage.OutputTokens)
	assert.Equal(t, 43785, usage.TotalTokens)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Equal(t, 43392, usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 200, usage.CacheCreationInputTokenCount())
}

func TestResponsesEventToChatChunks_CacheCreationUsage(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.IncludeUsage = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage: &ResponsesUsage{
				InputTokens:              20,
				OutputTokens:             5,
				CacheCreationInputTokens: 4,
			},
		},
	}, state)

	require.Len(t, chunks, 2)
	require.NotNil(t, chunks[1].Usage)
	assert.Equal(t, 4, chunks[1].Usage.CacheCreationInputTokens)
	assert.Equal(t, 4, chunks[1].Usage.CacheWriteInputTokens)
	require.NotNil(t, chunks[1].Usage.PromptTokensDetails)
	assert.Equal(t, 4, chunks[1].Usage.PromptTokensDetails.CacheCreationTokens)
	assert.Equal(t, 4, chunks[1].Usage.PromptTokensDetails.CacheWriteTokens)
}

func TestResponsesToChatCompletions_WebSearch(t *testing.T) {
	resp := &ResponsesResponse{
		ID:     "resp_ws",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type:   "web_search_call",
				Action: &WebSearchAction{Type: "search", Query: "test"},
			},
			{
				Type:    "message",
				Content: []ResponsesContentPart{{Type: "output_text", Text: "search results"}},
			},
		},
	}

	chat := ResponsesToChatCompletions(resp, "gpt-4o")
	require.Len(t, chat.Choices, 1)
	assert.Equal(t, "stop", chat.Choices[0].FinishReason)

	var content string
	require.NoError(t, json.Unmarshal(chat.Choices[0].Message.Content, &content))
	assert.Equal(t, "search results", content)
}

// ---------------------------------------------------------------------------
// Streaming: ResponsesEventToChatChunks tests
// ---------------------------------------------------------------------------

func TestResponsesEventToChatChunks_TextDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"

	// response.created → role chunk
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.created",
		Response: &ResponsesResponse{
			ID: "resp_stream",
		},
	}, state)
	require.Len(t, chunks, 1)
	assert.Equal(t, "assistant", chunks[0].Choices[0].Delta.Role)
	assert.True(t, state.SentRole)

	// response.output_text.delta → content chunk
	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: "Hello",
	}, state)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Choices[0].Delta.Content)
	assert.Equal(t, "Hello", *chunks[0].Choices[0].Delta.Content)
}

func TestResponsesEventToChatChunks_ToolCallDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.SentRole = true

	// response.output_item.added (function_call) — output_index=1 (e.g. after a message item at 0)
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_1",
			Name:   "get_weather",
		},
	}, state)
	require.Len(t, chunks, 1)
	require.Len(t, chunks[0].Choices[0].Delta.ToolCalls, 1)
	tc := chunks[0].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Function.Name)
	require.NotNil(t, tc.Index)
	assert.Equal(t, 0, *tc.Index)

	// response.function_call_arguments.delta — uses output_index (NOT call_id) to find tool
	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 1, // matches the output_index from output_item.added above
		Delta:       `{"city":`,
	}, state)
	require.Len(t, chunks, 1)
	tc = chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index)
	assert.Equal(t, 0, *tc.Index, "argument delta must use same index as the tool call")
	assert.Equal(t, `{"city":`, tc.Function.Arguments)

	// Add a second function call at output_index=2
	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 2,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_2",
			Name:   "get_time",
		},
	}, state)
	require.Len(t, chunks, 1)
	tc = chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index)
	assert.Equal(t, 1, *tc.Index, "second tool call should get index 1")

	// Argument delta for second tool call
	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 2,
		Delta:       `{"tz":"UTC"}`,
	}, state)
	require.Len(t, chunks, 1)
	tc = chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index)
	assert.Equal(t, 1, *tc.Index, "second tool arg delta must use index 1")

	// Argument delta for first tool call (interleaved)
	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 1,
		Delta:       `"Tokyo"}`,
	}, state)
	require.Len(t, chunks, 1)
	tc = chunks[0].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tc.Index)
	assert.Equal(t, 0, *tc.Index, "first tool arg delta must still use index 0")
}

func TestResponsesEventToChatChunks_Completed(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.IncludeUsage = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage: &ResponsesUsage{
				InputTokens:  50,
				OutputTokens: 20,
				TotalTokens:  70,
				InputTokensDetails: &ResponsesInputTokensDetails{
					CachedTokens: 30,
				},
			},
		},
	}, state)
	// finish chunk + usage chunk
	require.Len(t, chunks, 2)

	// First chunk: finish_reason
	require.NotNil(t, chunks[0].Choices[0].FinishReason)
	assert.Equal(t, "stop", *chunks[0].Choices[0].FinishReason)

	// Second chunk: usage
	require.NotNil(t, chunks[1].Usage)
	assert.Equal(t, 50, chunks[1].Usage.PromptTokens)
	assert.Equal(t, 20, chunks[1].Usage.CompletionTokens)
	assert.Equal(t, 70, chunks[1].Usage.TotalTokens)
	require.NotNil(t, chunks[1].Usage.PromptTokensDetails)
	assert.Equal(t, 30, chunks[1].Usage.PromptTokensDetails.CachedTokens)
}

func TestResponsesEventToChatChunks_ResponseDoneTopLevelUsage(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5.6-sol"
	state.IncludeUsage = true
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:     "response.done",
		Response: &ResponsesResponse{Status: "completed"},
		Usage: &ResponsesUsage{
			InputTokens: 20, OutputTokens: 10,
			OutputTokensDetails: &ResponsesOutputTokensDetails{ReasoningTokens: 8},
		},
	}, state)
	require.Len(t, chunks, 2)
	require.NotNil(t, chunks[1].Usage.CompletionTokensDetails)
	require.Equal(t, 8, chunks[1].Usage.CompletionTokensDetails.ReasoningTokens)
	require.Nil(t, FinalizeResponsesChatStream(state))
}

func TestResponsesEventToChatChunks_TopLevelUsageWinsOverNestedUsage(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5.6-sol"
	state.IncludeUsage = true
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.done",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 99, OutputTokens: 88},
		},
		Usage: &ResponsesUsage{InputTokens: 20, OutputTokens: 10},
	}, state)
	require.Len(t, chunks, 2)
	require.Equal(t, 20, chunks[1].Usage.PromptTokens)
	require.Equal(t, 10, chunks[1].Usage.CompletionTokens)
}

func TestResponsesEventToChatChunks_ReasoningTextAndCustomToolDeltas(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5.6-sol"
	reasoning := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.reasoning_text.delta", Delta: "plan",
	}, state)
	require.Len(t, reasoning, 1)
	require.Equal(t, "plan", *reasoning[0].Choices[0].Delta.ReasoningContent)

	_ = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.output_item.added", OutputIndex: 2,
		Item: &ResponsesOutput{Type: "custom_tool_call", CallID: "call_1", Name: "lookup"},
	}, state)
	tool := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.custom_tool_call_input.delta", OutputIndex: 2, Delta: `{"q":"x"}`,
	}, state)
	require.Len(t, tool, 1)
	require.Equal(t, `{"q":"x"}`, tool[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)
}

func TestResponsesEventToChatChunks_CompletedWithToolCalls(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.SawToolCall = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
		},
	}, state)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *chunks[0].Choices[0].FinishReason)
}

func TestResponsesEventToChatChunks_ReasoningDelta(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.SentRole = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:  "response.reasoning_summary_text.delta",
		Delta: "Thinking...",
	}, state)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Choices[0].Delta.ReasoningContent)
	assert.Equal(t, "Thinking...", *chunks[0].Choices[0].Delta.ReasoningContent)

	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.reasoning_summary_text.done",
	}, state)
	require.Len(t, chunks, 0)
}

func TestResponsesEventToChatChunks_ReasoningThenTextAutoCloseTag(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.SentRole = true

	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:  "response.reasoning_summary_text.delta",
		Delta: "plan",
	}, state)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Choices[0].Delta.ReasoningContent)
	assert.Equal(t, "plan", *chunks[0].Choices[0].Delta.ReasoningContent)

	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:  "response.output_text.delta",
		Delta: "answer",
	}, state)
	require.Len(t, chunks, 1)
	require.NotNil(t, chunks[0].Choices[0].Delta.Content)
	assert.Equal(t, "answer", *chunks[0].Choices[0].Delta.Content)
}

func TestFinalizeResponsesChatStream(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.IncludeUsage = true
	state.Usage = &ChatUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	chunks := FinalizeResponsesChatStream(state)
	require.Len(t, chunks, 2)

	// Finish chunk
	require.NotNil(t, chunks[0].Choices[0].FinishReason)
	assert.Equal(t, "stop", *chunks[0].Choices[0].FinishReason)

	// Usage chunk
	require.NotNil(t, chunks[1].Usage)
	assert.Equal(t, 100, chunks[1].Usage.PromptTokens)

	// Idempotent: second call returns nil
	assert.Nil(t, FinalizeResponsesChatStream(state))
}

func TestFinalizeResponsesChatStream_AfterCompleted(t *testing.T) {
	// If response.completed already emitted the finish chunk, FinalizeResponsesChatStream
	// must be a no-op (prevents double finish_reason being sent to the client).
	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.IncludeUsage = true

	// Simulate response.completed
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage: &ResponsesUsage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
			},
		},
	}, state)
	require.NotEmpty(t, chunks) // finish + usage chunks

	// Now FinalizeResponsesChatStream should return nil — already finalized.
	assert.Nil(t, FinalizeResponsesChatStream(state))
}

func TestChatChunkToSSE(t *testing.T) {
	chunk := ChatCompletionsChunk{
		ID:      "chatcmpl-test",
		Object:  "chat.completion.chunk",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []ChatChunkChoice{
			{
				Index:        0,
				Delta:        ChatDelta{Role: "assistant"},
				FinishReason: nil,
			},
		},
	}

	sse, err := ChatChunkToSSE(chunk)
	require.NoError(t, err)
	assert.Contains(t, sse, "data: ")
	assert.Contains(t, sse, "chatcmpl-test")
	assert.Contains(t, sse, "assistant")
	assert.True(t, len(sse) > 10)
}

// ---------------------------------------------------------------------------
// Stream round-trip test
// ---------------------------------------------------------------------------

func TestChatCompletionsStreamRoundTrip(t *testing.T) {
	// Simulate: client sends chat completions request, upstream returns Responses SSE events.
	// Verify that the streaming state machine produces correct chat completions chunks.

	state := NewResponsesEventToChatState()
	state.Model = "gpt-4o"
	state.IncludeUsage = true

	var allChunks []ChatCompletionsChunk

	// 1. response.created
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_rt"},
	}, state)
	allChunks = append(allChunks, chunks...)

	// 2. text deltas
	for _, text := range []string{"Hello", ", ", "world", "!"} {
		chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
			Type:  "response.output_text.delta",
			Delta: text,
		}, state)
		allChunks = append(allChunks, chunks...)
	}

	// 3. response.completed
	chunks = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage: &ResponsesUsage{
				InputTokens:  10,
				OutputTokens: 4,
				TotalTokens:  14,
			},
		},
	}, state)
	allChunks = append(allChunks, chunks...)

	// Verify: role chunk + 4 text chunks + finish chunk + usage chunk = 7
	require.Len(t, allChunks, 7)

	// First chunk has role
	assert.Equal(t, "assistant", allChunks[0].Choices[0].Delta.Role)

	// Text chunks
	var fullText string
	for i := 1; i <= 4; i++ {
		require.NotNil(t, allChunks[i].Choices[0].Delta.Content)
		fullText += *allChunks[i].Choices[0].Delta.Content
	}
	assert.Equal(t, "Hello, world!", fullText)

	// Finish chunk
	require.NotNil(t, allChunks[5].Choices[0].FinishReason)
	assert.Equal(t, "stop", *allChunks[5].Choices[0].FinishReason)

	// Usage chunk
	require.NotNil(t, allChunks[6].Usage)
	assert.Equal(t, 10, allChunks[6].Usage.PromptTokens)
	assert.Equal(t, 4, allChunks[6].Usage.CompletionTokens)

	// All chunks share the same ID
	for _, c := range allChunks {
		assert.Equal(t, "resp_rt", c.ID)
	}
}

// ---------------------------------------------------------------------------
// BufferedResponseAccumulator tests
// ---------------------------------------------------------------------------

func TestBufferedResponseAccumulator_TextOnly(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hello"})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: ", world!"})

	assert.True(t, acc.HasContent())

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "message", output[0].Type)
	assert.Equal(t, "assistant", output[0].Role)
	require.Len(t, output[0].Content, 1)
	assert.Equal(t, "output_text", output[0].Content[0].Type)
	assert.Equal(t, "Hello, world!", output[0].Content[0].Text)
}

func TestBufferedResponseAccumulator_ToolCalls(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	// Add function call at output_index=1
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 1,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_abc",
			Name:   "get_weather",
		},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 1,
		Delta:       `{"city":`,
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 1,
		Delta:       `"NYC"}`,
	})

	assert.True(t, acc.HasContent())

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "function_call", output[0].Type)
	assert.Equal(t, "call_abc", output[0].CallID)
	assert.Equal(t, "get_weather", output[0].Name)
	assert.Equal(t, `{"city":"NYC"}`, output[0].Arguments)
}

func TestBufferedResponseAccumulator_CustomToolCall(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 3,
		Item: &ResponsesOutput{
			Type:   "custom_tool_call",
			CallID: "call_custom",
			Name:   "apply_patch",
		},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.delta",
		OutputIndex: 3,
		Delta:       `{"patch":`,
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.custom_tool_call_input.delta",
		OutputIndex: 3,
		Delta:       `"file.txt"}`,
	})

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	// The buffered bridge normalizes custom tools into the Chat-compatible
	// function_call output shape after preserving their input deltas.
	assert.Equal(t, "function_call", output[0].Type)
	assert.Equal(t, "call_custom", output[0].CallID)
	assert.Equal(t, "apply_patch", output[0].Name)
	assert.Equal(t, `{"patch":"file.txt"}`, output[0].Arguments)
}

func TestBufferedResponseAccumulator_Reasoning(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "Step 1: "})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "think about it"})

	assert.True(t, acc.HasContent())

	output := acc.BuildOutput()
	require.Len(t, output, 1)
	assert.Equal(t, "reasoning", output[0].Type)
	require.Len(t, output[0].Summary, 1)
	assert.Equal(t, "summary_text", output[0].Summary[0].Type)
	assert.Equal(t, "Step 1: think about it", output[0].Summary[0].Text)
}

func TestBufferedResponseAccumulator_Mixed(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	// Reasoning first
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", Delta: "I thought about it."})

	// Then text
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "The answer is 42."})

	// Then a tool call
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 2,
		Item: &ResponsesOutput{
			Type:   "function_call",
			CallID: "call_1",
			Name:   "verify",
		},
	})
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 2,
		Delta:       `{}`,
	})

	assert.True(t, acc.HasContent())

	output := acc.BuildOutput()
	// Order: reasoning → message → function_calls
	require.Len(t, output, 3)
	assert.Equal(t, "reasoning", output[0].Type)
	assert.Equal(t, "message", output[1].Type)
	assert.Equal(t, "function_call", output[2].Type)
	assert.Equal(t, "The answer is 42.", output[1].Content[0].Text)
	assert.Equal(t, "verify", output[2].Name)
}

func TestBufferedResponseAccumulator_SupplementEmptyOutput(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "Hello"})

	resp := &ResponsesResponse{
		ID:     "resp_1",
		Status: "completed",
		Output: nil, // empty output
		Usage:  &ResponsesUsage{InputTokens: 10, OutputTokens: 5},
	}

	acc.SupplementResponseOutput(resp)

	require.Len(t, resp.Output, 1)
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "Hello", resp.Output[0].Content[0].Text)
	// Usage should be untouched
	assert.Equal(t, 10, resp.Usage.InputTokens)
}

func TestBufferedResponseAccumulator_NoSupplementWhenOutputExists(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: "from deltas"})

	resp := &ResponsesResponse{
		ID:     "resp_2",
		Status: "completed",
		Output: []ResponsesOutput{
			{
				Type: "message",
				Content: []ResponsesContentPart{
					{Type: "output_text", Text: "from terminal event"},
				},
			},
		},
	}

	acc.SupplementResponseOutput(resp)

	// Output should NOT be overwritten
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "from terminal event", resp.Output[0].Content[0].Text)
}

func TestBufferedResponseAccumulator_EmptyDeltas(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	// Process events with empty delta — should not accumulate
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_text.delta", Delta: ""})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.created"})

	assert.False(t, acc.HasContent())

	resp := &ResponsesResponse{ID: "resp_3", Status: "completed"}
	acc.SupplementResponseOutput(resp)
	assert.Nil(t, resp.Output)
}

func TestBufferedResponseAccumulator_IgnoresNonFunctionCallItems(t *testing.T) {
	acc := NewBufferedResponseAccumulator()

	// output_item.added with type "message" should be ignored
	acc.ProcessEvent(&ResponsesStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item:        &ResponsesOutput{Type: "message"},
	})

	assert.False(t, acc.HasContent())
}
