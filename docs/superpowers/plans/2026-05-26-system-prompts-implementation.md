# System Prompt Configuration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add hierarchical system prompt configuration for system settings, groups, and API keys with cached resolution and platform-specific request injection.

**Architecture:** Store APIKey and Group prompt config on their tables and include both in the existing API key auth cache snapshot. Store platform-level defaults in settings, read them through a 7-day `SettingService` in-process cache refreshed by `UpdateSettings`, then resolve `APIKey > Group > platform settings`. Apply prompts through a shared helper at the handler/service boundary before upstream forwarding so Anthropic, OpenAI Responses/Messages/WS, Gemini native, and Antigravity paths share the same mode semantics.

**Tech Stack:** Go 1.26.2, ent, PostgreSQL SQL migrations, Gin handlers, `atomic.Value` + `singleflight` caching, `gjson/sjson`, wire, Vue 3, TypeScript, pnpm, Vitest.

---

## File Structure

- Create `backend/migrations/116_add_system_prompt_configuration.sql`: add prompt columns to `api_keys` and `groups`, plus settings defaults.
- Modify `backend/ent/schema/api_key.go`: add `system_prompt` and `system_prompt_mode`.
- Modify `backend/ent/schema/group.go`: add `system_prompt` and `system_prompt_mode`.
- Regenerate ent files under `backend/ent/*` using `go generate ./ent`.
- Modify `backend/internal/service/domain_constants.go`: add mode constants and setting keys.
- Create `backend/internal/service/system_prompt.go`: mode validation, effective config resolution, and request injection helpers.
- Create `backend/internal/service/system_prompt_test.go`: resolver and injection unit tests.
- Modify `backend/internal/service/settings_view.go`: add platform prompt fields.
- Modify `backend/internal/service/setting_service.go`: persist settings fields, add 7-day cache, refresh cache on update.
- Create `backend/internal/service/setting_service_system_prompt_test.go`: platform settings cache tests.
- Modify `backend/internal/service/api_key.go`: add APIKey fields.
- Modify `backend/internal/service/group.go`: add Group fields.
- Modify `backend/internal/service/api_key_auth_cache.go`: add fields to snapshots.
- Modify `backend/internal/service/api_key_auth_cache_impl.go`: snapshot mapping and version bump.
- Modify `backend/internal/service/api_key_service.go`: create/update validation and persistence.
- Modify `backend/internal/service/admin_service.go`: group create/update validation and persistence.
- Modify `backend/internal/service/gemini_messages_compat_service.go`: keep native Gemini requests compatible with injected system prompts.
- Modify `backend/internal/repository/api_key_repo.go`: persist/select/map APIKey fields, auth query fields.
- Modify `backend/internal/repository/group_repo.go`: persist/select/map Group fields.
- Modify `backend/internal/repository/api_key_repo_messages_dispatch_unit_test.go`: add auth-path field preservation assertions.
- Modify `backend/internal/repository/api_key_repo_integration_test.go`: add repository round-trip assertions.
- Modify `backend/internal/repository/group_repo_integration_test.go`: add repository round-trip assertions.
- Modify `backend/internal/handler/dto/types.go`: expose prompt fields on DTOs.
- Modify `backend/internal/handler/dto/mappers.go`: map prompt fields.
- Modify `backend/internal/handler/dto/api_key_mapper_last_used_test.go`: add DTO field assertion.
- Modify `backend/internal/handler/api_key_handler.go`: accept user APIKey prompt fields.
- Modify `backend/internal/handler/admin/group_handler.go`: accept admin Group prompt fields.
- Modify `backend/internal/handler/admin/setting_handler.go`: accept/return platform prompt fields and audit diffs.
- Modify `backend/internal/handler/gateway_handler.go`: inject effective prompt for Anthropic/Gemini/Antigravity compatible bodies before forwarding.
- Modify `backend/internal/handler/openai_gateway_handler.go`: inject effective prompt into OpenAI Responses HTTP and WebSocket first payload, and add `SettingService` dependency.
- Modify `backend/internal/handler/gemini_v1beta_handler.go`: inject effective prompt into Gemini native REST bodies before `ForwardNative`/`ForwardGemini`.
- Modify `backend/internal/service/openai_gateway_messages.go`: ensure Anthropic-to-OpenAI conversion path preserves configured prompt precedence.
- Modify `backend/internal/service/openai_gateway_chat_completions.go`: ensure Chat Completions OpenAI-compatible conversion applies prompt helper.
- Modify `backend/internal/service/gateway_forward_as_chat_completions.go`: ensure Chat Completions to Anthropic path applies prompt helper.
- Modify `backend/internal/service/antigravity_gateway_service.go`: keep identity patch first and business prompt after it for Antigravity native `systemInstruction`.
- Modify `backend/cmd/server/wire_gen.go`: regenerate constructor wiring after `OpenAIGatewayHandler` gains `SettingService`.
- Modify `frontend/src/api/admin/settings.ts`: add platform prompt fields and modes.
- Modify `frontend/src/api/admin/groups.ts`: add group prompt fields and modes.
- Modify `frontend/src/api/keys.ts`: add APIKey prompt fields and modes.
- Modify `frontend/src/types/index.ts`: add shared prompt mode and DTO fields.
- Modify `frontend/src/views/admin/SettingsView.vue`: add platform prompt controls in gateway settings.
- Modify `frontend/src/views/admin/GroupsView.vue`: add group prompt controls in create/edit forms.
- Modify `frontend/src/views/user/KeysView.vue`: add APIKey prompt controls in create/edit forms.
- Create frontend tests only if existing component seams allow small focused coverage; otherwise rely on `pnpm -C frontend typecheck` and `pnpm -C frontend build`.

