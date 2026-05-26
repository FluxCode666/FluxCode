package service

import (
	"context"
	"strings"
)

type EffectiveSystemPrompt struct {
	Prompt string
	Mode   string
	Source string
}

func (p EffectiveSystemPrompt) Enabled() bool {
	return strings.TrimSpace(p.Prompt) != "" && IsSystemPromptInjectionMode(p.Mode)
}

type SystemPromptSettingsProvider interface {
	GetSystemPromptSettings(ctx context.Context) map[string]EffectiveSystemPrompt
}

func NormalizeSystemPromptMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case SystemPromptModePassthrough, SystemPromptModeOverride, SystemPromptModeAppend:
		return strings.TrimSpace(mode)
	default:
		return SystemPromptModeInherit
	}
}

func IsSystemPromptInjectionMode(mode string) bool {
	switch mode {
	case SystemPromptModePassthrough, SystemPromptModeOverride, SystemPromptModeAppend:
		return true
	default:
		return false
	}
}

func ResolveEffectiveSystemPrompt(ctx context.Context, apiKey *APIKey, platform string, settings SystemPromptSettingsProvider) EffectiveSystemPrompt {
	if apiKey != nil {
		if p := promptFromLayer(apiKey.SystemPrompt, apiKey.SystemPromptMode, SystemPromptSourceAPIKey); p.Enabled() {
			return p
		}
		if apiKey.Group != nil {
			if p := promptFromLayer(apiKey.Group.SystemPrompt, apiKey.Group.SystemPromptMode, SystemPromptSourceGroup); p.Enabled() {
				return p
			}
		}
	}
	if settings != nil {
		byPlatform := settings.GetSystemPromptSettings(ctx)
		if p, ok := byPlatform[strings.TrimSpace(platform)]; ok && p.Enabled() {
			return p
		}
	}
	return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
}

func promptFromLayer(prompt, mode, source string) EffectiveSystemPrompt {
	mode = strings.TrimSpace(mode)
	switch mode {
	case "":
		return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
	case SystemPromptModePassthrough, SystemPromptModeOverride, SystemPromptModeAppend:
	default:
		return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
	}
	return EffectiveSystemPrompt{Prompt: prompt, Mode: mode, Source: source}
}
