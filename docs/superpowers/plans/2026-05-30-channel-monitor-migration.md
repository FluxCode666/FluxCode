# Channel Monitor Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 局部移植上游 `Wei-Shaw/sub2api` 的「渠道状态 / 渠道监控」闭环，默认关闭，保留当前仓库本地定制。

**Architecture:** 后端新增独立 Channel Monitor 领域：ent schema + migrations + repository + service/checker/runner + admin/user handlers；设置项 `channel_monitor_enabled=false` 作为展示和 runner 总门禁。前端只局部引入上游用户页、管理页、API、组件和格式化工具，路由与侧边栏按当前仓库结构手工接线，不整文件覆盖上游。

**Tech Stack:** Go 1.26、Gin、ent、PostgreSQL migrations、Wire、Vue 3、TypeScript、Pinia、Vue Router、Vitest、vue-tsc、Tailwind。

---

## 文件结构

### 后端新增文件

- 新建 `backend/ent/schema/channel_monitor.go`：监控项 ent schema，字段包含 provider、api_mode、endpoint、加密 key、模型、分组、模板快照和检测间隔。
- 新建 `backend/ent/schema/channel_monitor_history.go`：单次检测历史 ent schema。
- 新建 `backend/ent/schema/channel_monitor_daily_rollup.go`：按日聚合 ent schema。
- 新建 `backend/ent/schema/channel_monitor_request_template.go`：请求模板 ent schema。
- 新建 `backend/migrations/118_channel_monitors.sql`：创建 `channel_monitors`、`channel_monitor_histories`，写入 settings 默认值。
- 新建 `backend/migrations/119_channel_monitor_aggregation.sql`：创建 `channel_monitor_daily_rollups`、`channel_monitor_aggregation_watermark`。
- 新建 `backend/migrations/120_channel_monitor_request_templates.sql`：创建 `channel_monitor_request_templates`，补齐模板快照字段。
- 新建 `backend/migrations/121_seed_channel_monitor_templates.sql`：写入 OpenAI `chat_completions` 与 `responses` 默认模板。
- 新建 `backend/internal/repository/channel_monitor_repo.go`：监控项、历史、用户聚合、日聚合、聚合水位持久化。
- 新建 `backend/internal/repository/channel_monitor_template_repo.go`：请求模板 CRUD、关联监控查询、模板应用。
- 新建 `backend/internal/handler/dto/channel_monitor.go`：admin/user handler 的请求响应 DTO 与 mapper。
- 新建 `backend/internal/handler/admin/channel_monitor_handler.go`：管理端监控项 CRUD、立即检测、历史查询。
- 新建 `backend/internal/handler/admin/channel_monitor_template_handler.go`：管理端请求模板 CRUD、关联监控、应用模板。
- 新建 `backend/internal/handler/channel_monitor_user_handler.go`：用户端只读列表和详情，受功能开关门禁控制。
- 新建 `backend/internal/service/channel_monitor_const.go`：provider、状态、OpenAI api mode、body override mode 常量。
- 新建 `backend/internal/service/channel_monitor_types.go`：service 输入输出类型。
- 新建 `backend/internal/service/channel_monitor_template_types.go`：模板 service 输入输出类型。
- 新建 `backend/internal/service/channel_monitor_validate.go`：字段校验、interval 归一化、模型列表归一化。
- 新建 `backend/internal/service/channel_monitor_ssrf.go`：endpoint SSRF 校验。
- 新建 `backend/internal/service/channel_monitor_checker.go`：OpenAI/Anthropic/Gemini 探测请求与响应判断。
- 新建 `backend/internal/service/channel_monitor_challenge.go`：检测请求内容构造。
- 新建 `backend/internal/service/channel_monitor_service.go`：监控项主服务、加解密、RunCheck、用户视图聚合。
- 新建 `backend/internal/service/channel_monitor_runner.go`：后台周期 runner，关闭时不执行检测。
- 新建 `backend/internal/service/channel_monitor_aggregator.go`：历史数据日聚合。
- 新建 `backend/internal/service/channel_monitor_template_service.go`：模板 service。

### 后端修改文件

- 修改 `backend/internal/repository/wire.go`：注册 `NewChannelMonitorRepository`、`NewChannelMonitorRequestTemplateRepository`。
- 修改 `backend/internal/service/wire.go`：注册 `ProvideChannelMonitorService`、`ProvideChannelMonitorRunner`、`NewChannelMonitorRequestTemplateService`。
- 修改 `backend/internal/handler/handler.go`：`Handlers` 增加用户端 `ChannelMonitor`；`AdminHandlers` 增加 `ChannelMonitor` 与 `ChannelMonitorTemplate`。
- 修改 `backend/internal/handler/wire.go`：注入新增 handler。
- 修改 `backend/internal/server/routes/admin.go`：新增 `/admin/channel-monitors` 与 `/admin/channel-monitor-templates` 路由。
- 修改 `backend/internal/server/routes/user.go`：新增 `/channel-monitors` 用户只读路由。
- 修改 `backend/internal/service/domain_constants.go`：新增 settings key。
- 修改 `backend/internal/service/settings_view.go`：`SystemSettings` 与 `PublicSettings` 增加渠道监控字段。
- 修改 `backend/internal/service/setting_service.go`：读取、更新、校验、默认值和 public settings 映射。
- 修改 `backend/internal/handler/dto/settings.go`：admin/public settings DTO 增加字段。
- 修改 `backend/internal/handler/setting_handler.go`：public settings 返回渠道监控开关。
- 修改 `backend/internal/handler/admin/setting_handler.go`：admin settings 读写渠道监控开关和默认 interval。
- 修改 `backend/cmd/server/wire.go`：cleanup 注入并停止 `ChannelMonitorRunner`。
- 生成修改 `backend/cmd/server/wire_gen.go`：Wire 输出。
- 生成修改 `backend/ent/*` 与 `backend/ent/migrate/schema.go`：ent 输出。

### 后端测试文件

- 新建 `backend/internal/repository/migrations_channel_monitor_sql_test.go`：迁移文件存在、默认设置和关键表字段检查。
- 新建 `backend/internal/service/channel_monitor_validate_test.go`：provider、api mode、status、interval、模型列表校验。
- 新建 `backend/internal/service/channel_monitor_ssrf_test.go`：拒绝内网、loopback、本地地址和非法 scheme。
- 新建 `backend/internal/service/channel_monitor_checker_test.go`：OpenAI `chat_completions`、OpenAI `responses`、Anthropic、Gemini 状态判断。
- 新建 `backend/internal/service/channel_monitor_service_test.go`：CRUD、加密、脱敏、解密失败、默认 interval。
- 新建 `backend/internal/service/channel_monitor_runner_test.go`：默认关闭不调度，开启后执行，停用后取消。
- 新建 `backend/internal/service/channel_monitor_template_service_test.go`：模板 CRUD、关联监控、应用快照。
- 新建 `backend/internal/handler/channel_monitor_user_handler_test.go`：关闭时用户列表为空、详情 404；开启时返回聚合。
- 新建 `backend/internal/handler/admin/channel_monitor_handler_test.go`：管理端 CRUD、run、history。
- 新建 `backend/internal/server/routes/channel_monitor_routes_test.go`：路由注册回归。
- 修改 `backend/internal/service/setting_service_public_test.go`：public settings 默认关闭和开启返回。
- 修改 `backend/internal/service/setting_service_update_test.go`：admin settings 更新渠道监控字段。
- 修改 `backend/internal/service/domain_constants_test.go`：settings key 常量回归。

### 前端新增文件