## Task 1: Domain Model and Prompt Helper Tests

**Files:**
- Create: `backend/internal/service/system_prompt.go`
- Create: `backend/internal/service/system_prompt_test.go`
- Modify: `backend/internal/service/domain_constants.go`

- [ ] **Step 1: Write failing resolver tests**

Add `backend/internal/service/system_prompt_test.go`:

```go
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
```

- [ ] **Step 2: Run resolver tests to verify RED**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt' -count=1
```

Expected: FAIL because `EffectiveSystemPrompt`, `SystemPromptModeAppend`, and `ResolveEffectiveSystemPrompt` do not exist.

- [ ] **Step 3: Implement mode constants and resolver**

Add constants to `backend/internal/service/domain_constants.go`:

```go
const (
	SystemPromptModeInherit     = "inherit"
	SystemPromptModePassthrough = "passthrough"
	SystemPromptModeOverride    = "override"
	SystemPromptModeAppend      = "append"

	SystemPromptSourceNone   = "none"
	SystemPromptSourceAPIKey = "api_key"
	SystemPromptSourceGroup  = "group"
	SystemPromptSourceSystem = "system"
)
```

Add `backend/internal/service/system_prompt.go`:

```go
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
		// ok
	default:
		return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
	}
	return EffectiveSystemPrompt{Prompt: prompt, Mode: mode, Source: source}
}
```

- [ ] **Step 4: Run resolver tests to verify GREEN**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/domain_constants.go backend/internal/service/system_prompt.go backend/internal/service/system_prompt_test.go
git commit -m "feat: add system prompt resolver"
```

## Task 2: Request Injection Helper

**Files:**
- Modify: `backend/internal/service/system_prompt.go`
- Modify: `backend/internal/service/system_prompt_test.go`

- [ ] **Step 1: Write failing injection tests**

Append to `backend/internal/service/system_prompt_test.go`:

```go
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
```

- [ ] **Step 2: Run injection tests to verify RED**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestApplySystemPrompt' -count=1
```

Expected: FAIL because `ApplySystemPromptToJSON` and `ApplySystemPromptToChatCompletionsJSON` do not exist.

- [ ] **Step 3: Implement injection helpers**

In `backend/internal/service/system_prompt.go`, add helpers that unmarshal to `map[string]any`, mutate the relevant fields, and marshal back:

```go
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
```

Also implement `applySystemPromptToAnthropic`, `applySystemPromptToOpenAIResponses`, and `applySystemPromptToGemini` in the same file using the behavior from the design spec. Import `encoding/json`.

- [ ] **Step 4: Run injection tests to verify GREEN**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestApplySystemPrompt' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/system_prompt.go backend/internal/service/system_prompt_test.go
git commit -m "feat: add system prompt injection helpers"
```

## Task 3: Database Schema and ent Fields

**Files:**
- Create: `backend/migrations/116_add_system_prompt_configuration.sql`
- Modify: `backend/ent/schema/api_key.go`
- Modify: `backend/ent/schema/group.go`
- Generated: `backend/ent/*`
- Test: `backend/internal/repository/migrations_schema_integration_test.go`

- [ ] **Step 1: Write failing migration schema assertions**

Add assertions to `backend/internal/repository/migrations_schema_integration_test.go`:

```go
requireColumn(t, tx, "api_keys", "system_prompt")
requireColumn(t, tx, "api_keys", "system_prompt_mode")
requireColumn(t, tx, "groups", "system_prompt")
requireColumn(t, tx, "groups", "system_prompt_mode")
```

Use the existing helper style in that file. If only table checks exist, add a helper:

```go
func requireColumn(t *testing.T, tx *sql.Tx, table, column string) {
	t.Helper()
	var exists bool
	err := tx.QueryRowContext(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "expected %s.%s to exist", table, column)
}
```

- [ ] **Step 2: Run migration schema test to verify RED**

Run:

```bash
cd backend && go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1
```

Expected: FAIL because the new columns do not exist.

- [ ] **Step 3: Add migration**

Create `backend/migrations/116_add_system_prompt_configuration.sql`:

