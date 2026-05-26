# 系统提示词配置实施计划

> **给执行 Agent：** 必须使用子技能：按任务逐项执行时，使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 增加分层系统提示词配置，支持系统设置、分组和 APIKey，并提供缓存解析与按平台注入请求能力。

**架构：** APIKey 和 Group 的提示词配置分别存储在各自表中，并写入现有 APIKey 鉴权缓存快照。平台级默认配置存储在 settings 中，通过 `SettingService` 的进程内缓存读取，缓存默认 7 天，并在 `UpdateSettings` 后刷新；最终按 `APIKey > Group > 平台设置` 解析生效配置。在 handler/service 边界通过共享 helper 注入提示词，再转发到上游，使 Anthropic、OpenAI Responses/Messages/WS、Gemini native 和 Antigravity 链路共享同一套模式语义。

**技术栈：** Go 1.26.2、ent、PostgreSQL SQL migrations、Gin handlers、`atomic.Value` + `singleflight` caching、`gjson/sjson`、wire、Vue 3、TypeScript、pnpm、Vitest。

---

## 文件结构

- 新建 `backend/migrations/116_add_system_prompt_configuration.sql`：为 `api_keys` 和 `groups` 增加提示词字段，并写入 settings 默认值。
- 修改 `backend/ent/schema/api_key.go`：增加 `system_prompt` 和 `system_prompt_mode`。
- 修改 `backend/ent/schema/group.go`：增加 `system_prompt` 和 `system_prompt_mode`。
- 使用 `go generate ./ent` 重新生成 `backend/ent/*` 下的 ent 文件。
- 修改 `backend/internal/service/domain_constants.go`：增加模式常量和 settings key。
- 新建 `backend/internal/service/system_prompt.go`：提供模式校验、生效配置解析和请求注入 helper。
- 新建 `backend/internal/service/system_prompt_test.go`：覆盖解析器和注入逻辑的 unit 测试。
- 修改 `backend/internal/service/settings_view.go`：增加平台提示词字段。
- 修改 `backend/internal/service/setting_service.go`：持久化 settings 字段，增加 7 天缓存，并在更新时刷新缓存。
- 新建 `backend/internal/service/setting_service_system_prompt_test.go`：覆盖平台 settings 缓存。
- 修改 `backend/internal/service/api_key.go`：增加 APIKey 字段。
- 修改 `backend/internal/service/group.go`：增加 Group 字段。
- 修改 `backend/internal/service/api_key_auth_cache.go`：在快照中增加字段。
- 修改 `backend/internal/service/api_key_auth_cache_impl.go`：补齐快照映射并提升版本。
- 修改 `backend/internal/service/api_key_service.go`：处理 create/update 校验与持久化。
- 修改 `backend/internal/service/admin_service.go`：处理 group create/update 校验与持久化。
- 修改 `backend/internal/service/gemini_messages_compat_service.go`：保持 native Gemini 请求与已注入系统提示词兼容。
- 修改 `backend/internal/repository/api_key_repo.go`：持久化、查询、映射 APIKey 字段和鉴权查询字段。
- 修改 `backend/internal/repository/group_repo.go`：持久化、查询、映射 Group 字段。
- 修改 `backend/internal/repository/api_key_repo_messages_dispatch_unit_test.go`：增加鉴权路径字段保留断言。
- 修改 `backend/internal/repository/api_key_repo_integration_test.go`：增加 repository 往返断言。
- 修改 `backend/internal/repository/group_repo_integration_test.go`：增加 repository 往返断言。
- 修改 `backend/internal/handler/dto/types.go`：在 DTO 上暴露提示词字段。
- 修改 `backend/internal/handler/dto/mappers.go`：映射提示词字段。
- 修改 `backend/internal/handler/dto/api_key_mapper_last_used_test.go`：增加 DTO 字段断言。
- 修改 `backend/internal/handler/api_key_handler.go`：接收用户 APIKey 提示词字段。
- 修改 `backend/internal/handler/admin/group_handler.go`：接收管理员 Group 提示词字段。
- 修改 `backend/internal/handler/admin/setting_handler.go`：接收/返回平台提示词字段，并记录审计 diff。
- 修改 `backend/internal/handler/gateway_handler.go`：在转发前为 Anthropic/Gemini/Antigravity 兼容请求体注入生效提示词。
- 修改 `backend/internal/handler/openai_gateway_handler.go`：向 OpenAI Responses HTTP 和 WebSocket 首个 payload 注入生效提示词，并增加 `SettingService` 依赖。
- 修改 `backend/internal/handler/gemini_v1beta_handler.go`：在 `ForwardNative`/`ForwardGemini` 前向 Gemini native REST 请求体注入生效提示词。
- 修改 `backend/internal/service/openai_gateway_messages.go`：确保 Anthropic-to-OpenAI 转换链路保留配置的提示词优先级。
- 修改 `backend/internal/service/openai_gateway_chat_completions.go`：确保 Chat Completions OpenAI-compatible 转换应用提示词 helper。
- 修改 `backend/internal/service/gateway_forward_as_chat_completions.go`：确保 Chat Completions to Anthropic 链路应用提示词 helper。
- 修改 `backend/internal/service/antigravity_gateway_service.go`：确保 Antigravity native `systemInstruction` 中 identity patch 在前，业务提示词在后。
- 修改 `backend/cmd/server/wire_gen.go`：`OpenAIGatewayHandler` 增加 `SettingService` 后重新生成构造 wiring。
- 修改 `frontend/src/api/admin/settings.ts`：增加平台提示词字段和模式。
- 修改 `frontend/src/api/admin/groups.ts`：增加 group 提示词字段和模式。
- 修改 `frontend/src/api/keys.ts`：增加 APIKey 提示词字段和模式。
- 修改 `frontend/src/types/index.ts`：增加共享提示词模式和 DTO 字段。
- 修改 `frontend/src/views/admin/SettingsView.vue`：在 gateway settings 中增加平台提示词控件。
- 修改 `frontend/src/views/admin/GroupsView.vue`：在 create/edit 表单中增加 group 提示词控件。
- 修改 `frontend/src/views/user/KeysView.vue`：在 create/edit 表单中增加 APIKey 提示词控件。
- 仅当现有组件边界允许小范围聚焦覆盖时创建前端测试；否则依赖 `pnpm -C frontend typecheck` 和 `pnpm -C frontend build`。

