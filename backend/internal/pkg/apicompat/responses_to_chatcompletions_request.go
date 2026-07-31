package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func ResponsesToChatCompletionsRequest(req *ResponsesRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("responses request is nil")
	}
	if req.Stream {
		return nil, fmt.Errorf("streaming Responses-to-Chat conversion is not enabled")
	}
	if req.Store != nil && *req.Store {
		return nil, fmt.Errorf("stored Responses state cannot be represented by Chat Completions")
	}
	if len(req.Tools) > 0 || req.Reasoning != nil || len(req.Include) > 0 || req.Text != nil || len(req.ToolChoice) > 0 {
		return nil, fmt.Errorf("request uses a feature outside the non-stream text profile")
	}
	messages, err := responsesTextInputToChatMessages(req.Input)
	if err != nil {
		return nil, err
	}
	if req.Instructions != "" {
		raw, err := json.Marshal(req.Instructions)
		if err != nil {
			return nil, err
		}
		messages = append([]ChatMessage{{Role: "system", Content: raw}}, messages...)
	}
	out := &ChatCompletionsRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      false,
		ServiceTier: req.ServiceTier,
	}
	if req.MaxOutputTokens != nil {
		value := *req.MaxOutputTokens
		out.MaxCompletionTokens = &value
	}
	return out, nil
}

func responsesTextInputToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("responses input is required")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		content, _ := json.Marshal(text)
		return []ChatMessage{{Role: "user", Content: content}}, nil
	}
	var items []ResponsesInputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parse Responses input: %w", err)
	}
	messages := make([]ChatMessage, 0, len(items))
	for _, item := range items {
		if item.Type != "" && item.Type != "message" {
			return nil, fmt.Errorf("Responses input item %q is not supported by text profile", item.Type)
		}
		if item.Role != "system" && item.Role != "user" && item.Role != "assistant" {
			return nil, fmt.Errorf("Responses role %q is not supported", item.Role)
		}
		text, err := flattenResponsesTextContent(item.Content)
		if err != nil {
			return nil, err
		}
		content, _ := json.Marshal(text)
		messages = append(messages, ChatMessage{Role: item.Role, Content: content})
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("Responses input must not be empty")
	}
	return messages, nil
}

func flattenResponsesTextContent(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", fmt.Errorf("Responses message content is required")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("parse Responses content: %w", err)
	}
	var buffer bytes.Buffer
	for _, part := range parts {
		if part.Type != "input_text" && part.Type != "output_text" {
			return "", fmt.Errorf("Responses content part %q is not supported by text profile", part.Type)
		}
		buffer.WriteString(part.Text)
	}
	return buffer.String(), nil
}