- 新建 `frontend/src/api/channelMonitor.ts`：用户端只读 API。
- 新建 `frontend/src/api/admin/channelMonitor.ts`：管理端监控项 API 与类型。
- 新建 `frontend/src/api/admin/channelMonitorTemplate.ts`：管理端请求模板 API 与类型。
- 新建 `frontend/src/constants/channelMonitor.ts`：状态、provider、api mode、时间窗口常量。
- 新建 `frontend/src/composables/useChannelMonitorFormat.ts`：状态文案、延迟、可用率格式化。
- 新建 `frontend/src/views/user/ChannelStatusView.vue`：用户端「渠道状态」页。
- 新建 `frontend/src/views/admin/ChannelMonitorView.vue`：管理端「渠道监控」页。
- 新建 `frontend/src/components/user/monitor/MonitorAvailabilityRow.vue`。
- 新建 `frontend/src/components/user/monitor/MonitorCard.vue`。
- 新建 `frontend/src/components/user/monitor/MonitorCardGrid.vue`。
- 新建 `frontend/src/components/user/monitor/MonitorHero.vue`。
- 新建 `frontend/src/components/user/monitor/MonitorMetricPair.vue`。
- 新建 `frontend/src/components/user/monitor/MonitorTimeline.vue`。
- 新建 `frontend/src/components/user/monitor/ProviderIcon.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorActionsCell.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorAdvancedRequestConfig.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorFiltersBar.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorFormDialog.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorKeyPickerDialog.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorPrimaryModelCell.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorRunResultDialog.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorTemplateApplyPickerDialog.vue`。
- 新建 `frontend/src/components/admin/monitor/MonitorTemplateManagerDialog.vue`。

### 前端修改文件

- 修改 `frontend/src/api/admin/index.ts`：导出 `channelMonitor` 与 `channelMonitorTemplate`。
- 修改 `frontend/src/api/auth.ts`：public settings 类型补齐渠道监控开关。
- 修改 `frontend/src/types/index.ts`：补齐 `PublicSettings`、`SystemPrompt` 附近的 shared setting 类型字段。
- 修改 `frontend/src/stores/app.ts`：缓存 public settings 中的 `channel_monitor_enabled`。
- 修改 `frontend/src/stores/adminSettings.ts`：缓存 admin 侧 `channel_monitor_enabled`，默认 `false`。
- 修改 `frontend/src/router/index.ts`：新增 `/monitor`、`/admin/channels/monitor`，并增加 `requiresChannelMonitor` 门禁。
- 修改 `frontend/src/router/meta.d.ts`：补充 `requiresChannelMonitor?: boolean`。
- 修改 `frontend/src/components/layout/AppSidebar.vue`：默认隐藏「渠道状态」和「渠道监控」，开启后显示。
- 修改 `frontend/src/views/admin/SettingsView.vue`：系统设置页新增渠道监控开关和默认 interval 输入。
- 修改 `frontend/src/i18n/locales/zh.ts`：新增中文文案。
- 修改 `frontend/src/i18n/locales/en.ts`：新增英文文案。

### 前端测试文件

- 新建 `frontend/src/views/user/__tests__/ChannelStatusView.spec.ts`：空状态、卡片、详情窗口、自动刷新回归。
- 新建 `frontend/src/views/admin/__tests__/ChannelMonitorView.spec.ts`：创建/编辑/模板/立即检测弹窗回归。
- 新建 `frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts`：默认关闭隐藏菜单，开启显示菜单。
- 修改 `frontend/src/router/__tests__/admin-routes.spec.ts`：管理端路由注册。
- 修改 `frontend/src/router/__tests__/guards.spec.ts`：`requiresChannelMonitor` 关闭时重定向。
- 修改 `frontend/src/i18n/__tests__/navigationLocales.spec.ts`：导航文案覆盖。

## 上游引用清单

执行任务时只读取这些上游文件作为参考，不全量合并：

```bash
git show wei-shaw/main:backend/internal/service/channel_monitor_service.go
git show wei-shaw/main:backend/internal/service/channel_monitor_runner.go
git show wei-shaw/main:backend/internal/service/channel_monitor_checker.go
git show wei-shaw/main:backend/internal/repository/channel_monitor_repo.go
git show wei-shaw/main:backend/internal/handler/admin/channel_monitor_handler.go
git show wei-shaw/main:backend/internal/handler/channel_monitor_user_handler.go
git show wei-shaw/main:frontend/src/views/user/ChannelStatusView.vue
git show wei-shaw/main:frontend/src/views/admin/ChannelMonitorView.vue
git show wei-shaw/main:frontend/src/components/layout/AppSidebar.vue
```

`AppSidebar.vue`、`router/index.ts`、`wire.go`、settings 相关文件只能手工合并当前仓库需要的片段；这些文件不能整文件覆盖。

## Task 1: 后端 schema、迁移和 ent 生成

**Files:**
- Create: `backend/ent/schema/channel_monitor.go`
- Create: `backend/ent/schema/channel_monitor_history.go`
- Create: `backend/ent/schema/channel_monitor_daily_rollup.go`
- Create: `backend/ent/schema/channel_monitor_request_template.go`
- Create: `backend/migrations/118_channel_monitors.sql`
- Create: `backend/migrations/119_channel_monitor_aggregation.sql`
- Create: `backend/migrations/120_channel_monitor_request_templates.sql`
- Create: `backend/migrations/121_seed_channel_monitor_templates.sql`
- Create: `backend/internal/repository/migrations_channel_monitor_sql_test.go`
- Generate: `backend/ent/*`
- Generate: `backend/ent/migrate/schema.go`

- [ ] **Step 1: 写失败的迁移文件测试**

创建 `backend/internal/repository/migrations_channel_monitor_sql_test.go`：

```go
package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorMigrationsDefineTablesAndDefaults(t *testing.T) {
	cases := map[string][]string{
		"118_channel_monitors.sql": {
			"CREATE TABLE IF NOT EXISTS channel_monitors",
			"CREATE TABLE IF NOT EXISTS channel_monitor_histories",
			"channel_monitor_enabled",
			"channel_monitor_default_interval_seconds",
			"false",
			"60",
			"api_mode",
		},
		"119_channel_monitor_aggregation.sql": {
			"CREATE TABLE IF NOT EXISTS channel_monitor_daily_rollups",
			"CREATE TABLE IF NOT EXISTS channel_monitor_aggregation_watermark",
		},
		"120_channel_monitor_request_templates.sql": {
			"CREATE TABLE IF NOT EXISTS channel_monitor_request_templates",
			"body_override_mode",
			"extra_headers",
		},
		"121_seed_channel_monitor_templates.sql": {
			"channel_monitor_request_templates",
			"chat_completions",
			"responses",
			"ON CONFLICT",
		},
	}

	for filename, expectedFragments := range cases {
		t.Run(filename, func(t *testing.T) {
			content, err := migrations.FS.ReadFile(filename)
			require.NoError(t, err)
			sql := string(content)
			for _, fragment := range expectedFragments {
				require.Contains(t, sql, fragment)
			}
			require.NotContains(t, strings.ToLower(sql), "drop table")
		})
	}
}
```

- [ ] **Step 2: 运行迁移测试确认 RED**

Run:

```bash
cd backend
go test ./internal/repository -run TestChannelMonitorMigrationsDefineTablesAndDefaults -count=1
```

Expected: FAIL，提示 `118_channel_monitors.sql` 等迁移文件不存在。

- [ ] **Step 3: 新增 ent schema**

从上游读取 schema 后，用当前仓库风格创建四个 schema 文件；字段必须包含以下最小集合：