## 任务 1：领域模型和提示词 Helper 测试

**文件：**
- 新建：`backend/internal/service/system_prompt.go`
- 新建：`backend/internal/service/system_prompt_test.go`
- 修改：`backend/internal/service/domain_constants.go`

- [ ] **步骤 1：编写失败的解析器测试**

新增 `backend/internal/service/system_prompt_test.go`：

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

- [ ] **步骤 2：运行解析器测试，确认 RED**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt' -count=1
```

预期：FAIL，因为 `EffectiveSystemPrompt`、`SystemPromptModeAppend` 和 `ResolveEffectiveSystemPrompt` 尚不存在。

- [ ] **步骤 3：实现模式常量和解析器**

向 `backend/internal/service/domain_constants.go` 增加常量：

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

新增 `backend/internal/service/system_prompt.go`：

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

- [ ] **步骤 4：运行解析器测试，确认 GREEN**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt' -count=1
```

预期：PASS。

- [ ] **步骤 5：提交**

```bash
git add backend/internal/service/domain_constants.go backend/internal/service/system_prompt.go backend/internal/service/system_prompt_test.go
git commit -m "feat: add system prompt resolver"
```

## 任务 2：请求注入 Helper

**文件：**
- 修改：`backend/internal/service/system_prompt.go`
- 修改：`backend/internal/service/system_prompt_test.go`

- [ ] **步骤 1：编写失败的注入测试**

追加到 `backend/internal/service/system_prompt_test.go`：

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

- [ ] **步骤 2：运行注入测试，确认 RED**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestApplySystemPrompt' -count=1
```

预期：FAIL，因为 `ApplySystemPromptToJSON` 和 `ApplySystemPromptToChatCompletionsJSON` 尚不存在。

- [ ] **步骤 3：实现注入 helper**

在 `backend/internal/service/system_prompt.go` 中增加 helper：先 unmarshal 到 `map[string]any`，修改相关字段，再 marshal 回去：

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

同时在同一文件中根据设计语义实现 `applySystemPromptToAnthropic`、`applySystemPromptToOpenAIResponses` 和 `applySystemPromptToGemini`，并 import `encoding/json`。

- [ ] **步骤 4：运行注入测试，确认 GREEN**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestApplySystemPrompt' -count=1
```

预期：PASS。

- [ ] **步骤 5：提交**

