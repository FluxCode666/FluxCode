//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type promptSettingsStub struct {
	byPlatform map[string]EffectiveSystemPrompt
}

func (s promptSettingsStub) GetSystemPromptSettings(context.Context) map[string]EffectiveSystemPrompt {
	return s.byPlatform
}

func TestResolveEffectiveSystemPrompt_Priority(t *testing.T) {
	settings := promptSettingsStub{byPlatform: map[string]EffectiveSystemPrompt{
		PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
	}}
	apiKey := &APIKey{
		SystemPrompt:     "key",
		SystemPromptMode: SystemPromptModeAppend,
		Group: &Group{
			SystemPrompt:     "group",
			SystemPromptMode: SystemPromptModePassthrough,
		},
	}

	got := ResolveEffectiveSystemPrompt(context.Background(), apiKey, PlatformOpenAI, settings)

	require.Equal(t, "key", got.Prompt)
	require.Equal(t, SystemPromptModeAppend, got.Mode)
	require.Equal(t, SystemPromptSourceAPIKey, got.Source)
}

func TestResolveEffectiveSystemPrompt_InheritsThroughGroupToSystem(t *testing.T) {
	settings := promptSettingsStub{byPlatform: map[string]EffectiveSystemPrompt{
		PlatformGemini: {Prompt: "platform", Mode: SystemPromptModePassthrough, Source: SystemPromptSourceSystem},
	}}
	apiKey := &APIKey{
		SystemPromptMode: SystemPromptModeInherit,
		Group:            &Group{SystemPromptMode: SystemPromptModeInherit},
	}

	got := ResolveEffectiveSystemPrompt(context.Background(), apiKey, PlatformGemini, settings)

	require.Equal(t, "platform", got.Prompt)
	require.Equal(t, SystemPromptModePassthrough, got.Mode)
	require.Equal(t, SystemPromptSourceSystem, got.Source)
}

func TestResolveEffectiveSystemPrompt_AllInheritReturnsDisabled(t *testing.T) {
	got := ResolveEffectiveSystemPrompt(context.Background(), &APIKey{
		SystemPromptMode: SystemPromptModeInherit,
		Group:            &Group{SystemPromptMode: SystemPromptModeInherit},
	}, PlatformAnthropic, promptSettingsStub{})

	require.False(t, got.Enabled())
	require.Equal(t, SystemPromptModeInherit, got.Mode)
}

func TestApplySystemPromptToAnthropic_AppendStringSystem(t *testing.T) {
	body := []byte(`{"model":"claude","system":"client","messages":[{"role":"user","content":"hi"}]}`)
	got, changed, err := ApplySystemPromptToJSON(body, PlatformAnthropic, EffectiveSystemPrompt{
		Prompt: "server",
		Mode:   SystemPromptModeAppend,
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"model":"claude","system":[{"type":"text","text":"server"},{"type":"text","text":"client"}],"messages":[{"role":"user","content":"hi"}]}`, string(got))
}

func TestApplySystemPromptToOpenAIResponses_PassthroughKeepsExisting(t *testing.T) {
	body := []byte(`{"model":"gpt","instructions":"client","input":"hi"}`)
	got, changed, err := ApplySystemPromptToJSON(body, PlatformOpenAI, EffectiveSystemPrompt{
		Prompt: "server",
		Mode:   SystemPromptModePassthrough,
	})
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(body), string(got))
}

func TestApplySystemPromptToChatCompletions_Override(t *testing.T) {
	body := []byte(`{"model":"gpt","messages":[{"role":"system","content":"client"},{"role":"user","content":"hi"}]}`)
	got, changed, err := ApplySystemPromptToChatCompletionsJSON(body, EffectiveSystemPrompt{
		Prompt: "server",
		Mode:   SystemPromptModeOverride,
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"model":"gpt","messages":[{"role":"system","content":"server"},{"role":"user","content":"hi"}]}`, string(got))
}

func TestApplySystemPromptToGemini_AppendSystemInstruction(t *testing.T) {
	body := []byte(`{"model":"gemini","systemInstruction":{"parts":[{"text":"client"}]},"contents":[]}`)
	got, changed, err := ApplySystemPromptToJSON(body, PlatformGemini, EffectiveSystemPrompt{
		Prompt: "server",
		Mode:   SystemPromptModeAppend,
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"model":"gemini","systemInstruction":{"parts":[{"text":"server"},{"text":"client"}]},"contents":[]}`, string(got))
}
