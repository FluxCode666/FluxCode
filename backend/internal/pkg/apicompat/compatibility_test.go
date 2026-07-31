package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryResponsesToChatAllowsOnlyDeclaredNonStreamTextSubset(t *testing.T) {
	registry := NewRegistry()

	compatible := []byte(`{
        "model":"deepseek-chat",
        "instructions":"Be concise",
        "input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],
        "max_output_tokens":256,
        "stream":false,
        "store":false
    }`)
	result := registry.CheckRequest(ProtocolResponses, ProtocolChatCompletions, compatible)
	require.True(t, result.Compatible)
	require.Equal(t, ProfileNonStreamText, result.Profile)

	for name, raw := range map[string][]byte{
		"managed continuation": []byte(`{"model":"m","input":"hi","previous_response_id":"resp_1"}`),
		"reasoning":            []byte(`{"model":"m","input":"hi","reasoning":{"effort":"high"}}`),
		"built in tool":        []byte(`{"model":"m","input":"hi","tools":[{"type":"web_search"}]}`),
		"stream separate":      []byte(`{"model":"m","input":"hi","stream":true}`),
		"unknown field":        []byte(`{"model":"m","input":"hi","vendor_magic":true}`),
		"store true":           []byte(`{"model":"m","input":"hi","store":true}`),
		"store omitted":        []byte(`{"model":"m","input":"hi"}`),
		"store null":           []byte(`{"model":"m","input":"hi","store":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			result := registry.CheckRequest(ProtocolResponses, ProtocolChatCompletions, raw)
			require.False(t, result.Compatible)
			require.NotEmpty(t, result.ReasonCode)
		})
	}
}

func TestRegistryNeverOffersEmbeddingConversion(t *testing.T) {
	registry := NewRegistry()
	result := registry.CheckRequest(ProtocolEmbeddings, ProtocolChatCompletions, []byte(`{"model":"m","input":"hi"}`))
	require.False(t, result.Compatible)
	require.Equal(t, ReasonNoAdapter, result.ReasonCode)
}

func TestResponsesToChatCompletionsRequestNonStreamText(t *testing.T) {
	maxTokens := 320
	store := false
	req := &ResponsesRequest{
		Model:        "logical-model",
		Instructions: "system text",
		Input: json.RawMessage(`[{
            "role":"user",
            "content":[{"type":"input_text","text":"hello"}]
        }]`),
		MaxOutputTokens: &maxTokens,
		Store:           &store,
	}

	chat, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Equal(t, "logical-model", chat.Model)
	require.Len(t, chat.Messages, 2)
	require.Equal(t, "system", chat.Messages[0].Role)
	require.JSONEq(t, `"system text"`, string(chat.Messages[0].Content))
	require.Equal(t, "user", chat.Messages[1].Role)
	require.JSONEq(t, `"hello"`, string(chat.Messages[1].Content))
	require.Equal(t, maxTokens, *chat.MaxCompletionTokens)
	require.False(t, chat.Stream)
}

func TestChatCompletionsToResponsesResponseNonStreamText(t *testing.T) {
	chat := &ChatCompletionsResponse{
		ID:    "chatcmpl_1",
		Model: "upstream-model",
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: json.RawMessage(`"answer"`)},
			FinishReason: "stop",
		}},
		Usage: &ChatUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10},
	}

	response, err := ChatCompletionsToResponsesResponse(chat, "logical-model")
	require.NoError(t, err)
	require.Equal(t, "completed", response.Status)
	require.Equal(t, "logical-model", response.Model)
	require.Len(t, response.Output, 1)
	require.Equal(t, "answer", response.Output[0].Content[0].Text)
	require.Equal(t, 7, response.Usage.InputTokens)
	require.Equal(t, 3, response.Usage.OutputTokens)
}

func TestCheckedResponsesEventRejectsUnknownEvent(t *testing.T) {
	state := NewResponsesEventToChatState()
	chunks, err := ResponsesEventToChatChunksChecked(&ResponsesStreamEvent{Type: "response.vendor_magic"}, state)
	require.Error(t, err)
	require.Nil(t, chunks)
}