```bash
git add backend/internal/service/system_prompt.go backend/internal/service/system_prompt_test.go
git commit -m "feat: add system prompt injection helpers"
```

## 任务 3：数据库 Schema 和 ent 字段

**文件：**
- 新建：`backend/migrations/116_add_system_prompt_configuration.sql`
- 修改：`backend/ent/schema/api_key.go`
- 修改：`backend/ent/schema/group.go`
- 生成：`backend/ent/*`
- 测试：`backend/internal/repository/migrations_schema_integration_test.go`

- [ ] **步骤 1：编写失败的 migration schema 断言**

向 `backend/internal/repository/migrations_schema_integration_test.go` 增加断言：

```go
requireColumn(t, tx, "api_keys", "system_prompt")
requireColumn(t, tx, "api_keys", "system_prompt_mode")
requireColumn(t, tx, "groups", "system_prompt")
requireColumn(t, tx, "groups", "system_prompt_mode")
```

沿用该文件中已有的 helper 风格。如果当前只有表检查，则增加 helper：

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

- [ ] **步骤 2：运行 migration schema 测试，确认 RED**

运行：

```bash
cd backend && go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1
```

预期：FAIL，因为新字段尚不存在。

- [ ] **步骤 3：增加 migration**

新建 `backend/migrations/116_add_system_prompt_configuration.sql`：

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

- [ ] **步骤 4：增加 ent schema 字段并生成代码**

在 `backend/ent/schema/api_key.go` 中增加：

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

在 `backend/ent/schema/group.go` 中增加：

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

运行：

```bash
cd backend && go generate ./ent
```

预期：ent 生成文件更新且没有错误。

- [ ] **步骤 5：运行 migration schema 测试，确认 GREEN**

运行：

```bash
cd backend && go test -tags integration ./internal/repository -run TestMigrationsSchema -count=1
```

预期：PASS。

- [ ] **步骤 6：提交**

```bash
git add backend/migrations/116_add_system_prompt_configuration.sql backend/ent backend/internal/repository/migrations_schema_integration_test.go
git commit -m "feat: add system prompt database fields"
```

## 任务 4：持久化、DTO 和鉴权缓存快照

**文件：**
- 修改：`backend/internal/service/api_key.go`
- 修改：`backend/internal/service/group.go`
- 修改：`backend/internal/service/api_key_auth_cache.go`
- 修改：`backend/internal/service/api_key_auth_cache_impl.go`
- 修改：`backend/internal/repository/api_key_repo.go`
- 修改：`backend/internal/repository/group_repo.go`
- 修改：`backend/internal/handler/dto/types.go`
- 修改：`backend/internal/handler/dto/mappers.go`
- 修改：`backend/internal/repository/api_key_repo_messages_dispatch_unit_test.go`
- 修改：`backend/internal/repository/api_key_repo_integration_test.go`
- 修改：`backend/internal/repository/group_repo_integration_test.go`
- 修改：`backend/internal/handler/dto/api_key_mapper_last_used_test.go`

- [ ] **步骤 1：编写失败的 repository 和 mapper 断言**

增加断言：

```go
require.Equal(t, "group prompt", got.Group.SystemPrompt)
require.Equal(t, service.SystemPromptModeAppend, got.Group.SystemPromptMode)
require.Equal(t, "key prompt", got.SystemPrompt)
require.Equal(t, service.SystemPromptModeOverride, got.SystemPromptMode)
```

放入以下位置：

- `TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite`
- `APIKeyRepoSuite.TestGetByKeyForAuth_PreservesMessagesDispatchModelConfig`
- 新增 `GroupRepoSuite.TestSystemPromptRoundTrip`
- `TestAPIKeyFromService_MapsLastUsedAt`

- [ ] **步骤 2：运行聚焦测试，确认 RED**

运行：

```bash
cd backend && go test -tags unit ./internal/repository -run 'TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite' -count=1
cd backend && go test -tags integration ./internal/repository -run 'TestAPIKeyRepoSuite|TestGroupRepoSuite' -count=1
cd backend && go test -tags unit ./internal/handler/dto -run TestAPIKeyFromService_MapsLastUsedAt -count=1
```

预期：FAIL，因为 service、ent、repo、cache 和 DTO 字段尚未映射。

- [ ] **步骤 3：增加 service 和 DTO 字段**

向 `service.APIKey`、`service.Group`、`dto.APIKey` 和 `dto.Group` 增加：