```sql
-- 116_add_system_prompt_configuration.sql
-- Adds hierarchical system prompt configuration for API keys, groups, and platform defaults.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS system_prompt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS system_prompt_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS system_prompt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS system_prompt_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';

INSERT INTO settings (key, value)
VALUES
    ('system_prompt_anthropic', ''),
    ('system_prompt_mode_anthropic', 'inherit'),
    ('system_prompt_openai', ''),
    ('system_prompt_mode_openai', 'inherit'),
    ('system_prompt_gemini', ''),
    ('system_prompt_mode_gemini', 'inherit'),
    ('system_prompt_antigravity', ''),
    ('system_prompt_mode_antigravity', 'inherit')
ON CONFLICT (key) DO NOTHING;
```

- [ ] **Step 4: Add ent schema fields and generate**

In `backend/ent/schema/api_key.go`, add:

```go
field.String("system_prompt").
	SchemaType(map[string]string{dialect.Postgres: "text"}).
	Default("").
	Comment("API key level system prompt"),
field.String("system_prompt_mode").
	MaxLen(20).
	Default("inherit").
	Comment("API key level system prompt mode"),
```

In `backend/ent/schema/group.go`, add:

```go
field.String("system_prompt").
	SchemaType(map[string]string{dialect.Postgres: "text"}).
	Default("").
	Comment("Group level system prompt"),
field.String("system_prompt_mode").
	MaxLen(20).
	Default("inherit").
	Comment("Group level system prompt mode"),
```

Run:

```bash
cd backend && go generate ./ent
```

Expected: ent generated files update without errors.

- [ ] **Step 5: Run migration schema test to verify GREEN**

Run:

```bash
cd backend && go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/116_add_system_prompt_configuration.sql backend/ent backend/internal/repository/migrations_schema_integration_test.go
git commit -m "feat: add system prompt database fields"
```

## Task 4: Persistence, DTOs, and Auth Cache Snapshot

**Files:**
- Modify: `backend/internal/service/api_key.go`
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/service/api_key_auth_cache.go`
- Modify: `backend/internal/service/api_key_auth_cache_impl.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Modify: `backend/internal/repository/group_repo.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/repository/api_key_repo_messages_dispatch_unit_test.go`
- Modify: `backend/internal/repository/api_key_repo_integration_test.go`
- Modify: `backend/internal/repository/group_repo_integration_test.go`
- Modify: `backend/internal/handler/dto/api_key_mapper_last_used_test.go`

- [ ] **Step 1: Write failing repository and mapper assertions**

Add assertions:

```go
require.Equal(t, "group prompt", got.Group.SystemPrompt)
require.Equal(t, service.SystemPromptModeAppend, got.Group.SystemPromptMode)
require.Equal(t, "key prompt", got.SystemPrompt)
require.Equal(t, service.SystemPromptModeOverride, got.SystemPromptMode)
```

Place them in:

- `TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite`
- `APIKeyRepoSuite.TestGetByKeyForAuth_PreservesMessagesDispatchModelConfig`
- a new `GroupRepoSuite.TestSystemPromptRoundTrip`
- `TestAPIKeyFromService_MapsLastUsedAt`

- [ ] **Step 2: Run focused tests to verify RED**

Run:

```bash
cd backend && go test -tags unit ./internal/repository -run 'TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite' -count=1
cd backend && go test -tags integration ./internal/repository -run 'TestAPIKeyRepoSuite|TestGroupRepoSuite' -count=1
cd backend && go test -tags unit ./internal/handler/dto -run TestAPIKeyFromService_MapsLastUsedAt -count=1
```

Expected: FAIL because service, ent, repo, cache, and DTO fields are not mapped.

- [ ] **Step 3: Add service and DTO fields**

Add to `service.APIKey`, `service.Group`, `dto.APIKey`, and `dto.Group`:

```go
SystemPrompt     string `json:"system_prompt"`
SystemPromptMode string `json:"system_prompt_mode"`
```

For service structs omit JSON tags if the surrounding struct does not use them.

- [ ] **Step 4: Map repository create/update/select**

In `api_key_repo.go`:

- `Create`: call `SetSystemPrompt(key.SystemPrompt)` and `SetSystemPromptMode(NormalizeSystemPromptMode(key.SystemPromptMode))`.
- `GetByKeyForAuth.Select`: include `apikey.FieldSystemPrompt` and `apikey.FieldSystemPromptMode`.
- `Update`: set both fields.
- `apiKeyEntityToService`: copy both fields.

In `group_repo.go`:

- `Create`: call `SetSystemPrompt(groupIn.SystemPrompt)` and `SetSystemPromptMode(NormalizeSystemPromptMode(groupIn.SystemPromptMode))`.
- `Update`: set both fields.
- `groupEntityToService`: copy both fields.

- [ ] **Step 5: Map auth cache snapshot**

In `api_key_auth_cache.go`, add APIKey and Group fields to snapshots:

```go
SystemPrompt     string `json:"system_prompt,omitempty"`
SystemPromptMode string `json:"system_prompt_mode,omitempty"`
```

