# 系统提示词用户范围控制实施计划

> **给执行 Agent：** 必须使用子技能：按任务逐项执行时，使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 在系统配置中增加系统提示词用户范围控制，支持全量、白名单、黑名单三种策略；管理员通过邮箱搜索选择用户，后端按用户 ID 持久化和运行时判断。

**架构：** 新增用户范围策略作为系统提示词注入前的总门禁，配置存储在 `settings` 表并进入现有系统提示词 7 天进程内缓存；`UpdateSettings` 成功后立即刷新缓存。运行时先按当前 API Key 的用户 ID 判断是否允许系统提示词生效，允许后再沿用现有优先级 `API Key > 分组 > 系统配置的平台提示词`。

**技术栈：** Go、Gin、ent、PostgreSQL migrations、`atomic.Value` + `singleflight` 缓存、Vue 3、TypeScript、Pinia、Vitest、vue-tsc、Vite。

---

## 文件结构

- 新建 `backend/migrations/117_system_prompt_user_scope.sql`：补充系统提示词用户范围控制 settings 默认值；不修改已存在迁移，避免 checksum 风险。
- 修改 `backend/internal/service/domain_constants.go`：新增范围模式常量和 settings key。
- 修改 `backend/internal/service/settings_view.go`：在 `SystemSettings` 中增加范围控制字段。
- 修改 `backend/internal/handler/dto/settings.go`：在管理员 settings 响应 DTO 中增加范围控制字段。
- 修改 `backend/internal/handler/admin/setting_handler.go`：接收、返回、审计范围控制字段。
- 修改 `backend/internal/service/setting_service.go`：持久化范围控制字段、归一化用户 ID 列表、写入系统提示词缓存。
- 修改 `backend/internal/service/system_prompt.go`：在 `ResolveEffectiveSystemPrompt` 中增加用户范围门禁。
- 修改 `backend/internal/service/system_prompt_test.go`：覆盖全量、白名单、黑名单、无 API Key 用户时的解析行为。
- 修改 `backend/internal/service/system_prompt_settings_cache_test.go`：覆盖范围配置缓存和更新后刷新。
- 修改 `frontend/src/api/admin/settings.ts`：增加 settings 类型和更新请求字段。
- 修改 `frontend/src/types/index.ts`：增加前端共享类型 `SystemPromptUserScopeMode` 和系统配置字段。
- 新建 `frontend/src/components/common/UserMultiSearchSelect.vue`：支持通过邮箱搜索选择多个用户，组件值为 `number[]`。
- 修改 `frontend/src/views/admin/SettingsView.vue`：在“系统提示词”页顶部增加用户范围控制可视化配置。
- 修改 `frontend/src/i18n/locales/zh.ts`：增加中文文案。
- 修改 `frontend/src/i18n/locales/en.ts`：增加英文文案。
- 新建 `frontend/src/components/common/__tests__/UserMultiSearchSelect.spec.ts`：覆盖搜索、选择、移除、多选去重。
- 修改或新建 `frontend/src/views/admin/__tests__/settingsSystemPromptPlacement.spec.ts`：覆盖系统提示词页展示范围控制，并确保平台配置仍在下方。

## 任务 1：后端领域常量和配置类型

**文件：**
- 修改：`backend/internal/service/domain_constants.go`
- 修改：`backend/internal/service/settings_view.go`
- 修改：`backend/internal/handler/dto/settings.go`
- 修改：`backend/internal/handler/admin/setting_handler.go`
- 修改：`frontend/src/api/admin/settings.ts`
- 修改：`frontend/src/types/index.ts`

- [ ] **步骤 1：增加失败的后端类型引用测试**

在 `backend/internal/service/system_prompt_test.go` 追加测试骨架，先只引用即将新增的范围模式常量：

```go
func TestSystemPromptUserScopeConstants(t *testing.T) {
	require.Equal(t, "all", SystemPromptUserScopeAll)
	require.Equal(t, "whitelist", SystemPromptUserScopeWhitelist)
	require.Equal(t, "blacklist", SystemPromptUserScopeBlacklist)
}
```