```go
SystemPrompt     string `json:"system_prompt"`
SystemPromptMode string `json:"system_prompt_mode"`
```

如果周边 service struct 未使用 JSON tag，则 service struct 中可省略 JSON tag。

- [ ] **步骤 4：映射 repository create/update/select**

在 `api_key_repo.go` 中：

- `Create`：调用 `SetSystemPrompt(key.SystemPrompt)` 和 `SetSystemPromptMode(NormalizeSystemPromptMode(key.SystemPromptMode))`。
- `GetByKeyForAuth.Select`：包含 `apikey.FieldSystemPrompt` 和 `apikey.FieldSystemPromptMode`。
- `Update`：设置这两个字段。
- `apiKeyEntityToService`：复制这两个字段。

在 `group_repo.go` 中：

- `Create`：调用 `SetSystemPrompt(groupIn.SystemPrompt)` 和 `SetSystemPromptMode(NormalizeSystemPromptMode(groupIn.SystemPromptMode))`。
- `Update`：设置这两个字段。
- `groupEntityToService`：复制这两个字段。

- [ ] **步骤 5：映射鉴权缓存快照**

在 `api_key_auth_cache.go` 中，将 APIKey 和 Group 字段加入快照：

```go
SystemPrompt     string `json:"system_prompt,omitempty"`
SystemPromptMode string `json:"system_prompt_mode,omitempty"`
```

在 `api_key_auth_cache_impl.go` 中：

- 提升 `apiKeyAuthSnapshotVersion`。
- 在 `snapshotFromAPIKey` 中复制字段。
- 在 `snapshotToAPIKey` 中复制字段。

- [ ] **步骤 6：映射 DTO**

在 `dto/mappers.go` 中设置：

```go
SystemPrompt:     k.SystemPrompt,
SystemPromptMode: k.SystemPromptMode,
```

以及在 `groupFromServiceBase` 中：

```go
SystemPrompt:     g.SystemPrompt,
SystemPromptMode: g.SystemPromptMode,
```

- [ ] **步骤 7：运行聚焦测试，确认 GREEN**

运行：

```bash
cd backend && go test -tags unit ./internal/repository -run 'TestAPIKeyRepository_GetByKeyForAuth_PreservesMessagesDispatchModelConfig_SQLite' -count=1
cd backend && go test -tags integration ./internal/repository -run 'TestAPIKeyRepoSuite|TestGroupRepoSuite' -count=1
cd backend && go test -tags unit ./internal/handler/dto -run TestAPIKeyFromService_MapsLastUsedAt -count=1
cd backend && go test -tags unit ./internal/service -run TestAPIKeyService_GetByKey_UsesL2Cache -count=1
```

预期：PASS。

- [ ] **步骤 8：提交**

```bash
git add backend/internal/service/api_key.go backend/internal/service/group.go backend/internal/service/api_key_auth_cache.go backend/internal/service/api_key_auth_cache_impl.go backend/internal/repository/api_key_repo.go backend/internal/repository/group_repo.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/repository/*api_key*test.go backend/internal/repository/group_repo_integration_test.go backend/internal/handler/dto/api_key_mapper_last_used_test.go
git commit -m "feat: persist system prompt settings on keys and groups"
```

## 任务 5：Settings 字段和 7 天缓存

**文件：**
- 修改：`backend/internal/service/settings_view.go`
- 修改：`backend/internal/service/setting_service.go`
- 修改：`backend/internal/service/domain_constants.go`
- 新建：`backend/internal/service/setting_service_system_prompt_test.go`
- 修改：`backend/internal/handler/dto/settings.go`
- 修改：`backend/internal/handler/admin/setting_handler.go`

- [ ] **步骤 1：编写失败的缓存测试**

新建 `backend/internal/service/setting_service_system_prompt_test.go`：

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

不要使用不存在的 `newSettingRepoStub`；可以保留上面的本地 `settingPromptRepoStub`，也可以沿用同包内已有的 `settingPublicRepoStub`/`settingUpdateRepoStub` 模式。只统计 system-prompt 相关的 `GetMultiple` 调用，避免现有 Codex CLI 缓存预热 goroutine 让缓存断言变得不稳定。