In `api_key_auth_cache_impl.go`:

- bump `apiKeyAuthSnapshotVersion`.
- copy fields in `snapshotFromAPIKey`.
- copy fields in `snapshotToAPIKey`.

- [ ] **Step 6: Map DTOs**

In `dto/mappers.go`, set:

```go
SystemPrompt:     k.SystemPrompt,
SystemPromptMode: k.SystemPromptMode,
```

and in `groupFromServiceBase`:

```go
SystemPrompt:     g.SystemPrompt,
SystemPromptMode: g.SystemPromptMode,
```

- [ ] **Step 7: Run focused tests to verify GREEN**

Run:

```bash
cd backend && go test -tags unit ./internal/repository -run 'TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite' -count=1
cd backend && go test -tags integration ./internal/repository -run 'TestAPIKeyRepoSuite|TestGroupRepoSuite' -count=1
cd backend && go test -tags unit ./internal/handler/dto -run TestAPIKeyFromService_MapsLastUsedAt -count=1
cd backend && go test -tags unit ./internal/service -run TestAPIKeyService_GetByKey_UsesL2Cache -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/service/api_key.go backend/internal/service/group.go backend/internal/service/api_key_auth_cache.go backend/internal/service/api_key_auth_cache_impl.go backend/internal/repository/api_key_repo.go backend/internal/repository/group_repo.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/repository/*api_key*test.go backend/internal/repository/group_repo_integration_test.go backend/internal/handler/dto/api_key_mapper_last_used_test.go
git commit -m "feat: persist system prompt settings on keys and groups"
```

## Task 5: Settings Fields and 7-Day Cache

**Files:**
- Modify: `backend/internal/service/settings_view.go`
- Modify: `backend/internal/service/setting_service.go`
- Modify: `backend/internal/service/domain_constants.go`
- Create: `backend/internal/service/setting_service_system_prompt_test.go`
- Modify: `backend/internal/handler/dto/settings.go`
- Modify: `backend/internal/handler/admin/setting_handler.go`

- [ ] **Step 1: Write failing cache tests**

Create `backend/internal/service/setting_service_system_prompt_test.go`:

```go
//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func resetSystemPromptSettingsCacheForTest(t *testing.T) {
	t.Helper()
	systemPromptSettingsCache.Store((*cachedSystemPromptSettings)(nil))
	t.Cleanup(func() {
		systemPromptSettingsCache.Store((*cachedSystemPromptSettings)(nil))
	})
}

type settingPromptRepoStub struct {
	values                       map[string]string
	updates                      map[string]string
	getSystemPromptMultipleCalls int
}

func isSystemPromptSettingKeyForTest(key string) bool {
	switch key {
	case SettingKeySystemPromptAnthropic, SettingKeySystemPromptModeAnthropic,
		SettingKeySystemPromptOpenAI, SettingKeySystemPromptModeOpenAI,
		SettingKeySystemPromptGemini, SettingKeySystemPromptModeGemini,
		SettingKeySystemPromptAntigravity, SettingKeySystemPromptModeAntigravity:
		return true
	default:
		return false
	}
}

func (s *settingPromptRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPromptRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPromptRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPromptRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	sawSystemPromptKey := false
	for _, key := range keys {
		if isSystemPromptSettingKeyForTest(key) {
			sawSystemPromptKey = true
		}
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	if sawSystemPromptKey {
		s.getSystemPromptMultipleCalls++
	}
	return out, nil
}

func (s *settingPromptRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range settings {
		s.updates[key] = value
		s.values[key] = value
	}
	return nil
}

func (s *settingPromptRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return s.values, nil
}

func (s *settingPromptRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetSystemPromptSettings_UsesCache(t *testing.T) {
	resetSystemPromptSettingsCacheForTest(t)

	repo := &settingPromptRepoStub{values: map[string]string{
		SettingKeySystemPromptOpenAI:     "cached prompt",
		SettingKeySystemPromptModeOpenAI: SystemPromptModeAppend,
	}}
	svc := NewSettingService(repo, nil)

	first := svc.GetSystemPromptSettings(context.Background())
	second := svc.GetSystemPromptSettings(context.Background())

	require.Equal(t, "cached prompt", first[PlatformOpenAI].Prompt)
	require.Equal(t, SystemPromptModeAppend, second[PlatformOpenAI].Mode)
	require.Equal(t, 1, repo.getSystemPromptMultipleCalls)
}

func TestSettingService_UpdateSettings_RefreshesSystemPromptCache(t *testing.T) {
	resetSystemPromptSettingsCacheForTest(t)

	repo := &settingPromptRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		SystemPromptOpenAI:     "new prompt",
		SystemPromptModeOpenAI: SystemPromptModeOverride,
	})
	require.NoError(t, err)

	got := svc.GetSystemPromptSettings(context.Background())
	require.Equal(t, "new prompt", got[PlatformOpenAI].Prompt)
	require.Equal(t, SystemPromptModeOverride, got[PlatformOpenAI].Mode)
}
```

