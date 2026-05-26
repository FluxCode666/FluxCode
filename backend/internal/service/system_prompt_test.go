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