```go
// backend/ent/schema/channel_monitor.go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ChannelMonitor struct {
	ent.Schema
}

func (ChannelMonitor) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("provider").NotEmpty(),
		field.String("api_mode").Default("chat_completions"),
		field.String("endpoint").NotEmpty(),
		field.String("api_key_encrypted").Sensitive(),
		field.String("primary_model").NotEmpty(),
		field.JSON("extra_models", []string{}).Optional(),
		field.String("group_name").Default(""),
		field.Bool("enabled").Default(true),
		field.Int("interval_seconds").Default(60),
		field.Int64("template_id").Optional().Nillable(),
		field.JSON("extra_headers", map[string]string{}).Optional(),
		field.String("body_override_mode").Default("off"),
		field.JSON("body_override", map[string]any{}).Optional(),
		field.Int64("created_by").Default(0),
		field.Time("last_checked_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ChannelMonitor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider"),
		index.Fields("enabled"),
		index.Fields("template_id"),
	}
}
```

同一任务内补齐：

```go
// backend/ent/schema/channel_monitor_history.go
type ChannelMonitorHistory struct {
	ent.Schema
}

// Required fields:
// monitor_id int64, model string, status string, latency_ms nullable int,
// ping_latency_ms nullable int, message string, checked_at time.Time
```

```go
// backend/ent/schema/channel_monitor_daily_rollup.go
type ChannelMonitorDailyRollup struct {
	ent.Schema
}

// Required fields:
// monitor_id int64, model string, day time.Time, total_checks int,
// successful_checks int, degraded_checks int, failed_checks int,
// avg_latency_ms nullable float, created_at time.Time, updated_at time.Time
```

```go
// backend/ent/schema/channel_monitor_request_template.go
type ChannelMonitorRequestTemplate struct {
	ent.Schema
}

// Required fields:
// name string, provider string, api_mode string, description string,
// extra_headers JSON map[string]string, body_override_mode string,
// body_override JSON map[string]any, created_at time.Time, updated_at time.Time
```

- [ ] **Step 4: 新增迁移 SQL**

创建 `backend/migrations/118_channel_monitors.sql`，包含默认关闭设置：

```sql
CREATE TABLE IF NOT EXISTS channel_monitors (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    api_mode VARCHAR(32) NOT NULL DEFAULT 'chat_completions',
    endpoint TEXT NOT NULL,
    api_key_encrypted TEXT NOT NULL,
    primary_model VARCHAR(255) NOT NULL,
    extra_models JSONB NOT NULL DEFAULT '[]'::jsonb,
    group_name VARCHAR(255) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    interval_seconds INTEGER NOT NULL DEFAULT 60,
    template_id BIGINT NULL,
    extra_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    body_override_mode VARCHAR(32) NOT NULL DEFAULT 'off',
    body_override JSONB NULL,
    created_by BIGINT NOT NULL DEFAULT 0,
    last_checked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_monitors_provider ON channel_monitors(provider);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_enabled ON channel_monitors(enabled);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_template_id ON channel_monitors(template_id);

CREATE TABLE IF NOT EXISTS channel_monitor_histories (
    id BIGSERIAL PRIMARY KEY,
    monitor_id BIGINT NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE,
    model VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    latency_ms INTEGER NULL,
    ping_latency_ms INTEGER NULL,
    message TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_histories_monitor_checked_at
    ON channel_monitor_histories(monitor_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_histories_monitor_model_checked_at
    ON channel_monitor_histories(monitor_id, model, checked_at DESC);

INSERT INTO settings (key, value, created_at, updated_at)
VALUES
    ('channel_monitor_enabled', 'false', NOW(), NOW()),
    ('channel_monitor_default_interval_seconds', '60', NOW(), NOW())
ON CONFLICT (key) DO NOTHING;
```

创建 `119`、`120`、`121` 时保持 `IF NOT EXISTS` 和 `ON CONFLICT DO NOTHING`，并把上游 `138_channel_monitor_openai_api_mode.sql` 的 `api_mode` 设计直接合入新建表，不再新增单独编号迁移。

- [ ] **Step 5: 生成 ent 代码**

Run:

```bash
cd backend
go generate ./ent
```

Expected: PASS，生成 `backend/ent/channelmonitor*`、`backend/ent/channelmonitorhistory*`、`backend/ent/channelmonitordailyrollup*`、`backend/ent/channelmonitorrequesttemplate*`。

- [ ] **Step 6: 运行迁移测试确认 GREEN**

Run:

```bash
cd backend
go test ./internal/repository -run TestChannelMonitorMigrationsDefineTablesAndDefaults -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交数据层变更**

```bash
git add backend/ent backend/migrations backend/internal/repository/migrations_channel_monitor_sql_test.go
git commit -m "feat: add channel monitor schema and migrations"
```

## Task 2: 后端 repository、service、checker、runner、template

**Files:**
- Create: `backend/internal/repository/channel_monitor_repo.go`
- Create: `backend/internal/repository/channel_monitor_template_repo.go`
- Create: `backend/internal/service/channel_monitor_const.go`
- Create: `backend/internal/service/channel_monitor_types.go`
- Create: `backend/internal/service/channel_monitor_template_types.go`
- Create: `backend/internal/service/channel_monitor_validate.go`
- Create: `backend/internal/service/channel_monitor_ssrf.go`
- Create: `backend/internal/service/channel_monitor_checker.go`
- Create: `backend/internal/service/channel_monitor_challenge.go`
- Create: `backend/internal/service/channel_monitor_service.go`
- Create: `backend/internal/service/channel_monitor_runner.go`
- Create: `backend/internal/service/channel_monitor_aggregator.go`
- Create: `backend/internal/service/channel_monitor_template_service.go`
- Test: `backend/internal/service/channel_monitor_validate_test.go`
- Test: `backend/internal/service/channel_monitor_ssrf_test.go`
- Test: `backend/internal/service/channel_monitor_checker_test.go`
- Test: `backend/internal/service/channel_monitor_service_test.go`
- Test: `backend/internal/service/channel_monitor_runner_test.go`
- Test: `backend/internal/service/channel_monitor_template_service_test.go`

- [ ] **Step 1: 写常量和校验失败测试**

创建 `backend/internal/service/channel_monitor_validate_test.go`：

```go
package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorConstants(t *testing.T) {
	require.Equal(t, "openai", ChannelMonitorProviderOpenAI)
	require.Equal(t, "anthropic", ChannelMonitorProviderAnthropic)
	require.Equal(t, "gemini", ChannelMonitorProviderGemini)
	require.Equal(t, "chat_completions", ChannelMonitorAPIModeChatCompletions)
	require.Equal(t, "responses", ChannelMonitorAPIModeResponses)
	require.Equal(t, "operational", ChannelMonitorStatusOperational)
	require.Equal(t, "degraded", ChannelMonitorStatusDegraded)
	require.Equal(t, "failed", ChannelMonitorStatusFailed)
	require.Equal(t, "error", ChannelMonitorStatusError)
}

func TestNormalizeChannelMonitorInterval(t *testing.T) {
	require.Equal(t, 60, NormalizeChannelMonitorInterval(0, 60))
	require.Equal(t, 15, NormalizeChannelMonitorInterval(1, 60))
	require.Equal(t, 3600, NormalizeChannelMonitorInterval(7200, 60))
	require.Equal(t, 120, NormalizeChannelMonitorInterval(120, 60))
}
```

- [ ] **Step 2: 运行校验测试确认 RED**

Run:

```bash
cd backend
go test ./internal/service -run 'TestChannelMonitorConstants|TestNormalizeChannelMonitorInterval' -count=1
```

Expected: FAIL，提示常量和函数未定义。

- [ ] **Step 3: 新增常量和校验实现**

创建 `backend/internal/service/channel_monitor_const.go`：

```go
package service