- [ ] **步骤 2：运行缓存测试，确认 RED**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestSettingService_.*SystemPrompt' -count=1
```

预期：FAIL，因为 setting keys、字段和缓存方法尚不存在。

- [ ] **步骤 3：增加 setting keys 和 settings 字段**

在 `domain_constants.go` 中增加：

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

在 `settings_view.go` 和 `dto/settings.go` 中增加这 8 个字段。

- [ ] **步骤 4：实现 settings 持久化和缓存**

在 `setting_service.go` 中：

- 增加 `cachedSystemPromptSettings`、`systemPromptSettingsCache`、`systemPromptSettingsSF`、`systemPromptSettingsCacheTTL = 7 * 24 * time.Hour`、`systemPromptSettingsErrorTTL = 5 * time.Second` 和 DB timeout。
- 增加 `GetSystemPromptSettings(ctx context.Context) map[string]EffectiveSystemPrompt`。
- 通过 `GetMultiple` 读取 8 个 key。
- 使用 `NormalizeSystemPromptMode` 归一化 mode。
- 在 `atomic.Value` 中存储副本。
- 更新 `UpdateSettings`，持久化这 8 个 key。
- 更新 `UpdateSettings`，在 `SetMultiple` 成功后用最新保存的 settings 刷新缓存。
- 更新 `parseSettings` 的默认值为 `inherit`。

- [ ] **步骤 5：增加 handler 请求/响应映射**

在 `admin/setting_handler.go` 中：

- 为所有 8 个 key 增加 request 字段。
- 将 request 映射到 `service.SystemSettings`。
- 在成功 DTO 中包含全部 8 个字段。
- 任一字段变更时，增加用于审计的 diff entries。

- [ ] **步骤 6：运行 settings 测试，确认 GREEN**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestSettingService_.*SystemPrompt|TestSettingService_UpdateSettings' -count=1
cd backend && go test -tags unit ./internal/handler/admin -run Test.*Settings -count=1
```

预期：service 测试 PASS。如果不存在 handler settings 测试，第二个命令可以报告没有匹配测试，但 package compile 不能出错。

- [ ] **步骤 7：提交**

```bash
git add backend/internal/service/settings_view.go backend/internal/service/setting_service.go backend/internal/service/domain_constants.go backend/internal/service/setting_service_system_prompt_test.go backend/internal/handler/dto/settings.go backend/internal/handler/admin/setting_handler.go
git commit -m "feat: cache platform system prompt settings"
```

## 任务 6：APIKey 和 Group 的 API 请求校验

**文件：**
- 修改：`backend/internal/service/api_key_service.go`
- 修改：`backend/internal/service/admin_service.go`
- 修改：`backend/internal/handler/api_key_handler.go`
- 修改：`backend/internal/handler/admin/group_handler.go`
- 修改：`backend/internal/service/api_key_service_cache_test.go`
- 修改：`backend/internal/service/admin_service_group_test.go`

- [ ] **步骤 1：编写失败的 service 校验测试**

向 `system_prompt_test.go` 追加校验测试；如果还没有 `infraerrors` import，则补充该 import：

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

追加到 `api_key_service_cache_test.go`，或新建一个聚焦的 API key service 测试：

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

追加到 `admin_service_group_test.go`：

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

使用包内已经存在的真实 stub：`authRepoStub`、`authCacheStub`、`userRepoStub` 和 `groupRepoStubForAdmin`。只在需要的位置增加 import，例如 `api_key_service_cache_test.go` 中的 `config`。

- [ ] **步骤 2：运行 service 校验测试，确认 RED**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestAPIKeyService_Create_RejectsPromptModeWithoutPrompt|TestAdminService_CreateGroup_PersistsSystemPromptConfig|TestValidateSystemPromptConfig' -count=1
```

预期：FAIL，因为 request structs 和校验逻辑尚未补齐。

- [ ] **步骤 3：增加请求字段和校验**

向 service request structs 增加：

```go
SystemPrompt     string `json:"system_prompt"`
SystemPromptMode string `json:"system_prompt_mode"`
```

在 `system_prompt.go` 中增加校验 helper：

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

在 `system_prompt.go` 中 import `infraerrors`。

在 APIKey create/update 和 Group create/update 持久化前使用该 helper。增加 `TestValidateSystemPromptConfig_RejectsInvalidMode`，证明非法 mode 不会被错误地归一化成 `inherit`。

- [ ] **步骤 4：增加 handler DTO 字段**

在 `api_key_handler.go` 中，为 create/update request structs 增加字段，并映射到 service request structs。

在 `admin/group_handler.go` 中，为 create/update request structs 增加字段，并映射到 service input structs。

- [ ] **步骤 5：运行校验测试，确认 GREEN**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestAPIKeyService_Create_RejectsPromptModeWithoutPrompt|TestAdminService_CreateGroup_PersistsSystemPromptConfig|TestValidateSystemPromptConfig' -count=1
cd backend && go test -tags unit ./internal/handler -run Test.*APIKey -count=1
cd backend && go test -tags unit ./internal/handler/admin -run TestGroupHandlerEndpoints -count=1
```