Do not use a nonexistent `newSettingRepoStub`; either keep the local `settingPromptRepoStub` above, or adapt the existing `settingPublicRepoStub`/`settingUpdateRepoStub` patterns in the same package. Count only system-prompt `GetMultiple` calls so the existing Codex CLI cache warmup goroutine cannot make the cache assertion flaky.

- [ ] **Step 2: Run cache tests to verify RED**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestSettingService_.*SystemPrompt' -count=1
```

Expected: FAIL because setting keys, fields, and cache method do not exist.

- [ ] **Step 3: Add setting keys and settings fields**

In `domain_constants.go`, add:

```go
SettingKeySystemPromptAnthropic       = "system_prompt_anthropic"
SettingKeySystemPromptModeAnthropic   = "system_prompt_mode_anthropic"
SettingKeySystemPromptOpenAI          = "system_prompt_openai"
SettingKeySystemPromptModeOpenAI      = "system_prompt_mode_openai"
SettingKeySystemPromptGemini          = "system_prompt_gemini"
SettingKeySystemPromptModeGemini      = "system_prompt_mode_gemini"
SettingKeySystemPromptAntigravity     = "system_prompt_antigravity"
SettingKeySystemPromptModeAntigravity = "system_prompt_mode_antigravity"
```

In `settings_view.go` and `dto/settings.go`, add the 8 fields.

- [ ] **Step 4: Implement settings persistence and cache**

In `setting_service.go`:

- add `cachedSystemPromptSettings`, `systemPromptSettingsCache`, `systemPromptSettingsSF`, `systemPromptSettingsCacheTTL = 7 * 24 * time.Hour`, `systemPromptSettingsErrorTTL = 5 * time.Second`, and DB timeout.
- add `GetSystemPromptSettings(ctx context.Context) map[string]EffectiveSystemPrompt`.
- read the 8 keys through `GetMultiple`.
- normalize mode with `NormalizeSystemPromptMode`.
- store a copy in `atomic.Value`.
- update `UpdateSettings` to persist the 8 keys.
- update `UpdateSettings` to refresh the cache with latest saved settings after `SetMultiple` succeeds.
- update `parseSettings` defaults to `inherit`.

- [ ] **Step 5: Add handler request/response mapping**

In `admin/setting_handler.go`:

- add request fields for all 8 keys.
- map request to `service.SystemSettings`.
- include all 8 fields in the success DTO.
- add diff entries for audit when any field changes.

- [ ] **Step 6: Run settings tests to verify GREEN**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestSettingService_.*SystemPrompt|TestSettingService_UpdateSettings' -count=1
cd backend && go test -tags unit ./internal/handler/admin -run Test.*Settings -count=1
```

Expected: PASS for service tests. If no handler settings tests exist, the second command should report no matching tests without package compile errors.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/settings_view.go backend/internal/service/setting_service.go backend/internal/service/domain_constants.go backend/internal/service/setting_service_system_prompt_test.go backend/internal/handler/dto/settings.go backend/internal/handler/admin/setting_handler.go
git commit -m "feat: cache platform system prompt settings"
```

## Task 6: API Request Validation for APIKey and Group

**Files:**
- Modify: `backend/internal/service/api_key_service.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/handler/api_key_handler.go`
- Modify: `backend/internal/handler/admin/group_handler.go`
- Modify: `backend/internal/service/api_key_service_cache_test.go`
- Modify: `backend/internal/service/admin_service_group_test.go`

- [ ] **Step 1: Write failing service validation tests**

Append validation tests to `system_prompt_test.go` and add the `infraerrors` import if it is not already present:

```go
func TestValidateSystemPromptConfig_RejectsInvalidMode(t *testing.T) {
	_, _, err := ValidateSystemPromptConfig("prompt", "not-a-real-mode")
	require.Error(t, err)
	require.Equal(t, "INVALID_SYSTEM_PROMPT_MODE", infraerrors.Reason(err))
}

func TestValidateSystemPromptConfig_RejectsModeWithoutPrompt(t *testing.T) {
	_, _, err := ValidateSystemPromptConfig("", SystemPromptModeAppend)
	require.Error(t, err)
	require.Equal(t, "SYSTEM_PROMPT_REQUIRED", infraerrors.Reason(err))
}