const (
	ChannelMonitorProviderOpenAI    = "openai"
	ChannelMonitorProviderAnthropic = "anthropic"
	ChannelMonitorProviderGemini    = "gemini"

	ChannelMonitorAPIModeChatCompletions = "chat_completions"
	ChannelMonitorAPIModeResponses       = "responses"

	ChannelMonitorStatusOperational = "operational"
	ChannelMonitorStatusDegraded    = "degraded"
	ChannelMonitorStatusFailed      = "failed"
	ChannelMonitorStatusError       = "error"

	ChannelMonitorBodyOverrideOff     = "off"
	ChannelMonitorBodyOverrideMerge   = "merge"
	ChannelMonitorBodyOverrideReplace = "replace"

	ChannelMonitorMinIntervalSeconds     = 15
	ChannelMonitorMaxIntervalSeconds     = 3600
	ChannelMonitorFallbackIntervalSecond = 60
)
```

创建 `backend/internal/service/channel_monitor_validate.go`：

```go
package service

import "strings"

func NormalizeChannelMonitorInterval(value int, defaultValue int) int {
	if value <= 0 {
		value = defaultValue
	}
	if value < ChannelMonitorMinIntervalSeconds {
		return ChannelMonitorMinIntervalSeconds
	}
	if value > ChannelMonitorMaxIntervalSeconds {
		return ChannelMonitorMaxIntervalSeconds
	}
	return value
}

func NormalizeChannelMonitorModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}
```

- [ ] **Step 4: 写 SSRF 失败测试**

创建 `backend/internal/service/channel_monitor_ssrf_test.go`：

```go
package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateChannelMonitorEndpointRejectsPrivateTargets(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1:8080/v1/chat/completions",
		"http://localhost:8080/v1/chat/completions",
		"http://10.0.0.1/v1/chat/completions",
		"http://172.16.1.1/v1/chat/completions",
		"http://192.168.1.1/v1/chat/completions",
		"file:///etc/passwd",
	}
	for _, endpoint := range blocked {
		require.Error(t, ValidateChannelMonitorEndpoint(endpoint), endpoint)
	}
}

func TestValidateChannelMonitorEndpointAllowsPublicHTTPS(t *testing.T) {
	require.NoError(t, ValidateChannelMonitorEndpoint("https://api.openai.com/v1/chat/completions"))
	require.NoError(t, ValidateChannelMonitorEndpoint("https://api.anthropic.com/v1/messages"))
}
```

- [ ] **Step 5: 实现 SSRF 校验**

创建 `backend/internal/service/channel_monitor_ssrf.go`，核心行为如下：

```go
package service

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

func ValidateChannelMonitorEndpoint(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return errors.New("invalid endpoint")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("endpoint scheme must be http or https")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("local endpoint is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && isChannelMonitorPrivateIP(ip) {
		return errors.New("private endpoint is not allowed")
	}
	return nil
}

func isChannelMonitorPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
```

- [ ] **Step 6: 移植 repository 和 service 类型**

读取上游实现并按当前 ent 生成字段适配：

```bash
git show wei-shaw/main:backend/internal/repository/channel_monitor_repo.go
git show wei-shaw/main:backend/internal/repository/channel_monitor_template_repo.go
git show wei-shaw/main:backend/internal/service/channel_monitor_types.go
git show wei-shaw/main:backend/internal/service/channel_monitor_template_types.go
```

创建 service 类型时至少包含：

```go
type ChannelMonitorRepository interface {
	List(ctx context.Context, params ChannelMonitorListParams) ([]*ChannelMonitor, int, error)
	Get(ctx context.Context, id int64) (*ChannelMonitor, error)
	Create(ctx context.Context, input ChannelMonitorCreateInput) (*ChannelMonitor, error)
	Update(ctx context.Context, id int64, input ChannelMonitorUpdateInput) (*ChannelMonitor, error)
	Delete(ctx context.Context, id int64) error
	AppendHistory(ctx context.Context, monitorID int64, result ChannelMonitorCheckResult) error
	ListHistory(ctx context.Context, monitorID int64, params ChannelMonitorHistoryParams) ([]ChannelMonitorHistoryItem, error)
	ListUserViews(ctx context.Context) ([]ChannelMonitorUserView, error)
	GetUserStatus(ctx context.Context, id int64) (*ChannelMonitorUserStatus, error)
}
```

`api_key` 只在 service 输入中出现；repository 持久化字段必须使用 `api_key_encrypted`。

- [ ] **Step 7: 写 checker 响应判断测试**

创建 `backend/internal/service/channel_monitor_checker_test.go`：

```go
package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyChannelMonitorHTTPStatus(t *testing.T) {
	require.Equal(t, ChannelMonitorStatusOperational, classifyChannelMonitorHTTPStatus(http.StatusOK, nil))
	require.Equal(t, ChannelMonitorStatusDegraded, classifyChannelMonitorHTTPStatus(http.StatusTooManyRequests, nil))
	require.Equal(t, ChannelMonitorStatusFailed, classifyChannelMonitorHTTPStatus(http.StatusUnauthorized, nil))
	require.Equal(t, ChannelMonitorStatusError, classifyChannelMonitorHTTPStatus(http.StatusInternalServerError, nil))
}
```

- [ ] **Step 8: 移植 checker、challenge、service、runner、aggregator、template service**

读取上游文件并按当前仓库适配：

```bash
git show wei-shaw/main:backend/internal/service/channel_monitor_checker.go
git show wei-shaw/main:backend/internal/service/channel_monitor_challenge.go
git show wei-shaw/main:backend/internal/service/channel_monitor_service.go
git show wei-shaw/main:backend/internal/service/channel_monitor_runner.go
git show wei-shaw/main:backend/internal/service/channel_monitor_aggregator.go
git show wei-shaw/main:backend/internal/service/channel_monitor_template_service.go
```

`ChannelMonitorRunner` 必须在每次触发前读取设置开关：

```go
func (r *ChannelMonitorRunner) canRun(ctx context.Context) bool {
	if r == nil || r.settingService == nil {
		return false
	}
	settings, err := r.settingService.GetPublicSettings(ctx)
	if err != nil {
		return false
	}
	return settings.ChannelMonitorEnabled
}
```

`RunCheck` 中 API key 解密失败必须返回清晰错误并跳过网络请求：

```go
apiKey, err := s.encryptor.Decrypt(monitor.APIKeyEncrypted)
if err != nil {
	return nil, ErrChannelMonitorAPIKeyDecryptFailed
}
```

- [ ] **Step 9: 写 service 和 runner 回归测试**

创建 `backend/internal/service/channel_monitor_runner_test.go`：

```go
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorRunnerDefaultDisabledSkipsChecks(t *testing.T) {
	runner := NewChannelMonitorRunner(&ChannelMonitorService{}, &channelMonitorSettingStub{
		public: &PublicSettings{ChannelMonitorEnabled: false},
	})
	require.False(t, runner.canRun(context.Background()))
}

func TestChannelMonitorRunnerEnabledCanRun(t *testing.T) {
	runner := NewChannelMonitorRunner(&ChannelMonitorService{}, &channelMonitorSettingStub{
		public: &PublicSettings{ChannelMonitorEnabled: true},
	})
	require.True(t, runner.canRun(context.Background()))
}

type channelMonitorSettingStub struct {
	public *PublicSettings
}

