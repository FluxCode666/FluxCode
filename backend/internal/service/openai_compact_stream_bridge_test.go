package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOpenAICompactSSEPayloadPreservesOutputAndTerminalResponse(t *testing.T) {
	payload, ok := buildOpenAICompactSSEPayload([]byte(`{
		"id":"resp_compact",
		"object":"response.compaction",
		"status":"completed",
		"output":[{"id":"msg_1","type":"message","role":"assistant","content":[]}],
		"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}
	}`))
	require.True(t, ok)
	text := string(payload)
	require.Contains(t, text, "event: response.output_item.done\n")
	require.Contains(t, text, `"id":"msg_1"`)
	require.Contains(t, text, "event: response.completed\n")
	require.Contains(t, text, `"id":"resp_compact"`)
	require.Contains(t, text, `"total_tokens":12`)
}

func TestBuildOpenAICompactSSEPayloadRejectsInvalidResponse(t *testing.T) {
	for _, body := range [][]byte{nil, []byte(`null`), []byte(`[]`), []byte(`not-json`)} {
		payload, ok := buildOpenAICompactSSEPayload(body)
		require.False(t, ok)
		require.Nil(t, payload)
	}
}
