package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ChatCompletionsToResponsesResponse(resp *ChatCompletionsResponse, logicalModel string) (*ResponsesResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("chat response is nil")
	}
	if len(resp.Choices) != 1 {
		return nil, fmt.Errorf("non-stream text profile requires exactly one Chat choice")
	}
	choice := resp.Choices[0]
	if len(choice.Message.ToolCalls) > 0 || choice.Message.FunctionCall != nil || choice.Message.ReasoningContent != "" {
		return nil, fmt.Errorf("chat response is outside the non-stream text profile")
	}
	var text string
	if len(choice.Message.Content) > 0 && string(choice.Message.Content) != "null" {
		if err := json.Unmarshal(choice.Message.Content, &text); err != nil {
			return nil, fmt.Errorf("chat response content is not text: %w", err)
		}
	}
	status := "completed"
	var incomplete *ResponsesIncompleteDetails
	switch choice.FinishReason {
	case "stop", "":
	case "length":
		status = "incomplete"
		incomplete = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	default:
		return nil, fmt.Errorf("chat finish reason %q is outside the text profile", choice.FinishReason)
	}
	id := strings.TrimSpace(resp.ID)
	if id == "" {
		id = "resp_" + generateChatCmplID()
	}
	model := strings.TrimSpace(logicalModel)
	if model == "" {
		model = resp.Model
	}
	out := &ResponsesResponse{
		ID:                id,
		Object:            "response",
		Model:             model,
		Status:            status,
		IncompleteDetails: incomplete,
		Output: []ResponsesOutput{{
			Type:    "message",
			ID:      "msg_" + id,
			Role:    "assistant",
			Status:  status,
			Content: []ResponsesContentPart{{Type: "output_text", Text: text}},
		}},
	}
	if resp.Usage != nil {
		out.Usage = &ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
		if resp.Usage.PromptTokensDetails != nil {
			out.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
				CachedTokens:        resp.Usage.PromptTokensDetails.CachedTokens,
				CacheCreationTokens: resp.Usage.PromptTokensDetails.CacheCreationTokens,
				CacheWriteTokens:    resp.Usage.PromptTokensDetails.CacheWriteTokens,
			}
		}
		if resp.Usage.CompletionTokensDetails != nil {
			out.Usage.OutputTokensDetails = &ResponsesOutputTokensDetails{
				ReasoningTokens: resp.Usage.CompletionTokensDetails.ReasoningTokens,
			}
		}
	}
	return out, nil
}
