package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// isCodexToolCallInputType matches call-input item types whose IDs are
// validated by the Responses API as function-call IDs (fc...). Output items
// use a different ID namespace and must not be sanitized by this helper.
func isCodexToolCallInputType(typ string) bool {
	switch typ {
	case "function_call", "tool_call", "local_shell_call", "tool_search_call", "custom_tool_call", "mcp_tool_call":
		return true
	default:
		return false
	}
}

// Invalid replayed IDs are removed rather than rewritten because a fabricated
// msg/fc ID may point at a different upstream object.
func shouldStripOpenAIResponsesInputItemID(itemType, id string) bool {
	if id == "" {
		return false
	}
	if itemType == "message" {
		return !strings.HasPrefix(id, "msg")
	}
	if itemType == "reasoning" {
		return !strings.HasPrefix(id, "rs")
	}
	if isCodexToolCallInputType(itemType) {
		return !strings.HasPrefix(id, "fc")
	}
	return false
}

func sanitizeOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}

	items := make([][]byte, 0)
	changed := false
	var sanitizeErr error
	index := 0
	input.ForEach(func(_, item gjson.Result) bool {
		currentIndex := index
		index++
		itemBody := []byte(item.Raw)
		if item.IsObject() {
			itemType := item.Get("type")
			id := item.Get("id")
			if itemType.Type == gjson.String && id.Type == gjson.String &&
				shouldStripOpenAIResponsesInputItemID(itemType.String(), id.String()) {
				itemBody, sanitizeErr = sjson.DeleteBytes(itemBody, "id")
				if sanitizeErr != nil {
					sanitizeErr = fmt.Errorf("delete input.%d.id: %w", currentIndex, sanitizeErr)
					return false
				}
				changed = true
			}
		}
		items = append(items, itemBody)
		return true
	})
	if sanitizeErr != nil {
		return nil, false, sanitizeErr
	}
	if !changed {
		return body, false, nil
	}

	rebuiltInput := make([]byte, 0, len(input.Raw))
	rebuiltInput = append(rebuiltInput, '[')
	for i, item := range items {
		if i > 0 {
			rebuiltInput = append(rebuiltInput, ',')
		}
		rebuiltInput = append(rebuiltInput, item...)
	}
	rebuiltInput = append(rebuiltInput, ']')

	sanitized, err := sjson.SetRawBytes(body, "input", rebuiltInput)
	if err != nil {
		return nil, false, fmt.Errorf("replace sanitized input: %w", err)
	}
	return sanitized, true, nil
}