func TestValidateSystemPromptConfig_DefaultsBlankModeToInherit(t *testing.T) {
	prompt, mode, err := ValidateSystemPromptConfig("", "")
	require.NoError(t, err)
	require.Empty(t, prompt)
	require.Equal(t, SystemPromptModeInherit, mode)
}
```

Add to `api_key_service_cache_test.go` or a new focused API key service test:

```go
func TestAPIKeyService_Create_RejectsPromptModeWithoutPrompt(t *testing.T) {
	repo := &authRepoStub{}
	userRepo := &userRepoStub{user: &User{ID: 1, Status: StatusActive}}
	svc := NewAPIKeyService(repo, userRepo, nil, nil, nil, &authCacheStub{}, &config.Config{})

	_, err := svc.Create(context.Background(), 1, CreateAPIKeyRequest{
		Name:             "key",
		SystemPromptMode: SystemPromptModeAppend,
	})
	require.Error(t, err)
}
```

Add to `admin_service_group_test.go`:

```go
func TestAdminService_CreateGroup_PersistsSystemPromptConfig(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "prompt group",
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeStandard,
		SystemPrompt:     "group prompt",
		SystemPromptMode: SystemPromptModePassthrough,
	})
	require.NoError(t, err)
	require.Equal(t, "group prompt", group.SystemPrompt)
	require.NotNil(t, repo.created)
	require.Equal(t, SystemPromptModePassthrough, repo.created.SystemPromptMode)
}
```

Use real stubs that already exist in the package: `authRepoStub`, `authCacheStub`, `userRepoStub`, and `groupRepoStubForAdmin`. Add imports only where needed, for example `config` in `api_key_service_cache_test.go`.

- [ ] **Step 2: Run service validation tests to verify RED**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestAPIKeyService_Create_RejectsPromptModeWithoutPrompt|TestAdminService_CreateGroup_PersistsSystemPromptConfig|TestValidateSystemPromptConfig' -count=1
```

Expected: FAIL because request structs and validation are missing.

- [ ] **Step 3: Add request fields and validation**

Add to service request structs:

```go
SystemPrompt     string `json:"system_prompt"`
SystemPromptMode string `json:"system_prompt_mode"`
```

Add validation helper in `system_prompt.go`:

```go
func ValidateSystemPromptConfig(prompt, mode string) (string, string, error) {
	prompt = strings.TrimSpace(prompt)
	mode = strings.TrimSpace(mode)
	if mode == SystemPromptModeInherit {
		return prompt, mode, nil
	}
	if mode == "" {
		return prompt, SystemPromptModeInherit, nil
	}
	switch mode {
	case SystemPromptModePassthrough, SystemPromptModeOverride, SystemPromptModeAppend:
		// ok
	default:
		return "", "", infraerrors.BadRequest("INVALID_SYSTEM_PROMPT_MODE", "invalid system prompt mode")
	}
	if prompt == "" {
		return "", "", infraerrors.BadRequest("SYSTEM_PROMPT_REQUIRED", "system prompt is required for selected mode")
	}
	return prompt, mode, nil
}
```

Import `infraerrors` in `system_prompt.go`.

Use it in APIKey create/update and Group create/update before persistence. Add `TestValidateSystemPromptConfig_RejectsInvalidMode` to prove invalid modes are not normalized into `inherit`.

- [ ] **Step 4: Add handler DTO fields**

In `api_key_handler.go`, add fields to create/update request structs and map them to service request structs.

In `admin/group_handler.go`, add fields to create/update request structs and map them to service input structs.

- [ ] **Step 5: Run validation tests to verify GREEN**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestAPIKeyService_Create_RejectsPromptModeWithoutPrompt|TestAdminService_CreateGroup_PersistsSystemPromptConfig|TestValidateSystemPromptConfig' -count=1
cd backend && go test -tags unit ./internal/handler -run Test.*APIKey -count=1
cd backend && go test -tags unit ./internal/handler/admin -run TestGroupHandlerEndpoints -count=1
```

Expected: PASS or no matching handler tests with successful package compile.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/api_key_service.go backend/internal/service/admin_service.go backend/internal/service/system_prompt.go backend/internal/handler/api_key_handler.go backend/internal/handler/admin/group_handler.go backend/internal/service/*test.go
git commit -m "feat: validate system prompt config inputs"
```

## Task 7: Gateway Integration

