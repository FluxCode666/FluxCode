package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type runtimePromptSettingsStub struct {
	byPlatform map[string]EffectiveSystemPrompt
}

func (s runtimePromptSettingsStub) GetSystemPromptSettings(context.Context) map[string]EffectiveSystemPrompt {
	return s.byPlatform
}

func TestApplyResolvedSystemPromptToJSONUsesAPIKeyPriority(t *testing.T) {
	settings := runtimePromptSettingsStub{byPlatform: map[string]EffectiveSystemPrompt{
		PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeAppend, Source: SystemPromptSourceSystem},
	}}
	apiKey := &APIKey{
		SystemPrompt:     "key",
		SystemPromptMode: SystemPromptModeOverride,
		Group: &Group{
			SystemPrompt:     "group",
			SystemPromptMode: SystemPromptModeAppend,
		},
	}
	ctx := WithAPIKeyContext(context.Background(), apiKey)
	body := []byte(`{"model":"gpt-5","instructions":"client"}`)

	got, changed, err := applyResolvedSystemPromptToJSON(ctx, nil, body, PlatformOpenAI, PlatformOpenAI, settings)

	require.NoError(t, err)
	require.True(t, changed)
	var req map[string]any
	require.NoError(t, json.Unmarshal(got, &req))
	require.Equal(t, "key", req["instructions"])
}

func TestApplyResolvedSystemPromptToJSONFallsBackToGroup(t *testing.T) {
	settings := runtimePromptSettingsStub{byPlatform: map[string]EffectiveSystemPrompt{
		PlatformAnthropic: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
	}}
	apiKey := &APIKey{
		SystemPromptMode: SystemPromptModeInherit,
		Group: &Group{
			SystemPrompt:     "group",
			SystemPromptMode: SystemPromptModeAppend,
		},
	}
	ctx := WithAPIKeyContext(context.Background(), apiKey)
	body := []byte(`{"model":"claude-sonnet-4-5","system":"client"}`)

	got, changed, err := applyResolvedSystemPromptToJSON(ctx, nil, body, PlatformAnthropic, PlatformAnthropic, settings)

	require.NoError(t, err)
	require.True(t, changed)
	systemParts := gjsonArray(t, got, "system")
	require.Len(t, systemParts, 2)
	require.Equal(t, "group", systemParts[0]["text"])
	require.Equal(t, "client", systemParts[1]["text"])
}

func TestApplyResolvedSystemPromptToChatCompletionsKeepsExistingOnPassthrough(t *testing.T) {
	settings := runtimePromptSettingsStub{byPlatform: map[string]EffectiveSystemPrompt{
		PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModePassthrough, Source: SystemPromptSourceSystem},
	}}
	ctx := WithAPIKeyContext(context.Background(), &APIKey{SystemPromptMode: SystemPromptModeInherit})
	body := []byte(`{"model":"gpt-5","messages":[{"role":"system","content":"client"},{"role":"user","content":"hi"}]}`)

	got, changed, err := applyResolvedSystemPromptToChatCompletionsJSON(ctx, nil, body, PlatformOpenAI, settings)

	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(body), string(got))
}

func TestApplyResolvedSystemPromptToJSONNoopsWhenInherited(t *testing.T) {
	ctx := WithAPIKeyContext(context.Background(), &APIKey{SystemPromptMode: SystemPromptModeInherit})
	body := []byte(`{"model":"gemini-2.5-pro","contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)

	got, changed, err := applyResolvedSystemPromptToJSON(ctx, nil, body, PlatformGemini, PlatformGemini, runtimePromptSettingsStub{})

	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(body), string(got))
}

func gjsonArray(t *testing.T, body []byte, key string) []map[string]any {
	t.Helper()
	var req map[string]any
	require.NoError(t, json.Unmarshal(body, &req))
	raw, ok := req[key].([]any)
	require.True(t, ok)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		out = append(out, m)
	}
	return out
}