func (s *channelMonitorSettingStub) GetPublicSettings(context.Context) (*PublicSettings, error) {
	return s.public, nil
}
```

如果 `SettingService` 不是接口，改为在 runner 中依赖一个小接口：

```go
type ChannelMonitorSettingsProvider interface {
	GetPublicSettings(ctx context.Context) (*PublicSettings, error)
}
```

- [ ] **Step 10: 运行 service 子集测试**

Run:

```bash
cd backend
go test ./internal/service -run 'ChannelMonitor|NormalizeChannelMonitor|ValidateChannelMonitorEndpoint' -count=1
```

Expected: PASS。

- [ ] **Step 11: 提交服务层变更**

```bash
git add backend/internal/repository/channel_monitor_repo.go backend/internal/repository/channel_monitor_template_repo.go backend/internal/service/channel_monitor*.go backend/internal/service/channel_monitor*_test.go
git commit -m "feat: add channel monitor services"
```

## Task 3: 后端 handler、routes、wire、settings 接线

**Files:**
- Create: `backend/internal/handler/dto/channel_monitor.go`
- Create: `backend/internal/handler/admin/channel_monitor_handler.go`
- Create: `backend/internal/handler/admin/channel_monitor_template_handler.go`
- Create: `backend/internal/handler/channel_monitor_user_handler.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/routes/user.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/service/settings_view.go`
- Modify: `backend/internal/service/setting_service.go`
- Modify: `backend/internal/handler/dto/settings.go`
- Modify: `backend/internal/handler/setting_handler.go`
- Modify: `backend/internal/handler/admin/setting_handler.go`
- Modify: `backend/cmd/server/wire.go`
- Generate: `backend/cmd/server/wire_gen.go`
- Test: `backend/internal/handler/channel_monitor_user_handler_test.go`
- Test: `backend/internal/handler/admin/channel_monitor_handler_test.go`
- Test: `backend/internal/server/routes/channel_monitor_routes_test.go`
- Test: `backend/internal/service/setting_service_public_test.go`
- Test: `backend/internal/service/setting_service_update_test.go`

- [ ] **Step 1: 写 settings 默认关闭测试**

在 `backend/internal/service/setting_service_public_test.go` 追加：

```go
func TestSettingServicePublicSettings_ChannelMonitorDefaultsDisabled(t *testing.T) {
	repo := &settingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	got, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, got.ChannelMonitorEnabled)
}
```

在 `backend/internal/service/setting_service_update_test.go` 追加：

```go
func TestSettingServiceUpdateSettings_ChannelMonitorFields(t *testing.T) {
	repo := &settingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	updated, err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ChannelMonitorEnabled:                true,
		ChannelMonitorDefaultIntervalSeconds: 120,
	})

	require.NoError(t, err)
	require.True(t, updated.ChannelMonitorEnabled)
	require.Equal(t, 120, updated.ChannelMonitorDefaultIntervalSeconds)
	require.Equal(t, "true", repo.updates[SettingKeyChannelMonitorEnabled])
	require.Equal(t, "120", repo.updates[SettingKeyChannelMonitorDefaultIntervalSeconds])
}
```

- [ ] **Step 2: 运行 settings 测试确认 RED**

Run:

```bash
cd backend
go test ./internal/service -run 'ChannelMonitor.*Settings|SettingService.*ChannelMonitor' -count=1
```

Expected: FAIL，提示 settings key 或字段未定义。

- [ ] **Step 3: 增加 settings key 和 DTO 字段**

在 `backend/internal/service/domain_constants.go` settings key 区增加：

```go
SettingKeyChannelMonitorEnabled                = "channel_monitor_enabled"
SettingKeyChannelMonitorDefaultIntervalSeconds = "channel_monitor_default_interval_seconds"
```

在 `backend/internal/service/settings_view.go` 的 `SystemSettings` 增加：

```go
ChannelMonitorEnabled                bool
ChannelMonitorDefaultIntervalSeconds int
```

在 `PublicSettings` 增加：

```go
ChannelMonitorEnabled bool
```

在 `backend/internal/handler/dto/settings.go` 的 admin DTO 增加：

```go
ChannelMonitorEnabled                bool `json:"channel_monitor_enabled"`
ChannelMonitorDefaultIntervalSeconds int  `json:"channel_monitor_default_interval_seconds"`
```

在 public DTO 增加：

```go
ChannelMonitorEnabled bool `json:"channel_monitor_enabled"`
```

- [ ] **Step 4: 修改 setting service 读写逻辑**

在 `backend/internal/service/setting_service.go` 的 settings 读取默认值中补齐：

```go
channelMonitorEnabled := parseBoolSetting(values[SettingKeyChannelMonitorEnabled], false)
channelMonitorDefaultInterval := NormalizeChannelMonitorInterval(
	parseIntSetting(values[SettingKeyChannelMonitorDefaultIntervalSeconds], ChannelMonitorFallbackIntervalSecond),
	ChannelMonitorFallbackIntervalSecond,
)
```

在 `UpdateSettings` 写入 map 中补齐：

```go
updates[SettingKeyChannelMonitorEnabled] = strconv.FormatBool(settings.ChannelMonitorEnabled)
updates[SettingKeyChannelMonitorDefaultIntervalSeconds] = strconv.Itoa(
	NormalizeChannelMonitorInterval(settings.ChannelMonitorDefaultIntervalSeconds, ChannelMonitorFallbackIntervalSecond),
)
```

在 `GetPublicSettings` 映射中只暴露开关：

```go
ChannelMonitorEnabled: systemSettings.ChannelMonitorEnabled,
```

- [ ] **Step 5: 移植 DTO 与 handlers**

读取上游文件：

```bash
git show wei-shaw/main:backend/internal/handler/dto/channel_monitor.go
git show wei-shaw/main:backend/internal/handler/admin/channel_monitor_handler.go
git show wei-shaw/main:backend/internal/handler/admin/channel_monitor_template_handler.go
git show wei-shaw/main:backend/internal/handler/channel_monitor_user_handler.go
```

用户端 handler 必须保留关闭门禁：

```go
func (h *ChannelMonitorUserHandler) List(c *gin.Context) {
	settings, err := h.settingService.GetPublicSettings(c.Request.Context())
	if err != nil || !settings.ChannelMonitorEnabled {
		c.JSON(http.StatusOK, gin.H{"items": []dto.UserMonitorView{}})
		return
	}
	// call service.ListUserViews
}

func (h *ChannelMonitorUserHandler) GetStatus(c *gin.Context) {
	settings, err := h.settingService.GetPublicSettings(c.Request.Context())
	if err != nil || !settings.ChannelMonitorEnabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel monitor disabled"})
		return
	}
	// call service.GetUserStatus
}
```

管理端 handler 不受 `channel_monitor_enabled` 限制。

- [ ] **Step 6: 接入 handler struct 和 routes**

在 `backend/internal/handler/handler.go` 增加字段：

```go
type AdminHandlers struct {
	// existing fields
	ChannelMonitor         *admin.ChannelMonitorHandler
	ChannelMonitorTemplate *admin.ChannelMonitorRequestTemplateHandler
}