- [ ] **步骤 2：运行测试确认 RED**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test -tags unit ./internal/service -run TestSystemPromptUserScopeConstants -count=1
```

预期：FAIL，提示范围模式常量未定义。

- [ ] **步骤 3：新增后端常量**

在 `backend/internal/service/domain_constants.go` 的系统提示词常量附近增加：

```go
const (
	SystemPromptUserScopeAll       = "all"
	SystemPromptUserScopeWhitelist = "whitelist"
	SystemPromptUserScopeBlacklist = "blacklist"
)
```

在 settings key 常量区增加：

```go
SettingKeySystemPromptUserScopeEnabled = "system_prompt_user_scope_enabled"
SettingKeySystemPromptUserScopeMode    = "system_prompt_user_scope_mode"
SettingKeySystemPromptUserScopeUserIDs = "system_prompt_user_scope_user_ids"
```

- [ ] **步骤 4：扩展后端 settings 类型**

在 `backend/internal/service/settings_view.go` 的系统提示词字段后增加：

```go
SystemPromptUserScopeEnabled bool
SystemPromptUserScopeMode    string
SystemPromptUserScopeUserIDs []int64
```

在 `backend/internal/handler/dto/settings.go` 的系统提示词字段后增加：

```go
SystemPromptUserScopeEnabled bool    `json:"system_prompt_user_scope_enabled"`
SystemPromptUserScopeMode    string  `json:"system_prompt_user_scope_mode"`
SystemPromptUserScopeUserIDs []int64 `json:"system_prompt_user_scope_user_ids"`
```

在 `backend/internal/handler/admin/setting_handler.go` 的 `UpdateSettingsRequest` 系统提示词字段后增加：

```go
SystemPromptUserScopeEnabled bool    `json:"system_prompt_user_scope_enabled"`
SystemPromptUserScopeMode    string  `json:"system_prompt_user_scope_mode"`
SystemPromptUserScopeUserIDs []int64 `json:"system_prompt_user_scope_user_ids"`
```

在 GET settings 响应映射、PUT request 到 service 映射、PUT response 映射里逐项补齐：

```go
SystemPromptUserScopeEnabled: settings.SystemPromptUserScopeEnabled,
SystemPromptUserScopeMode:    settings.SystemPromptUserScopeMode,
SystemPromptUserScopeUserIDs: settings.SystemPromptUserScopeUserIDs,
```

- [ ] **步骤 5：扩展前端类型**

在 `frontend/src/types/index.ts` 的 `SystemPromptMode` 附近增加：

```ts
export type SystemPromptUserScopeMode = 'all' | 'whitelist' | 'blacklist'
```

在 `frontend/src/api/admin/settings.ts` 中导入并加入类型：

```ts
import type { CustomMenuItem, CustomEndpoint, NotifyEmailEntry, SystemPromptMode, SystemPromptUserScopeMode } from '@/types'
```

在 `SystemSettings` 和 `UpdateSettingsRequest` 中增加：

```ts
system_prompt_user_scope_enabled: boolean
system_prompt_user_scope_mode: SystemPromptUserScopeMode
system_prompt_user_scope_user_ids: number[]
```

- [ ] **步骤 6：运行类型相关测试确认 GREEN**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test -tags unit ./internal/service -run TestSystemPromptUserScopeConstants -count=1
```

预期：PASS。

## 任务 2：settings 持久化、归一化和缓存

**文件：**
- 新建：`backend/migrations/117_system_prompt_user_scope.sql`
- 修改：`backend/internal/service/setting_service.go`
- 修改：`backend/internal/service/system_prompt_settings_cache_test.go`

- [ ] **步骤 1：写失败的缓存测试**

在 `backend/internal/service/system_prompt_settings_cache_test.go` 追加：

```go
func TestSettingService_GetSystemPromptSettings_CachesUserScope(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{values: map[string]string{
		SettingKeySystemPromptUserScopeEnabled: "true",
		SettingKeySystemPromptUserScopeMode:    SystemPromptUserScopeWhitelist,
		SettingKeySystemPromptUserScopeUserIDs: "[101,202,101]",
	}}
	svc := NewSettingService(repo, &config.Config{})

	first := svc.GetSystemPromptSettings(context.Background())
	second := svc.GetSystemPromptSettings(context.Background())

	require.Equal(t, 1, repo.getMultipleCalls)
	require.True(t, first.UserScope.Enabled)
	require.Equal(t, SystemPromptUserScopeWhitelist, first.UserScope.Mode)
	require.Equal(t, []int64{101, 202}, first.UserScope.UserIDs)
	require.Equal(t, first.UserScope, second.UserScope)
}

func TestSettingService_UpdateSettingsRefreshesSystemPromptUserScopeCache(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{values: map[string]string{
		SettingKeySystemPromptUserScopeEnabled: "false",
		SettingKeySystemPromptUserScopeMode:    SystemPromptUserScopeAll,
		SettingKeySystemPromptUserScopeUserIDs: "[]",
	}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		SystemPromptUserScopeEnabled: true,
		SystemPromptUserScopeMode:    SystemPromptUserScopeBlacklist,
		SystemPromptUserScopeUserIDs: []int64{7, 3, 7},
	})
	require.NoError(t, err)

	got := svc.GetSystemPromptSettings(context.Background())
	require.True(t, got.UserScope.Enabled)
	require.Equal(t, SystemPromptUserScopeBlacklist, got.UserScope.Mode)
	require.Equal(t, []int64{3, 7}, got.UserScope.UserIDs)
	require.Equal(t, "true", repo.updates[SettingKeySystemPromptUserScopeEnabled])
	require.Equal(t, SystemPromptUserScopeBlacklist, repo.updates[SettingKeySystemPromptUserScopeMode])
	require.JSONEq(t, `[3,7]`, repo.updates[SettingKeySystemPromptUserScopeUserIDs])
}
```

同时把 `runtimePromptSettingsStub.GetSystemPromptSettings` 的返回类型按任务 2 的新结构调整。

- [ ] **步骤 2：运行缓存测试确认 RED**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test ./internal/service -run 'TestSettingService_.*SystemPrompt.*UserScope' -count=1
```

预期：FAIL，因为缓存返回结构和范围字段尚未实现。

- [ ] **步骤 3：升级缓存返回结构**

在 `backend/internal/service/system_prompt.go` 增加：

```go
type SystemPromptUserScope struct {
	Enabled bool
	Mode    string
	UserIDs []int64
}

