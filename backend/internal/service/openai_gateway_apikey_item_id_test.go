package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_APIKeyPassthrough_StripsInvalidReasoningItemIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_test","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		)),
	}}
	svc := newImageControlService(upstream)
	c, _ := newImageControlContext("/v1/responses", true, "codex_cli_rs/0.144.1")
	account := newImageControlAccount()
	account.Extra = map[string]any{"openai_passthrough": true}

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":false,
		"input":[
			{"type":"reasoning","id":"item_bad_reasoning","summary":[]},
			{"type":"reasoning","id":"rs_valid","summary":[]},
			{"type":"message","id":"msg_valid","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]
	}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.False(t, gjson.GetBytes(upstream.lastBody, "input.0.id").Exists())
	require.Equal(t, "rs_valid", gjson.GetBytes(upstream.lastBody, "input.1.id").String())
	require.Equal(t, "msg_valid", gjson.GetBytes(upstream.lastBody, "input.2.id").String())
}

func TestShouldStripOpenAIResponsesInputItemID_Reasoning(t *testing.T) {
	tests := []struct {
		name     string
		itemType string
		id       string
		want     bool
	}{
		{name: "reasoning item id", itemType: "reasoning", id: "item_bad_reasoning", want: true},
		{name: "reasoning rs id", itemType: "reasoning", id: "rs_abc123", want: false},
		{name: "reasoning empty id", itemType: "reasoning", id: "", want: false},
		{name: "message msg id", itemType: "message", id: "msg_abc", want: false},
		{name: "message item id", itemType: "message", id: "item_x", want: true},
		{name: "function call fc id", itemType: "function_call", id: "fc_abc", want: false},
		{name: "function call item id", itemType: "function_call", id: "item_x", want: true},
		{name: "unconstrained type", itemType: "web_search_call", id: "ws_001", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldStripOpenAIResponsesInputItemID(tt.itemType, tt.id))
		})
	}
}