type Handlers struct {
	// existing fields
	ChannelMonitor *ChannelMonitorUserHandler
}
```

在 `backend/internal/server/routes/admin.go` 添加注册函数：

```go
func registerChannelMonitorRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	monitors := admin.Group("/channel-monitors")
	{
		monitors.GET("", h.Admin.ChannelMonitor.List)
		monitors.POST("", h.Admin.ChannelMonitor.Create)
		monitors.GET("/:id", h.Admin.ChannelMonitor.Get)
		monitors.PUT("/:id", h.Admin.ChannelMonitor.Update)
		monitors.DELETE("/:id", h.Admin.ChannelMonitor.Delete)
		monitors.POST("/:id/run", h.Admin.ChannelMonitor.Run)
		monitors.GET("/:id/history", h.Admin.ChannelMonitor.History)
	}

	templates := admin.Group("/channel-monitor-templates")
	{
		templates.GET("", h.Admin.ChannelMonitorTemplate.List)
		templates.POST("", h.Admin.ChannelMonitorTemplate.Create)
		templates.GET("/:id", h.Admin.ChannelMonitorTemplate.Get)
		templates.PUT("/:id", h.Admin.ChannelMonitorTemplate.Update)
		templates.DELETE("/:id", h.Admin.ChannelMonitorTemplate.Delete)
		templates.GET("/:id/monitors", h.Admin.ChannelMonitorTemplate.AssociatedMonitors)
		templates.POST("/:id/apply", h.Admin.ChannelMonitorTemplate.Apply)
	}
}
```

在 `RegisterAdminRoutes` 的渠道管理附近调用：

```go
registerChannelMonitorRoutes(admin, h)
```

在 `backend/internal/server/routes/user.go` 认证路由内增加：

```go
monitors := authenticated.Group("/channel-monitors")
{
	monitors.GET("", h.ChannelMonitor.List)
	monitors.GET("/:id/status", h.ChannelMonitor.GetStatus)
}
```

- [ ] **Step 7: 接入 wire 和 cleanup**

在 `backend/internal/repository/wire.go` 注册：

```go
NewChannelMonitorRepository,
NewChannelMonitorRequestTemplateRepository,
```

在 `backend/internal/service/wire.go` 增加 provider：

```go
func ProvideChannelMonitorService(repo ChannelMonitorRepository, encryptor SecretEncryptor) *ChannelMonitorService {
	return NewChannelMonitorService(repo, encryptor)
}