**Files:**
- Modify: `backend/internal/handler/gateway_handler.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/gemini_v1beta_handler.go`
- Modify: `backend/internal/service/openai_gateway_messages.go`
- Modify: `backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `backend/internal/service/gateway_forward_as_chat_completions.go`
- Modify: `backend/internal/service/gemini_messages_compat_service.go`
- Modify: `backend/internal/service/antigravity_gateway_service.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`
- Modify: `backend/internal/handler/gemini_v1beta_handler_test.go`
- Modify: `backend/internal/service/gateway_prompt_test.go`
- Modify: `backend/cmd/server/wire_gen.go`

- [ ] **Step 1: Write failing integration-level gateway tests**

Add a focused handler test for OpenAI Responses body mutation in `openai_gateway_handler_test.go`:

```go
func TestOpenAIResponses_AppliesAPIKeySystemPromptBeforeForward(t *testing.T) {
	body := []byte(`{"model":"gpt-5.2","instructions":"client","input":"hi"}`)
	apiKey := &service.APIKey{
		ID:               1,
		UserID:           1,
		SystemPrompt:     "server",
		SystemPromptMode: service.SystemPromptModeAppend,
		Group:            &service.Group{ID: 1, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}

	got, changed, err := service.ApplySystemPromptToJSON(body, service.PlatformOpenAI, service.ResolveEffectiveSystemPrompt(context.Background(), apiKey, service.PlatformOpenAI, nil))

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "server\n\nclient", gjson.GetBytes(got, "instructions").String())
}
```

This starts as a service helper guard and becomes a regression test for the handler once injection is called before `Forward`.

Add a focused handler test for Gemini native REST bodies in `gemini_v1beta_handler_test.go` that proves the body handed to `ForwardNative` / `ForwardGemini` already contains the resolved system prompt.

- [ ] **Step 2: Run gateway tests to verify RED if handler-specific assertion is added**

Run:

```bash
cd backend && go test -tags unit ./internal/handler -run TestOpenAIResponses_AppliesAPIKeySystemPromptBeforeForward -count=1
cd backend && go test -tags unit ./internal/handler -run 'TestGeminiV1Beta.*SystemPrompt' -count=1
```

Expected: FAIL if the test uses the handler and captures forwarded body; PASS if the test only verifies helper behavior. If it passes as helper-only, add the handler capture before moving to implementation.

- [ ] **Step 3: Add handler-level injection before forwarding**

In `gateway_handler.go`:

- after fallback group resolution and before `c.Set("parsed_request", parsedReq)`, resolve effective prompt using `currentAPIKey`, current group platform, and `h.settingService`.
- for Antigravity OAuth `body`, call `ApplySystemPromptToJSON(body, service.PlatformAntigravity, prompt)`.
- for Anthropic parsed request, update both `parsedReq.Body` and `body`, then re-parse if the body changed so `parsedReq.System` remains consistent.

In `openai_gateway_handler.go`:

- in `Responses`, resolve prompt from APIKey and `resolveOpenAICompatibleGroupPlatform(apiKey)`, mutate `body` before channel model mapping and session hash if prompt should influence session hash.
- in `Messages`, mutate Anthropic `body` before `ForwardAsAnthropic`.
- in `ResponsesWebSocket`, mutate `firstMessage` before session hash and channel model mapping.

In `gemini_v1beta_handler.go`:

- resolve the effective prompt with `h.settingService` before `setOpsRequestContext` / `ParseGatewayRequest`.
- mutate the raw `body` before any session hash or forward call so `ForwardNative` and `ForwardGemini` see the injected prompt.

In `gemini_messages_compat_service.go`:

- keep `ForwardNative` as a transport-only path that preserves the already injected body.

- [ ] **Step 4: Add service-level conversion safeguards**

In OpenAI and Chat Completions conversion services, apply helpers before marshalling upstream bodies when the raw handler path cannot cover that format:

- `openai_gateway_messages.go`: ensure configured prompt is included before `AnthropicToResponses`.
- `openai_gateway_chat_completions.go`: apply `ApplySystemPromptToChatCompletionsJSON` before `ChatCompletionsToResponses`.
- `gateway_forward_as_chat_completions.go`: apply `ApplySystemPromptToChatCompletionsJSON` before Chat Completions to Anthropic conversion.
- `antigravity_gateway_service.go`: split the Gemini prompt injection so the identity patch stays first and the business prompt is inserted immediately after it, before schema cleanup and wrapping.

After these edits, run `cd backend && go generate ./cmd/server` so `backend/cmd/server/wire_gen.go` picks up the `SettingService` constructor change on `NewOpenAIGatewayHandler`.

- [ ] **Step 5: Run gateway prompt tests**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestApplySystemPrompt|TestGatewayPrompt|TestOpenAI.*Instructions|TestGemini.*SystemInstruction' -count=1
cd backend && go test -tags unit ./internal/handler -run 'TestOpenAIResponses|TestOpenAIHandler_InstructionsInjection|TestGeminiV1Beta.*SystemPrompt' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/gateway_handler.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/gemini_v1beta_handler.go backend/internal/service/openai_gateway_messages.go backend/internal/service/openai_gateway_chat_completions.go backend/internal/service/gateway_forward_as_chat_completions.go backend/internal/service/gemini_messages_compat_service.go backend/internal/service/antigravity_gateway_service.go backend/internal/handler/openai_gateway_handler_test.go backend/internal/handler/gemini_v1beta_handler_test.go backend/internal/service/gateway_prompt_test.go backend/cmd/server/wire_gen.go
git commit -m "feat: inject effective system prompts in gateways"
```

## Task 8: Frontend API Types and Forms

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/admin/settings.ts`
- Modify: `frontend/src/api/admin/groups.ts`
- Modify: `frontend/src/api/keys.ts`
- Modify: `frontend/src/views/admin/SettingsView.vue`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/views/user/KeysView.vue`

- [ ] **Step 1: Add TypeScript types first**

Add to `frontend/src/types/index.ts`:

```ts
export type SystemPromptMode = 'inherit' | 'passthrough' | 'override' | 'append'
```

