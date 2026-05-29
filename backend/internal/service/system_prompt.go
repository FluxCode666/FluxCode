package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

type EffectiveSystemPrompt struct {
	Prompt string
	Mode   string
	Source string
}

type SystemPromptUserScope struct {
	Enabled bool
	Mode    string
	UserIDs []int64
}

type SystemPromptRuntimeSettings struct {
	Prompts   map[string]EffectiveSystemPrompt
	UserScope SystemPromptUserScope
}

var ErrInvalidSystemPromptMode = infraerrors.BadRequest("INVALID_SYSTEM_PROMPT_MODE", "invalid system prompt mode")

func (p EffectiveSystemPrompt) Enabled() bool {
	return strings.TrimSpace(p.Prompt) != "" && IsSystemPromptInjectionMode(p.Mode)
}

type SystemPromptSettingsProvider interface {
	GetSystemPromptSettings(ctx context.Context) SystemPromptRuntimeSettings
}

type systemPromptAPIKeyContextKey struct{}

const ginAPIKeyContextKey = "api_key"

func WithAPIKeyContext(ctx context.Context, apiKey *APIKey) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, systemPromptAPIKeyContextKey{}, apiKey)
}

func APIKeyFromContext(ctx context.Context) (*APIKey, bool) {
	if ctx == nil {
		return nil, false
	}
	apiKey, ok := ctx.Value(systemPromptAPIKeyContextKey{}).(*APIKey)
	return apiKey, ok && apiKey != nil
}

func NormalizeSystemPromptMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case SystemPromptModePassthrough, SystemPromptModeOverride, SystemPromptModeAppend:
		return strings.TrimSpace(mode)
	default:
		return SystemPromptModeInherit
	}
}

func ValidateSystemPromptMode(mode string) error {
	switch strings.TrimSpace(mode) {
	case "", SystemPromptModeInherit, SystemPromptModePassthrough, SystemPromptModeOverride, SystemPromptModeAppend:
		return nil
	default:
		return ErrInvalidSystemPromptMode.WithMetadata(map[string]string{"mode": strings.TrimSpace(mode)})
	}
}

func NormalizeSystemPromptConfig(prompt, mode string) (string, string, error) {
	if err := ValidateSystemPromptMode(mode); err != nil {
		return "", "", err
	}
	normalizedMode := strings.TrimSpace(mode)
	if normalizedMode == "" {
		normalizedMode = SystemPromptModeInherit
	}
	if normalizedMode == SystemPromptModeInherit {
		return "", normalizedMode, nil
	}
	return strings.TrimSpace(prompt), normalizedMode, nil
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
	runtimeSettings := defaultSystemPromptSettings()
	if !isNilSystemPromptSettingsProvider(settings) {
		runtimeSettings = settings.GetSystemPromptSettings(ctx)
	}
	if !isSystemPromptAllowedForUser(apiKey, runtimeSettings.UserScope) {
		return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
	}

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
	if p, ok := runtimeSettings.Prompts[strings.TrimSpace(platform)]; ok && p.Enabled() {
		return p
	}
	return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
}

func isSystemPromptAllowedForUser(apiKey *APIKey, scope SystemPromptUserScope) bool {
	if !scope.Enabled {
		return false
	}
	userID := int64(0)
	if apiKey != nil {
		userID = apiKey.UserID
	}
	switch normalizeSystemPromptUserScopeMode(scope.Mode) {
	case SystemPromptUserScopeWhitelist:
		return userID > 0 && systemPromptScopeContainsUserID(scope.UserIDs, userID)
	case SystemPromptUserScopeBlacklist:
		return userID <= 0 || !systemPromptScopeContainsUserID(scope.UserIDs, userID)
	default:
		return true
	}
}

func IsSystemPromptAllowedForUserID(userID int64, scope SystemPromptUserScope) bool {
	return isSystemPromptAllowedForUser(&APIKey{UserID: userID}, scope)
}

func systemPromptScopeContainsUserID(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isNilSystemPromptSettingsProvider(settings SystemPromptSettingsProvider) bool {
	if settings == nil {
		return true
	}
	value := reflect.ValueOf(settings)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func applyResolvedSystemPromptToJSON(
	ctx context.Context,
	c *gin.Context,
	body []byte,
	requestPlatform string,
	settingsPlatform string,
	settings SystemPromptSettingsProvider,
) ([]byte, bool, error) {
	runtimeCtx := systemPromptRuntimeContext(ctx, c)
	apiKey := resolveRuntimeAPIKey(ctx, c)
	effective := ResolveEffectiveSystemPrompt(runtimeCtx, apiKey, settingsPlatform, settings)
	return ApplySystemPromptToJSON(body, requestPlatform, effective)
}

func applyResolvedSystemPromptToChatCompletionsJSON(
	ctx context.Context,
	c *gin.Context,
	body []byte,
	platform string,
	settings SystemPromptSettingsProvider,
) ([]byte, bool, error) {
	runtimeCtx := systemPromptRuntimeContext(ctx, c)
	apiKey := resolveRuntimeAPIKey(ctx, c)
	effective := ResolveEffectiveSystemPrompt(runtimeCtx, apiKey, platform, settings)
	return ApplySystemPromptToChatCompletionsJSON(body, effective)
}

func systemPromptRuntimeContext(ctx context.Context, c *gin.Context) context.Context {
	if c != nil && c.Request != nil && c.Request.Context() != nil {
		return c.Request.Context()
	}
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func resolveRuntimeAPIKey(ctx context.Context, c *gin.Context) *APIKey {
	var apiKey *APIKey
	runtimeCtx := systemPromptRuntimeContext(ctx, c)
	if c != nil {
		if value, exists := c.Get(ginAPIKeyContextKey); exists {
			if key, ok := value.(*APIKey); ok {
				apiKey = key
			}
		}
	}
	if apiKey == nil {
		if key, ok := APIKeyFromContext(ctx); ok {
			apiKey = key
		}
	}
	if apiKey == nil {
		if key, ok := APIKeyFromContext(runtimeCtx); ok {
			apiKey = key
		}
	}

	group := runtimeGroupFromContext(runtimeCtx)
	if apiKey == nil {
		if group == nil {
			return nil
		}
		return &APIKey{SystemPromptMode: SystemPromptModeInherit, Group: group}
	}
	if apiKey.Group != nil || group == nil {
		return apiKey
	}
	keyCopy := *apiKey
	keyCopy.Group = group
	return &keyCopy
}

func runtimeGroupFromContext(ctx context.Context) *Group {
	if ctx == nil {
		return nil
	}
	group, _ := ctx.Value(ctxkey.Group).(*Group)
	if !IsGroupContextValid(group) {
		return nil
	}
	return group
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