func ProvideChannelMonitorRunner(svc *ChannelMonitorService, settingService *SettingService) *ChannelMonitorRunner {
	r := NewChannelMonitorRunner(svc, settingService)
	svc.SetScheduler(r)
	r.Start()
	return r
}
```

在 ProviderSet 增加：

```go
ProvideChannelMonitorService,
ProvideChannelMonitorRunner,
NewChannelMonitorRequestTemplateService,
```

在 `backend/cmd/server/wire.go` 的 `provideCleanup` 参数增加：

```go
channelMonitorRunner *service.ChannelMonitorRunner,
```

在 `parallelSteps` 增加：

```go
{"ChannelMonitorRunner", func() error {
	if channelMonitorRunner != nil {
		channelMonitorRunner.Stop()
	}
	return nil
}},
```

- [ ] **Step 8: 生成 Wire**

Run:

```bash
cd backend
go generate ./cmd/server
```

Expected: PASS，`backend/cmd/server/wire_gen.go` 包含 `NewChannelMonitorRepository`、`ProvideChannelMonitorRunner`、`NewChannelMonitorUserHandler`。

- [ ] **Step 9: 运行后端接线测试**

Run:

```bash
cd backend
go test ./internal/service ./internal/handler ./internal/server/routes -run 'ChannelMonitor|channel monitor|SettingService.*ChannelMonitor' -count=1
```

Expected: PASS。

- [ ] **Step 10: 提交后端接线**

```bash
git add backend/internal/handler backend/internal/server/routes backend/internal/repository/wire.go backend/internal/service/wire.go backend/internal/service/domain_constants.go backend/internal/service/settings_view.go backend/internal/service/setting_service.go backend/cmd/server/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat: wire channel monitor APIs"
```

## Task 4: 前端 API、类型、组件和页面移植

**Files:**
- Create: `frontend/src/api/channelMonitor.ts`
- Create: `frontend/src/api/admin/channelMonitor.ts`
- Create: `frontend/src/api/admin/channelMonitorTemplate.ts`
- Create: `frontend/src/constants/channelMonitor.ts`
- Create: `frontend/src/composables/useChannelMonitorFormat.ts`
- Create: `frontend/src/views/user/ChannelStatusView.vue`
- Create: `frontend/src/views/admin/ChannelMonitorView.vue`
- Create: `frontend/src/components/user/monitor/*.vue`
- Create: `frontend/src/components/admin/monitor/*.vue`
- Modify: `frontend/src/api/admin/index.ts`
- Modify: `frontend/src/api/auth.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/views/user/__tests__/ChannelStatusView.spec.ts`
- Test: `frontend/src/views/admin/__tests__/ChannelMonitorView.spec.ts`

- [ ] **Step 1: 写用户 API 类型测试**

创建 `frontend/src/views/user/__tests__/ChannelStatusView.spec.ts` 的首个用例：

```ts
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelStatusView from '../ChannelStatusView.vue'

vi.mock('@/api/channelMonitor', () => ({
  default: {
    list: vi.fn().mockResolvedValue({ items: [] }),
    status: vi.fn()
  }
}))

describe('ChannelStatusView', () => {
  it('renders empty state when no monitors exist', async () => {
    const wrapper = mount(ChannelStatusView, {
      global: {
        stubs: {
          MonitorHero: true,
          MonitorCardGrid: true
        }
      }
    })
    await vi.dynamicImportSettled()
    expect(wrapper.text()).toContain('渠道状态')
  })
})
```

- [ ] **Step 2: 运行前端测试确认 RED**

Run:

```bash
cd frontend
npm run test -- --run frontend/src/views/user/__tests__/ChannelStatusView.spec.ts
```

Expected: FAIL，提示 `ChannelStatusView.vue` 不存在。

- [ ] **Step 3: 移植 API 和常量文件**

读取并移植上游 API：

```bash
git show wei-shaw/main:frontend/src/api/channelMonitor.ts
git show wei-shaw/main:frontend/src/api/admin/channelMonitor.ts
git show wei-shaw/main:frontend/src/api/admin/channelMonitorTemplate.ts
git show wei-shaw/main:frontend/src/constants/channelMonitor.ts
git show wei-shaw/main:frontend/src/composables/useChannelMonitorFormat.ts
```

`frontend/src/api/admin/channelMonitor.ts` 必须保留这些类型：

```ts
export type Provider = 'openai' | 'anthropic' | 'gemini'
export type MonitorStatus = 'operational' | 'degraded' | 'failed' | 'error'
export type BodyOverrideMode = 'off' | 'merge' | 'replace'
export type APIMode = 'chat_completions' | 'responses'
```

在 `frontend/src/api/admin/index.ts` 导出：

```ts
import channelMonitor from './channelMonitor'
import channelMonitorTemplate from './channelMonitorTemplate'

export const adminAPI = {
  // existing APIs
  channelMonitor,
  channelMonitorTemplate,
}
```

- [ ] **Step 4: 补齐前端 public/admin settings 类型**

在 `frontend/src/types/index.ts` 的 `PublicSettings` 增加：

```ts
channel_monitor_enabled?: boolean
```

在 `frontend/src/api/admin/settings.ts` 的 `SystemSettings` 增加：

```ts
channel_monitor_enabled: boolean
channel_monitor_default_interval_seconds: number
```

在 `UpdateSettingsRequest` 增加：

```ts
channel_monitor_enabled?: boolean
channel_monitor_default_interval_seconds?: number
```

- [ ] **Step 5: 移植用户端页面和用户组件**

读取上游文件：

```bash
git show wei-shaw/main:frontend/src/views/user/ChannelStatusView.vue
git show wei-shaw/main:frontend/src/components/user/monitor/MonitorAvailabilityRow.vue
git show wei-shaw/main:frontend/src/components/user/monitor/MonitorCard.vue
git show wei-shaw/main:frontend/src/components/user/monitor/MonitorCardGrid.vue
git show wei-shaw/main:frontend/src/components/user/monitor/MonitorHero.vue
git show wei-shaw/main:frontend/src/components/user/monitor/MonitorMetricPair.vue
git show wei-shaw/main:frontend/src/components/user/monitor/MonitorTimeline.vue
git show wei-shaw/main:frontend/src/components/user/monitor/ProviderIcon.vue
```

落地后做当前仓库适配：

```ts
import channelMonitorUserAPI from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
```

页面不显示使用说明型文案；空状态使用当前 `EmptyState` 风格。

- [ ] **Step 6: 写管理页 smoke 测试**

创建 `frontend/src/views/admin/__tests__/ChannelMonitorView.spec.ts`：

```ts
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelMonitorView from '../ChannelMonitorView.vue'

vi.mock('@/api/admin/channelMonitor', () => ({
  default: {
    list: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 }),
    runNow: vi.fn()
  }
}))

vi.mock('@/api/admin/channelMonitorTemplate', () => ({
  default: {
    list: vi.fn().mockResolvedValue({ items: [] })
  }
}))

describe('ChannelMonitorView', () => {
  it('renders monitor management shell', async () => {
    const wrapper = mount(ChannelMonitorView, {
      global: {
        stubs: {
          MonitorFiltersBar: true,
          MonitorFormDialog: true,
          MonitorTemplateManagerDialog: true
        }
      }
    })
    await vi.dynamicImportSettled()
    expect(wrapper.text()).toContain('渠道监控')
  })
})
```

- [ ] **Step 7: 移植管理页和管理组件**

读取上游文件：

```bash
git show wei-shaw/main:frontend/src/views/admin/ChannelMonitorView.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorActionsCell.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorAdvancedRequestConfig.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorFiltersBar.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorFormDialog.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorKeyPickerDialog.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorPrimaryModelCell.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorRunResultDialog.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorTemplateApplyPickerDialog.vue
git show wei-shaw/main:frontend/src/components/admin/monitor/MonitorTemplateManagerDialog.vue
```

适配点：

```ts
import channelMonitorAPI from '@/api/admin/channelMonitor'
import channelMonitorTemplateAPI from '@/api/admin/channelMonitorTemplate'
```

如果上游引用的通用组件名称与当前仓库不同，优先使用当前已有组件：

```ts
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
```

- [ ] **Step 8: 增加 i18n 文案**

在 `frontend/src/i18n/locales/zh.ts` 增加：

```ts
channelMonitor: {
  userTitle: '渠道状态',
  adminTitle: '渠道监控',
  operational: '正常',
  degraded: '降级',
  failed: '失败',
  error: '异常',
  noData: '暂无渠道状态数据'
}
```

在 `frontend/src/i18n/locales/en.ts` 增加同名 key：

```ts
channelMonitor: {
  userTitle: 'Channel Status',
  adminTitle: 'Channel Monitor',
  operational: 'Operational',
  degraded: 'Degraded',
  failed: 'Failed',
  error: 'Error',
  noData: 'No channel status data'
}
```

- [ ] **Step 9: 运行页面测试和类型检查子集**

Run:

```bash
cd frontend
npm run test -- --run frontend/src/views/user/__tests__/ChannelStatusView.spec.ts frontend/src/views/admin/__tests__/ChannelMonitorView.spec.ts
npm run type-check
```

Expected: PASS。

- [ ] **Step 10: 提交前端页面移植**

```bash
git add frontend/src/api/channelMonitor.ts frontend/src/api/admin/channelMonitor.ts frontend/src/api/admin/channelMonitorTemplate.ts frontend/src/api/admin/index.ts frontend/src/constants/channelMonitor.ts frontend/src/composables/useChannelMonitorFormat.ts frontend/src/views/user/ChannelStatusView.vue frontend/src/views/admin/ChannelMonitorView.vue frontend/src/components/user/monitor frontend/src/components/admin/monitor frontend/src/views/user/__tests__/ChannelStatusView.spec.ts frontend/src/views/admin/__tests__/ChannelMonitorView.spec.ts frontend/src/types/index.ts frontend/src/api/auth.ts frontend/src/api/admin/settings.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: add channel monitor frontend views"
```

## Task 5: 前端路由、侧边栏、设置页开关

**Files:**
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/router/meta.d.ts`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Modify: `frontend/src/stores/app.ts`
- Modify: `frontend/src/stores/adminSettings.ts`
- Modify: `frontend/src/views/admin/SettingsView.vue`
- Test: `frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts`
- Test: `frontend/src/router/__tests__/admin-routes.spec.ts`
- Test: `frontend/src/router/__tests__/guards.spec.ts`
- Test: `frontend/src/i18n/__tests__/navigationLocales.spec.ts`

- [ ] **Step 1: 写侧边栏默认隐藏测试**

创建 `frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import AppSidebar from '../AppSidebar.vue'
import { useAppStore, useAuthStore, useAdminSettingsStore } from '@/stores'

describe('AppSidebar channel monitor menu', () => {
  it('hides channel monitor entries by default', () => {
    const wrapper = mount(AppSidebar, {
      global: {
        plugins: [createTestingPinia({ stubActions: false })],
        stubs: { RouterLink: true, VersionBadge: true }
      }
    })
    const appStore = useAppStore()
    const authStore = useAuthStore()
    const adminSettingsStore = useAdminSettingsStore()
    appStore.cachedPublicSettings = { channel_monitor_enabled: false } as any
    authStore.user = { role: 'admin' } as any
    adminSettingsStore.channelMonitorEnabled = false

    expect(wrapper.text()).not.toContain('渠道状态')
    expect(wrapper.text()).not.toContain('渠道监控')
  })
})
```

如果当前测试环境没有 `@pinia/testing`，改用已有 `AppSidebar.spec.ts` 的 store 初始化方式，断言保持不变。

- [ ] **Step 2: 运行侧边栏测试确认 RED**

Run:

```bash
cd frontend
npm run test -- --run frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts
```

Expected: FAIL，提示 `channelMonitorEnabled` 字段未定义或菜单逻辑不存在。

- [ ] **Step 3: 更新 app/admin settings store**

在 `frontend/src/stores/adminSettings.ts` 增加默认关闭缓存：

```ts
const channelMonitorEnabled = ref(readCachedBool('channel_monitor_enabled_cached', false))
```

在 `fetch` 成功后写入：

```ts
channelMonitorEnabled.value = settings.channel_monitor_enabled ?? false
writeCachedBool('channel_monitor_enabled_cached', channelMonitorEnabled.value)
```

在 return 中导出：

```ts
channelMonitorEnabled,
```

在 `frontend/src/stores/app.ts` 增加 computed：

```ts
const channelMonitorEnabled = computed(() => cachedPublicSettings.value?.channel_monitor_enabled ?? false)
```

并在 return 中导出：

```ts
channelMonitorEnabled,
```

- [ ] **Step 4: 更新路由 meta 和 guard**

在 `frontend/src/router/meta.d.ts` 增加：

```ts
requiresChannelMonitor?: boolean
```

在 `frontend/src/router/index.ts` 用户路由中加入：

```ts
{
  path: '/monitor',
  name: 'ChannelStatus',
  component: () => import('@/views/user/ChannelStatusView.vue'),
  meta: {
    requiresAuth: true,
    requiresAdmin: false,
    requiresChannelMonitor: true,
    title: 'Channel Status',
    titleKey: 'channelMonitor.userTitle'
  }
}
```

在 admin 路由中加入：

```ts
{
  path: '/admin/channels/monitor',
  name: 'AdminChannelMonitor',
  component: () => import('@/views/admin/ChannelMonitorView.vue'),
  meta: {
    requiresAuth: true,
    requiresAdmin: true,
    requiresChannelMonitor: true,
    title: 'Channel Monitor',
    titleKey: 'channelMonitor.adminTitle'
  }
}
```

在 `beforeEach` 中 payment/referral 门禁附近增加：

```ts
if (to.meta.requiresChannelMonitor) {
  const enabled = appStore.cachedPublicSettings?.channel_monitor_enabled ?? false
  if (!enabled) {
    next(authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
    return
  }
}
```

- [ ] **Step 5: 更新侧边栏菜单**

在 `frontend/src/components/layout/AppSidebar.vue` 增加用户菜单项：

```ts
...(appStore.channelMonitorEnabled
  ? [{ path: '/monitor', label: t('nav.channelStatus', '渠道状态'), icon: ChannelIcon, hideInSimpleMode: true }]
  : []),
```

在 admin 菜单中把当前单项 `'/admin/channels'` 改为 children 结构：

```ts
{
  path: '/admin/channels',
  label: t('nav.channels', '渠道管理'),
  icon: ChannelIcon,
  hideInSimpleMode: true,
  children: [
    { path: '/admin/channels', label: t('nav.channelManagement', '渠道管理'), icon: ChannelIcon },
    ...(adminSettingsStore.channelMonitorEnabled
      ? [{ path: '/admin/channels/monitor', label: t('nav.channelMonitor', '渠道监控'), icon: ChartIcon }]
      : []),
  ],
},
```

当前仓库的侧边栏不使用上游 `featureFlags.ts`；不要引入该文件。

- [ ] **Step 6: 更新设置页开关**

在 `frontend/src/views/admin/SettingsView.vue` 的功能开关区域增加：

```vue
<div class="settings-row">
  <div>
    <h3>{{ t('channelMonitor.adminTitle') }}</h3>
    <p>{{ t('settings.channelMonitorDescription') }}</p>
  </div>
  <Toggle v-model="form.channel_monitor_enabled" />
</div>
<div v-if="form.channel_monitor_enabled" class="settings-row">
  <label>{{ t('settings.channelMonitorDefaultInterval') }}</label>
  <Input
    v-model.number="form.channel_monitor_default_interval_seconds"
    type="number"
    min="15"
    max="3600"
  />
</div>
```

保存前归一化：

```ts
form.channel_monitor_default_interval_seconds = Math.min(
  3600,
  Math.max(15, Number(form.channel_monitor_default_interval_seconds || 60))
)
```

- [ ] **Step 7: 更新导航文案**

在 `frontend/src/i18n/locales/zh.ts` 的 `nav` 增加：

```ts
channelStatus: '渠道状态',
channelMonitor: '渠道监控',
channelManagement: '渠道管理',
```

在 `frontend/src/i18n/locales/en.ts` 的 `nav` 增加：

```ts
channelStatus: 'Channel Status',
channelMonitor: 'Channel Monitor',
channelManagement: 'Channels',
```

在 settings 文案中增加：

```ts
channelMonitorDescription: '配置渠道健康检测与用户端渠道状态展示',
channelMonitorDefaultInterval: '默认检测间隔（秒）',
```

- [ ] **Step 8: 运行路由、菜单、i18n 测试**

Run:

```bash
cd frontend
npm run test -- --run frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts frontend/src/router/__tests__/admin-routes.spec.ts frontend/src/router/__tests__/guards.spec.ts frontend/src/i18n/__tests__/navigationLocales.spec.ts
npm run type-check
```

Expected: PASS。

- [ ] **Step 9: 提交前端接线**

```bash
git add frontend/src/router/index.ts frontend/src/router/meta.d.ts frontend/src/components/layout/AppSidebar.vue frontend/src/stores/app.ts frontend/src/stores/adminSettings.ts frontend/src/views/admin/SettingsView.vue frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts frontend/src/router/__tests__/admin-routes.spec.ts frontend/src/router/__tests__/guards.spec.ts frontend/src/i18n/__tests__/navigationLocales.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: gate channel monitor navigation"
```

## Task 6: 全量验证、diff 审查和收口

**Files:**
- Inspect: `git diff main...HEAD --stat`
- Inspect: `git diff main...HEAD -- backend frontend docs`

- [ ] **Step 1: 运行后端生成检查**

Run:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
git diff --exit-code -- ent cmd/server/wire_gen.go
```

Expected: PASS；如果 `git diff --exit-code` 有输出，提交遗漏的生成文件。

- [ ] **Step 2: 运行后端目标测试**

Run:

```bash
cd backend
go test ./internal/repository ./internal/service ./internal/handler ./internal/server/routes -run 'ChannelMonitor|channel monitor|SettingService.*ChannelMonitor|TestChannelMonitorMigrationsDefineTablesAndDefaults' -count=1
```

Expected: PASS。

- [ ] **Step 3: 运行前端目标测试和类型检查**

Run:

```bash
cd frontend
npm run test -- --run frontend/src/views/user/__tests__/ChannelStatusView.spec.ts frontend/src/views/admin/__tests__/ChannelMonitorView.spec.ts frontend/src/components/layout/__tests__/AppSidebar.channelMonitor.spec.ts frontend/src/router/__tests__/admin-routes.spec.ts frontend/src/router/__tests__/guards.spec.ts frontend/src/i18n/__tests__/navigationLocales.spec.ts
npm run type-check
```

Expected: PASS。

- [ ] **Step 4: 扫描未完成标记和上游无关引入**

Run:

```bash
rg -n 'TB[D]|TO[D]O|待[定]|占[位]' backend/internal backend/ent/schema backend/migrations frontend/src docs/superpowers/plans/2026-05-30-channel-monitor-migration.md
git diff --name-only main...HEAD | rg -v 'channel_monitor|ChannelMonitor|ChannelStatus|monitor|settings|setting|routes|wire|AppSidebar|router|i18n|types|docs/superpowers|migrations|ent|api/auth|api/admin/index'
```

Expected: 第一条命令无输出；第二条命令无输出，或只出现已确认的必要接线文件。

- [ ] **Step 5: 审查默认关闭行为**

Run:

```bash
rg -n 'channel_monitor_enabled|ChannelMonitorEnabled|channelMonitorEnabled|requiresChannelMonitor' backend frontend
```

Expected:

- `backend/migrations/118_channel_monitors.sql` 默认写入 `'false'`。
- `backend/internal/handler/channel_monitor_user_handler.go` 关闭时列表返回空、详情返回 404。
- `backend/internal/service/channel_monitor_runner.go` 关闭时不执行检测。
- `frontend/src/components/layout/AppSidebar.vue` 默认关闭时隐藏用户和管理菜单。
- `frontend/src/router/index.ts` 关闭时拦截 `/monitor` 与 `/admin/channels/monitor`。

- [ ] **Step 6: 最终提交验证修正**

如果 Step 1 到 Step 5 产生修正：

```bash
git add backend frontend docs/superpowers/plans/2026-05-30-channel-monitor-migration.md
git commit -m "test: verify channel monitor migration"
```

如果没有修正：

```bash
git status --short
```

Expected: 工作区干净。

## 自检记录

- Spec coverage：本计划覆盖用户端「渠道状态」页、管理端「渠道监控」页、后台 runner、历史聚合、模板、settings 开关、默认关闭、SSRF、API key 密文落库、路由菜单和测试验证。
- Placeholder scan：保存后运行 `rg -n 'TB[D]|TO[D]O|待[定]|占[位]' docs/superpowers/plans/2026-05-30-channel-monitor-migration.md`，期望无输出。
- Type consistency：后端统一使用 `ChannelMonitorEnabled` / `channel_monitor_enabled`；默认 interval 统一为 `ChannelMonitorDefaultIntervalSeconds` / `channel_monitor_default_interval_seconds`；前端门禁统一为 `requiresChannelMonitor`。
