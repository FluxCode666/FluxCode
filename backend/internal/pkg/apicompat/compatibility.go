package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
)

var responsesToChatTopLevelFields = map[string]struct{}{
	"model": {}, "input": {}, "instructions": {}, "max_output_tokens": {},
	"temperature": {}, "top_p": {}, "stream": {}, "store": {}, "service_tier": {},
}

func checkResponsesToChatRequest(raw []byte) CompatibilityResult {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil || root == nil {
		return incompatible(ReasonInvalidJSON, "request must be a JSON object")
	}
	for field := range root {
		if _, ok := responsesToChatTopLevelFields[field]; !ok {
			reason := ReasonUnknownField
			if field == "previous_response_id" || field == "conversation" || field == "background" {
				reason = ReasonManagedState
			}
			return incompatible(reason, fmt.Sprintf("field %q is not in the adapter allowlist", field))
		}
	}
	if rawStream, ok := root["stream"]; ok {
		var stream bool
		if err := json.Unmarshal(rawStream, &stream); err != nil {
			return incompatible(ReasonInvalidJSON, "stream must be boolean")
		}
		if stream {
			return incompatible(ReasonStreamUnsupported, "streaming has a separate compatibility profile")
		}
	}
	rawStore, ok := root["store"]
	if !ok || bytes.Equal(bytes.TrimSpace(rawStore), []byte("null")) {
		return incompatible(ReasonManagedState, "Responses-to-Chat requires explicit store=false")
	}
	var store bool
	if err := json.Unmarshal(rawStore, &store); err != nil {
		return incompatible(ReasonInvalidJSON, "store must be boolean")
	}
	if store {
		return incompatible(ReasonManagedState, "stored Responses state cannot be represented by Chat Completions")
	}
	if rawInput, ok := root["input"]; !ok {
		return incompatible(ReasonInputUnsupported, "input is required")
	} else if err := validateTextResponsesInput(rawInput); err != nil {
		return incompatible(ReasonInputUnsupported, err.Error())
	}
	var model string
	if err := json.Unmarshal(root["model"], &model); err != nil || model == "" {
		return incompatible(ReasonInputUnsupported, "model is required")
	}
	return CompatibilityResult{Compatible: true, Profile: ProfileNonStreamText}
}

func validateTextResponsesInput(raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("input must not be empty")
	}
	var text string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return fmt.Errorf("input string is invalid")
		}
		return nil
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return fmt.Errorf("input must be a string or non-empty message array")
	}
	for _, item := range items {
		for key := range item {
			if key != "type" && key != "role" && key != "content" {
				return fmt.Errorf("input item field %q is unsupported", key)
			}
		}
		if rawType, ok := item["type"]; ok {
			var typ string
			if err := json.Unmarshal(rawType, &typ); err != nil || (typ != "" && typ != "message") {
				return fmt.Errorf("only message input items are supported")
			}
		}
		var role string
		if err := json.Unmarshal(item["role"], &role); err != nil || (role != "system" && role != "user" && role != "assistant") {
			return fmt.Errorf("only system, user and assistant roles are supported")
		}
		if err := validateTextContent(item["content"]); err != nil {
			return err
		}
	}
	return nil
}

func validateTextContent(raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("message content is required")
	}
	var text string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return fmt.Errorf("message text is invalid")
		}
		return nil
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) == 0 {
		return fmt.Errorf("message content must be text")
	}
	for _, part := range parts {
		if len(part) != 2 {
			return fmt.Errorf("content part contains unsupported fields")
		}
		var typ string
		if err := json.Unmarshal(part["type"], &typ); err != nil || (typ != "input_text" && typ != "output_text") {
			return fmt.Errorf("only input_text and output_text parts are supported")
		}
		if _, ok := part["text"]; !ok {
			return fmt.Errorf("text content part is missing text")
		}
	}
	return nil
}

func incompatible(reason, detail string) CompatibilityResult {
	return CompatibilityResult{ReasonCode: reason, Detail: detail}
}
