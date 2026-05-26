package service

import (
	"context"
	"encoding/json"
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

func ApplySystemPromptToJSON(body []byte, platform string, prompt EffectiveSystemPrompt) ([]byte, bool, error) {
	if !prompt.Enabled() {
		return body, false, nil
	}
	switch platform {
	case PlatformOpenAI:
		return applySystemPromptToOpenAIResponses(body, prompt)
	case PlatformGemini, PlatformAntigravity:
		return applySystemPromptToGemini(body, prompt)
	default:
		return applySystemPromptToAnthropic(body, prompt)
	}
}

func ApplySystemPromptToChatCompletionsJSON(body []byte, prompt EffectiveSystemPrompt) ([]byte, bool, error) {
	if !prompt.Enabled() {
		return body, false, nil
	}
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false, err
	}
	messages, _ := req["messages"].([]any)
	hasSystem := false
	filtered := make([]any, 0, len(messages)+1)
	for _, msg := range messages {
		m, _ := msg.(map[string]any)
		if m != nil && m["role"] == "system" {
			hasSystem = true
			if prompt.Mode == SystemPromptModePassthrough {
				return body, false, nil
			}
			if prompt.Mode == SystemPromptModeAppend {
				filtered = append(filtered, msg)
			}
			continue
		}
		filtered = append(filtered, msg)
	}
	if prompt.Mode == SystemPromptModePassthrough && hasSystem {
		return body, false, nil
	}
	req["messages"] = append([]any{map[string]any{"role": "system", "content": prompt.Prompt}}, filtered...)
	out, err := json.Marshal(req)
	return out, err == nil, err
}

func applySystemPromptToAnthropic(body []byte, prompt EffectiveSystemPrompt) ([]byte, bool, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false, err
	}
	existing, hasExisting := req["system"]
	if hasExisting && prompt.Mode == SystemPromptModePassthrough && hasSystemPromptValue(existing) {
		return body, false, nil
	}

	parts := []any{anthropicTextPart(prompt.Prompt)}
	if prompt.Mode == SystemPromptModeAppend && hasSystemPromptValue(existing) {
		parts = append(parts, anthropicSystemParts(existing)...)
	}
	req["system"] = parts
	out, err := json.Marshal(req)
	return out, err == nil, err
}

func applySystemPromptToOpenAIResponses(body []byte, prompt EffectiveSystemPrompt) ([]byte, bool, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false, err
	}
	existing, _ := req["instructions"].(string)
	if strings.TrimSpace(existing) != "" && prompt.Mode == SystemPromptModePassthrough {
		return body, false, nil
	}
	if strings.TrimSpace(existing) != "" && prompt.Mode == SystemPromptModeAppend {
		req["instructions"] = prompt.Prompt + "\n\n" + existing
	} else {
		req["instructions"] = prompt.Prompt
	}
	out, err := json.Marshal(req)
	return out, err == nil, err
}

func applySystemPromptToGemini(body []byte, prompt EffectiveSystemPrompt) ([]byte, bool, error) {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false, err
	}
	existing, hasExisting := req["systemInstruction"]
	if hasExisting && prompt.Mode == SystemPromptModePassthrough && hasSystemPromptValue(existing) {
		return body, false, nil
	}

	parts := []any{geminiTextPart(prompt.Prompt)}
	if prompt.Mode == SystemPromptModeAppend && hasSystemPromptValue(existing) {
		parts = append(parts, geminiSystemParts(existing)...)
	}
	req["systemInstruction"] = map[string]any{"parts": parts}
	out, err := json.Marshal(req)
	return out, err == nil, err
}

func anthropicTextPart(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}

func geminiTextPart(text string) map[string]any {
	return map[string]any{"text": text}
}

func anthropicSystemParts(system any) []any {
	switch v := system.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []any{anthropicTextPart(v)}
	case []any:
		return v
	case map[string]any:
		return []any{v}
	default:
		return nil
	}
}

func geminiSystemParts(system any) []any {
	m, _ := system.(map[string]any)
	if m == nil {
		return nil
	}
	parts, _ := m["parts"].([]any)
	return parts
}

func hasSystemPromptValue(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		if parts, ok := v["parts"].([]any); ok {
			return len(parts) > 0
		}
		return len(v) > 0
	default:
		return false
	}
}