Add `system_prompt?: string` and `system_prompt_mode?: SystemPromptMode` to APIKey and Group types.

- [ ] **Step 2: Run typecheck to verify RED**

Run:

```bash
pnpm -C frontend typecheck
```

Expected: FAIL after API usage is updated in the next step if component fields are missing. If it passes here, continue.

- [ ] **Step 3: Add API request fields**

In `frontend/src/api/admin/settings.ts`, add the 8 settings fields to `SystemSettings` and `UpdateSettingsRequest`.

In `frontend/src/api/admin/groups.ts`, add:

```ts
system_prompt?: string
system_prompt_mode?: SystemPromptMode
```

to create/update payload types.

In `frontend/src/api/keys.ts`, include `system_prompt` and `system_prompt_mode` in create/update payloads.

- [ ] **Step 4: Add UI controls in SettingsView**

In `SettingsView.vue`:

- initialize all 8 form fields with `inherit` and empty prompt values.
- in the gateway settings section, add one compact textarea row per platform.
- use a select with options: `不配置`, `透传`, `覆盖`, `追加`.
- disable or visually de-emphasize textarea when mode is `inherit`.
- include all 8 fields in the save payload.

- [ ] **Step 5: Add UI controls in GroupsView**

In `GroupsView.vue`:

- add `system_prompt: ''` and `system_prompt_mode: 'inherit'` to create and edit form state.
- in create/edit modals, add a select and textarea near platform/routing settings.
- include both fields in `handleCreateGroup` and `handleUpdateGroup` payloads.
- populate both fields in `openEditGroup`.

- [ ] **Step 6: Add UI controls in KeysView**

In `KeysView.vue`:

- add `system_prompt: ''` and `system_prompt_mode: 'inherit'` to form state.
- show select and textarea in create/edit modal.
- include both fields in create/update API calls.
- reset both fields when modal closes.

- [ ] **Step 7: Run frontend verification**

Run:

```bash
pnpm -C frontend typecheck
pnpm -C frontend build
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/admin/settings.ts frontend/src/api/admin/groups.ts frontend/src/api/keys.ts frontend/src/views/admin/SettingsView.vue frontend/src/views/admin/GroupsView.vue frontend/src/views/user/KeysView.vue
git commit -m "feat: add system prompt controls"
```

## Task 9: Final Verification and Audit

**Files:**
- Modify only if verification reveals a defect.

- [ ] **Step 1: Run focused backend suites**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'SystemPrompt|SettingService_.*SystemPrompt|APIKeyService|AdminService_CreateGroup|AdminService_UpdateGroup' -count=1
cd backend && go test -tags integration ./internal/repository -run 'APIKey|Group|MigrationsSchema' -count=1
cd backend && go test -tags unit ./internal/handler ./internal/handler/admin ./internal/handler/dto -run 'APIKey|Group|Settings|SystemPrompt|OpenAIResponses|GeminiV1Beta' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run broader backend compile test**

Run:

```bash
cd backend && go test -tags unit ./internal/service ./internal/handler ./internal/handler/admin ./internal/handler/dto -count=1
cd backend && go test -tags integration ./internal/repository -count=1
```

Expected: PASS.

- [ ] **Step 3: Run frontend verification**

Run:

```bash
pnpm -C frontend typecheck
pnpm -C frontend build
```

Expected: PASS.

- [ ] **Step 4: Inspect git diff**

Run:

```bash
git status --short
git diff --stat
git diff --check
```

Expected: only planned files changed, no whitespace errors.

- [ ] **Step 5: Request code review**

Use the `requesting-code-review` skill. Ask the reviewer to focus on:

- mode semantics and inheritance behavior
- cache correctness and update refresh
- auth cache snapshot version and invalidation
- request injection order for OpenAI Codex/Antigravity identity patch
- migration safety and default compatibility
- frontend payload completeness

- [ ] **Step 6: Apply review fixes, rerun verification, and commit**

Run the same verification commands after fixes, then:

```bash
git add backend frontend docs
git commit -m "feat: support hierarchical system prompts"
```

## Self-Review

- Spec coverage: The plan covers database fields, settings keys, 7-day settings cache, auth cache snapshot, API/DTO fields, resolver priority, injection modes, Gemini/OpenAI/Anthropic/Antigravity entry points, frontend controls, and verification.
- Execution safety: The test commands now separate `unit` and `integration` tags so no suite is skipped by accident.
- Placeholder scan: The plan no longer uses fake helpers like `newSettingRepoStub`, `newAPIKeyServiceForCacheTest`, or `newAdminServiceWithGroupRepoForTest`; their only mention is an explicit warning.
- Type consistency: The same names are used throughout: `system_prompt`, `system_prompt_mode`, `SystemPromptModeInherit`, `SystemPromptModePassthrough`, `SystemPromptModeOverride`, `SystemPromptModeAppend`, `EffectiveSystemPrompt`.
