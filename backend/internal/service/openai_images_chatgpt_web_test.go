package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatGPTWebImageModelSlug(t *testing.T) {
	require.Equal(t, "gpt-5-3", chatGPTWebImageModelSlug("gpt-image-2"))
	require.Equal(t, "auto", chatGPTWebImageModelSlug(""))
	require.Equal(t, "auto", chatGPTWebImageModelSlug("unknown-model"))
	require.Equal(t, "auto", chatGPTWebImageModelSlug("  "))
}

func TestIsOpenAIFreeAccount(t *testing.T) {
	tests := []struct {
		name     string
		planType string
		want     bool
	}{
		{"free", "free", true},
		{"Free uppercase", "Free", true},
		{"empty", "", true},
		{"plus", "plus", false},
		{"pro", "pro", false},
		{"team", "team", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Credentials: map[string]any{}}
			if tt.planType != "" {
				a.Credentials["plan_type"] = tt.planType
			}
			require.Equal(t, tt.want, isOpenAIFreeAccount(a))
		})
	}
}

func TestBuildChatGPTWebPreparePayload(t *testing.T) {
	body := buildChatGPTWebPreparePayload("draw a cat", "gpt-image-2")
	require.Equal(t, "gpt-5-3", body["model"])
	require.Equal(t, []string{"picture_v2"}, body["system_hints"])
	require.Equal(t, "success", body["client_prepare_state"])
	require.Equal(t, "next", body["action"])

	pq, ok := body["partial_query"].(map[string]any)
	require.True(t, ok)
	content, ok := pq["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content["content_type"])
	parts, ok := content["parts"].([]string)
	require.True(t, ok)
	require.Equal(t, []string{"draw a cat"}, parts)
}

func TestBuildChatGPTWebGeneratePayload_TextOnly(t *testing.T) {
	body := buildChatGPTWebGeneratePayload("draw a cat", "gpt-image-2", nil)
	require.Equal(t, "gpt-5-3", body["model"])
	require.Equal(t, "sent", body["client_prepare_state"])
	require.Equal(t, []string{"picture_v2"}, body["system_hints"])

	msgs, ok := body["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, msgs, 1)
	content, ok := msgs[0]["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", content["content_type"])
}

func TestBuildChatGPTWebGeneratePayload_WithReferences(t *testing.T) {
	refs := []chatGPTWebImageRef{{FileID: "file-abc", Width: 100, Height: 100, FileSize: 1024, MimeType: "image/png", FileName: "test.png"}}
	body := buildChatGPTWebGeneratePayload("edit this", "gpt-image-2", refs)

	msgs, ok := body["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, msgs, 1)

	content, ok := msgs[0]["content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "multimodal_text", content["content_type"])

	parts, ok := content["parts"].([]any)
	require.True(t, ok)
	require.Len(t, parts, 2) // 1 image pointer + 1 text prompt

	meta, ok := msgs[0]["metadata"].(map[string]any)
	require.True(t, ok)
	attachments, ok := meta["attachments"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, attachments, 1)
	require.Equal(t, "file-abc", attachments[0]["id"])
}

func TestParseChatGPTWebSSE_ExtractsFileIDs(t *testing.T) {
	sseData := strings.Join([]string{
		`data: {"message":{"id":"msg-1","author":{"role":"tool"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://file-abc123"}]}},"conversation_id":"conv-xyz"}`,
		`data: [DONE]`,
	}, "\n")
	state := parseChatGPTWebSSEStream(strings.NewReader(sseData))
	require.Equal(t, "conv-xyz", state.ConversationID)
	require.Contains(t, state.FileIDs, "file-abc123")
	require.False(t, state.Blocked)
}

func TestParseChatGPTWebSSE_DetectsBlocked(t *testing.T) {
	sseData := strings.Join([]string{
		`data: {"type":"moderation","moderation_response":{"blocked":true}}`,
		`data: [DONE]`,
	}, "\n")
	state := parseChatGPTWebSSEStream(strings.NewReader(sseData))
	require.True(t, state.Blocked)
}

func TestParseChatGPTWebSSE_ExtractsSedimentIDs(t *testing.T) {
	sseData := strings.Join([]string{
		`data: {"message":{"id":"msg-1","author":{"role":"tool"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":["sediment://sed-abc123"]}},"conversation_id":"conv-123"}`,
		`data: [DONE]`,
	}, "\n")
	state := parseChatGPTWebSSEStream(strings.NewReader(sseData))
	require.Equal(t, "conv-123", state.ConversationID)
	require.Contains(t, state.SedimentIDs, "sed-abc123")
}