预期：PASS；如果没有匹配的 handler 测试，也必须保证 package compile 成功。

- [ ] **步骤 6：提交**

```bash
git add backend/internal/service/api_key_service.go backend/internal/service/admin_service.go backend/internal/service/system_prompt.go backend/internal/handler/api_key_handler.go backend/internal/handler/admin/group_handler.go backend/internal/service/*test.go
git commit -m "feat: validate system prompt config inputs"
```

## 任务 7：网关集成

**文件：**
- 修改：`backend/internal/handler/gateway_handler.go`
- 修改：`backend/internal/handler/openai_gateway_handler.go`
- 修改：`backend/internal/handler/gemini_v1beta_handler.go`
- 修改：`backend/internal/service/openai_gateway_messages.go`
- 修改：`backend/internal/service/openai_gateway_chat_completions.go`
- 修改：`backend/internal/service/gateway_forward_as_chat_completions.go`
- 修改：`backend/internal/service/gemini_messages_compat_service.go`
- 修改：`backend/internal/service/antigravity_gateway_service.go`
- 修改：`backend/internal/handler/openai_gateway_handler_test.go`
- 修改：`backend/internal/handler/gemini_v1beta_handler_test.go`
- 修改：`backend/internal/service/gateway_prompt_test.go`
- 修改：`backend/cmd/server/wire_gen.go`

- [ ] **步骤 1：编写失败的集成级网关测试**

在 `openai_gateway_handler_test.go` 中增加一个聚焦的 handler 测试，覆盖 OpenAI Responses 请求体变更：

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

这个测试先作为 service helper 防护；等 handler 在 `Forward` 前调用注入逻辑后，它会成为 handler 回归测试。

在 `gemini_v1beta_handler_test.go` 中增加一个聚焦的 handler 测试，证明传给 `ForwardNative` / `ForwardGemini` 的 body 已经包含解析后的系统提示词。

- [ ] **步骤 2：运行网关测试；如果增加了 handler 专属断言，则确认 RED**

运行：

```bash
cd backend && go test -tags unit ./internal/handler -run TestOpenAIResponses_AppliesAPIKeySystemPromptBeforeForward -count=1
cd backend && go test -tags unit ./internal/handler -run 'TestGeminiV1Beta.*SystemPrompt' -count=1
```

预期：如果测试使用 handler 并捕获 forwarded body，则 FAIL；如果只验证 helper 行为，则可能 PASS。若它只是 helper-only 且已经 PASS，在进入实现前要补上 handler capture。

- [ ] **步骤 3：在转发前增加 handler 级注入**

在 `gateway_handler.go` 中：

- fallback group 解析之后、`c.Set("parsed_request", parsedReq)` 之前，使用 `currentAPIKey`、当前 group platform 和 `h.settingService` 解析生效提示词。
- 对 Antigravity OAuth `body` 调用 `ApplySystemPromptToJSON(body, service.PlatformAntigravity, prompt)`。
- 对 Anthropic parsed request 同时更新 `parsedReq.Body` 和 `body`；如果 body 发生变化，则重新 parse，保证 `parsedReq.System` 保持一致。

在 `openai_gateway_handler.go` 中：

- 在 `Responses` 中，根据 APIKey 和 `resolveOpenAICompatibleGroupPlatform(apiKey)` 解析提示词；如果提示词需要影响 session hash，则在 channel model mapping 和 session hash 前修改 `body`。
- 在 `Messages` 中，在 `ForwardAsAnthropic` 前修改 Anthropic `body`。
- 在 `ResponsesWebSocket` 中，在 session hash 和 channel model mapping 前修改 `firstMessage`。

在 `gemini_v1beta_handler.go` 中：

- 在 `setOpsRequestContext` / `ParseGatewayRequest` 前通过 `h.settingService` 解析生效提示词。
- 在任何 session hash 或 forward call 前修改原始 `body`，确保 `ForwardNative` 和 `ForwardGemini` 看到已经注入的提示词。