type SystemPromptRuntimeSettings struct {
	Prompts   map[string]EffectiveSystemPrompt
	UserScope SystemPromptUserScope
}
```

把接口改为：

```go
type SystemPromptSettingsProvider interface {
	GetSystemPromptSettings(ctx context.Context) SystemPromptRuntimeSettings
}
```

把所有测试 stub 从 `map[string]EffectiveSystemPrompt` 调整为 `SystemPromptRuntimeSettings{Prompts: ...}`。

- [ ] **步骤 4：实现范围归一化 helper**

在 `backend/internal/service/setting_service.go` 增加：

```go
func normalizeSystemPromptUserScopeMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case SystemPromptUserScopeWhitelist, SystemPromptUserScopeBlacklist:
		return strings.TrimSpace(mode)
	default:
		return SystemPromptUserScopeAll
	}
}

func normalizeSystemPromptUserScopeUserIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func parseSystemPromptUserScopeUserIDs(raw string) []int64 {
	var ids []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ids); err != nil {
		return []int64{}
	}
	return normalizeSystemPromptUserScopeUserIDs(ids)
}

func marshalSystemPromptUserScopeUserIDs(ids []int64) (string, error) {
	normalized := normalizeSystemPromptUserScopeUserIDs(ids)
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
```

确保 import 增加 `sort`。

当模式为 `all` 时，保存阶段把 `SystemPromptUserScopeUserIDs` 归一化为空数组，避免 UI 再次切换模式时带出旧名单。

- [ ] **步骤 5：纳入 settings key、默认值、GetSettings、UpdateSettings**

把 `systemPromptSettingKeys` 改为包含三个新 key：

```go
SettingKeySystemPromptUserScopeEnabled,
SettingKeySystemPromptUserScopeMode,
SettingKeySystemPromptUserScopeUserIDs,
```

在 `InitializeDefaults` 默认值中增加：

```go
SettingKeySystemPromptUserScopeEnabled: "false",
SettingKeySystemPromptUserScopeMode:    SystemPromptUserScopeAll,
SettingKeySystemPromptUserScopeUserIDs: "[]",
```

在 `UpdateSettings` 的系统提示词配置归一化后增加：

```go
settings.SystemPromptUserScopeMode = normalizeSystemPromptUserScopeMode(settings.SystemPromptUserScopeMode)
settings.SystemPromptUserScopeUserIDs = normalizeSystemPromptUserScopeUserIDs(settings.SystemPromptUserScopeUserIDs)
if settings.SystemPromptUserScopeMode == SystemPromptUserScopeAll {
	settings.SystemPromptUserScopeUserIDs = []int64{}
}
scopeUserIDsJSON, err := marshalSystemPromptUserScopeUserIDs(settings.SystemPromptUserScopeUserIDs)
if err != nil {
	return fmt.Errorf("marshal system prompt user scope user ids: %w", err)
}
updates[SettingKeySystemPromptUserScopeEnabled] = strconv.FormatBool(settings.SystemPromptUserScopeEnabled)
updates[SettingKeySystemPromptUserScopeMode] = settings.SystemPromptUserScopeMode
updates[SettingKeySystemPromptUserScopeUserIDs] = scopeUserIDsJSON
```

在 settings 读取映射中增加：

```go
result.SystemPromptUserScopeEnabled = settings[SettingKeySystemPromptUserScopeEnabled] == "true"
result.SystemPromptUserScopeMode = normalizeSystemPromptUserScopeMode(settings[SettingKeySystemPromptUserScopeMode])
result.SystemPromptUserScopeUserIDs = parseSystemPromptUserScopeUserIDs(settings[SettingKeySystemPromptUserScopeUserIDs])
```

- [ ] **步骤 6：刷新缓存结构**

把 `cachedSystemPromptSettings.values` 改为：

```go
values SystemPromptRuntimeSettings
```

把 `defaultSystemPromptSettings()` 改为返回：

```go
return SystemPromptRuntimeSettings{
	Prompts: map[string]EffectiveSystemPrompt{
		PlatformAnthropic:   {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
		PlatformOpenAI:      {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
		PlatformGemini:      {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
		PlatformAntigravity: {Mode: SystemPromptModeInherit, Source: SystemPromptSourceSystem},
	},
	UserScope: SystemPromptUserScope{
		Enabled: false,
		Mode:    SystemPromptUserScopeAll,
		UserIDs: []int64{},
	},
}
```

让 `buildSystemPromptSettings(values)` 设置 `Prompts` 并同时设置：

```go
out.UserScope = SystemPromptUserScope{
	Enabled: values[SettingKeySystemPromptUserScopeEnabled] == "true",
	Mode:    normalizeSystemPromptUserScopeMode(values[SettingKeySystemPromptUserScopeMode]),
	UserIDs: parseSystemPromptUserScopeUserIDs(values[SettingKeySystemPromptUserScopeUserIDs]),
}
```

让 `refreshSystemPromptSettingsCache(settings)` 把三个新字段写入 `values`。

把 clone helper 改为深拷贝 map 和 user IDs：

```go
func cloneSystemPromptSettings(in SystemPromptRuntimeSettings) SystemPromptRuntimeSettings {
	prompts := make(map[string]EffectiveSystemPrompt, len(in.Prompts))
	for key, value := range in.Prompts {
		prompts[key] = value
	}
	userIDs := append([]int64(nil), in.UserScope.UserIDs...)
	return SystemPromptRuntimeSettings{
		Prompts: prompts,
		UserScope: SystemPromptUserScope{
			Enabled: in.UserScope.Enabled,
			Mode:    in.UserScope.Mode,
			UserIDs: userIDs,
		},
	}
}
```

- [ ] **步骤 7：新增迁移默认值**

新建 `backend/migrations/117_system_prompt_user_scope.sql`：

```sql
-- 117_system_prompt_user_scope.sql
-- Adds user-scope control defaults for system prompt injection.

INSERT INTO settings (key, value)
VALUES
    ('system_prompt_user_scope_enabled', 'false'),
    ('system_prompt_user_scope_mode', 'all'),
    ('system_prompt_user_scope_user_ids', '[]')
ON CONFLICT (key) DO NOTHING;
```

- [ ] **步骤 8：运行缓存测试确认 GREEN**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test ./internal/service -run 'TestSettingService_.*SystemPrompt.*UserScope|TestSettingService_GetSystemPromptSettings_CachesByPlatform|TestSettingService_UpdateSettingsRefreshesSystemPromptCache' -count=1
```

预期：PASS。

## 任务 3：运行时用户范围门禁

**文件：**
- 修改：`backend/internal/service/system_prompt.go`
- 修改：`backend/internal/service/system_prompt_test.go`
- 修改：`backend/internal/service/system_prompt_runtime_test.go`

- [ ] **步骤 1：写失败的解析测试**

在 `backend/internal/service/system_prompt_test.go` 追加：

```go
func TestResolveEffectiveSystemPrompt_UserScopeWhitelistAllowsListedUser(t *testing.T) {
	settings := promptSettingsStub{runtime: SystemPromptRuntimeSettings{
		Prompts: map[string]EffectiveSystemPrompt{
			PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
		},
		UserScope: SystemPromptUserScope{Enabled: true, Mode: SystemPromptUserScopeWhitelist, UserIDs: []int64{42}},
	}}
	apiKey := &APIKey{UserID: 42, SystemPromptMode: SystemPromptModeInherit}

	got := ResolveEffectiveSystemPrompt(context.Background(), apiKey, PlatformOpenAI, settings)

	require.True(t, got.Enabled())
	require.Equal(t, "system", got.Prompt)
}

func TestResolveEffectiveSystemPrompt_UserScopeWhitelistBlocksUnlistedUser(t *testing.T) {
	settings := promptSettingsStub{runtime: SystemPromptRuntimeSettings{
		Prompts: map[string]EffectiveSystemPrompt{
			PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
		},
		UserScope: SystemPromptUserScope{Enabled: true, Mode: SystemPromptUserScopeWhitelist, UserIDs: []int64{42}},
	}}
	apiKey := &APIKey{UserID: 99, SystemPrompt: "key", SystemPromptMode: SystemPromptModeOverride}

	got := ResolveEffectiveSystemPrompt(context.Background(), apiKey, PlatformOpenAI, settings)

	require.False(t, got.Enabled())
	require.Equal(t, SystemPromptSourceNone, got.Source)
}

func TestResolveEffectiveSystemPrompt_UserScopeBlacklistBlocksListedUser(t *testing.T) {
	settings := promptSettingsStub{runtime: SystemPromptRuntimeSettings{
		Prompts: map[string]EffectiveSystemPrompt{
			PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
		},
		UserScope: SystemPromptUserScope{Enabled: true, Mode: SystemPromptUserScopeBlacklist, UserIDs: []int64{42}},
	}}
	apiKey := &APIKey{UserID: 42, SystemPrompt: "key", SystemPromptMode: SystemPromptModeOverride}

	got := ResolveEffectiveSystemPrompt(context.Background(), apiKey, PlatformOpenAI, settings)

	require.False(t, got.Enabled())
	require.Equal(t, SystemPromptSourceNone, got.Source)
}

func TestResolveEffectiveSystemPrompt_UserScopeDisabledKeepsExistingBehavior(t *testing.T) {
	settings := promptSettingsStub{runtime: SystemPromptRuntimeSettings{
		Prompts: map[string]EffectiveSystemPrompt{
			PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
		},
		UserScope: SystemPromptUserScope{Enabled: false, Mode: SystemPromptUserScopeWhitelist, UserIDs: []int64{42}},
	}}
	apiKey := &APIKey{UserID: 99, SystemPromptMode: SystemPromptModeInherit}

	got := ResolveEffectiveSystemPrompt(context.Background(), apiKey, PlatformOpenAI, settings)

	require.True(t, got.Enabled())
	require.Equal(t, "system", got.Prompt)
}
```

把 `promptSettingsStub` 改为：

```go
type promptSettingsStub struct {
	runtime SystemPromptRuntimeSettings
}

func (s promptSettingsStub) GetSystemPromptSettings(context.Context) SystemPromptRuntimeSettings {
	return s.runtime
}
```

- [ ] **步骤 2：运行测试确认 RED**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt_UserScope' -count=1
```

预期：FAIL，因为运行时尚未检查用户范围。

- [ ] **步骤 3：实现范围判断**

在 `backend/internal/service/system_prompt.go` 增加：

```go
func isSystemPromptAllowedForUser(apiKey *APIKey, scope SystemPromptUserScope) bool {
	if !scope.Enabled {
		return true
	}
	userID := int64(0)
	if apiKey != nil {
		userID = apiKey.UserID
	}
	switch normalizeSystemPromptUserScopeMode(scope.Mode) {
	case SystemPromptUserScopeWhitelist:
		return userID > 0 && containsInt64(scope.UserIDs, userID)
	case SystemPromptUserScopeBlacklist:
		return userID <= 0 || !containsInt64(scope.UserIDs, userID)
	default:
		return true
	}
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
```

在 `ResolveEffectiveSystemPrompt` 中改为先读取 runtime settings：

```go
runtimeSettings := defaultSystemPromptSettings()
if !isNilSystemPromptSettingsProvider(settings) {
	runtimeSettings = settings.GetSystemPromptSettings(ctx)
}
if !isSystemPromptAllowedForUser(apiKey, runtimeSettings.UserScope) {
	return EffectiveSystemPrompt{Mode: SystemPromptModeInherit, Source: SystemPromptSourceNone}
}
```

然后平台默认读取改为：

```go
if p, ok := runtimeSettings.Prompts[strings.TrimSpace(platform)]; ok && p.Enabled() {
	return p
}
```

- [ ] **步骤 4：补充无 API Key 用户的 runtime 测试**

在 `backend/internal/service/system_prompt_runtime_test.go` 增加：

```go
func TestApplyResolvedSystemPromptToJSONNoopsWhenWhitelistHasNoAPIKeyUser(t *testing.T) {
	settings := runtimePromptSettingsStub{runtime: SystemPromptRuntimeSettings{
		Prompts: map[string]EffectiveSystemPrompt{
			PlatformOpenAI: {Prompt: "system", Mode: SystemPromptModeOverride, Source: SystemPromptSourceSystem},
		},
		UserScope: SystemPromptUserScope{Enabled: true, Mode: SystemPromptUserScopeWhitelist, UserIDs: []int64{42}},
	}}
	body := []byte(`{"model":"gpt-5","instructions":"client"}`)

	got, changed, err := applyResolvedSystemPromptToJSON(context.Background(), nil, body, PlatformOpenAI, PlatformOpenAI, settings)

	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(body), string(got))
}
```

- [ ] **步骤 5：运行运行时测试确认 GREEN**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt|TestApplyResolvedSystemPrompt' -count=1
go test ./internal/service -run 'TestApplyResolvedSystemPrompt|TestSettingService_.*SystemPrompt' -count=1
```

预期：PASS。

## 任务 4：前端多用户邮箱搜索组件

**文件：**
- 新建：`frontend/src/components/common/UserMultiSearchSelect.vue`
- 新建：`frontend/src/components/common/__tests__/UserMultiSearchSelect.spec.ts`

- [ ] **步骤 1：写失败的组件测试**

新建 `frontend/src/components/common/__tests__/UserMultiSearchSelect.spec.ts`：

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import UserMultiSearchSelect from '../UserMultiSearchSelect.vue'

const listMock = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: listMock
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('UserMultiSearchSelect', () => {
  beforeEach(() => {
    listMock.mockReset()
    document.body.innerHTML = ''
  })

  it('通过邮箱搜索选择用户并只输出用户 ID', async () => {
    listMock.mockResolvedValue({
      items: [{ id: 42, email: 'user@example.com', username: 'user' }]
    })
    const wrapper = mount(UserMultiSearchSelect, {
      props: { modelValue: [], placeholder: 'search' },
      attachTo: document.body
    })

    await wrapper.get('input').setValue('user@example.com')
    await new Promise(resolve => setTimeout(resolve, 350))
    await wrapper.find('[data-test="user-multi-search-option"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[42]])
  })

  it('移除已选择用户', async () => {
    listMock.mockResolvedValue({
      items: [{ id: 42, email: 'user@example.com', username: 'user' }]
    })
    const wrapper = mount(UserMultiSearchSelect, {
      props: {
        modelValue: [42],
        selectedUsers: [{ id: 42, email: 'user@example.com', username: 'user' }]
      },
      attachTo: document.body
    })

    await wrapper.find('[data-test="user-multi-remove"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[]])
  })
})
```

- [ ] **步骤 2：运行测试确认 RED**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/frontend
./node_modules/.bin/vitest run src/components/common/__tests__/UserMultiSearchSelect.spec.ts --config vitest.config.ts
```

预期：FAIL，因为组件不存在。

- [ ] **步骤 3：实现多选组件**

新建 `frontend/src/components/common/UserMultiSearchSelect.vue`，复用现有 `UserSearchSelect.vue` 的搜索逻辑，但组件值为 `number[]`：

```vue
<template>
  <div ref="containerRef" class="relative space-y-2">
    <div class="flex flex-wrap gap-2">
      <span
        v-for="user in displayUsers"
        :key="user.id"
        class="inline-flex max-w-full items-center gap-1 rounded border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-dark-100"
      >
        <span class="truncate">{{ user.email }}</span>
        <span class="text-gray-400">#{{ user.id }}</span>
        <button
          type="button"
          data-test="user-multi-remove"
          class="text-gray-400 hover:text-red-500"
          @click="removeUser(user.id)"
        >
          ×
        </button>
      </span>
    </div>

    <div class="input flex cursor-text items-center gap-1.5" :class="{ 'ring-2 ring-primary-500': isOpen }" @click="openDropdown">
      <input
        ref="inputRef"
        v-model="searchQuery"
        type="text"
        class="w-full border-0 bg-transparent p-0 text-sm outline-none placeholder:text-gray-400 focus:ring-0 dark:text-white dark:placeholder:text-dark-500"
        :placeholder="placeholder"
        @input="onInput"
        @focus="openDropdown"
        @keydown.escape="closeDropdown"
      />
    </div>

    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="dropdownRef"
        class="fixed z-[100000020] max-h-52 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
        :style="dropdownStyle"
      >
        <div v-if="loading" class="px-3 py-2 text-center text-xs text-gray-400">{{ t('common.loading') }}...</div>
        <div v-else-if="options.length === 0 && searchQuery.length > 0" class="px-3 py-2 text-center text-xs text-gray-400">{{ t('common.noResults') }}</div>
        <div v-else-if="options.length === 0" class="px-3 py-2 text-center text-xs text-gray-400">{{ t('common.typeToSearch') }}</div>
        <button
          v-for="user in options"
          :key="user.id"
          type="button"
          data-test="user-multi-search-option"
          class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700"
          :disabled="modelValue.includes(user.id)"
          @click="selectUser(user)"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</div>
            <div v-if="user.username" class="truncate text-xs text-gray-500 dark:text-dark-400">{{ user.username }}</div>
          </div>
          <span class="flex-shrink-0 text-xs text-gray-400 dark:text-dark-500">#{{ user.id }}</span>
        </button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'

type SelectedUser = { id: number; email: string; username?: string }

const props = withDefaults(defineProps<{
  modelValue: number[]
  selectedUsers?: SelectedUser[]
  placeholder?: string
}>(), {
  selectedUsers: () => [],
  placeholder: ''
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: number[]): void
  (event: 'update:selectedUsers', value: SelectedUser[]): void
}>()

const { t } = useI18n()
const containerRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const isOpen = ref(false)
const searchQuery = ref('')
const loading = ref(false)
const options = ref<AdminUser[]>([])
const localUsers = ref<SelectedUser[]>([...props.selectedUsers])
let searchTimeout: ReturnType<typeof setTimeout> | null = null
let abortController: AbortController | null = null

const displayUsers = computed(() => localUsers.value.filter(user => props.modelValue.includes(user.id)))

const dropdownStyle = computed(() => {
  if (!containerRef.value) return {}
  const rect = containerRef.value.getBoundingClientRect()
  return {
    top: `${rect.bottom + 4}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`
  }
})

watch(() => props.selectedUsers, users => {
  localUsers.value = [...users]
})

function openDropdown() {
  isOpen.value = true
  nextTick(() => inputRef.value?.focus())
}

function closeDropdown() {
  isOpen.value = false
  searchQuery.value = ''
  options.value = []
}

function onInput() {
  if (searchTimeout) clearTimeout(searchTimeout)
  searchTimeout = setTimeout(doSearch, 300)
}

async function doSearch() {
  const q = searchQuery.value.trim()
  if (!q) {
    options.value = []
    return
  }
  if (abortController) abortController.abort()
  abortController = new AbortController()
  loading.value = true
  try {
    const res = await adminAPI.users.list(1, 10, { search: q }, { signal: abortController.signal })
    options.value = res.items || []
  } catch (error: any) {
    if (error?.name !== 'AbortError') options.value = []
  } finally {
    loading.value = false
  }
}

function selectUser(user: AdminUser) {
  if (props.modelValue.includes(user.id)) return
  const selected = { id: user.id, email: user.email, username: user.username }
  const nextIDs = [...props.modelValue, user.id]
  const nextUsers = [...localUsers.value.filter(item => item.id !== user.id), selected]
  localUsers.value = nextUsers
  emit('update:modelValue', nextIDs)
  emit('update:selectedUsers', nextUsers)
  closeDropdown()
}

function removeUser(userID: number) {
  const nextIDs = props.modelValue.filter(id => id !== userID)
  const nextUsers = localUsers.value.filter(user => nextIDs.includes(user.id))
  localUsers.value = nextUsers
  emit('update:modelValue', nextIDs)
  emit('update:selectedUsers', nextUsers)
}

function handleClickOutside(event: MouseEvent) {
  if (!isOpen.value) return
  const target = event.target as Node
  if (containerRef.value?.contains(target)) return
  if (dropdownRef.value?.contains(target)) return
  closeDropdown()
}

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleClickOutside)
  if (searchTimeout) clearTimeout(searchTimeout)
  if (abortController) abortController.abort()
})
</script>
```

- [ ] **步骤 4：运行组件测试确认 GREEN**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/frontend
./node_modules/.bin/vitest run src/components/common/__tests__/UserMultiSearchSelect.spec.ts --config vitest.config.ts
```

预期：PASS。

## 任务 5：系统配置 UI 可视化接入

**文件：**
- 修改：`frontend/src/views/admin/SettingsView.vue`
- 修改：`frontend/src/views/admin/__tests__/settingsSystemPromptPlacement.spec.ts`
- 修改：`frontend/src/i18n/locales/zh.ts`
- 修改：`frontend/src/i18n/locales/en.ts`

- [ ] **步骤 1：写失败的布局测试**

在 `frontend/src/views/admin/__tests__/settingsSystemPromptPlacement.spec.ts` 中增加断言：

```ts
it('在平台系统提示词之前展示用户范围控制', () => {
  const source = readFileSync(resolve(__dirname, '../SettingsView.vue'), 'utf8')

  expect(source).toContain('data-test="system-prompt-user-scope"')
  expect(source.indexOf('data-test="system-prompt-user-scope"')).toBeLessThan(
    source.indexOf('data-test="system-prompt-platform-list"')
  )
  expect(source).toContain('system_prompt_user_scope_enabled')
  expect(source).toContain('system_prompt_user_scope_mode')
  expect(source).toContain('system_prompt_user_scope_user_ids')
})
```

- [ ] **步骤 2：运行测试确认 RED**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/frontend
./node_modules/.bin/vitest run src/views/admin/__tests__/settingsSystemPromptPlacement.spec.ts --config vitest.config.ts
```

预期：FAIL，因为 UI 尚未接入。

- [ ] **步骤 3：增加 i18n 文案**

在 `frontend/src/i18n/locales/zh.ts` 的 `admin.settings.systemPrompt` 下增加：

```ts
userScopeTitle: '用户范围控制',
userScopeDescription: '控制哪些用户允许应用 API Key、分组和系统配置中的系统提示词。通过后仍按优先级：API Key > 分组 > 系统配置。',
userScopeEnabled: '启用用户范围控制',
userScopeEnabledHint: '关闭时不限制用户，保持现有系统提示词行为。',
userScopeMode: '生效范围',
userScopeUsers: '用户列表',
userScopeUsersHint: '通过邮箱搜索选择用户；保存时仅存储用户 ID。',
userScopeSearchPlaceholder: '输入邮箱搜索用户',
userScopeModes: {
  all: '全量开启',
  whitelist: '白名单',
  blacklist: '黑名单'
},
userScopeModeHints: {
  all: '所有用户都允许应用系统提示词配置。',
  whitelist: '只有列表中的用户允许应用系统提示词配置。',
  blacklist: '列表中的用户不应用系统提示词配置，其他用户允许。'
}
```

在 `frontend/src/i18n/locales/en.ts` 对应增加英文文案：

```ts
userScopeTitle: 'User Scope Control',
userScopeDescription: 'Controls which users can receive API Key, Group, and System Settings system prompts. After this gate passes, priority remains: API Key > Group > System Settings.',
userScopeEnabled: 'Enable user scope control',
userScopeEnabledHint: 'When disabled, users are not restricted and current system prompt behavior is preserved.',
userScopeMode: 'Scope',
userScopeUsers: 'Users',
userScopeUsersHint: 'Search users by email. Only user IDs are stored when saving.',
userScopeSearchPlaceholder: 'Search users by email',
userScopeModes: {
  all: 'All users',
  whitelist: 'Whitelist',
  blacklist: 'Blacklist'
},
userScopeModeHints: {
  all: 'All users can receive system prompt configuration.',
  whitelist: 'Only selected users can receive system prompt configuration.',
  blacklist: 'Selected users do not receive system prompt configuration; others do.'
}
```

- [ ] **步骤 4：接入 SettingsView 表单状态**

在 `frontend/src/views/admin/SettingsView.vue` import 中增加：

```ts
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import UserMultiSearchSelect from '@/components/common/UserMultiSearchSelect.vue'
import type { AdminUser, SystemPromptUserScopeMode } from '@/types'
```

新增选项：

```ts
const systemPromptUserScopeModeOptions = computed(() => [
  { value: 'all' as SystemPromptUserScopeMode, label: t('admin.settings.systemPrompt.userScopeModes.all'), description: t('admin.settings.systemPrompt.userScopeModeHints.all') },
  { value: 'whitelist' as SystemPromptUserScopeMode, label: t('admin.settings.systemPrompt.userScopeModes.whitelist'), description: t('admin.settings.systemPrompt.userScopeModeHints.whitelist') },
  { value: 'blacklist' as SystemPromptUserScopeMode, label: t('admin.settings.systemPrompt.userScopeModes.blacklist'), description: t('admin.settings.systemPrompt.userScopeModeHints.blacklist') },
])

const systemPromptScopeSelectedUsers = ref<Array<Pick<AdminUser, 'id' | 'email' | 'username'>>>([])
```

在 `form` 默认值增加：

```ts
system_prompt_user_scope_enabled: false,
system_prompt_user_scope_mode: 'all',
system_prompt_user_scope_user_ids: [],
```

在保存 payload 中增加：

```ts
system_prompt_user_scope_enabled: form.system_prompt_user_scope_enabled,
system_prompt_user_scope_mode: form.system_prompt_user_scope_mode,
system_prompt_user_scope_user_ids: form.system_prompt_user_scope_user_ids,
```

在 `SettingsView` 的 settings 加载成功分支中，把返回值赋给 form 后调用以下函数，优先回显已保存用户的邮箱，接口失败时才展示 `#id`：

```ts
async function hydrateSystemPromptScopeUsers(userIDs: number[]) {
  const users = await Promise.allSettled(
    userIDs.map(id => adminAPI.users.getById(id))
  )
  systemPromptScopeSelectedUsers.value = users.map((result, index) => {
    if (result.status === 'fulfilled') {
      return {
        id: result.value.id,
        email: result.value.email,
        username: result.value.username
      }
    }
    const id = userIDs[index]
    return { id, email: `#${id}` }
  })
}
```

加载 settings 并写入 `form.system_prompt_user_scope_user_ids` 后调用 `hydrateSystemPromptScopeUsers(form.system_prompt_user_scope_user_ids || [])`，让已保存配置优先回显邮箱。失败时才降级显示 `#id`。

- [ ] **步骤 5：增加系统提示词页顶部 UI**

在平台列表上方加入：

```vue
<div data-test="system-prompt-user-scope" class="rounded border border-gray-200 p-4 dark:border-dark-700">
  <div class="mb-4">
    <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.settings.systemPrompt.userScopeTitle') }}
    </h3>
    <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.systemPrompt.userScopeDescription') }}
    </p>
  </div>

  <div class="space-y-4">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="font-medium text-gray-900 dark:text-white">
          {{ t('admin.settings.systemPrompt.userScopeEnabled') }}
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.systemPrompt.userScopeEnabledHint') }}
        </p>
      </div>
      <Toggle v-model="form.system_prompt_user_scope_enabled" />
    </div>

    <div v-if="form.system_prompt_user_scope_enabled" class="grid gap-4 md:grid-cols-[220px_1fr]">
      <div>
        <label class="input-label">{{ t('admin.settings.systemPrompt.userScopeMode') }}</label>
        <Select
          v-model="form.system_prompt_user_scope_mode"
          :options="systemPromptUserScopeModeOptions"
        >
          <template #option="{ option, selected }">
            <div class="flex min-w-0 flex-1 items-center gap-2">
              <span class="select-option-label">{{ (option as any).label }}</span>
              <Icon
                v-if="selected"
                name="check"
                size="sm"
                class="flex-shrink-0 text-primary-500"
                :stroke-width="2"
              />
              <HelpTooltip v-if="(option as any).description" :content="(option as any).description">
                <template #trigger>
                  <span
                    class="inline-flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
                    :title="(option as any).description"
                    @click.stop
                    @mousedown.stop
                  >
                    <Icon name="questionCircle" size="sm" :stroke-width="2" />
                  </span>
                </template>
              </HelpTooltip>
            </div>
          </template>
        </Select>
      </div>

      <div v-if="form.system_prompt_user_scope_mode !== 'all'">
        <label class="input-label">{{ t('admin.settings.systemPrompt.userScopeUsers') }}</label>
        <UserMultiSearchSelect
          v-model="form.system_prompt_user_scope_user_ids"
          v-model:selected-users="systemPromptScopeSelectedUsers"
          :placeholder="t('admin.settings.systemPrompt.userScopeSearchPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.settings.systemPrompt.userScopeUsersHint') }}</p>
      </div>
    </div>
  </div>
</div>

<div data-test="system-prompt-platform-list" class="space-y-5">
  <!-- 保留现有平台 v-for -->
</div>
```

- [ ] **步骤 6：运行前端相关测试确认 GREEN**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/frontend
./node_modules/.bin/vitest run src/views/admin/__tests__/settingsSystemPromptPlacement.spec.ts src/components/common/__tests__/UserMultiSearchSelect.spec.ts --config vitest.config.ts
```

预期：PASS。

## 任务 6：联调验证和收口

**文件：**
- 无新增业务文件；根据前面任务的实际修改做验证。

- [ ] **步骤 1：运行后端系统提示词相关测试**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test -tags unit ./internal/service -run 'TestResolveEffectiveSystemPrompt|TestApplySystemPrompt|TestSystemPromptUserScopeConstants' -count=1
go test ./internal/service -run 'TestSettingService_.*SystemPrompt|TestApplyResolvedSystemPrompt' -count=1
```

预期：PASS。

- [ ] **步骤 2：运行迁移 schema 测试**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/backend
go test ./internal/repository -run TestMigrationsSchemaIntegration -count=1
```

预期：PASS；如果本地没有数据库环境，记录未运行原因，不用改测试本身。

- [ ] **步骤 3：运行前端单测**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/frontend
./node_modules/.bin/vitest run src/views/admin/__tests__/settingsSystemPromptPlacement.spec.ts src/components/common/__tests__/UserMultiSearchSelect.spec.ts src/i18n/__tests__/staticLocaleCoverage.spec.ts --config vitest.config.ts
```

预期：PASS。

- [ ] **步骤 4：运行前端类型检查和构建**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode/frontend
./node_modules/.bin/vue-tsc --noEmit
./node_modules/.bin/vite build
```

预期：PASS；`vite build` 如果只出现既有 chunk/dynamic import 警告，可记录为非阻塞。

- [ ] **步骤 5：运行格式和工作区检查**

运行：

```bash
cd /Volumes/T7/project/new/FluxCode
git diff --check
git status --short
```

预期：`git diff --check` 无输出；`git status --short` 只包含本需求相关文件。

## 自查清单

- [ ] 用户范围控制关闭时，行为与现有系统提示词逻辑一致。
- [ ] 全量开启时，所有用户都可应用系统提示词。
- [ ] 白名单模式只允许列表中的用户，未携带 API Key 用户按不允许处理。
- [ ] 黑名单模式阻止列表中的用户，未携带 API Key 用户按允许处理。
- [ ] 门禁通过后，原优先级仍为 `API Key > 分组 > 系统配置`。
- [ ] settings 缓存仍是 7 天 TTL，`UpdateSettings` 成功后立即刷新。
- [ ] 前端通过邮箱搜索选择用户，保存 payload 只提交用户 ID 数组。
- [ ] 用户 ID 列表归一化去重、过滤非正数、稳定排序。