在 `gemini_messages_compat_service.go` 中：

- 保持 `ForwardNative` 只作为 transport-only 路径，保留已经注入的 body。

- [ ] **步骤 4：增加 service 级转换兜底**

在 OpenAI 和 Chat Completions 转换服务中，如果 raw handler 路径无法覆盖对应格式，则在 marshal 上游 body 前应用 helper：

- `openai_gateway_messages.go`：确保配置提示词在 `AnthropicToResponses` 前被包含。
- `openai_gateway_chat_completions.go`：在 `ChatCompletionsToResponses` 前应用 `ApplySystemPromptToChatCompletionsJSON`。
- `gateway_forward_as_chat_completions.go`：在 Chat Completions to Anthropic 转换前应用 `ApplySystemPromptToChatCompletionsJSON`。
- `antigravity_gateway_service.go`：拆分 Gemini 提示词注入，确保 identity patch 保持第一位，业务提示词紧随其后插入，并发生在 schema cleanup 和 wrapping 之前。

完成这些编辑后，运行 `cd backend && go generate ./cmd/server`，让 `backend/cmd/server/wire_gen.go` 获取 `NewOpenAIGatewayHandler` 上 `SettingService` 构造参数的变化。

- [ ] **步骤 5：运行网关提示词测试**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'TestApplySystemPrompt|TestGatewayPrompt|TestOpenAI.*Instructions|TestGemini.*SystemInstruction' -count=1
cd backend && go test -tags unit ./internal/handler -run 'TestOpenAIResponses|TestOpenAIHandler_InstructionsInjection|TestGeminiV1Beta.*SystemPrompt' -count=1
```

预期：PASS。

- [ ] **步骤 6：提交**

```bash
git add backend/internal/handler/gateway_handler.go backend/internal/handler/openai_gateway_handler.go backend/internal/handler/gemini_v1beta_handler.go backend/internal/service/openai_gateway_messages.go backend/internal/service/openai_gateway_chat_completions.go backend/internal/service/gateway_forward_as_chat_completions.go backend/internal/service/gemini_messages_compat_service.go backend/internal/service/antigravity_gateway_service.go backend/internal/handler/openai_gateway_handler_test.go backend/internal/handler/gemini_v1beta_handler_test.go backend/internal/service/gateway_prompt_test.go backend/cmd/server/wire_gen.go
git commit -m "feat: inject effective system prompts in gateways"
```

## 任务 8：前端 API 类型和表单

**文件：**
- 修改：`frontend/src/types/index.ts`
- 修改：`frontend/src/api/admin/settings.ts`
- 修改：`frontend/src/api/admin/groups.ts`
- 修改：`frontend/src/api/keys.ts`
- 修改：`frontend/src/views/admin/SettingsView.vue`
- 修改：`frontend/src/views/admin/GroupsView.vue`
- 修改：`frontend/src/views/user/KeysView.vue`

- [ ] **步骤 1：先增加 TypeScript 类型**

向 `frontend/src/types/index.ts` 增加：

```ts
export type SystemPromptMode = 'inherit' | 'passthrough' | 'override' | 'append'
```

向 APIKey 和 Group 类型增加 `system_prompt?: string` 与 `system_prompt_mode?: SystemPromptMode`。

- [ ] **步骤 2：运行 typecheck，确认 RED**

运行：

```bash
pnpm -C frontend typecheck
```

预期：下一步更新 API 使用后，如果组件字段缺失会 FAIL。如果这里已经 PASS，继续执行。

- [ ] **步骤 3：增加 API 请求字段**

在 `frontend/src/api/admin/settings.ts` 中，将 8 个 settings 字段加入 `SystemSettings` 和 `UpdateSettingsRequest`。

在 `frontend/src/api/admin/groups.ts` 中增加：

```ts
system_prompt?: string
system_prompt_mode?: SystemPromptMode
```

加入 create/update payload 类型。

在 `frontend/src/api/keys.ts` 中，将 `system_prompt` 和 `system_prompt_mode` 加入 create/update payload。

- [ ] **步骤 4：在 SettingsView 中增加 UI 控件**

在 `SettingsView.vue` 中：

- 初始化全部 8 个表单字段，mode 默认为 `inherit`，prompt 默认为空。
- 在 gateway settings 区域中，每个平台增加一行紧凑的 textarea。
- select 选项使用：`不配置`、`透传`、`覆盖`、`追加`。
- 当 mode 为 `inherit` 时禁用 textarea 或降低其视觉强调。
- 保存 payload 中包含全部 8 个字段。

- [ ] **步骤 5：在 GroupsView 中增加 UI 控件**

在 `GroupsView.vue` 中：

- 向 create 和 edit form state 增加 `system_prompt: ''` 与 `system_prompt_mode: 'inherit'`。
- 在 create/edit modals 中，将 select 和 textarea 放在 platform/routing settings 附近。
- 在 `handleCreateGroup` 和 `handleUpdateGroup` payload 中包含这两个字段。
- 在 `openEditGroup` 中回填这两个字段。

- [ ] **步骤 6：在 KeysView 中增加 UI 控件**

在 `KeysView.vue` 中：

- 向 form state 增加 `system_prompt: ''` 与 `system_prompt_mode: 'inherit'`。
- 在 create/edit modal 中展示 select 和 textarea。
- 在 create/update API calls 中包含这两个字段。
- modal 关闭时重置这两个字段。

- [ ] **步骤 7：运行前端验证**

运行：

```bash
pnpm -C frontend typecheck
pnpm -C frontend build
```

预期：PASS。

- [ ] **步骤 8：提交**

```bash
git add frontend/src/types/index.ts frontend/src/api/admin/settings.ts frontend/src/api/admin/groups.ts frontend/src/api/keys.ts frontend/src/views/admin/SettingsView.vue frontend/src/views/admin/GroupsView.vue frontend/src/views/user/KeysView.vue
git commit -m "feat: add system prompt controls"
```

## 任务 9：最终验证与审计

**文件：**
- 仅当验证发现缺陷时修改文件。

- [ ] **步骤 1：运行聚焦后端套件**

运行：

```bash
cd backend && go test -tags unit ./internal/service -run 'SystemPrompt|SettingService_.*SystemPrompt|APIKeyService|AdminService_CreateGroup|AdminService_UpdateGroup' -count=1
cd backend && go test -tags integration ./internal/repository -run 'APIKey|Group|MigrationsSchema' -count=1
cd backend && go test -tags unit ./internal/handler ./internal/handler/admin ./internal/handler/dto -run 'APIKey|Group|Settings|SystemPrompt|OpenAIResponses|GeminiV1Beta' -count=1
```

预期：PASS。

- [ ] **步骤 2：运行更广的后端编译测试**

运行：

```bash
cd backend && go test -tags unit ./internal/service ./internal/handler ./internal/handler/admin ./internal/handler/dto -count=1
cd backend && go test -tags integration ./internal/repository -count=1
```

预期：PASS。

- [ ] **步骤 3：运行前端验证**

运行：

```bash
pnpm -C frontend typecheck
pnpm -C frontend build
```

预期：PASS。

- [ ] **步骤 4：检查 git diff**

运行：

```bash
git status --short
git diff --stat
git diff --check
```

预期：只改动计划内文件，并且没有空白字符错误。

- [ ] **步骤 5：请求代码审查**

使用 `requesting-code-review` 技能。请 reviewer 重点关注：

- mode 语义和继承行为
- 缓存正确性与更新刷新
- 鉴权缓存快照版本和失效逻辑
- OpenAI Codex/Antigravity identity patch 的请求注入顺序
- migration 安全性与默认兼容性
- 前端 payload 完整性

- [ ] **步骤 6：应用 review 修复，重新验证并提交**

修复后运行同样的验证命令，然后：

```bash
git add backend frontend docs
git commit -m "feat: support hierarchical system prompts"
```

## 自查

- 需求覆盖：计划覆盖数据库字段、settings keys、7 天 settings 缓存、鉴权缓存快照、API/DTO 字段、解析优先级、注入模式、Gemini/OpenAI/Anthropic/Antigravity 入口点、前端控件和验证。
- 执行安全：测试命令已经区分 `unit` 和 `integration` tags，避免意外跳过套件。
- 占位符扫描：计划不再使用 `newSettingRepoStub`、`newAPIKeyServiceForCacheTest` 或 `newAdminServiceWithGroupRepoForTest` 这类虚构 helper；唯一出现的位置是明确警告。
- 类型一致性：全文使用一致命名：`system_prompt`、`system_prompt_mode`、`SystemPromptModeInherit`、`SystemPromptModePassthrough`、`SystemPromptModeOverride`、`SystemPromptModeAppend`、`EffectiveSystemPrompt`。
