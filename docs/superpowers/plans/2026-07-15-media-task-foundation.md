# 媒体任务基础设施实施计划

> **供代理执行者：** 必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，逐任务执行本计划。所有步骤使用复选框（`- [ ]`）跟踪。

**Goal:** 建立不承载现有生产图片流量的统一媒体任务基础设施，包括持久化状态机、模型注册、账号与分组媒体配置、可视化系统设置、Redis 队列、Worker、Fake Adapter 和 API 契约测试。

**Architecture:** 数据库保存任务最终状态，Redis Streams 负责同步高优先级和异步普通优先级投递，Worker 通过租约和 CAS 执行可恢复任务。模型注册负责能力校验，分组绑定账号形成候选集合，媒体调度器复用现有账号优先级、负载、冷却和并发槽位逻辑；本阶段仅使用 Fake Adapter 验证同步与原生异步执行，不切换 `/v1/images/*` 生产路由。

**Tech Stack:** Go 1.26、Gin、Ent、PostgreSQL、Redis Streams、Google Wire、Vue 3、TypeScript、Pinia、Vitest、Vue Test Utils、pnpm。

---

## 范围约束

- 设计依据：`docs/superpowers/specs/2026-07-15-unified-media-task-architecture-design.md`。
- 本计划只交付设计中的“媒体任务基础设施”子项目。
- 不注册新的生产图片或视频创建路由；Handler 通过独立 Gin 测试路由验证契约。
- 不实现 OpenAI、xAI、Gemini、Vertex 或其他真实 Media Adapter。
- 不修改 OpenAI Chat/Responses、Anthropic Messages 或 Gemini 文本链路。
- 不提供用户或管理员任务取消接口。
- API 契约以同一创建接口的 `async=true` 选择异步，省略或 false 选择同步。
- 领域类型、Registry 和小粒度 Adapter 不硬编码供应商，必须能承载后续 Grok、Nano Banana、Z-Image、Seedance、Veo 和 Agens Video 接入。

## 文件结构

### 后端领域与服务

- Create: `backend/internal/service/media_types.go` — 媒体类型、操作、任务状态、执行阶段、规格和原生异步模式。
- Create: `backend/internal/service/media_task.go` — 任务、产物、账单快照、仓储接口和状态服务。
- Create: `backend/internal/service/media_model_registry.go` — 模型定义、缓存注册表和能力验证。
- Create: `backend/internal/service/media_account_config.go` — 账号 `media_config` 解析、默认值和模型级覆盖。
- Create: `backend/internal/service/media_adapter.go` — 小粒度 Adapter 接口、注册表和执行结果。
- Create: `backend/internal/service/media_fake_adapter.go` — 仅供契约和集成测试显式构造的确定性 Fake Adapter，不注入生产容器。
- Create: `backend/internal/service/account_candidate_selector.go` — 从既有调度链路抽出的候选排序、负载、粘性与并发槽位选择器。
- Create: `backend/internal/service/media_scheduler.go` — 跨平台媒体候选过滤和账号选择。
- Create: `backend/internal/service/media_queue.go` — 队列与终态通知接口。
- Create: `backend/internal/service/media_worker.go` — Worker、租约续期、恢复和终态推进。
- Create: `backend/internal/service/media_metrics.go` — 队列、阶段耗时、恢复、重复消息、存储和结算指标端口。
- Create: `backend/internal/service/media_billing.go` — 媒体预扣与结算端口；本阶段提供禁用实现和测试替身。
- Create: `backend/internal/service/media_orchestrator.go` — 创建任务、入队、同步等待和超时决策。
- Create: `backend/internal/service/media_content.go` — 视频内容归属校验、对象存储/安全代理选择和 Range 元数据。
- Modify: `backend/internal/service/account.go` — 暴露规范化账号媒体配置。
- Modify: `backend/internal/service/account_service.go` — 跨平台媒体分组绑定校验。
- Modify: `backend/internal/service/group.go` — 图片、视频和媒体跨平台字段。
- Modify: `backend/internal/service/settings_view.go` — 媒体系统设置 DTO。
- Modify: `backend/internal/service/setting_service.go` — 默认值、校验、保存和解析。
- Modify: `backend/internal/service/domain_constants.go` — 媒体设置键。
- Modify: `backend/internal/service/wire.go` — 服务构造与 Worker 生命周期。

### 后端持久化与基础设施

- Create: `backend/ent/schema/media_task.go` — 任务 Ent Schema。
- Create: `backend/ent/schema/media_artifact.go` — 产物元数据 Ent Schema。
- Create: `backend/ent/schema/media_model_definition.go` — 模型定义 Ent Schema。
- Modify: `backend/ent/schema/group.go` — 视频权限和媒体跨平台字段。
- Create: `backend/migrations/128_media_task_foundation.sql` — 三张表、分组字段和索引。
- Create: `backend/migrations/media_task_foundation_migration_test.go` — 迁移契约测试。
- Create: `backend/internal/repository/media_task_repo.go` — 任务与产物 Ent 仓储、CAS 和租约。
- Create: `backend/internal/repository/media_model_repo.go` — 模型定义仓储。
- Create: `backend/internal/repository/media_task_stream.go` — 双优先级 Redis Streams 队列和 Pub/Sub 通知。
- Create: `backend/internal/repository/media_http_content.go` — 带 SSRF、重定向、超时、大小和 Range 防护的媒体 HTTP 读取器。
- Modify: `backend/internal/repository/wire.go` — 仓储和队列 Provider。
- Generated: `backend/ent/` — `go generate ./ent` 生成的 MediaTask、MediaArtifact、MediaModelDefinition 客户端代码。

### 后端 API 契约

- Create: `backend/internal/handler/media_task_handler.go` — 创建、查询和视频内容响应；本阶段不进入生产路由。
- Create: `backend/internal/handler/media_task_handler_test.go` — 独立 Gin 路由契约测试。
- Modify: `backend/internal/handler/wire.go` — 构造 Handler，但不在 `backend/internal/server/routes/gateway.go` 挂载。
- Modify: `backend/internal/handler/handler.go` — 在顶层聚合结构中持有 Handler，供下一阶段挂路由。
- Modify: `backend/internal/handler/admin/setting_handler.go` — 媒体设置请求、响应和保存映射。
- Modify: `backend/internal/handler/dto/types.go` — 媒体设置响应字段。

### 后端配置与启动

- Modify: `backend/internal/config/config.go` — Worker、租约、轮询和恢复配置。
- Modify: `deploy/config.example.yaml` — 部署参数示例。
- Modify: `backend/cmd/server/wire.go` — Worker 清理生命周期依赖。
- Modify: `backend/cmd/server/wire_gen.go` — 由 `go generate ./cmd/server` 更新。

### 前端

- Create: `frontend/src/components/admin/settings/MediaGenerationSettingsCard.vue` — 系统设置可视化卡片；新建当前不存在的 `settings` 子目录。
- Create: `frontend/src/components/admin/settings/__tests__/MediaGenerationSettingsCard.spec.ts` — 设置行为测试。
- Create: `frontend/src/components/account/MediaConfigEditor.vue` — 账号 Adapter、默认原生异步模式和模型覆盖编辑器。
- Create: `frontend/src/components/account/__tests__/MediaConfigEditor.spec.ts` — 账号配置行为测试。
- Create: `frontend/src/components/admin/group/GroupMediaSettings.vue` — 图片、视频与跨平台媒体开关。
- Create: `frontend/src/components/admin/group/__tests__/GroupMediaSettings.spec.ts` — 分组媒体配置测试。
- Modify: `frontend/src/views/admin/SettingsView.vue` — 新增媒体设置页签并保存字段。
- Modify: `frontend/src/components/account/CreateAccountModal.vue` — 写入 `extra.media_config`。
- Modify: `frontend/src/components/account/EditAccountModal.vue` — 读取和更新 `extra.media_config`。
- Modify: `frontend/src/views/admin/GroupsView.vue` — 创建和编辑分组时接入媒体配置组件。
- Modify: `frontend/src/components/common/GroupSelector.vue` — 账号可选择显式开启跨平台媒体的异平台分组。
- Modify: `frontend/src/api/admin/settings.ts` — 媒体设置类型。
- Modify: `frontend/src/types/index.ts` — 账号媒体配置、分组字段和请求类型。
- Modify: `frontend/src/i18n/locales/zh.ts` — 设置、账号和分组媒体配置中文文案。
- Modify: `frontend/src/i18n/locales/en.ts` — 设置、账号和分组媒体配置英文文案。

---

### Task 1: 定义媒体领域类型和状态转换

**Files:**
- Create: `backend/internal/service/media_types.go`
- Test: `backend/internal/service/media_types_test.go`

- [ ] **Step 1: 写状态转换和异步模式的失败测试**

```go
func TestMediaTaskStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		from MediaTaskStatus
		to   MediaTaskStatus
		want bool
	}{
		{MediaTaskStatusQueued, MediaTaskStatusInProgress, true},
		{MediaTaskStatusInProgress, MediaTaskStatusCompleted, true},
		{MediaTaskStatusInProgress, MediaTaskStatusFailed, true},
		{MediaTaskStatusCompleted, MediaTaskStatusInProgress, false},
		{MediaTaskStatusFailed, MediaTaskStatusQueued, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.from.CanTransitionTo(tt.to))
	}
}

func TestNormalizeNativeAsyncMode(t *testing.T) {
	require.Equal(t, NativeAsyncUnsupported, NormalizeNativeAsyncMode(""))
	require.Equal(t, NativeAsyncOptional, NormalizeNativeAsyncMode("OPTIONAL"))
	require.Equal(t, NativeAsyncRequired, NormalizeNativeAsyncMode(" required "))
	require.Equal(t, NativeAsyncUnsupported, NormalizeNativeAsyncMode("invalid"))
}

func TestMediaSpecValidateRequiresMatchingExclusiveSpec(t *testing.T) {
	validImage := &ImageSpec{Prompt: "cat", Count: 1}
	validVideo := &VideoSpec{Prompt: "sunset"}
	tests := []struct {
		name      string
		mediaType MediaType
		spec      MediaSpec
		wantErr   bool
	}{
		{"image", MediaTypeImage, MediaSpec{Image: validImage}, false},
		{"video", MediaTypeVideo, MediaSpec{Video: validVideo}, false},
		{"both", MediaTypeImage, MediaSpec{Image: validImage, Video: validVideo}, true},
		{"wrong_type", MediaTypeVideo, MediaSpec{Image: validImage}, true},
		{"empty_prompt", MediaTypeImage, MediaSpec{Image: &ImageSpec{Count: 1}}, true},
		{"zero_count", MediaTypeImage, MediaSpec{Image: &ImageSpec{Prompt: "cat"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate(tt.mediaType)
			require.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestMediaTaskStageCanTransitionTo(t *testing.T) {
	require.True(t, MediaTaskStageQueued.CanTransitionTo(MediaTaskStageScheduling))
	require.True(t, MediaTaskStageScheduling.CanTransitionTo(MediaTaskStageSubmitting))
	require.True(t, MediaTaskStagePolling.CanTransitionTo(MediaTaskStageStoring))
	require.True(t, MediaTaskStageSettling.CanTransitionTo(MediaTaskStageCompleted))
	require.False(t, MediaTaskStageCompleted.CanTransitionTo(MediaTaskStagePolling))
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run 'TestMedia(TaskStatus|TaskStage|Spec)|TestNormalizeNativeAsyncMode' -count=1`

Expected: FAIL，提示 `MediaTaskStatus` 和 `NormalizeNativeAsyncMode` 未定义。

- [ ] **Step 3: 实现最小领域类型**

```go
type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

func (s MediaTaskStage) CanTransitionTo(next MediaTaskStage) bool {
	allowed := map[MediaTaskStage][]MediaTaskStage{
		MediaTaskStageQueued:     {MediaTaskStageScheduling, MediaTaskStageFailed},
		MediaTaskStageScheduling: {MediaTaskStageSubmitting, MediaTaskStageFailed},
		MediaTaskStageSubmitting: {MediaTaskStageGenerating, MediaTaskStagePolling, MediaTaskStageFailed},
		MediaTaskStageGenerating: {MediaTaskStageStoring, MediaTaskStageFailed},
		MediaTaskStagePolling:    {MediaTaskStageStoring, MediaTaskStageFailed},
		MediaTaskStageStoring:    {MediaTaskStageSettling, MediaTaskStageFailed},
		MediaTaskStageSettling:   {MediaTaskStageCompleted, MediaTaskStageFailed},
	}
	return slices.Contains(allowed[s], next)
}

type MediaOperation string

const (
	MediaOperationTextToImage     MediaOperation = "text_to_image"
	MediaOperationImageToImage    MediaOperation = "image_to_image"
	MediaOperationImageEdit       MediaOperation = "image_edit"
	MediaOperationTextToVideo     MediaOperation = "text_to_video"
	MediaOperationImageToVideo    MediaOperation = "image_to_video"
	MediaOperationReferenceVideo  MediaOperation = "reference_to_video"
	MediaOperationVideoExtend     MediaOperation = "video_extend"
	MediaOperationVideoRemix      MediaOperation = "video_remix"
)

type MediaTaskStatus string

const (
	MediaTaskStatusQueued     MediaTaskStatus = "queued"
	MediaTaskStatusInProgress MediaTaskStatus = "in_progress"
	MediaTaskStatusCompleted  MediaTaskStatus = "completed"
	MediaTaskStatusFailed     MediaTaskStatus = "failed"
)

func (s MediaTaskStatus) CanTransitionTo(next MediaTaskStatus) bool {
	switch s {
	case MediaTaskStatusQueued:
		return next == MediaTaskStatusInProgress || next == MediaTaskStatusFailed
	case MediaTaskStatusInProgress:
		return next == MediaTaskStatusCompleted || next == MediaTaskStatusFailed
	default:
		return false
	}
}

func (s MediaTaskStatus) IsTerminal() bool {
	return s == MediaTaskStatusCompleted || s == MediaTaskStatusFailed
}

type NativeAsyncMode string

const (
	NativeAsyncUnsupported NativeAsyncMode = "unsupported"
	NativeAsyncOptional    NativeAsyncMode = "optional"
	NativeAsyncRequired    NativeAsyncMode = "required"
)

func NormalizeNativeAsyncMode(raw string) NativeAsyncMode {
	switch NativeAsyncMode(strings.ToLower(strings.TrimSpace(raw))) {
	case NativeAsyncOptional:
		return NativeAsyncOptional
	case NativeAsyncRequired:
		return NativeAsyncRequired
	default:
		return NativeAsyncUnsupported
	}
}
```

同一文件定义内部阶段和互斥规格：

```go
type MediaTaskStage string

const (
	MediaTaskStageQueued     MediaTaskStage = "queued"
	MediaTaskStageScheduling MediaTaskStage = "scheduling"
	MediaTaskStageSubmitting MediaTaskStage = "submitting"
	MediaTaskStageGenerating MediaTaskStage = "generating"
	MediaTaskStagePolling    MediaTaskStage = "polling"
	MediaTaskStageStoring    MediaTaskStage = "storing"
	MediaTaskStageSettling   MediaTaskStage = "settling"
	MediaTaskStageCompleted  MediaTaskStage = "completed"
	MediaTaskStageFailed     MediaTaskStage = "failed"
)

type ImageSpec struct {
	Prompt           string  `json:"prompt"`
	Size             string  `json:"size,omitempty"`
	Quality          string  `json:"quality,omitempty"`
	OutputFormat     string  `json:"output_format,omitempty"`
	ResponseFormat   string  `json:"response_format,omitempty"`
	Count            int     `json:"n"`
	InputArtifactIDs []int64 `json:"input_artifact_ids,omitempty"`
}

type VideoSpec struct {
	Prompt               string  `json:"prompt"`
	DurationSeconds      int     `json:"duration_seconds,omitempty"`
	Resolution           string  `json:"resolution,omitempty"`
	FPS                  int     `json:"fps,omitempty"`
	ReferenceArtifactIDs []int64 `json:"reference_artifact_ids,omitempty"`
	SourceArtifactID     *int64  `json:"source_artifact_id,omitempty"`
}

type MediaSpec struct {
	Image *ImageSpec `json:"image,omitempty"`
	Video *VideoSpec `json:"video,omitempty"`
}

func (s MediaSpec) Validate(mediaType MediaType) error {
	if mediaType != MediaTypeImage && mediaType != MediaTypeVideo {
		return ErrInvalidMediaSpec
	}
	if (s.Image == nil) == (s.Video == nil) {
		return ErrInvalidMediaSpec
	}
	if mediaType == MediaTypeImage && s.Image == nil {
		return ErrInvalidMediaSpec
	}
	if mediaType == MediaTypeVideo && s.Video == nil {
		return ErrInvalidMediaSpec
	}
	if s.Image != nil && (strings.TrimSpace(s.Image.Prompt) == "" || s.Image.Count < 1) {
		return ErrInvalidMediaSpec
	}
	if s.Video != nil && strings.TrimSpace(s.Video.Prompt) == "" {
		return ErrInvalidMediaSpec
	}
	return nil
}
```

- [ ] **Step 4: 运行领域测试**

Run: `cd backend && go test ./internal/service -run 'TestMedia(TaskStatus|TaskStage|Spec)|TestNormalizeNativeAsyncMode' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/media_types.go backend/internal/service/media_types_test.go
git commit -m "feat(media): define task domain types"
```

### Task 2: 建立 Ent Schema 和 SQL 迁移

**Files:**
- Create: `backend/ent/schema/media_task.go`
- Create: `backend/ent/schema/media_artifact.go`
- Create: `backend/ent/schema/media_model_definition.go`
- Modify: `backend/ent/schema/group.go`
- Create: `backend/migrations/128_media_task_foundation.sql`
- Create: `backend/migrations/media_task_foundation_migration_test.go`
- Generated: `backend/ent/`

- [ ] **Step 1: 写迁移契约失败测试**

```go
func TestMediaTaskFoundationMigrationContainsRequiredObjects(t *testing.T) {
	body, err := FS.ReadFile("128_media_task_foundation.sql")
	require.NoError(t, err)
	sql := string(body)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS media_tasks",
		"CREATE TABLE IF NOT EXISTS media_artifacts",
		"CREATE TABLE IF NOT EXISTS media_model_definitions",
		"allow_video_generation",
		"media_cross_platform_enabled",
		"public_id",
		"idempotency_key",
		"settlement_plan",
		"candidate_snapshot",
		"lease_until",
		"version",
		"UNIQUE (task_id, direction, position)",
		"idx_media_tasks_user_created",
		"idx_media_tasks_status_lease",
		"idx_media_tasks_account",
		"idx_media_tasks_idempotency",
		"idx_media_artifacts_task",
		"idx_media_model_definitions_enabled",
	} {
		require.Contains(t, sql, fragment)
	}
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./migrations -run TestMediaTaskFoundationMigrationContainsRequiredObjects -count=1`

Expected: FAIL，提示迁移文件不存在。

- [ ] **Step 3: 添加 Schema 和幂等迁移**

`MediaTask` Schema 使用以下核心字段；JSON 字段使用 `json.RawMessage`，时间字段使用 `timestamptz`：

```go
func (MediaTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("public_id").MaxLen(64).Unique(),
		field.Int64("user_id"),
		field.Int64("api_key_id"),
		field.Int64("group_id"),
		field.Int64("channel_id").Optional().Nillable(),
		field.Int64("account_id").Optional().Nillable(),
		field.String("media_type").MaxLen(16),
		field.String("operation").MaxLen(40),
		field.String("requested_model").MaxLen(128),
		field.String("upstream_model").MaxLen(128).Default(""),
		field.String("adapter").MaxLen(64).Default(""),
		field.String("native_async_mode").MaxLen(16).Default("unsupported"),
		field.Bool("client_async").Default(false),
		field.Bool("sync_fallback").Default(false),
		field.String("status").MaxLen(20).Default("queued"),
		field.String("stage").MaxLen(20).Default("queued"),
		field.Int("progress").Default(0),
		field.JSON("request_spec", json.RawMessage{}),
		field.JSON("candidate_snapshot", json.RawMessage{}),
		field.String("request_fingerprint").MaxLen(64),
		field.String("idempotency_key").MaxLen(255).Default(""),
		field.String("upstream_task_id").Optional().Nillable(),
		field.JSON("poll_metadata", json.RawMessage{}).Optional(),
		field.JSON("billing_snapshot", json.RawMessage{}).Optional(),
		field.JSON("settlement_plan", json.RawMessage{}).Optional(),
		field.String("billing_status").MaxLen(24).Default("pending"),
		field.Float("precharged_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Float("final_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Float("refunded_amount").SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).Default(0),
		field.Int("retry_count").Default(0),
		field.String("error_code").MaxLen(64).Default(""),
		field.String("error_message").Default(""),
		field.String("worker_id").MaxLen(128).Default(""),
		field.Time("lease_until").Optional().Nillable(),
		field.Int64("version").Default(1),
		field.Time("submitted_at").Optional().Nillable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("finished_at").Optional().Nillable(),
		field.Time("sync_fallback_at").Optional().Nillable(),
	}
}
```

`MediaArtifact` 使用以下字段，不能把视频内容写入数据库：

```go
func (MediaArtifact) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("task_id"),
		field.String("direction").MaxLen(16),
		field.Int("position").Default(0),
		field.String("media_type").MaxLen(16),
		field.String("content_type").MaxLen(128),
		field.Int64("size_bytes").Default(0),
		field.String("checksum_sha256").MaxLen(64).Default(""),
		field.Int("width").Optional().Nillable(),
		field.Int("height").Optional().Nillable(),
		field.Float("duration_seconds").Optional().Nillable(),
		field.String("resolution").MaxLen(32).Default(""),
		field.Float("fps").Optional().Nillable(),
		field.String("storage_status").MaxLen(24).Default("pending"),
		field.String("object_key").Optional().Nillable(),
		field.String("public_url").Optional().Nillable(),
		field.Text("upstream_reference").Optional().Nillable().Sensitive(),
		field.Time("expires_at").Optional().Nillable(),
	}
}
```

`MediaModelDefinition` 保存模型 ID、媒体类型、操作数组、约束 JSON、计费单位和启用状态：

```go
func (MediaModelDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_id").MaxLen(128).Unique(),
		field.String("media_type").MaxLen(16),
		field.JSON("operations", []string{}),
		field.JSON("constraints", json.RawMessage{}),
		field.String("billing_unit").MaxLen(32),
		field.Bool("enabled").Default(true),
	}
}
```

三张新 Schema 使用 `mixins.TimeMixin{}`；任务和产物分别为 `public_id`、`(task_id, direction, position)`、状态/租约、用户/创建时间建立索引。Group 增加：

```go
field.Bool("allow_video_generation").Default(false),
field.Bool("media_cross_platform_enabled").Default(false),
```

迁移 SQL 与 Schema 使用相同列名，并显式建立幂等约束：

```sql
CREATE TABLE IF NOT EXISTS media_tasks (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    channel_id BIGINT NULL,
    account_id BIGINT NULL,
    media_type VARCHAR(16) NOT NULL CHECK (media_type IN ('image','video')),
    operation VARCHAR(40) NOT NULL,
    requested_model VARCHAR(128) NOT NULL,
    upstream_model VARCHAR(128) NOT NULL DEFAULT '',
    adapter VARCHAR(64) NOT NULL DEFAULT '',
    native_async_mode VARCHAR(16) NOT NULL DEFAULT 'unsupported' CHECK (native_async_mode IN ('unsupported','optional','required')),
    client_async BOOLEAN NOT NULL DEFAULT FALSE,
    sync_fallback BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','in_progress','completed','failed')),
    stage VARCHAR(20) NOT NULL DEFAULT 'queued' CHECK (stage IN ('queued','scheduling','submitting','generating','polling','storing','settling','completed','failed')),
    progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    request_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    candidate_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    request_fingerprint VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL DEFAULT '',
    upstream_task_id TEXT NULL,
    poll_metadata JSONB NULL,
    billing_snapshot JSONB NULL,
    settlement_plan JSONB NULL,
    billing_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    precharged_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    final_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    refunded_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    worker_id VARCHAR(128) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ NULL,
    version BIGINT NOT NULL DEFAULT 1,
    submitted_at TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    sync_fallback_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS media_artifacts (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES media_tasks(id) ON DELETE CASCADE,
    direction VARCHAR(16) NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    media_type VARCHAR(16) NOT NULL CHECK (media_type IN ('image','video')),
    content_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    width INTEGER NULL,
    height INTEGER NULL,
    duration_seconds DOUBLE PRECISION NULL,
    resolution VARCHAR(32) NOT NULL DEFAULT '',
    fps DOUBLE PRECISION NULL,
    storage_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    object_key TEXT NULL,
    public_url TEXT NULL,
    upstream_reference TEXT NULL,
    expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (task_id, direction, position)
);

CREATE TABLE IF NOT EXISTS media_model_definitions (
    id BIGSERIAL PRIMARY KEY,
    model_id VARCHAR(128) NOT NULL UNIQUE,
    media_type VARCHAR(16) NOT NULL CHECK (media_type IN ('image','video')),
    operations JSONB NOT NULL DEFAULT '[]'::jsonb,
    constraints JSONB NOT NULL DEFAULT '{}'::jsonb,
    billing_unit VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE groups ADD COLUMN IF NOT EXISTS allow_video_generation BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS media_cross_platform_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_media_tasks_user_created ON media_tasks(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_tasks_status_lease ON media_tasks(status, lease_until);
CREATE INDEX IF NOT EXISTS idx_media_tasks_account ON media_tasks(account_id) WHERE account_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_tasks_idempotency ON media_tasks(user_id, api_key_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_media_artifacts_task ON media_artifacts(task_id, direction, position);
CREATE INDEX IF NOT EXISTS idx_media_model_definitions_enabled ON media_model_definitions(enabled, media_type);
```

- [ ] **Step 4: 生成 Ent 代码**

Run: `cd backend && go generate ./ent`

Expected: 生成 `MediaTask`、`MediaArtifact`、`MediaModelDefinition` 查询、创建、更新和 mutation 文件，命令退出码为 0。

- [ ] **Step 5: 运行迁移和 Ent 编译测试**

Run: `cd backend && go test ./migrations ./ent/... -count=1`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add backend/ent backend/migrations/128_media_task_foundation.sql backend/migrations/media_task_foundation_migration_test.go
git commit -m "feat(media): add task persistence schema"
```

### Task 3: 实现任务、产物、租约和 CAS 仓储

**Files:**
- Create: `backend/internal/service/media_task.go`
- Create: `backend/internal/repository/media_task_repo.go`
- Test: `backend/internal/repository/media_task_repo_test.go`
- Modify: `backend/internal/repository/wire.go`

- [ ] **Step 1: 写仓储失败测试**

```go
func TestMediaTaskRepositoryClaimAndCompleteWithCAS(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	task, err := repo.Create(context.Background(), &service.MediaTask{
		PublicID:           "task_repo_claim",
		UserID:             1,
		APIKeyID:           2,
		GroupID:            3,
		MediaType:          service.MediaTypeImage,
		Operation:          service.MediaOperationTextToImage,
		RequestedModel:     "fake-image",
		RequestFingerprint: "fp",
		Status:             service.MediaTaskStatusQueued,
	})
	require.NoError(t, err)

	claimed, err := repo.Claim(context.Background(), task.ID, "worker-a", time.Now().Add(time.Minute), task.Version)
	require.NoError(t, err)
	require.True(t, claimed)

	completed, err := repo.Transition(context.Background(), task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, map[string]any{"progress": 100})
	require.NoError(t, err)
	require.True(t, completed)

	stale, err := repo.Transition(context.Background(), task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusFailed, nil)
	require.NoError(t, err)
	require.False(t, stale)

	stored, err := client.MediaTask.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", stored.Status)
}

func TestMediaArtifactRepositoryCreateIsIdempotentByPosition(t *testing.T) {
	repo, task := newMediaArtifactRepositoryTestHarness(t)
	input := &service.MediaArtifact{
		TaskID: task.ID, Direction: "output", Position: 0,
		MediaType: service.MediaTypeImage, ContentType: "image/png",
	}
	first, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	second, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/repository -run TestMediaTaskRepositoryClaimAndCompleteWithCAS -count=1`

Expected: FAIL，提示 `NewMediaTaskRepository` 未定义。

- [ ] **Step 3: 定义领域实体与仓储接口**

```go
type MediaTask struct {
	ID                 int64
	PublicID           string
	UserID             int64
	APIKeyID           int64
	GroupID            int64
	ChannelID           *int64
	AccountID           *int64
	MediaType           MediaType
	Operation           MediaOperation
	RequestedModel      string
	UpstreamModel       string
	Adapter             string
	NativeAsyncMode     NativeAsyncMode
	ClientAsync         bool
	SyncFallback        bool
	Status              MediaTaskStatus
	Stage               MediaTaskStage
	Progress            int
	RequestSpec         json.RawMessage
	CandidateSnapshot   json.RawMessage
	RequestFingerprint  string
	IdempotencyKey      string
	UpstreamTaskID      string
	PollMetadata        json.RawMessage
	BillingSnapshot     json.RawMessage
	SettlementPlan      json.RawMessage
	BillingStatus       string
	PrechargedAmount    float64
	FinalAmount         float64
	RefundedAmount      float64
	RetryCount          int
	ErrorCode           string
	ErrorMessage        string
	WorkerID            string
	LeaseUntil          *time.Time
	Version             int64
	SubmittedAt         *time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
	SyncFallbackAt      *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type MediaArtifact struct {
	ID                    int64
	TaskID                int64
	Direction             string
	Position              int
	MediaType              MediaType
	ContentType            string
	SizeBytes              int64
	ChecksumSHA256         string
	Width                  *int
	Height                 *int
	DurationSeconds        *float64
	Resolution             string
	FPS                    *float64
	StorageStatus          string
	ObjectKey              string
	PublicURL              string
	UpstreamReference      string
	ExpiresAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type MediaTaskRepository interface {
	Create(ctx context.Context, task *MediaTask) (*MediaTask, error)
	GetByID(ctx context.Context, id int64) (*MediaTask, error)
	GetByPublicIDForUser(ctx context.Context, publicID string, userID int64) (*MediaTask, error)
	GetByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*MediaTask, error)
	UpdateQueued(ctx context.Context, id, version int64, updates map[string]any) (bool, error)
	Claim(ctx context.Context, id int64, workerID string, leaseUntil time.Time, version int64) (bool, error)
	RenewLease(ctx context.Context, id int64, workerID string, leaseUntil time.Time) (bool, error)
	UpdateClaimed(ctx context.Context, id int64, workerID string, updates map[string]any) (bool, error)
	Transition(ctx context.Context, id int64, from, to MediaTaskStatus, updates map[string]any) (bool, error)
	MarkSyncFallback(ctx context.Context, id int64, at time.Time) (bool, error)
	ListRecoverable(ctx context.Context, now time.Time, limit int) ([]MediaTask, error)
	ListSettlementPending(ctx context.Context, limit int) ([]MediaTask, error)
	UpdateBilling(ctx context.Context, id int64, fromStatus string, updates map[string]any) (bool, error)
}

type MediaArtifactRepository interface {
	Create(ctx context.Context, artifact *MediaArtifact) (*MediaArtifact, error)
	ListByTaskID(ctx context.Context, taskID int64) ([]MediaArtifact, error)
}
```

实现使用 Ent 条件更新：`WHERE id = ? AND status = ?`；`UpdateQueued` 和 Claim 额外比较 `version`，成功后 `version = version + 1`。`UpdateClaimed` 必须同时匹配 `worker_id` 和未过期租约，成功后递增版本；`MarkSyncFallback` 只允许非终态任务且只能从 false 改为 true。`ListRecoverable` 只返回 `queued` 或 `in_progress` 且租约为空/过期的任务；`ListSettlementPending` 只返回有 `settlement_plan` 且 `billing_status IN ('precharged','settling','retry')` 的任务。`UpdateBilling` 比较 billing_status 而不回退执行状态。产物创建依赖迁移中的 `(task_id, direction, position)` 唯一约束，把重复 Worker 写入转为读取已存在记录。

- [ ] **Step 4: 运行仓储测试**

Run: `cd backend && go test ./internal/repository -run 'TestMediaTaskRepository|TestMediaArtifactRepository' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/media_task.go backend/internal/repository/media_task_repo.go backend/internal/repository/media_task_repo_test.go backend/internal/repository/wire.go
git commit -m "feat(media): persist task lifecycle with leases"
```

### Task 4: 实现持久化模型注册表

**Files:**
- Create: `backend/internal/service/media_model_registry.go`
- Create: `backend/internal/repository/media_model_repo.go`
- Test: `backend/internal/service/media_model_registry_test.go`
- Test: `backend/internal/repository/media_model_repo_test.go`
- Modify: `backend/internal/repository/wire.go`

- [ ] **Step 1: 写能力验证失败测试**

```go
func TestMediaModelRegistryValidateOperation(t *testing.T) {
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{{
		ModelID:    "fake-image",
		MediaType:  MediaTypeImage,
		Operations: []MediaOperation{MediaOperationTextToImage},
		Constraints: json.RawMessage(`{"image_sizes":["1024x1024"],"max_image_count":2}`),
		Enabled:    true,
	}}}
	registry := NewMediaModelRegistry(repo)
	require.NoError(t, registry.Refresh(context.Background()))

	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	_, err = registry.Resolve("fake-image", MediaOperationTextToVideo)
	require.ErrorIs(t, err, ErrMediaOperationUnsupported)
	_, err = registry.Resolve("missing", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
	err = registry.ValidateSpec("fake-image", MediaOperationTextToImage, MediaSpec{Image: &ImageSpec{
		Prompt: "cat", Size: "2048x2048", Count: 1,
	}})
	require.ErrorIs(t, err, ErrMediaSpecOutsideModelConstraints)
}

// backend/internal/repository/media_model_repo_test.go
func TestMediaModelRepositoryListEnabledExcludesDisabled(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	seedMediaModelDefinition(t, client, "enabled-image", true)
	seedMediaModelDefinition(t, client, "disabled-image", false)
	repo := NewMediaModelRepository(client)
	items, err := repo.ListEnabled(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"enabled-image"}, collectMediaModelIDs(items))
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run TestMediaModelRegistryValidateOperation -count=1`

Expected: FAIL，提示 `NewMediaModelRegistry` 未定义。

- [ ] **Step 3: 实现仓储和原子快照 Registry**

```go
type MediaModelDefinition struct {
	ID          int64
	ModelID     string
	MediaType   MediaType
	Operations  []MediaOperation
	Constraints json.RawMessage
	BillingUnit string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MediaModelConstraints struct {
	ImageSizes          []string `json:"image_sizes,omitempty"`
	MaxImageCount       int      `json:"max_image_count,omitempty"`
	VideoDurations      []int    `json:"video_durations,omitempty"`
	VideoResolutions    []string `json:"video_resolutions,omitempty"`
	MinFPS              int      `json:"min_fps,omitempty"`
	MaxFPS              int      `json:"max_fps,omitempty"`
	MaxReferenceImages  int      `json:"max_reference_images,omitempty"`
}

func (d MediaModelDefinition) Supports(operation MediaOperation) bool {
	for _, candidate := range d.Operations {
		if candidate == operation {
			return true
		}
	}
	return false
}

type MediaModelDefinitionRepository interface {
	ListEnabled(ctx context.Context) ([]MediaModelDefinition, error)
}

type MediaModelRegistry struct {
	repo     MediaModelDefinitionRepository
	snapshot atomic.Value // map[string]MediaModelDefinition
}

func (r *MediaModelRegistry) Resolve(model string, operation MediaOperation) (*MediaModelDefinition, error) {
	items, _ := r.snapshot.Load().(map[string]MediaModelDefinition)
	definition, ok := items[strings.ToLower(strings.TrimSpace(model))]
	if !ok || !definition.Enabled {
		return nil, ErrMediaModelNotFound
	}
	if !definition.Supports(operation) {
		return nil, ErrMediaOperationUnsupported
	}
	copy := definition
	return &copy, nil
}
```

`ValidateSpec` 先调用 `Resolve`，再把 `Constraints` 解码成 `MediaModelConstraints`，逐项校验尺寸、数量、时长、分辨率、FPS 和参考图数量；约束字段为空表示 Registry 不额外限制，通用安全上限仍由 Handler 执行。`Refresh` 先完整读取并校验新快照，再一次性 `Store`，不能让读取方看到半更新状态。

- [ ] **Step 4: 运行 Registry 和仓储测试**

Run: `cd backend && go test ./internal/service ./internal/repository -run 'TestMediaModelRegistry|TestMediaModelRepository' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/media_model_registry.go backend/internal/service/media_model_registry_test.go backend/internal/repository/media_model_repo.go backend/internal/repository/media_model_repo_test.go backend/internal/repository/wire.go
git commit -m "feat(media): add persistent model registry"
```

### Task 5: 增加账号媒体配置和模型覆盖

**Files:**
- Create: `backend/internal/service/media_account_config.go`
- Test: `backend/internal/service/media_account_config_test.go`
- Modify: `backend/internal/service/account.go`
- Modify: `backend/internal/service/admin_service.go`
- Test: `backend/internal/service/admin_service_media_config_test.go`

- [ ] **Step 1: 写账号配置继承与校验失败测试**

```go
func TestAccountResolveMediaConfigUsesModelOverride(t *testing.T) {
	account := &Account{Extra: map[string]any{
		"media_config": map[string]any{
			"adapter": "gemini",
			"native_async_mode": "optional",
			"model_overrides": map[string]any{
				"veo-3.1": map[string]any{
					"upstream_model": "veo-3.1-generate",
					"native_async_mode": "required",
				},
			},
		},
	}}

	resolved := account.ResolveMediaModel("veo-3.1")
	require.Equal(t, "gemini", resolved.Adapter)
	require.Equal(t, "veo-3.1-generate", resolved.UpstreamModel)
	require.Equal(t, NativeAsyncRequired, resolved.NativeAsyncMode)
}

func TestNormalizeMediaAccountConfigRejectsUnknownMode(t *testing.T) {
	_, err := NormalizeMediaAccountConfig(MediaAccountConfig{Adapter: "gemini", NativeAsyncMode: "sometimes"})
	require.ErrorIs(t, err, ErrInvalidNativeAsyncMode)
}

func TestAdminServiceCreateAccountNormalizesMediaConfig(t *testing.T) {
	svc, repo := newAdminServiceMediaConfigFixture(t)
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name: "media", Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Extra: map[string]any{"media_config": map[string]any{
			"adapter": " gemini ", "native_async_mode": "OPTIONAL",
		}},
	})
	require.NoError(t, err)
	stored := repo.LastCreated().Extra["media_config"].(map[string]any)
	require.Equal(t, "gemini", stored["adapter"])
	require.Equal(t, "optional", stored["native_async_mode"])
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run 'TestAccountResolveMediaConfig|TestNormalizeMediaAccountConfig' -count=1`

Expected: FAIL，提示媒体配置类型未定义。

- [ ] **Step 3: 实现配置类型、解析和 Admin 输入校验**

```go
type MediaAccountConfig struct {
	Adapter         string                              `json:"adapter"`
	NativeAsyncMode NativeAsyncMode                     `json:"native_async_mode"`
	ModelOverrides  map[string]MediaAccountModelOverride `json:"model_overrides,omitempty"`
}

type MediaAccountModelOverride struct {
	UpstreamModel  string          `json:"upstream_model,omitempty"`
	NativeAsyncMode NativeAsyncMode `json:"native_async_mode,omitempty"`
}

type ResolvedMediaAccountModel struct {
	Adapter         string
	UpstreamModel   string
	NativeAsyncMode NativeAsyncMode
}
```

`NormalizeMediaAccountConfig` 只接受非空 Adapter 和三种异步模式；模型覆盖 Key 统一 trim，不允许空模型名。创建或更新账号时，在持久化 `Extra` 前调用规范化并把结果写回 `extra.media_config`。

- [ ] **Step 4: 运行账号配置和 Admin Service 测试**

Run: `cd backend && go test ./internal/service -run 'Test(AccountResolveMediaConfig|NormalizeMediaAccountConfig|AdminService.*MediaConfig)' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/media_account_config.go backend/internal/service/media_account_config_test.go backend/internal/service/account.go backend/internal/service/admin_service.go backend/internal/service/admin_service_media_config_test.go
git commit -m "feat(media): configure account adapters and async modes"
```

### Task 6: 增加分组视频权限与跨平台媒体绑定

**Files:**
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/service/account_service.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/handler/admin/group_handler.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Test: `backend/internal/service/account_service_media_group_test.go`
- Test: `backend/internal/handler/admin/group_handler_media_test.go`

- [ ] **Step 1: 写跨平台绑定和 DTO 失败测试**

```go
func TestValidateAccountGroupBindingAllowsCrossPlatformOnlyForMediaGroups(t *testing.T) {
	mediaGroup := &Group{Name: "media", Platform: PlatformOpenAI, MediaCrossPlatformEnabled: true}
	require.NoError(t, validateAccountGroupBinding(mediaGroup, PlatformGemini, AccountTypeAPIKey))

	plainGroup := &Group{Name: "plain", Platform: PlatformOpenAI}
	require.Error(t, validateAccountGroupBinding(plainGroup, PlatformGemini, AccountTypeAPIKey))
}

func TestAdminGroupDTOIncludesMediaFlags(t *testing.T) {
	dto := handlerdto.AdminGroupFromService(&service.Group{
		AllowImageGeneration:      true,
		AllowVideoGeneration:      true,
		MediaCrossPlatformEnabled: true,
	})
	require.True(t, dto.AllowVideoGeneration)
	require.True(t, dto.MediaCrossPlatformEnabled)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service ./internal/handler/admin -run 'TestValidateAccountGroupBindingAllowsCrossPlatformOnlyForMediaGroups|TestAdminGroupDTOIncludesMediaFlags' -count=1`

Expected: FAIL，提示分组媒体字段未定义。

- [ ] **Step 3: 贯通领域、请求、服务和 DTO**

```go
type Group struct {
	// existing fields
	AllowImageGeneration      bool
	AllowVideoGeneration      bool
	MediaCrossPlatformEnabled bool
}
```

创建请求使用普通 bool，更新请求使用 `*bool`。绑定校验调整为：平台兼容时保持原规则；平台不兼容但 `MediaCrossPlatformEnabled` 为 true 时允许绑定；跨平台账号不应用分组的 `RequireOAuthOnly` 文本账号约束。

- [ ] **Step 4: 运行分组和账号回归测试**

Run: `cd backend && go test ./internal/service ./internal/handler/admin -run 'Test.*(Group|Account).*' -count=1`

Expected: PASS；现有非媒体分组平台隔离测试仍通过。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/group.go backend/internal/service/account_service.go backend/internal/service/admin_service.go backend/internal/handler/admin/group_handler.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/service/account_service_media_group_test.go backend/internal/handler/admin/group_handler_media_test.go
git commit -m "feat(media): allow cross-platform media groups"
```

### Task 7: 增加媒体系统设置后端

**Files:**
- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/service/settings_view.go`
- Modify: `backend/internal/service/setting_service.go`
- Modify: `backend/internal/handler/admin/setting_handler.go`
- Modify: `backend/internal/handler/dto/types.go`
- Create: `backend/internal/service/setting_service_media_test.go`
- Create: `backend/internal/handler/admin/setting_handler_media_test.go`

- [ ] **Step 1: 写默认值和校验失败测试**

```go
func TestSettingServiceMediaDefaults(t *testing.T) {
	svc := newSettingServiceForMediaTest(map[string]string{})
	settings := svc.parseSettings(map[string]string{})
	require.Equal(t, 240, settings.MediaSyncWaitTimeoutSeconds)
	require.False(t, settings.MediaSyncTimeoutFallbackAsyncEnabled)
	require.Equal(t, MediaTimeoutBillingPolicyPenalty, settings.MediaSyncTimeoutBillingPolicy)
	require.Equal(t, 0.8, settings.MediaSyncTimeoutPenaltyRatio)
	require.Equal(t, MediaVideoStorageModeHybrid, settings.MediaVideoStorageMode)
	require.True(t, settings.MediaVideoProxyFallbackEnabled)
}

func TestSettingServiceRejectsInvalidMediaPenaltyRatio(t *testing.T) {
	svc := newSettingServiceForMediaTest(map[string]string{})
	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		MediaSyncWaitTimeoutSeconds:            240,
		MediaSyncTimeoutBillingPolicy:          MediaTimeoutBillingPolicyPenalty,
		MediaSyncTimeoutPenaltyRatio:           1.2,
		MediaVideoStorageMode:                  MediaVideoStorageModeHybrid,
		MediaVideoProxyFallbackEnabled:         true,
	})
	require.Error(t, err)
}

func TestSettingHandlerRoundTripsMediaSettings(t *testing.T) {
	router, repo := newSettingHandlerMediaTestRouter(t)
	body := `{"media_sync_wait_timeout_seconds":0,"media_sync_timeout_fallback_async_enabled":true,"media_sync_timeout_billing_policy":"refund","media_sync_timeout_penalty_ratio":0.8,"media_video_storage_mode":"hybrid","media_video_proxy_fallback_enabled":false}`
	rec := performAdminJSONRequest(router, http.MethodPut, "/settings", body)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "0", repo.values[service.SettingKeyMediaSyncWaitTimeoutSeconds])
	require.Equal(t, "true", repo.values[service.SettingKeyMediaSyncTimeoutFallbackAsyncEnabled])
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service ./internal/handler/admin -run 'TestSetting(ServiceMediaDefaults|ServiceRejectsInvalidMediaPenaltyRatio|HandlerRoundTripsMediaSettings)' -count=1`

Expected: FAIL，提示媒体设置字段和 Handler 请求映射未定义。

- [ ] **Step 3: 添加设置键、DTO、默认值、解析和保存**

```go
const (
	SettingKeyMediaSyncWaitTimeoutSeconds            = "media_sync_wait_timeout_seconds"
	SettingKeyMediaSyncTimeoutFallbackAsyncEnabled   = "media_sync_timeout_fallback_async_enabled"
	SettingKeyMediaSyncTimeoutBillingPolicy          = "media_sync_timeout_billing_policy"
	SettingKeyMediaSyncTimeoutPenaltyRatio           = "media_sync_timeout_penalty_ratio"
	SettingKeyMediaVideoStorageMode                  = "media_video_storage_mode"
	SettingKeyMediaVideoProxyFallbackEnabled         = "media_video_proxy_fallback_enabled"
)

const (
	MediaTimeoutBillingPolicyRefund  = "refund"
	MediaTimeoutBillingPolicyPenalty = "penalty"
	MediaVideoStorageModeHybrid      = "hybrid"
)
```

校验规则：等待时间必须大于等于 0；计费策略只能是 `refund`/`penalty`；比例范围为 `[0,1]`；视频存储模式本阶段只接受 `hybrid`。`InitializeDefaultSettings` 写入批准的默认值。

`UpdateSettingsRequest` 对六个字段使用指针，保证部分更新时能区分“未传”和显式 `0/false`；`dto.SystemSettings`、`GetSettings` 响应映射、更新时的 `service.SystemSettings` 构造和审计差异列表必须同时贯通：

```go
MediaSyncWaitTimeoutSeconds          *int     `json:"media_sync_wait_timeout_seconds"`
MediaSyncTimeoutFallbackAsyncEnabled *bool    `json:"media_sync_timeout_fallback_async_enabled"`
MediaSyncTimeoutBillingPolicy        *string  `json:"media_sync_timeout_billing_policy"`
MediaSyncTimeoutPenaltyRatio         *float64 `json:"media_sync_timeout_penalty_ratio"`
MediaVideoStorageMode                *string  `json:"media_video_storage_mode"`
MediaVideoProxyFallbackEnabled       *bool    `json:"media_video_proxy_fallback_enabled"`
```

- [ ] **Step 4: 运行全部 SettingService 测试**

Run: `cd backend && go test ./internal/service ./internal/handler/admin -run 'TestSetting(Service|Handler).*Media' -count=1`

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add backend/internal/service/domain_constants.go backend/internal/service/settings_view.go backend/internal/service/setting_service.go backend/internal/service/setting_service_media_test.go backend/internal/handler/admin/setting_handler.go backend/internal/handler/admin/setting_handler_media_test.go backend/internal/handler/dto/types.go
git commit -m "feat(media): add runtime media settings"
```

### Task 8: 在系统设置页增加媒体生成可视化配置

**Files:**
- Create: `frontend/src/components/admin/settings/MediaGenerationSettingsCard.vue`
- Create: `frontend/src/components/admin/settings/__tests__/MediaGenerationSettingsCard.spec.ts`
- Modify: `frontend/src/api/admin/settings.ts`
- Modify: `frontend/src/views/admin/SettingsView.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: 写设置卡片行为失败测试**

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MediaGenerationSettingsCard from '../MediaGenerationSettingsCard.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const baseSettings = {
  media_sync_wait_timeout_seconds: 240,
  media_sync_timeout_fallback_async_enabled: false,
  media_sync_timeout_billing_policy: 'penalty' as const,
  media_sync_timeout_penalty_ratio: 0.8,
  media_video_storage_mode: 'hybrid' as const,
  media_video_proxy_fallback_enabled: true,
}

describe('MediaGenerationSettingsCard', () => {
  it('提示 0 秒不保证自动转异步，并发出数值 0', async () => {
    const wrapper = mount(MediaGenerationSettingsCard, {
      props: { modelValue: baseSettings },
    })
    await wrapper.get('[data-test="media-sync-timeout"]').setValue('0')
    expect(wrapper.text()).toContain('admin.settings.mediaGeneration.timeoutDisabledWarning')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({
      media_sync_wait_timeout_seconds: 0,
    })
  })

  it('全额退款策略下隐藏扣费比例', async () => {
    const wrapper = mount(MediaGenerationSettingsCard, {
      props: { modelValue: baseSettings },
    })
    await wrapper.get('[data-test="media-timeout-billing-policy"]').setValue('refund')
    expect(wrapper.find('[data-test="media-timeout-penalty-ratio"]').exists()).toBe(false)
  })
})
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd frontend && pnpm test:run src/components/admin/settings/__tests__/MediaGenerationSettingsCard.spec.ts`

Expected: FAIL，提示 `MediaGenerationSettingsCard.vue` 不存在。

- [ ] **Step 3: 定义前端类型并实现设置卡片**

在 `frontend/src/api/admin/settings.ts` 定义并让 `SystemSettings` 扩展以下结构，同时把六个字段以可选形式加入 `UpdateSettingsRequest`：

```ts
export type MediaTimeoutBillingPolicy = 'refund' | 'penalty'
export type MediaVideoStorageMode = 'hybrid'

export interface MediaGenerationSettings {
  media_sync_wait_timeout_seconds: number
  media_sync_timeout_fallback_async_enabled: boolean
  media_sync_timeout_billing_policy: MediaTimeoutBillingPolicy
  media_sync_timeout_penalty_ratio: number
  media_video_storage_mode: MediaVideoStorageMode
  media_video_proxy_fallback_enabled: boolean
}

export interface SystemSettings extends MediaGenerationSettings {
  // 保留现有字段
}
```

创建卡片组件；扣费比例在 UI 中使用百分数，写回 API 模型时转换成 `[0,1]` 小数：

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MediaGenerationSettings } from '@/api/admin/settings'

const props = defineProps<{ modelValue: MediaGenerationSettings }>()
const emit = defineEmits<{
  'update:modelValue': [value: MediaGenerationSettings]
}>()
const { t } = useI18n()

const penaltyPercent = computed(() => Math.round(props.modelValue.media_sync_timeout_penalty_ratio * 100))

function update<K extends keyof MediaGenerationSettings>(key: K, value: MediaGenerationSettings[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function updateTimeout(event: Event) {
  const value = Math.max(0, Math.floor(Number((event.target as HTMLInputElement).value) || 0))
  update('media_sync_wait_timeout_seconds', value)
}

function updatePenalty(event: Event) {
  const value = Math.min(100, Math.max(0, Number((event.target as HTMLInputElement).value) || 0))
  update('media_sync_timeout_penalty_ratio', value / 100)
}
</script>

<template>
  <section data-test="media-generation-settings" class="card space-y-5">
    <div>
      <h2 class="text-lg font-semibold">{{ t('admin.settings.mediaGeneration.title') }}</h2>
      <p class="text-sm text-gray-500">{{ t('admin.settings.mediaGeneration.description') }}</p>
    </div>

    <label class="block space-y-1">
      <span>{{ t('admin.settings.mediaGeneration.syncWaitTimeout') }}</span>
      <input
        data-test="media-sync-timeout"
        class="input"
        type="number"
        min="0"
        :value="modelValue.media_sync_wait_timeout_seconds"
        @input="updateTimeout"
      />
    </label>
    <p v-if="modelValue.media_sync_wait_timeout_seconds === 0" class="text-sm text-amber-600">
      {{ t('admin.settings.mediaGeneration.timeoutDisabledWarning') }}
    </p>

    <label class="flex items-center justify-between gap-4">
      <span>{{ t('admin.settings.mediaGeneration.fallbackAsync') }}</span>
      <input
        data-test="media-fallback-async"
        type="checkbox"
        :checked="modelValue.media_sync_timeout_fallback_async_enabled"
        @change="update('media_sync_timeout_fallback_async_enabled', ($event.target as HTMLInputElement).checked)"
      />
    </label>

    <label class="block space-y-1">
      <span>{{ t('admin.settings.mediaGeneration.timeoutBillingPolicy') }}</span>
      <select
        data-test="media-timeout-billing-policy"
        class="input"
        :value="modelValue.media_sync_timeout_billing_policy"
        @change="update('media_sync_timeout_billing_policy', ($event.target as HTMLSelectElement).value as 'refund' | 'penalty')"
      >
        <option value="penalty">{{ t('admin.settings.mediaGeneration.penalty') }}</option>
        <option value="refund">{{ t('admin.settings.mediaGeneration.refund') }}</option>
      </select>
    </label>

    <label v-if="modelValue.media_sync_timeout_billing_policy === 'penalty'" class="block space-y-1">
      <span>{{ t('admin.settings.mediaGeneration.penaltyRatio') }}</span>
      <input
        data-test="media-timeout-penalty-ratio"
        class="input"
        type="number"
        min="0"
        max="100"
        :value="penaltyPercent"
        @input="updatePenalty"
      />
    </label>

    <label class="block space-y-1">
      <span>{{ t('admin.settings.mediaGeneration.videoStorageMode') }}</span>
      <select data-test="media-video-storage-mode" class="input" disabled value="hybrid">
        <option value="hybrid">{{ t('admin.settings.mediaGeneration.hybrid') }}</option>
      </select>
    </label>

    <label class="flex items-center justify-between gap-4">
      <span>{{ t('admin.settings.mediaGeneration.proxyFallback') }}</span>
      <input
        data-test="media-proxy-fallback"
        type="checkbox"
        :checked="modelValue.media_video_proxy_fallback_enabled"
        @change="update('media_video_proxy_fallback_enabled', ($event.target as HTMLInputElement).checked)"
      />
    </label>
  </section>
</template>
```

- [ ] **Step 4: 接入 SettingsView 的页签、加载默认值和保存请求**

把 `media` 加入 `SettingsTab` 和 `settingsTabs`，导入组件，并在表单中设置与后端一致的默认值：

```ts
type SettingsTab = 'general' | 'security' | 'users' | 'gateway' | 'media' | 'systemPrompt' | 'payment' | 'email' | 'backup'

const settingsTabs = [
  // 保留现有页签
  { key: 'media' as SettingsTab, icon: 'image' as const },
]

const form = reactive<SettingsForm>({
  // 保留现有字段
  media_sync_wait_timeout_seconds: 240,
  media_sync_timeout_fallback_async_enabled: false,
  media_sync_timeout_billing_policy: 'penalty',
  media_sync_timeout_penalty_ratio: 0.8,
  media_video_storage_mode: 'hybrid',
  media_video_proxy_fallback_enabled: true,
})

const mediaGenerationSettings = computed({
  get: () => ({
    media_sync_wait_timeout_seconds: form.media_sync_wait_timeout_seconds,
    media_sync_timeout_fallback_async_enabled: form.media_sync_timeout_fallback_async_enabled,
    media_sync_timeout_billing_policy: form.media_sync_timeout_billing_policy,
    media_sync_timeout_penalty_ratio: form.media_sync_timeout_penalty_ratio,
    media_video_storage_mode: form.media_video_storage_mode,
    media_video_proxy_fallback_enabled: form.media_video_proxy_fallback_enabled,
  }),
  set: (value: MediaGenerationSettings) => Object.assign(form, value),
})
```

模板在 `activeTab === 'media'` 时渲染 `<MediaGenerationSettingsCard v-model="mediaGenerationSettings" />`。`saveSettings` 的 `UpdateSettingsRequest` 明确包含六个媒体字段，不依赖对象扩展偶然透传。中英文增加 `admin.settings.tabs.media` 和 `admin.settings.mediaGeneration.*`，中文明确说明惩罚扣费只适用于“同步等待超时且已提交上游”的情况。

- [ ] **Step 5: 运行组件、SettingsView 和类型检查**

Run: `cd frontend && pnpm test:run src/components/admin/settings/__tests__/MediaGenerationSettingsCard.spec.ts src/views/admin/__tests__/settingsFormState.spec.ts && pnpm typecheck`

Expected: PASS，且 `vue-tsc` 无类型错误。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/admin/settings frontend/src/api/admin/settings.ts frontend/src/views/admin/SettingsView.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(media): expose generation runtime settings"
```

### Task 9: 增加账号媒体 Adapter 与原生异步配置编辑器

**Files:**
- Create: `frontend/src/components/account/MediaConfigEditor.vue`
- Create: `frontend/src/components/account/__tests__/MediaConfigEditor.spec.ts`
- Modify: `frontend/src/components/account/CreateAccountModal.vue`
- Modify: `frontend/src/components/account/EditAccountModal.vue`
- Modify: `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`
- Modify: `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: 写编辑器和 Modal 持久化失败测试**

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MediaConfigEditor from '../MediaConfigEditor.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('MediaConfigEditor', () => {
  it('编辑默认模式并添加模型覆盖', async () => {
    const wrapper = mount(MediaConfigEditor, {
      props: {
        modelValue: { adapter: 'gemini', native_async_mode: 'optional', model_overrides: {} },
      },
    })
    await wrapper.get('[data-test="media-default-async-mode"]').setValue('required')
    await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
    await wrapper.get('[data-test="media-override-model-0"]').setValue('veo-3.1')
    await wrapper.get('[data-test="media-override-upstream-0"]').setValue('veo-3.1-generate')
    const emitted = wrapper.emitted('update:modelValue')?.at(-1)?.[0]
    expect(emitted).toMatchObject({ native_async_mode: 'required' })
    expect(emitted?.model_overrides['veo-3.1'].upstream_model).toBe('veo-3.1-generate')
  })
})
```

在现有 `CreateAccountModal.spec.ts` 和 `EditAccountModal.spec.ts` 分别增加：

```ts
async function fillMinimalCreateAccountForm(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.findAll('button').find((button) => button.text().includes('OpenAI'))?.trigger('click')
  await wrapper.findAll('button').find((button) => button.text().includes('API Key'))?.trigger('click')
  const textInputs = wrapper.findAll<HTMLInputElement>('form#create-account-form input[type="text"]')
  await textInputs[0].setValue('Media Key')
  await wrapper.get<HTMLInputElement>('form#create-account-form input[type="password"]').setValue('sk-test')
}

it('creates account with media_config while preserving other extra fields', async () => {
  const wrapper = mountModal()
  const editor = wrapper.getComponent(MediaConfigEditor)
  await editor.vm.$emit('update:modelValue', {
    adapter: 'xai', native_async_mode: 'required',
    model_overrides: { 'grok-imagine': { upstream_model: 'grok-imagine-v1' } },
  })
  await fillMinimalCreateAccountForm(wrapper)
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  expect(createAccountMock.mock.calls[0]?.[0]?.extra).toMatchObject({
    media_config: {
      adapter: 'xai', native_async_mode: 'required',
      model_overrides: { 'grok-imagine': { upstream_model: 'grok-imagine-v1' } },
    },
  })
})

it('updates media_config without deleting existing extra keys', async () => {
  const wrapper = mountModal({ ...buildAccount(),
    extra: { allow_overages: true, media_config: {
      adapter: 'gemini', native_async_mode: 'optional', model_overrides: {},
    } },
  })
  await wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
    adapter: 'gemini', native_async_mode: 'unsupported', model_overrides: {},
  })
  await wrapper.get('form#edit-account-form').trigger('submit.prevent')
  expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({
    allow_overages: true,
    media_config: { adapter: 'gemini', native_async_mode: 'unsupported' },
  })
})
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd frontend && pnpm test:run src/components/account/__tests__/MediaConfigEditor.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts`

Expected: FAIL，提示编辑器不存在或提交 Payload 没有 `media_config`。

- [ ] **Step 3: 定义账号媒体配置类型和编辑器**

```ts
export type NativeAsyncMode = 'unsupported' | 'optional' | 'required'

export interface MediaAccountModelOverride {
  upstream_model?: string
  native_async_mode?: NativeAsyncMode
}

export interface MediaAccountConfig {
  adapter: string
  native_async_mode: NativeAsyncMode
  model_overrides: Record<string, MediaAccountModelOverride>
}
```

编辑器内部使用行数组保证模型名可编辑，发出事件前过滤空模型名并转换回 Map；覆盖模式为空表示继承账号默认值：

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MediaAccountConfig, MediaAccountModelOverride, NativeAsyncMode } from '@/types'

type OverrideRow = MediaAccountModelOverride & { model: string }
const props = defineProps<{ modelValue: MediaAccountConfig }>()
const emit = defineEmits<{ 'update:modelValue': [value: MediaAccountConfig] }>()
const { t } = useI18n()
const local = ref<MediaAccountConfig>({ ...props.modelValue, model_overrides: { ...props.modelValue.model_overrides } })
const rows = ref<OverrideRow[]>([])

watch(
  () => props.modelValue.model_overrides,
  (value) => {
	local.value = { ...props.modelValue, model_overrides: { ...(value || {}) } }
    rows.value = Object.entries(value || {}).map(([model, override]) => ({ model, ...override }))
  },
  { immediate: true, deep: true },
)

function publish(patch: Partial<MediaAccountConfig> = {}) {
  const model_overrides = Object.fromEntries(
    rows.value
      .map((row) => [row.model.trim(), {
        upstream_model: row.upstream_model?.trim() || undefined,
        native_async_mode: row.native_async_mode || undefined,
      }])
      .filter(([model]) => model),
  )
  local.value = { ...local.value, ...patch, model_overrides }
  emit('update:modelValue', { ...local.value })
}

function addOverride() {
  rows.value.push({ model: '', upstream_model: '', native_async_mode: undefined })
  publish()
}

function removeOverride(index: number) {
  rows.value.splice(index, 1)
  publish()
}
</script>

<template>
  <section data-test="media-config-editor" class="space-y-4 border-t pt-4">
    <label class="block space-y-1">
      <span>{{ t('admin.accounts.mediaConfig.adapter') }}</span>
      <input
        data-test="media-adapter"
        class="input"
        :value="modelValue.adapter"
        @input="publish({ adapter: ($event.target as HTMLInputElement).value.trim() })"
      />
    </label>
    <label class="block space-y-1">
      <span>{{ t('admin.accounts.mediaConfig.nativeAsyncMode') }}</span>
      <select
        data-test="media-default-async-mode"
        class="input"
        :value="modelValue.native_async_mode"
        @change="publish({ native_async_mode: ($event.target as HTMLSelectElement).value as NativeAsyncMode })"
      >
        <option value="unsupported">unsupported</option>
        <option value="optional">optional</option>
        <option value="required">required</option>
      </select>
    </label>
    <div v-for="(row, index) in rows" :key="index" class="grid gap-2 md:grid-cols-3">
      <input v-model="row.model" :data-test="`media-override-model-${index}`" class="input" @input="publish()" />
      <input v-model="row.upstream_model" :data-test="`media-override-upstream-${index}`" class="input" @input="publish()" />
      <select v-model="row.native_async_mode" class="input" @change="publish()">
        <option :value="undefined">{{ t('admin.accounts.mediaConfig.inherit') }}</option>
        <option value="unsupported">unsupported</option>
        <option value="optional">optional</option>
        <option value="required">required</option>
      </select>
      <button type="button" @click="removeOverride(index)">{{ t('common.remove') }}</button>
    </div>
    <button data-test="media-add-model-override" type="button" @click="addOverride">
      {{ t('admin.accounts.mediaConfig.addOverride') }}
    </button>
  </section>
</template>
```

- [ ] **Step 4: 把编辑器接入账号创建和编辑**

两个 Modal 都使用以下默认值；创建时仅当 Adapter 非空才写入配置，编辑时从现有 `extra.media_config` 水合并始终保留其他 `extra` 键：

```ts
const mediaConfig = ref<MediaAccountConfig>({
  adapter: '',
  native_async_mode: 'unsupported',
  model_overrides: {},
})

function withMediaConfig(extra: Record<string, unknown> | undefined) {
  const next = { ...(extra || {}) }
  if (mediaConfig.value.adapter.trim()) {
    next.media_config = {
      adapter: mediaConfig.value.adapter.trim(),
      native_async_mode: mediaConfig.value.native_async_mode,
      model_overrides: mediaConfig.value.model_overrides,
    }
  } else {
    delete next.media_config
  }
  return next
}
```

创建和编辑 Payload 的最终 `extra` 都通过 `withMediaConfig`，不能在不同平台分支中覆盖该结果。模板在账号通用调度设置之后渲染 `<MediaConfigEditor v-model="mediaConfig" />`。中英文说明 Adapter 是协议类型，不是新供应商实体；原生异步模式可被模型覆盖，但不限制下游同步/异步选择。

- [ ] **Step 5: 运行账号前端测试和类型检查**

Run: `cd frontend && pnpm test:run src/components/account/__tests__/MediaConfigEditor.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts && pnpm typecheck`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/account/MediaConfigEditor.vue frontend/src/components/account/__tests__/MediaConfigEditor.spec.ts frontend/src/components/account/CreateAccountModal.vue frontend/src/components/account/EditAccountModal.vue frontend/src/components/account/__tests__/CreateAccountModal.spec.ts frontend/src/components/account/__tests__/EditAccountModal.spec.ts frontend/src/types/index.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(media): edit account adapter capabilities"
```

### Task 10: 增加分组图片、视频和跨平台媒体设置

**Files:**
- Create: `frontend/src/components/admin/group/GroupMediaSettings.vue`
- Create: `frontend/src/components/admin/group/__tests__/GroupMediaSettings.spec.ts`
- Create: `frontend/src/components/common/__tests__/GroupSelector.media.spec.ts`
- Modify: `frontend/src/components/common/GroupSelector.vue`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1: 写分组组件和页面贯通失败测试**

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupMediaSettings from '../GroupMediaSettings.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('GroupMediaSettings', () => {
  it('独立更新视频权限和跨平台开关', async () => {
    const wrapper = mount(GroupMediaSettings, {
      props: {
        modelValue: {
          allow_image_generation: true,
          allow_video_generation: false,
          media_cross_platform_enabled: false,
        },
      },
    })
    await wrapper.get('[data-test="allow-video-generation"]').setValue(true)
    await wrapper.get('[data-test="media-cross-platform-enabled"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({
      allow_image_generation: true,
      allow_video_generation: true,
      media_cross_platform_enabled: true,
    })
  })
})

```

`frontend/src/components/common/__tests__/GroupSelector.media.spec.ts`：

```ts
import { mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import GroupSelector from '../GroupSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const group = (overrides: Record<string, unknown>) => ({
  id: 1, name: 'group', platform: 'openai', subscription_type: 'standard',
  rate_multiplier: 1, status: 'active', allow_image_generation: false,
  allow_video_generation: false, media_cross_platform_enabled: false,
  ...overrides,
}) as any

it('shows cross-platform media groups for a different account platform', () => {
  const wrapper = mount(GroupSelector, {
    props: {
      modelValue: [],
      platform: 'gemini',
      groups: [
        group({ id: 1, name: 'gemini', platform: 'gemini', media_cross_platform_enabled: false }),
        group({ id: 2, name: 'xai-media', platform: 'openai', media_cross_platform_enabled: true }),
        group({ id: 3, name: 'openai-text', platform: 'openai', media_cross_platform_enabled: false }),
      ],
    },
    global: { stubs: { GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' } } },
  })
  expect(wrapper.text()).toContain('gemini')
  expect(wrapper.text()).toContain('xai-media')
  expect(wrapper.text()).not.toContain('openai-text')
})
```

扩展 `GroupsView.imageGeneration.spec.ts`：

```ts
it('binds all media flags in create and edit flows', () => {
  expect(typesSource).toContain('allow_video_generation: boolean')
  expect(typesSource).toContain('media_cross_platform_enabled: boolean')
  expect(viewSource).toContain('allow_video_generation: false')
  expect(viewSource).toContain('media_cross_platform_enabled: false')
  expect(viewSource).toContain('editForm.allow_video_generation = group.allow_video_generation ?? false')
  expect(viewSource).toContain('editForm.media_cross_platform_enabled = group.media_cross_platform_enabled ?? false')
  expect(viewSource).toContain('Object.assign(createForm, value)')
  expect(viewSource).toContain('Object.assign(editForm, value)')
})
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd frontend && pnpm test:run src/components/admin/group/__tests__/GroupMediaSettings.spec.ts src/components/common/__tests__/GroupSelector.media.spec.ts src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts`

Expected: FAIL，提示组件或新增字段不存在。

- [ ] **Step 3: 定义类型并实现分组媒体组件**

```ts
export interface GroupMediaConfig {
  allow_image_generation: boolean
  allow_video_generation: boolean
  media_cross_platform_enabled: boolean
}
```

`AdminGroup` 三个字段为必填 bool；`CreateGroupRequest` 和 `UpdateGroupRequest` 三个字段为可选 bool。组件必须按字段合并，避免连续触发时基于旧 Props 丢失前一次更新：

```vue
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupMediaConfig } from '@/types'

const props = defineProps<{ modelValue: GroupMediaConfig }>()
const emit = defineEmits<{ 'update:modelValue': [value: GroupMediaConfig] }>()
const { t } = useI18n()
const local = ref<GroupMediaConfig>({ ...props.modelValue })

watch(() => props.modelValue, (value) => { local.value = { ...value } }, { deep: true })

function update<K extends keyof GroupMediaConfig>(key: K, value: GroupMediaConfig[K]) {
  local.value = { ...local.value, [key]: value }
  emit('update:modelValue', { ...local.value })
}
</script>

<template>
  <section data-test="group-media-settings" class="space-y-3">
    <label class="flex items-center justify-between">
      <span>{{ t('admin.groups.allowImageGeneration') }}</span>
      <input data-test="allow-image-generation" type="checkbox" :checked="local.allow_image_generation" @change="update('allow_image_generation', ($event.target as HTMLInputElement).checked)" />
    </label>
    <label class="flex items-center justify-between">
      <span>{{ t('admin.groups.allowVideoGeneration') }}</span>
      <input data-test="allow-video-generation" type="checkbox" :checked="local.allow_video_generation" @change="update('allow_video_generation', ($event.target as HTMLInputElement).checked)" />
    </label>
    <label class="flex items-center justify-between">
      <span>{{ t('admin.groups.mediaCrossPlatformEnabled') }}</span>
      <input data-test="media-cross-platform-enabled" type="checkbox" :checked="local.media_cross_platform_enabled" @change="update('media_cross_platform_enabled', ($event.target as HTMLInputElement).checked)" />
    </label>
    <p class="text-xs text-gray-500">{{ t('admin.groups.mediaCrossPlatformHint') }}</p>
  </section>
</template>
```

- [ ] **Step 4: 接入 GroupsView 创建和编辑表单**

把原有图片开关替换为组件，防止同一字段出现两份控件：

```vue
<GroupMediaSettings
  v-model="createMediaConfig"
/>

<GroupMediaSettings
  v-model="editMediaConfig"
/>
```

```ts
const createMediaConfig = computed<GroupMediaConfig>({
  get: () => ({
    allow_image_generation: createForm.allow_image_generation,
    allow_video_generation: createForm.allow_video_generation,
    media_cross_platform_enabled: createForm.media_cross_platform_enabled,
  }),
  set: (value) => Object.assign(createForm, value),
})

const editMediaConfig = computed<GroupMediaConfig>({
  get: () => ({
    allow_image_generation: editForm.allow_image_generation,
    allow_video_generation: editForm.allow_video_generation,
    media_cross_platform_enabled: editForm.media_cross_platform_enabled,
  }),
  set: (value) => Object.assign(editForm, value),
})
```

创建表单默认三项均为 `false`；编辑时使用 `group.<field> ?? false` 水合；创建和更新请求显式包含三个字段。跨平台提示说明它只扩大媒体账号候选范围，不改变文本请求的平台边界。

`GroupSelector.filteredGroups` 保留同平台和 Antigravity Mixed Scheduling 逻辑，并额外显示 `media_cross_platform_enabled === true` 的分组：

```ts
return props.groups.filter((group) =>
  group.platform === groupPlatform.value || group.media_cross_platform_enabled === true
)
```

这个前端放宽只影响账号与分组的管理绑定；Gateway 文本调度仍按原平台过滤，媒体 Scheduler 才使用跨平台候选。

- [ ] **Step 5: 运行分组测试和类型检查**

Run: `cd frontend && pnpm test:run src/components/admin/group/__tests__/GroupMediaSettings.spec.ts src/components/common/__tests__/GroupSelector.media.spec.ts src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts && pnpm typecheck`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/admin/group/GroupMediaSettings.vue frontend/src/components/admin/group/__tests__/GroupMediaSettings.spec.ts frontend/src/components/common/GroupSelector.vue frontend/src/components/common/__tests__/GroupSelector.media.spec.ts frontend/src/views/admin/GroupsView.vue frontend/src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts frontend/src/types/index.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(media): configure group media permissions"
```

### Task 11: 定义小粒度 Media Adapter、注册表和 Fake Adapter

**Files:**
- Create: `backend/internal/service/media_adapter.go`
- Create: `backend/internal/service/media_fake_adapter.go`
- Create: `backend/internal/service/media_adapter_test.go`

- [ ] **Step 1: 写 Adapter 能力组合和 Fake 执行失败测试**

```go
func TestMediaAdapterRegistryReportsSmallCapabilities(t *testing.T) {
	registry := NewMediaAdapterRegistry()
	registry.Register("fake-sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncUnsupported,
	}))
	registry.Register("fake-async", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
	}))

	syncAdapter, err := registry.Resolve("fake-sync")
	require.NoError(t, err)
	_, supportsSync := syncAdapter.(MediaSyncGenerator)
	_, supportsSubmit := syncAdapter.(MediaAsyncSubmitter)
	require.True(t, supportsSync)
	require.False(t, supportsSubmit)

	asyncAdapter, err := registry.Resolve("fake-async")
	require.NoError(t, err)
	_, supportsSubmit = asyncAdapter.(MediaAsyncSubmitter)
	_, supportsPoll := asyncAdapter.(MediaAsyncPoller)
	require.True(t, supportsSubmit)
	require.True(t, supportsPoll)
}

func TestFakeNativeAsyncAdapterCompletesAfterPoll(t *testing.T) {
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		PollsBeforeDone:  2,
		Artifacts: []MediaArtifactInput{{MediaType: MediaTypeVideo, ContentType: "video/mp4"}},
	})
	submitter := adapter.(MediaAsyncSubmitter)
	poller := adapter.(MediaAsyncPoller)
	submission, err := submitter.Submit(context.Background(), MediaExecutionRequest{IdempotencyKey: "task_fake"})
	require.NoError(t, err)
	first, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, MediaPollStateRunning, first.State)
	second, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, MediaPollStateCompleted, second.State)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run 'TestMediaAdapterRegistry|TestFakeNativeAsyncAdapter' -count=1`

Expected: FAIL，提示 Adapter 接口和 Registry 未定义。

- [ ] **Step 3: 定义协议无关请求、结果和小接口**

```go
type MediaExecutionRequest struct {
	Task           *MediaTask
	Account        *Account
	Definition     *MediaModelDefinition
	Spec           MediaSpec
	UpstreamModel  string
	IdempotencyKey string
}

type MediaArtifactInput struct {
	Direction         string
	Position          int
	MediaType         MediaType
	ContentType       string
	Data              []byte
	ExternalURL       string
	UpstreamReference string
	Width             int
	Height            int
	DurationSeconds   float64
	Resolution        string
	FPS               float64
}

type MediaUsage struct {
	ImageCount       int
	ImageSize        string
	OutputTokens     int
	VideoSeconds     float64
	VideoResolution string
}

type MediaGenerateResult struct {
	Artifacts []MediaArtifactInput
	Usage     MediaUsage
}

type MediaAsyncSubmission struct {
	UpstreamTaskID string
	PollMetadata   json.RawMessage
}

type MediaPollState string

const (
	MediaPollStateRunning   MediaPollState = "running"
	MediaPollStateCompleted MediaPollState = "completed"
	MediaPollStateFailed    MediaPollState = "failed"
	MediaPollStateCanceled  MediaPollState = "canceled"
)

type MediaPollRequest struct {
	Account        *Account
	UpstreamTaskID string
	PollMetadata   json.RawMessage
}

type MediaPollResult struct {
	State    MediaPollState
	Progress int
	Result   *MediaGenerateResult
	Error    *MediaAdapterError
}

type MediaAdapter interface {
	Name() string
}

type MediaSyncGenerator interface {
	Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error)
}

type MediaAsyncSubmitter interface {
	Submit(ctx context.Context, req MediaExecutionRequest) (*MediaAsyncSubmission, error)
}

type MediaIdempotentSubmitter interface {
	MediaAsyncSubmitter
	SupportsIdempotentSubmit() bool
}

type MediaAsyncPoller interface {
	Poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error)
}

type MediaAborter interface {
	Abort(ctx context.Context, req MediaPollRequest) error
}

```

`MediaAdapterError` 必须带稳定 `Code`、安全 `Message`、`Retryable`、`SubmissionUnknown` 和 `SystemFailure`，Worker 据此决定账号切换、同账号轮询重试或全额退款。Adapter 不接触余额、Gin Context、Redis 或 Ent。

- [ ] **Step 4: 实现并发安全 Registry 和确定性 Fake Adapter**

```go
type MediaAdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]MediaAdapter
}

func NewMediaAdapterRegistry() *MediaAdapterRegistry {
	return &MediaAdapterRegistry{adapters: make(map[string]MediaAdapter)}
}

func (r *MediaAdapterRegistry) Register(name string, adapter MediaAdapter) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || adapter == nil {
		panic("media adapter name and implementation are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[key]; exists {
		panic("duplicate media adapter: " + key)
	}
	r.adapters[key] = adapter
}

func (r *MediaAdapterRegistry) Resolve(name string) (MediaAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, ErrMediaAdapterNotFound
	}
	return adapter, nil
}
```

Fake Adapter 的选项完整控制同步结果、轮询完成次数、提交/轮询错误和调用计数；`unsupported` 类型只实现 `MediaSyncGenerator`，`required` 类型只实现提交与轮询，`optional` 类型同时实现三者。使用三个私有包装类型实现不同 Go 方法集，不能让 `unsupported` 因底层结构碰巧拥有 `Submit` 方法。

- [ ] **Step 5: 运行 Adapter 测试**

Run: `cd backend && go test ./internal/service -run 'TestMedia(Adapter|Fake)' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add backend/internal/service/media_adapter.go backend/internal/service/media_fake_adapter.go backend/internal/service/media_adapter_test.go
git commit -m "feat(media): define provider adapter contracts"
```

### Task 12: 实现跨平台媒体候选过滤并复用当前调度语义

**Files:**
- Create: `backend/internal/service/account_candidate_selector.go`
- Create: `backend/internal/service/account_candidate_selector_test.go`
- Create: `backend/internal/service/media_scheduler.go`
- Create: `backend/internal/service/media_scheduler_test.go`

- [ ] **Step 1: 写候选过滤、负载选择和故障切换失败测试**

```go
func TestMediaSchedulerSelectsAcrossPlatformsForSameModel(t *testing.T) {
	groupID := int64(7)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{
		{ID: 1, Platform: PlatformGemini, Priority: 20, Concurrency: 2, Status: StatusActive, Extra: mediaExtra("gemini", "optional"), Credentials: modelMapping("veo-3.1", "veo-gemini")},
		{ID: 2, Platform: PlatformOpenAI, Priority: 10, Concurrency: 2, Status: StatusActive, Extra: mediaExtra("xai", "required"), Credentials: modelMapping("veo-3.1", "veo-xai")},
	}}
	selector := &accountCandidateSelectorStub{selectedID: 2}
	registry := NewMediaAdapterRegistry()
	registry.Register("gemini", NewFakeMediaAdapter(FakeMediaAdapterOptions{NativeAsyncMode: NativeAsyncOptional}))
	registry.Register("xai", NewFakeMediaAdapter(FakeMediaAdapterOptions{NativeAsyncMode: NativeAsyncRequired}))
	scheduler := NewMediaScheduler(repo, selector, registry)

	snapshot, err := scheduler.SnapshotCandidates(context.Background(), groupID, "veo-3.1")
	require.NoError(t, err)
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{
		GroupID: groupID, RequestedModel: "veo-3.1", Operation: MediaOperationTextToVideo,
		CandidateSnapshot: snapshot,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	require.Equal(t, "xai", selection.ResolvedModel.Adapter)
	require.Equal(t, "veo-xai", selection.ResolvedModel.UpstreamModel)
	require.Len(t, selector.candidates, 2)
}

func TestMediaSchedulerExcludesFailedAccountBeforeUpstreamTaskID(t *testing.T) {
	scheduler := newMediaSchedulerForTest(11, 12)
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "fake-image")
	require.NoError(t, err)
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{
		GroupID: 1, RequestedModel: "fake-image", Operation: MediaOperationTextToImage,
		ExcludedAccountIDs: map[int64]struct{}{11: {}},
		CandidateSnapshot: snapshot,
	})
	require.NoError(t, err)
	require.Equal(t, int64(12), selection.Account.ID)
}

func TestAccountCandidateSelectorUsesPriorityThenLoadAndAcquiresSlot(t *testing.T) {
	loads := &concurrencyServiceStub{loads: map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 90},
		2: {AccountID: 2, LoadRate: 20},
	}}
	selector := NewAccountCandidateSelector(loads, &gatewayCacheStub{}, testSchedulingConfig())
	result, err := selector.Select(context.Background(), AccountCandidateSelectionRequest{
		GroupID: 1,
		Candidates: []*Account{
			{ID: 1, Priority: 10, Concurrency: 2},
			{ID: 2, Priority: 10, Concurrency: 2},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), result.Account.ID)
	require.True(t, result.Acquired)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run 'TestMediaScheduler|TestAccountCandidateSelector' -count=1`

Expected: FAIL，提示 `NewMediaScheduler` 和 `NewAccountCandidateSelector` 未定义。

- [ ] **Step 3: 抽出只接收候选集合的通用账号选择器**

```go
type AccountCandidateSelectionRequest struct {
	GroupID           int64
	SessionHash       string
	Candidates        []*Account
	ExcludedAccountIDs map[int64]struct{}
}

type AccountCandidateSelector interface {
	Select(ctx context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error)
	Wait(ctx context.Context, plan *AccountWaitPlan) (release func(), err error)
}
```

默认实现复用现有 `ConcurrencyService.GetAccountsLoadBatch`/账号槽位获取、`GatewayCache` 粘性键、`AccountSelectionResult`、`AccountWaitPlan`、`sortAccountsByPriorityAndLastUsed` 和 `GatewaySchedulingConfig`。算法与当前调度链一致：

1. 粘性账号仍在候选集合、未排除且可调度时优先尝试。
2. 候选按 Priority 升序，再按负载率、等待数和 LastUsedAt 选择。
3. 每个候选在返回前原子获取账号并发槽位；成功时返回幂等 `ReleaseFunc`。
4. 无空闲槽位时为最优候选返回现有 fallback `AccountWaitPlan`。
5. Redis 负载读取失败时使用 Priority + LRU 降级，不能把请求判为上游失败。
6. 有 `SessionHash` 时，选中账号使用现有 Sticky Session TTL 写回 `GatewayCache`；候选失效或被排除时清除旧绑定。

`Wait` 复用现有 `IncrementAccountWaitCount`、周期性 `AcquireAccountSlot`、`DecrementAccountWaitCount` 和 Plan Timeout；成功返回幂等 Release，超时返回稳定的 `ErrAccountConcurrencySaturated`。MediaScheduler 暴露 `WaitForSlot(ctx, selection)` 包装该方法，供 Worker 使用。

MediaScheduler 另提供 `MarkUsed(ctx, accountID)`，调用现有 `AccountRepository.UpdateLastUsed`，从而继续驱动 Priority + LRU 公平性和 Scheduler Outbox；Worker 只在同步上游已返回或原生异步 Submit 已成功后调用。

`GetFixedAccount(ctx, accountID)` 使用 `AccountRepository.GetByID` 读取完整凭证，供已有 `upstream_task_id` 的恢复 Poll/下载使用；它不因账号后来被禁用或冷却而改选其他账号，但账号已删除或凭证不可解密时让任务按系统恢复失败全额退款。

此文件只抽取既有调度原语，不修改 OpenAI、Anthropic 或 Gemini Handler/Adapter。

- [ ] **Step 4: 实现媒体专用候选过滤和解析结果**

```go
type MediaScheduleRequest struct {
	GroupID            int64
	RequestedModel     string
	Operation          MediaOperation
	SessionHash        string
	ExcludedAccountIDs map[int64]struct{}
	CandidateSnapshot  []MediaAccountCandidateSnapshot
}

type MediaAccountCandidateSnapshot struct {
	AccountID     int64                     `json:"account_id"`
	Platform      string                    `json:"platform"`
	ResolvedModel ResolvedMediaAccountModel `json:"resolved_model"`
}

type MediaAccountSelection struct {
	Account       *Account
	ResolvedModel ResolvedMediaAccountModel
	Acquired      bool
	ReleaseFunc   func()
	WaitPlan      *AccountWaitPlan
}

func (s *MediaScheduler) SnapshotCandidates(ctx context.Context, groupID int64, requestedModel string) ([]MediaAccountCandidateSnapshot, error) {
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := make([]MediaAccountCandidateSnapshot, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if !account.IsSchedulable() || !account.IsModelSupported(requestedModel) {
			continue
		}
		resolved := account.ResolveMediaModel(requestedModel)
		adapter, adapterErr := s.adapters.Resolve(resolved.Adapter)
		if resolved.Adapter == "" || adapterErr != nil || !adapterSupportsNativeMode(adapter, resolved.NativeAsyncMode) {
			continue
		}
		result = append(result, MediaAccountCandidateSnapshot{
			AccountID: account.ID, Platform: account.Platform, ResolvedModel: resolved,
		})
	}
	if len(result) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	return result, nil
}

func (s *MediaScheduler) Select(ctx context.Context, req MediaScheduleRequest) (*MediaAccountSelection, error) {
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, req.GroupID)
	if err != nil {
		return nil, err
	}
	resolved := make(map[int64]ResolvedMediaAccountModel, len(req.CandidateSnapshot))
	for _, candidate := range req.CandidateSnapshot {
		resolved[candidate.AccountID] = candidate.ResolvedModel
	}
	candidates := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if _, excluded := req.ExcludedAccountIDs[account.ID]; excluded || !account.IsSchedulable() {
			continue
		}
		model, snapshotted := resolved[account.ID]
		if !snapshotted {
			continue
		}
		if model.Adapter == "" {
			continue
		}
		adapter, adapterErr := s.adapters.Resolve(model.Adapter)
		if adapterErr != nil || !adapterSupportsNativeMode(adapter, model.NativeAsyncMode) {
			continue
		}
		candidates = append(candidates, account)
	}
	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	selected, err := s.selector.Select(ctx, AccountCandidateSelectionRequest{
		GroupID: req.GroupID, SessionHash: req.SessionHash,
		Candidates: candidates, ExcludedAccountIDs: req.ExcludedAccountIDs,
	})
	if err != nil {
		return nil, err
	}
	return &MediaAccountSelection{
		Account: selected.Account, ResolvedModel: resolved[selected.Account.ID],
		Acquired: selected.Acquired, ReleaseFunc: selected.ReleaseFunc, WaitPlan: selected.WaitPlan,
	}, nil
}
```

`SnapshotCandidates(ctx, groupID, requestedModel)` 在创建任务时读取分组账号，按模型映射和媒体配置生成稳定的 `MediaAccountCandidateSnapshot`；空结果返回 `ErrNoAvailableAccounts`。Registry 已在 Orchestrator 验证请求操作；执行时 Scheduler 只在快照账号中选择，并仍需拒绝 Adapter 缺失、Adapter 方法集不满足已解析原生异步模式、禁用、冷却、临时不可调度和并发耗尽的账号。`unsupported` 要求同步接口，`required` 要求 Submit + Poll，`optional` 三者都要具备。任务创建后修改账号模型映射或异步模式不会改变快照；禁用/冷却仍实时生效。取得上游任务 ID后的 Worker 不再调用 Select，直接使用任务中固定的 `account_id`。

- [ ] **Step 5: 运行调度和现有账号池回归测试**

Run: `cd backend && go test ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector|SelectAccountWithLoadAwareness)' -count=1`

Expected: PASS，现有文本账号调度测试不回退。

- [ ] **Step 6: 提交**

```bash
git add backend/internal/service/account_candidate_selector.go backend/internal/service/account_candidate_selector_test.go backend/internal/service/media_scheduler.go backend/internal/service/media_scheduler_test.go
git commit -m "feat(media): schedule cross-platform account candidates"
```

### Task 13: 实现双优先级 Redis Streams 队列和终态通知

**Files:**
- Create: `backend/internal/service/media_queue.go`
- Create: `backend/internal/repository/media_task_stream.go`
- Create: `backend/internal/repository/media_task_stream_integration_test.go`
- Modify: `backend/internal/repository/wire.go`

- [ ] **Step 1: 写优先级、ACK 和通知集成失败测试**

```go
//go:build integration

func TestMediaTaskStreamPrefersSyncAndPublishesTerminal(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	stream := NewMediaTaskStream(rdb, "worker-test", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 101, service.MediaQueuePriorityAsync))
	require.NoError(t, stream.Enqueue(ctx, 202, service.MediaQueuePrioritySync))

	message, err := stream.Receive(ctx, 100*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, int64(202), message.TaskID)
	require.Equal(t, service.MediaQueuePrioritySync, message.Priority)
	require.NoError(t, stream.Ack(ctx, message))

	waitCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	done, unsubscribe, err := stream.SubscribeTerminal(waitCtx, 202)
	require.NoError(t, err)
	defer unsubscribe()
	require.NoError(t, stream.PublishTerminal(ctx, 202, service.MediaTaskStatusCompleted))
	require.Equal(t, service.MediaTaskStatusCompleted, <-done)
}

func TestMediaTaskStreamLeavesUnackedMessageRecoverable(t *testing.T) {
	ctx := context.Background()
	stream := NewMediaTaskStream(testRedis(t), "worker-a", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 303, service.MediaQueuePriorityAsync))
	message, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, message.ID)
	pending, err := stream.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, err)
	require.Equal(t, int64(1), pending)
}

func TestMediaTaskStreamDoesNotStarveAsyncQueue(t *testing.T) {
	ctx := context.Background()
	stream := NewMediaTaskStream(testRedis(t), "worker-fair", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	for id := int64(1); id <= 10; id++ {
		require.NoError(t, stream.Enqueue(ctx, id, service.MediaQueuePrioritySync))
	}
	require.NoError(t, stream.Enqueue(ctx, 99, service.MediaQueuePriorityAsync))
	seenAsync := false
	for i := 0; i < 9; i++ {
		message, err := stream.Receive(ctx, time.Second)
		require.NoError(t, err)
		seenAsync = seenAsync || message.TaskID == 99
		require.NoError(t, stream.Ack(ctx, message))
	}
	require.True(t, seenAsync)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test -tags=integration ./internal/repository -run TestMediaTaskStream -count=1`

Expected: FAIL，提示 `NewMediaTaskStream` 未定义。

- [ ] **Step 3: 定义队列和通知端口**

```go
type MediaQueuePriority string

const (
	MediaQueuePrioritySync  MediaQueuePriority = "sync"
	MediaQueuePriorityAsync MediaQueuePriority = "async"
)

type MediaQueueMessage struct {
	ID       string
	TaskID   int64
	Priority MediaQueuePriority
}

type MediaTaskQueue interface {
	EnsureGroups(ctx context.Context) error
	Enqueue(ctx context.Context, taskID int64, priority MediaQueuePriority) error
	Receive(ctx context.Context, block time.Duration) (*MediaQueueMessage, error)
	Ack(ctx context.Context, message *MediaQueueMessage) error
	PublishTerminal(ctx context.Context, taskID int64, status MediaTaskStatus) error
	SubscribeTerminal(ctx context.Context, taskID int64) (<-chan MediaTaskStatus, func(), error)
}
```

`SubscribeTerminal` 只是低延迟通知，调用方必须先订阅再读取数据库；收到通知或 Pub/Sub 断开后仍重新读取数据库状态，不能把通知负载当作事实源。

- [ ] **Step 4: 实现 Redis Streams**

使用固定键 `media:tasks:sync`、`media:tasks:async`，Consumer Group `media-workers`，终态 Channel `media:task:<id>:terminal`。`Enqueue` 使用 `XADD MAXLEN ~`；`Receive` 先用 `XAUTOCLAIM` 领取空闲时间超过租约的 Pending 消息，再对同步 Stream 做非阻塞 `XREADGROUP`，无消息才以短 Block 同时读取两条 Stream，并在返回多条时优先同步消息。为避免异步永久饥饿，每个 Consumer 连续处理 8 条同步消息后必须优先尝试 1 条异步消息。只有 Worker 完成数据库推进后才 `XACK`。

```go
func (s *MediaTaskStream) Enqueue(ctx context.Context, taskID int64, priority service.MediaQueuePriority) error {
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: s.streamKey(priority),
		MaxLen: 100000,
		Approx: true,
		Values: map[string]any{"task_id": taskID},
	}).Err()
}

func (s *MediaTaskStream) PublishTerminal(ctx context.Context, taskID int64, status service.MediaTaskStatus) error {
	return s.rdb.Publish(ctx, terminalChannel(taskID), string(status)).Err()
}
```

`EnsureGroups` 将 `BUSYGROUP` 视为成功。构造 Provider 使用主机名加随机后缀生成 Consumer Name，避免多实例冲突。

- [ ] **Step 5: 运行队列集成测试**

Run: `cd backend && go test -tags=integration ./internal/repository -run TestMediaTaskStream -count=1`

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add backend/internal/service/media_queue.go backend/internal/repository/media_task_stream.go backend/internal/repository/media_task_stream_integration_test.go backend/internal/repository/wire.go
git commit -m "feat(media): queue tasks by request priority"
```

### Task 14: 实现 Worker、租约续期、恢复扫描和执行矩阵

**Files:**
- Create: `backend/internal/service/media_billing.go`
- Create: `backend/internal/service/media_billing_test.go`
- Create: `backend/internal/service/media_worker.go`
- Create: `backend/internal/service/media_worker_test.go`
- Create: `backend/internal/service/media_metrics.go`
- Create: `backend/internal/service/media_metrics_test.go`
- Create: `backend/internal/repository/media_worker_integration_test.go`

- [ ] **Step 1: 写同步/异步矩阵、重复消息和恢复失败测试**

```go
func TestMediaWorkerExecutionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		clientAsync bool
		mode        NativeAsyncMode
		wantSync    int
		wantSubmit  int
	}{
		{"sync_unsupported", false, NativeAsyncUnsupported, 1, 0},
		{"sync_optional", false, NativeAsyncOptional, 1, 0},
		{"sync_required", false, NativeAsyncRequired, 0, 1},
		{"async_unsupported", true, NativeAsyncUnsupported, 1, 0},
		{"async_optional", true, NativeAsyncOptional, 0, 1},
		{"async_required", true, NativeAsyncRequired, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, tt.clientAsync, tt.mode)
			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Equal(t, tt.wantSync, fixture.adapter.SyncCalls())
			require.Equal(t, tt.wantSubmit, fixture.adapter.SubmitCalls())
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.MustGet(fixture.task.ID).Status)
		})
	}
}

func TestMediaWorkerIgnoresDuplicateTerminalMessage(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, 1, fixture.adapter.SubmitCalls())
	require.Equal(t, 1, fixture.billing.SettlementCalls())
}

func TestMediaWorkerRecoverOnceRequeuesExpiredLease(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.SetExpiredLease(fixture.task.ID, "dead-worker")
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.EnqueuedTaskIDs())
}

func TestMediaWorkerRecordsRecoveryAndDuplicateMetrics(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	fixture.repo.SetExpiredLease(fixture.task.ID, "dead-worker")
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, int64(1), fixture.metrics.DuplicateMessages())
	require.Equal(t, int64(1), fixture.metrics.Recoveries())
}

func TestMediaWorkerStorageFailureAlwaysRefunds(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.artifactWriter.err = errors.New("object storage unavailable")
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	stored := fixture.repo.MustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_storage", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	}, fixture.billing.LastFailure())
}

func TestMediaWorkerUpstreamCanceledAlwaysRefunds(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.SetPollResult(MediaPollResult{State: MediaPollStateCanceled})
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: "upstream_canceled",
	}, fixture.billing.LastFailure())
}

func TestMediaWorkerRenewsLeaseWhilePolling(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 10 * time.Millisecond
	fixture.adapter.BlockPollUntil(fixture.releasePoll)
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	require.Eventually(t, func() bool { return fixture.repo.RenewLeaseCalls() >= 1 }, time.Second, 10*time.Millisecond)
	close(fixture.releasePoll)
	require.NoError(t, <-done)
}

func TestMediaBillingCoordinatorRetriesPersistedSettlementIdempotently(t *testing.T) {
	repo := newMediaTaskRepoWithCompletedTask(t)
	port := &recordingMediaBilling{failFirstSettlement: true}
	coordinator := NewMediaBillingCoordinator(repo, port)
	task := repo.CompletedTask()
	err := coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	})
	require.Error(t, err)
	require.Equal(t, "retry", repo.MustGet(task.ID).BillingStatus)
	require.NoError(t, coordinator.RetryPending(context.Background(), task.ID))
	require.NoError(t, coordinator.RetryPending(context.Background(), task.ID))
	require.Equal(t, "settled", repo.MustGet(task.ID).BillingStatus)
	require.Equal(t, 1, port.SuccessfulSettlementCalls())
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run TestMediaWorker -count=1`

Expected: FAIL，提示 `MediaWorker` 未定义。

- [ ] **Step 3: 实现领取、续租、终止控制和执行路径选择**

```go
type MediaWorkerConfig struct {
	WorkerCount       int
	TaskTimeout       time.Duration
	LeaseTTL          time.Duration
	LeaseRenewInterval time.Duration
	PollInterval      time.Duration
	RecoveryInterval  time.Duration
	RecoveryBatchSize int
}

type MediaExecutionPath string

const (
	MediaExecutionPathSync        MediaExecutionPath = "sync"
	MediaExecutionPathNativeAsync MediaExecutionPath = "native_async"
)

type MediaBillingSnapshot struct {
	RequestedModel  string          `json:"requested_model"`
	CandidateModels []string        `json:"candidate_models"`
	EstimatedAmount float64         `json:"estimated_amount"`
	GroupMultiplier float64         `json:"group_multiplier"`
	PricingSnapshot json.RawMessage `json:"pricing_snapshot"`
}

type MediaFailureKind string

const (
	MediaFailureKindUpstream    MediaFailureKind = "upstream"
	MediaFailureKindSystem      MediaFailureKind = "system"
	MediaFailureKindSyncTimeout MediaFailureKind = "sync_timeout"
)

type MediaFailureSettlement struct {
	Kind         MediaFailureKind
	RefundRatio  float64
	PenaltyRatio float64
	ErrorCode    string
}

type MediaBillingPort interface {
	Precharge(ctx context.Context, task *MediaTask, snapshot MediaBillingSnapshot) error
	SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error
	SettleFailure(ctx context.Context, task *MediaTask, settlement MediaFailureSettlement) error
}

type MediaSettlementType string

const (
	MediaSettlementTypeSuccess MediaSettlementType = "success"
	MediaSettlementTypeFailure MediaSettlementType = "failure"
)

type MediaSettlementPlan struct {
	Type    MediaSettlementType     `json:"type"`
	Usage   *MediaUsage             `json:"usage,omitempty"`
	Failure *MediaFailureSettlement `json:"failure,omitempty"`
}

type MediaSettlementCoordinator interface {
	SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error
	SettleFailure(ctx context.Context, task *MediaTask, settlement MediaFailureSettlement) error
	RetryPending(ctx context.Context, taskID int64) error
}

type MediaExecutionController interface {
	StopForSyncTimeout(taskID int64) bool
}

type MediaTaskMetrics interface {
	ObserveStage(mediaType MediaType, stage MediaTaskStage, elapsed time.Duration)
	IncrementRecovery(mediaType MediaType)
	IncrementDuplicateMessage(mediaType MediaType)
	IncrementStorageFailure(mediaType MediaType)
	IncrementSettlementRetry(mediaType MediaType)
}

type MediaArtifactWriter interface {
	PersistOutputs(ctx context.Context, task *MediaTask, inputs []MediaArtifactInput) ([]MediaArtifact, error)
}

func chooseMediaExecutionPath(clientAsync bool, mode NativeAsyncMode) MediaExecutionPath {
	if mode == NativeAsyncRequired || (clientAsync && mode == NativeAsyncOptional) {
		return MediaExecutionPathNativeAsync
	}
	return MediaExecutionPathSync
}
```

`ProcessOne` 的顺序固定为：读取任务 → 若执行已终态则仅重试未完成结算 → CAS Claim → 注册可终止 Context → 启动续租 → 状态推进为 `in_progress/scheduling` → 选择或恢复固定账号 → 若 Selection 未取得槽位则调用 `MediaScheduler.WaitForSlot` → 执行 Adapter → 调用 `MediaArtifactWriter.PersistOutputs` 保存产物 → CAS 写入执行终态 → 由 `MediaSettlementCoordinator` 持久化 Plan 并尝试结算 → 发布终态。账单失败不能让执行状态回退；所有退出路径都释放账号槽位、停止续租并移除执行 Context。Task 14 的单元测试注入内存 Artifact Writer；Task 16 提供混合存储实现。

`StopForSyncTimeout` 只供 Orchestrator 的“同步等待超时且自动转异步关闭”路径调用：对同步 Generate 取消请求 Context；对已有上游任务 ID且 Adapter 实现 `MediaAborter` 的任务，用固定账号调用 `Abort`，随后停止 Poll。Abort 不支持或失败时仍停止本地交付并让 Orchestrator 按同步超时策略结算；该方法不暴露给任何用户或管理员 Handler。

`media_metrics.go` 提供无锁原子计数和阶段耗时聚合；Worker 的结构化日志固定包含 `task_id`、媒体类型、操作、请求/上游模型、Adapter、平台、账号、分组、重试数、Poll 次数、各阶段耗时和错误类别，绝不记录 Prompt、凭证、上游 URL或 `upstream_reference`。

原生异步路径在成功提交后，必须先持久化 `account_id`、`adapter`、`upstream_model`、`native_async_mode`、`upstream_task_id`、`poll_metadata`、`submitted_at` 和 `stage=polling`，再开始 Poll。进程恢复时只要任务已有 `upstream_task_id` 就跳过 Scheduler 和 Submit。

首次调度必须从任务 `candidate_snapshot` 解码 `MediaAccountCandidateSnapshot` 传给 Scheduler，不能重新解析账号当前的模型映射或异步模式；实时禁用、冷却、负载和并发状态仍在 Select 时检查。

同步 Generate 返回或原生异步 Submit 成功后调用 `MediaScheduler.MarkUsed`；失败只记录日志，不回滚已接受的上游任务。

- [ ] **Step 4: 实现错误分类、恢复限制和至少一次安全性**

```go
func (w *MediaWorker) mayResubmit(task *MediaTask, adapter MediaAdapter, err *MediaAdapterError) bool {
	if task.UpstreamTaskID != "" {
		return false
	}
	if err == nil || !err.Retryable {
		return false
	}
	if err.SubmissionUnknown {
		idempotent, supportsIdempotency := adapter.(MediaIdempotentSubmitter)
		return supportsIdempotency && idempotent.SupportsIdempotentSubmit()
	}
	return true
}
```

- 明确拒绝且可重试时，在没有上游任务 ID前把账号加入排除集合并重新调度。
- 提交结果不确定且 Adapter 不声明 `MediaIdempotentSubmitter` 时失败并全额退款。
- Poll 临时错误保留固定账号和任务 ID重试；上游 canceled 映射 `failed/upstream_canceled` 并全额退款。
- 从任务 `started_at` 起超过部署级 `TaskTimeout` 映射 `failed/system_timeout` 并全额退款，不使用同步超时惩罚策略。
- 存储失败映射系统错误并全额退款。
- `Transition`、结算端口和产物写入均以任务 ID/终态条件幂等；重复 Redis 消息不能再次提交或结算。

恢复扫描器调用 `ListRecoverable(now, batch)`，按 `!ClientAsync && !SyncFallback` 重新进入同步高优先级，否则进入普通优先级。Worker 停机 Context 取消不能把在途任务误标失败，租约过期后由其他节点恢复。

Consumer 只有在 `ProcessOne` 已确认任务终态、已完成一次安全状态推进，或发现消息对应任务已终态时才 ACK；数据库/Redis 暂时错误和进程停机取消保持 Pending，由 `XAUTOCLAIM` 或恢复扫描重新交付。

`MediaBillingCoordinator` 在调用余额端口前先把不可变 `MediaSettlementPlan` 持久化并把 `billing_status` CAS 为 `settling`；端口成功后写 `settled` 和金额，失败写 `retry`。Worker 遇到执行终态但账单未终态时只调用 `RetryPending`，不重新执行 Adapter。恢复扫描把 `ListSettlementPending` 返回的任务投递普通优先级；端口和 Coordinator 都以 `task.PublicID + plan.Type` 幂等。

- [ ] **Step 5: 添加 PostgreSQL + Redis + Fake Adapter 集成测试**

集成测试放在 `repository` 包，从而复用现有 `testEntClient(t)` 和 `testRedis(t)` Harness：创建任务并投递两次，启动一个 Worker，等待终态后断言只有一个产物、一笔结算和一次 Adapter 提交；另一个用例在原生异步提交后停止首个 Worker、人工过期租约，再由第二个 Worker 恢复 Poll，断言未重复 Submit。

```go
//go:build integration

func TestMediaWorkerIntegrationDuplicateDeliverySettlesOnce(t *testing.T) {
	client := testEntClient(t)
	queue := NewMediaTaskStream(testRedis(t), "worker-integration", time.Second)
	require.NoError(t, queue.EnsureGroups(context.Background()))
	taskRepo := NewMediaTaskRepository(client)
	artifactRepo := NewMediaArtifactRepository(client)
	fixture := newIntegrationMediaWorker(t, queue, taskRepo, artifactRepo, service.NativeAsyncRequired)
	task := fixture.CreateQueuedTask(t, true)
	require.NoError(t, queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))
	require.NoError(t, queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))
	require.NoError(t, fixture.Worker.Start())
	t.Cleanup(fixture.Worker.Stop)
	fixture.WaitForStatus(t, task.ID, service.MediaTaskStatusCompleted)
	require.Equal(t, 1, fixture.Adapter.SubmitCalls())
	require.Equal(t, 1, fixture.Billing.SettlementCalls())
	artifacts, err := artifactRepo.ListByTaskID(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
}

func TestMediaWorkerIntegrationResumesPollWithoutResubmit(t *testing.T) {
	fixture := newRecoverableIntegrationMediaWorker(t)
	task := fixture.CreateSubmittedExpiredTask(t, "upstream-123")
	require.NoError(t, fixture.Worker.RecoverOnce(context.Background()))
	require.NoError(t, fixture.Worker.ProcessOne(context.Background(), task.ID))
	require.Equal(t, 0, fixture.Adapter.SubmitCalls())
	require.GreaterOrEqual(t, fixture.Adapter.PollCalls(), 1)
	require.Equal(t, service.MediaTaskStatusCompleted, fixture.MustGetTask(t, task.ID).Status)
}
```

Run: `cd backend && go test -tags=integration ./internal/repository -run TestMediaWorkerIntegration -count=1`

Expected: PASS。

- [ ] **Step 6: 运行 Worker 单元测试**

Run: `cd backend && go test ./internal/service -run TestMediaWorker -count=1`

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add backend/internal/service/media_billing.go backend/internal/service/media_billing_test.go backend/internal/service/media_worker.go backend/internal/service/media_worker_test.go backend/internal/service/media_metrics.go backend/internal/service/media_metrics_test.go backend/internal/repository/media_worker_integration_test.go
git commit -m "feat(media): execute recoverable media tasks"
```

### Task 15: 实现 Billing Port、Orchestrator、同步等待和超时策略

**Files:**
- Modify: `backend/internal/service/media_billing.go`
- Modify: `backend/internal/service/media_billing_test.go`
- Create: `backend/internal/service/media_orchestrator.go`
- Create: `backend/internal/service/media_orchestrator_test.go`

- [ ] **Step 1: 写异步返回、同步等待、0 秒和超时结算失败测试**

```go
func TestMediaOrchestratorAsyncReturnsAfterDurableEnqueue(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	result, err := fixture.orchestrator.Create(context.Background(), MediaCreateRequest{
		UserID: 1, APIKeyID: 2, GroupID: 3,
		MediaType: MediaTypeImage, Operation: MediaOperationTextToImage,
		RequestedModel: "fake-image", Spec: validImageMediaSpec(), ClientAsync: true,
	})
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionAccepted, result.Disposition)
	require.Equal(t, MediaQueuePriorityAsync, fixture.queue.LastPriority())
	require.Equal(t, 1, fixture.billing.PrechargeCalls())
}

func TestMediaOrchestratorSyncTimeoutFallbackKeepsTaskRunning(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.MediaSyncWaitTimeoutSeconds = 1
	fixture.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	fixture.clock.AdvanceOnWait(time.Second)
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, result.Disposition)
	require.True(t, fixture.repo.MustGet(result.Task.ID).SyncFallback)
	require.Equal(t, 0, fixture.billing.SettleFailureCalls())
	require.Equal(t, 0, fixture.controller.StopCalls())
}

func TestMediaOrchestratorSyncTimeoutBeforeSubmitAlwaysRefunds(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.settings.MediaSyncTimeoutBillingPolicy = MediaTimeoutBillingPolicyPenalty
	fixture.settings.MediaSyncTimeoutPenaltyRatio = 0.8
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, result.Disposition)
	require.Equal(t, MediaFailureSettlement{Kind: MediaFailureKindSyncTimeout, RefundRatio: 1}, fixture.billing.LastFailure())
}

func TestMediaOrchestratorSyncTimeoutAfterSubmitAppliesConfiguredPenalty(t *testing.T) {
	submittedAt := time.Now()
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, &submittedAt)
	fixture.settings.MediaSyncTimeoutBillingPolicy = MediaTimeoutBillingPolicyPenalty
	fixture.settings.MediaSyncTimeoutPenaltyRatio = 0.8
	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSyncTimeout, RefundRatio: 0.2, PenaltyRatio: 0.8,
	}, fixture.billing.LastFailure())
}

func TestMediaOrchestratorZeroTimeoutHasNoApplicationTimer(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.MediaSyncWaitTimeoutSeconds = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fixture.orchestrator.Create(ctx, validSyncMediaCreateRequest())
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 0, fixture.clock.NewTimerCalls())
	require.Equal(t, 0, fixture.controller.StopCalls())
	require.Equal(t, 0, fixture.billing.SettleFailureCalls())
}

func TestMediaOrchestratorIdempotencyKeyReusesTaskWithoutSecondCharge(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := MediaCreateRequest{
		UserID: 1, APIKeyID: 2, GroupID: 3,
		MediaType: MediaTypeImage, Operation: MediaOperationTextToImage,
		RequestedModel: "fake-image", Spec: validImageMediaSpec(), ClientAsync: true,
		IdempotencyKey: "idem-1",
	}
	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.Task.PublicID, second.Task.PublicID)
	require.Equal(t, 1, fixture.billing.PrechargeCalls())

	req.Spec = anotherValidImageMediaSpec()
	_, err = fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaIdempotencyConflict)
}

func TestMediaOrchestratorRejectsContentPolicyBeforeTaskAndCharge(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.contentPolicy.err = ErrMediaContentRejected
	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.ErrorIs(t, err, ErrMediaContentRejected)
	require.Equal(t, 0, fixture.repo.CreateCalls())
	require.Equal(t, 0, fixture.billing.PrechargeCalls())
}

func TestMediaOrchestratorPersistsResolvedCandidateSnapshot(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.scheduler.candidates = []MediaAccountCandidateSnapshot{{
		AccountID: 7, Platform: PlatformGemini,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: "gemini", UpstreamModel: "veo-3.1-generate", NativeAsyncMode: NativeAsyncRequired,
		},
	}}
	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	var snapshot []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(result.Task.CandidateSnapshot, &snapshot))
	require.Equal(t, fixture.scheduler.candidates, snapshot)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service -run 'TestMedia(Orchestrator|Billing)' -count=1`

Expected: FAIL，提示 Orchestrator、创建结果或禁用计费实现未定义。

- [ ] **Step 3: 完成幂等计费约束和本阶段禁用实现**

```go
type DisabledMediaBilling struct{}

func (DisabledMediaBilling) Precharge(context.Context, *MediaTask, MediaBillingSnapshot) error {
	return nil
}

func (DisabledMediaBilling) SettleSuccess(context.Context, *MediaTask, MediaUsage) error {
	return nil
}

func (DisabledMediaBilling) SettleFailure(context.Context, *MediaTask, MediaFailureSettlement) error {
	return nil
}
```

Task 14 已建立 Worker 所需的 Billing Port。本步骤固定所有实现以 `task.PublicID + settlement_type` 作为幂等键。`DisabledMediaBilling` 的三个方法不修改余额但返回成功，用于第一阶段未承载生产流量时安全启动；测试用 `RecordingMediaBilling` 对相同幂等键只记录一次，并验证退款比例与惩罚比例之和为 1。

- [ ] **Step 4: 定义 Orchestrator 输入、结果和运行时依赖**

```go
type MediaCreateRequest struct {
	UserID         int64
	APIKeyID       int64
	GroupID        int64
	MediaType      MediaType
	Operation      MediaOperation
	RequestedModel string
	Spec           MediaSpec
	Inputs         []MediaArtifactInput
	ClientAsync    bool
	SessionHash    string
	IdempotencyKey string
}

type MediaCreateDisposition string

const (
	MediaCreateDispositionCompleted     MediaCreateDisposition = "completed"
	MediaCreateDispositionFailed        MediaCreateDisposition = "failed"
	MediaCreateDispositionAccepted      MediaCreateDisposition = "accepted"
	MediaCreateDispositionFallbackAsync MediaCreateDisposition = "fallback_async"
	MediaCreateDispositionGatewayTimeout MediaCreateDisposition = "gateway_timeout"
)

type MediaCreateResult struct {
	Task        *MediaTask
	Artifacts   []MediaArtifact
	Disposition MediaCreateDisposition
}

type MediaSettingsProvider interface {
	GetAllSettings(ctx context.Context) (*SystemSettings, error)
}

type MediaContentPolicy interface {
	Check(ctx context.Context, userID int64, mediaType MediaType, spec MediaSpec) error
}

type AllowAllMediaContentPolicy struct{}
func (AllowAllMediaContentPolicy) Check(context.Context, int64, MediaType, MediaSpec) error { return nil }

type MediaPricingPort interface {
	Snapshot(ctx context.Context, req MediaCreateRequest, definition *MediaModelDefinition, candidates []MediaAccountCandidateSnapshot) (MediaBillingSnapshot, error)
}

type ZeroMediaPricing struct{}
func (ZeroMediaPricing) Snapshot(_ context.Context, req MediaCreateRequest, _ *MediaModelDefinition, _ []MediaAccountCandidateSnapshot) (MediaBillingSnapshot, error) {
	return MediaBillingSnapshot{RequestedModel: req.RequestedModel}, nil
}

type MediaTimer interface {
	Channel() <-chan time.Time
	Stop() bool
}

type MediaClock interface {
	Now() time.Time
	NewTimer(d time.Duration) MediaTimer
}

type realMediaTimer struct{ timer *time.Timer }
func (t realMediaTimer) Channel() <-chan time.Time { return t.timer.C }
func (t realMediaTimer) Stop() bool                 { return t.timer.Stop() }

type realMediaClock struct{}
func (realMediaClock) Now() time.Time { return time.Now() }
func (realMediaClock) NewTimer(d time.Duration) MediaTimer {
	return realMediaTimer{timer: time.NewTimer(d)}
}
```

`Create` 必须按顺序执行：验证规格和 Registry 能力 → 校验 Group 图片/视频权限及输入引用可恢复性（`Inputs` 不得再含原始 Data）→ 调用 `MediaContentPolicy.Check` → 计算规范化请求指纹 → 若有 Idempotency Key，按 `(user_id, api_key_id, key)` 查询并复用同指纹任务、拒绝不同指纹 → 调用 `MediaScheduler.SnapshotCandidates` 固化账号/Adapter/上游模型/异步模式 → 通过 `MediaPricingPort` 结合候选快照生成随机 `task_` 公共 ID和定价快照 → 先以 `billing_status=pending` 创建任务行 → 把已暂存输入写成 `direction=input` Artifact，并通过 `UpdateQueued` CAS 把 ID写回 RequestSpec → 幂等预扣，再通过 `UpdateQueued` CAS 写入 `precharged` → 投递队列。第一阶段注入 `AllowAllMediaContentPolicy` 和零金额 Pricing，真实 Adapter 子项目把现有渠道价格解析适配到图片数量/尺寸/Token和视频秒数/分辨率单位。Multipart/URL 输入由 Handler 调用 Task 16 的 `MediaInputStager` 在进入 `Create` 前变成对象 Key或经 SSRF 校验的内部引用。唯一索引冲突时重新读取并执行同样的指纹比较，竞争失败方不能调用预扣。预扣失败把任务转为 `failed/billing_precharge`；任何“任务已写 DB但入队失败”的情况转为 `failed/system_queue` 并全额退款；恢复扫描仍可看到任务，但终态会阻止执行。

- [ ] **Step 5: 实现同步等待和竞态安全的超时决策**

```go
func (o *MediaOrchestrator) waitSync(ctx context.Context, task *MediaTask, settings *SystemSettings) (*MediaCreateResult, error) {
	terminal, unsubscribe, err := o.queue.SubscribeTerminal(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	defer unsubscribe()

	var timeout <-chan time.Time
	var timer MediaTimer
	if settings.MediaSyncWaitTimeoutSeconds > 0 {
		timer = o.clock.NewTimer(time.Duration(settings.MediaSyncWaitTimeoutSeconds) * time.Second)
		timeout = timer.Channel()
		defer timer.Stop()
	}

	for {
		current, err := o.tasks.GetByID(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		if current.Status == MediaTaskStatusCompleted {
			artifacts, err := o.artifacts.ListByTaskID(ctx, current.ID)
			return &MediaCreateResult{Task: current, Artifacts: artifacts, Disposition: MediaCreateDispositionCompleted}, err
		}
		if current.Status == MediaTaskStatusFailed {
			return &MediaCreateResult{Task: current, Disposition: MediaCreateDispositionFailed}, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return o.handleSyncTimeout(context.WithoutCancel(ctx), current, settings)
		case _, ok := <-terminal:
			if !ok {
				current, readErr := o.tasks.GetByID(ctx, task.ID)
				if readErr == nil && current.Status.IsTerminal() {
					continue
				}
				return nil, ErrMediaTerminalSubscriptionClosed
			}
			// 通知只负责唤醒；下一轮重新读取 DB。
		}
	}
}
```

订阅必须发生在首次读取 DB之前，从而关闭“先完成后订阅”的竞态窗口；若 Pub/Sub Channel 意外关闭，立即再读一次 DB并返回可重试的系统错误，不能无限空转。最大等待时间 `0` 时 `timeout` 永远为 nil，只受客户端/反向代理 Context 影响；客户端断开不等于应用级同步超时，不停止任务、不惩罚扣费。

`handleSyncTimeout` 先重读任务避免“完成与计时器同时触发”误判：

- 开关开启：CAS 写入 `sync_fallback=true` 和 `sync_fallback_at`，返回 `FallbackAsync`；不停止、不重新入队、不结算。
- 开关关闭：先 CAS 转为 `failed/sync_timeout`；CAS 失败时重读任务并按真实终态返回，不能和 Worker 同时做成功/失败结算。CAS 成功后调用 `StopForSyncTimeout`，再由 `MediaSettlementCoordinator.SettleFailure` 持久化并执行结算。只有 `submitted_at != nil` 且阶段为 `submitting/generating/polling` 才允许按策略惩罚；`queued/scheduling/storing/settling`、队列/Worker/存储错误均全额退款。
- `refund` 策略始终 `RefundRatio=1`；`penalty` 策略使用 `PenaltyRatio` 和 `1-PenaltyRatio`。失败结算后返回 `GatewayTimeout`，不把公共任务 ID交付给下游。

结算端口暂时失败不改变 504/202 的 HTTP 决策；Coordinator 已把 Plan 和 `billing_status=retry` 写入数据库，Worker 的结算恢复扫描后续重试。

`GetForUser` 只调用 `GetByPublicIDForUser` 和 `ListByTaskID`，把仓储 NotFound 与归属不匹配统一映射为 `ErrMediaTaskNotFound`；它不返回账号、上游任务 ID、Poll Metadata 或 Billing Snapshot。

- [ ] **Step 6: 运行计费和 Orchestrator 测试**

Run: `cd backend && go test ./internal/service -run 'TestMedia(Orchestrator|Billing)' -count=1`

Expected: PASS；覆盖全额退款、默认 80% 扣费、自动转异步和 0 秒语义。

- [ ] **Step 7: 提交**

```bash
git add backend/internal/service/media_billing.go backend/internal/service/media_billing_test.go backend/internal/service/media_orchestrator.go backend/internal/service/media_orchestrator_test.go
git commit -m "feat(media): orchestrate sync and async requests"
```

### Task 16: 实现未挂生产路由的媒体 API 契约和安全视频内容读取

**Files:**
- Create: `backend/internal/service/media_content.go`
- Create: `backend/internal/service/media_content_test.go`
- Modify: `backend/internal/service/media_adapter.go`
- Create: `backend/internal/repository/media_http_content.go`
- Create: `backend/internal/repository/media_http_content_test.go`
- Create: `backend/internal/handler/media_task_handler.go`
- Create: `backend/internal/handler/media_task_handler_test.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/handler/handler.go`

- [ ] **Step 1: 写创建、归属、Range、SSRF 和无取消接口失败测试**

```go
func TestMediaTaskHandlerAsyncVideoReturns202(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{"model":"fake-video","prompt":"sunset","async":true}`, 42)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.JSONEq(t, `{
		"id":"task_public","object":"media.task","media_type":"video",
		"operation":"text_to_video","model":"fake-video","status":"queued",
		"progress":0,"created_at":1784112000
	}`, rec.Body.String())
	require.True(t, app.lastCreate.ClientAsync)
}

func TestMediaTaskHandlerSyncImageKeepsOpenAIResponseShape(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = completedImageCreateResult("https://cdn.example/image.png")
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"created":1784112000,"data":[{"url":"https://cdn.example/image.png"}]}`, rec.Body.String())
}

func TestMediaTaskHandlerImageEditMultipartAsync(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	body, contentType := imageEditMultipartBody(t, map[string]string{
		"model": "fake-edit", "prompt": "add a hat", "async": "true",
	}, "image", []byte("fake-png"))
	req := newAPIKeyRequest(http.MethodPost, "/v1/images/edits", body, 42)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Equal(t, service.MediaOperationImageEdit, app.lastCreate.Operation)
	require.True(t, app.lastCreate.ClientAsync)
	require.NotEmpty(t, app.lastCreate.Inputs)
}

func TestMediaTaskHandlerGatewayTimeoutDoesNotExposeTaskID(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = gatewayTimeoutCreateResult("task_secret")
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{"model":"fake-video","prompt":"sunset"}`, 42)
	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
	require.NotContains(t, rec.Body.String(), "task_secret")
}

func TestMediaTaskHandlerHidesTaskFromDifferentUser(t *testing.T) {
	router, _ := newStandaloneMediaRouter(t)
	rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/videos/task_other", "", 99)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "owner")
}

func TestMediaTaskHandlerVideoContentForwardsRangeWithoutLeakingUpstream(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.content = &service.MediaContent{
		Body: io.NopCloser(strings.NewReader("2345")), StatusCode: http.StatusPartialContent,
		ContentType: "video/mp4", ContentLength: 4, ContentRange: "bytes 2-5/10", AcceptRanges: "bytes",
	}
	req := newAuthenticatedRequest(http.MethodGet, "/v1/videos/task_public/content", "", 42)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, "bytes 2-5/10", rec.Header().Get("Content-Range"))
	require.Equal(t, "2345", rec.Body.String())
	require.NotContains(t, rec.Header().Get("Location"), "upstream")
}

func TestMediaRouterDoesNotExposeCancel(t *testing.T) {
	router, _ := newStandaloneMediaRouter(t)
	rec := performAuthenticatedRequest(router, http.MethodDelete, "/v1/videos/task_public", "", 42)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMediaHTTPContentReaderRejectsPrivateAddress(t *testing.T) {
	reader := NewMediaHTTPContentReader(&httpUpstreamNeverCalled{}, mediaContentTestConfig())
	_, err := reader.Open(context.Background(), service.MediaHTTPContentRequest{
		URL: "http://127.0.0.1/private.mp4", Account: &service.Account{ID: 1, Concurrency: 1},
	})
	require.Error(t, err)
}

func TestMediaContentServiceDecodesDataURLAndAppliesRange(t *testing.T) {
	svc := newMediaContentServiceWithArtifact(t, "data:video/mp4;base64,MDEyMzQ1Njc4OQ==")
	content, err := svc.OpenVideo(context.Background(), "task_public", 42, "bytes=2-5")
	require.NoError(t, err)
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "bytes 2-5/10", content.ContentRange)
	require.Equal(t, []byte("2345"), body)
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)' -count=1`

Expected: FAIL，提示内容服务、HTTP Reader 和 Handler 未定义。

- [ ] **Step 3: 定义视频内容与输入暂存端口**

```go
type MediaContent struct {
	Body          io.ReadCloser
	StatusCode    int
	ContentType   string
	ContentLength int64
	ContentRange  string
	AcceptRanges  string
}

type MediaHTTPContentRequest struct {
	URL       string
	Headers   http.Header
	Account   *Account
	ByteRange string
}

type MediaHTTPContentReader interface {
	ValidateURL(raw string) (string, error)
	Open(ctx context.Context, req MediaHTTPContentRequest) (*MediaContent, error)
}

type MediaContentFetcher interface {
	OpenContent(ctx context.Context, account *Account, artifact *MediaArtifact, byteRange string) (*MediaContent, error)
}

type MediaArtifactObjectStore interface {
	Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error)
	Open(ctx context.Context, artifact *MediaArtifact, byteRange string) (*MediaContent, error)
}

type MediaInputStager interface {
	Stage(ctx context.Context, userID int64, input MediaArtifactInput) (MediaArtifactInput, error)
}
```

`DisabledMediaArtifactObjectStore` 的 `Put/Open` 返回 `ErrMediaArtifactObjectStoreDisabled`，作为第一阶段没有配置通用对象存储时的安全 Provider。URL输入在入队前用同一 URL Validator 校验；上传图片/视频必须完成 `Stage` 后才调用 Orchestrator，返回值只含对象 Key、校验值和内部引用，不含 Data。上传视频在对象存储不可用时返回 `ErrMediaVideoObjectStorageRequired`，且不创建任务。数据模型只保存 Artifact ID，不把 Multipart 字节放进任务 JSON。

```go
func (s *MediaContentService) Stage(ctx context.Context, userID int64, input MediaArtifactInput) (MediaArtifactInput, error) {
	if input.ExternalURL != "" {
		normalized, err := s.httpReader.ValidateURL(input.ExternalURL)
		if err != nil {
			return MediaArtifactInput{}, err
		}
		input.ExternalURL = normalized
		input.Data = nil
		return input, nil
	}
	if len(input.Data) == 0 {
		return MediaArtifactInput{}, ErrInvalidMediaInput
	}
	stored, err := s.objectStore.Put(ctx, input)
	if err != nil {
		if input.MediaType == MediaTypeVideo {
			return MediaArtifactInput{}, ErrMediaVideoObjectStorageRequired
		}
		return MediaArtifactInput{}, err
	}
	return mediaArtifactInputFromStored(stored), nil
}
```

- [ ] **Step 4: 实现安全 HTTP 内容读取器**

`MediaHTTPContentReader` 复用 `internal/util/urlvalidator.ValidateHTTPURL` 和现有 `HTTPUpstream`：前者阻止不允许的 Scheme、私网字面量和非白名单 Host，后者在实际 Dial 与每次重定向时调用 `ValidateResolvedIP`，防止 DNS Rebinding。请求只允许 GET，过滤 Hop-by-Hop Header，只从内部 Adapter 接受鉴权 Header，并原样传递合法单 Range。

```go
func (r *mediaHTTPContentReader) ValidateURL(raw string) (string, error) {
	return urlvalidator.ValidateHTTPURL(raw, r.allowInsecureHTTP, urlvalidator.ValidationOptions{
		AllowedHosts: r.allowedHosts, RequireAllowlist: r.requireAllowlist, AllowPrivate: false,
	})
}

func (r *mediaHTTPContentReader) Open(ctx context.Context, input service.MediaHTTPContentRequest) (*service.MediaContent, error) {
	if input.Account == nil {
		return nil, service.ErrMediaContentAccountRequired
	}
	normalized, err := r.ValidateURL(input.URL)
	if err != nil {
		return nil, fmt.Errorf("unsafe media content url: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, normalized, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	copyAllowedMediaHeaders(req.Header, input.Headers)
	if input.ByteRange != "" {
		if !validSingleByteRange(input.ByteRange) {
			return nil, service.ErrInvalidMediaRange
		}
		req.Header.Set("Range", input.ByteRange)
	}
	resp, err := r.upstream.Do(req, input.Account.EffectiveProxyURL(), input.Account.ID, input.Account.Concurrency)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.ContentLength > r.maxBytes {
		_ = resp.Body.Close()
		cancel()
		return nil, service.ErrMediaContentTooLarge
	}
	resp.Body = newCancelOnCloseReadCloser(resp.Body, cancel)
	return mediaContentFromHTTPResponse(resp, r.maxBytes), nil
}
```

`mediaContentFromHTTPResponse` 只接受 `200/206`，包装读取器确保无 Content-Length 时也不超过部署上限，并保留 `Content-Type`、`Content-Length`、`Content-Range`、`Accept-Ranges`。增加重定向到私网、超大响应、非法多 Range 和 Data URL/Base64 服务端解码测试。

- [ ] **Step 5: 实现 MediaContentService 的归属和混合读取**

`MediaContentService` 同时实现 `MediaArtifactWriter`。对每个输出先调用对象存储；失败时只有系统设置允许安全代理且 Adapter 给出了内部 `UpstreamReference` 或经验证的 `ExternalURL` 才写入 `storage_status=proxy` 元数据，否则返回 `ErrMediaContentUnavailable` 让 Worker 将任务按系统存储故障全额退款：

```go
func (s *MediaContentService) PersistOutputs(ctx context.Context, task *MediaTask, inputs []MediaArtifactInput) ([]MediaArtifact, error) {
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	stored := make([]MediaArtifact, 0, len(inputs))
	for i := range inputs {
		input := inputs[i]
		input.Direction = "output"
		input.Position = i
		if s.objectStore != nil {
			if artifact, putErr := s.objectStore.Put(ctx, input); putErr == nil {
				artifact.TaskID = task.ID
				created, createErr := s.artifacts.Create(ctx, artifact)
				if createErr != nil {
					return nil, createErr
				}
				stored = append(stored, *created)
				continue
			}
		}
		proxyAllowed := input.MediaType == MediaTypeImage || settings.MediaVideoProxyFallbackEnabled
		if !proxyAllowed || (input.UpstreamReference == "" && input.ExternalURL == "") {
			return nil, ErrMediaContentUnavailable
		}
		created, createErr := s.artifacts.Create(ctx, artifactFromProxyInput(task.ID, input))
		if createErr != nil {
			return nil, createErr
		}
		stored = append(stored, *created)
	}
	return stored, nil
}
```

```go
func (s *MediaContentService) OpenVideo(ctx context.Context, publicID string, userID int64, byteRange string) (*MediaContent, error) {
	task, err := s.tasks.GetByPublicIDForUser(ctx, publicID, userID)
	if err != nil || task.MediaType != MediaTypeVideo || task.Status != MediaTaskStatusCompleted {
		return nil, ErrMediaTaskNotFound
	}
	artifacts, err := s.artifacts.ListByTaskID(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	artifact := firstOutputVideo(artifacts)
	if artifact == nil {
		return nil, ErrMediaArtifactNotFound
	}
	if artifact.ObjectKey != "" && s.objectStore != nil {
		if content, err := s.objectStore.Open(ctx, artifact, byteRange); err == nil {
			return content, nil
		}
	}
	if data, contentType, ok := decodeMediaDataReference(artifact.UpstreamReference); ok {
		if byteRange != "" {
			return sliceMediaContent(data, contentType, byteRange)
		}
		return &MediaContent{
			Body: io.NopCloser(bytes.NewReader(data)), StatusCode: http.StatusOK,
			ContentType: contentType, ContentLength: int64(len(data)), AcceptRanges: "bytes",
		}, nil
	}
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil || !settings.MediaVideoProxyFallbackEnabled || artifact.UpstreamReference == "" {
		return nil, ErrMediaContentUnavailable
	}
	account, err := s.accounts.GetByID(ctx, *task.AccountID)
	if err != nil {
		return nil, ErrMediaContentUnavailable
	}
	adapter, err := s.adapters.Resolve(task.Adapter)
	if err != nil {
		return nil, ErrMediaContentUnavailable
	}
	fetcher, ok := adapter.(MediaContentFetcher)
	if !ok {
		return nil, ErrMediaContentUnavailable
	}
	return fetcher.OpenContent(ctx, account, artifact, byteRange)
}
```

公开层始终只看到本系统内容路径；`UpstreamReference`、上游 Task ID、签名 URL、Authorization Header 和账号凭证不得进入 DTO、错误或日志。

- [ ] **Step 6: 实现 Handler 和稳定 DTO**

Handler 方法固定为 `CreateImageGeneration`、`CreateImageEdit`、`GetImageTask`、`CreateVideo`、`GetVideoTask`、`GetVideoContent`。创建接口从 API Key Context 获取 User/API Key/Group，把规范化后的 `Idempotency-Key` 传给 Orchestrator；Key 超过 255 字节返回 400。查询和下载从 `AuthSubject.UserID` 做归属查询，找不到与越权都返回相同 404。

JSON DTO 的 `Async` 使用 `*bool`，nil/false 都进入同步；Multipart 只接受空串、`true` 或 `false`，其他值返回 400。Handler 消费该控制字段并写入 `ClientAsync`，绝不把 `async` 作为未解析字段交给 Adapter。上传通过 `http.DetectContentType`、扩展名白名单、文件数量和 `cfg.Server.MaxRequestBodySize` 双重校验；Registry 约束与全局安全上限在创建任务前完成。

Handler 依赖小端口，便于独立 Gin 契约测试；`MediaOrchestrator` 实现创建/查询端口，`MediaContentService` 实现内容端口：

```go
type MediaTaskApplication interface {
	Create(ctx context.Context, req MediaCreateRequest) (*MediaCreateResult, error)
	GetForUser(ctx context.Context, publicID string, userID int64) (*MediaTask, []MediaArtifact, error)
}

type MediaVideoContentOpener interface {
	OpenVideo(ctx context.Context, publicID string, userID int64, byteRange string) (*MediaContent, error)
}
```

```go
type mediaTaskResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	MediaType string          `json:"media_type"`
	Operation string          `json:"operation"`
	Model     string          `json:"model"`
	Status    string          `json:"status"`
	Progress  int             `json:"progress"`
	CreatedAt int64           `json:"created_at"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *mediaTaskError `json:"error,omitempty"`
}
```

- `Accepted` 和 `FallbackAsync` 返回 `202 + media.task`。
- `Completed` 图片返回现有 OpenAI Images `created/data`；视频返回完成任务对象，`result.content_url` 只能是 `/v1/videos/{task_id}/content`。
- `GatewayTimeout` 返回 504 和安全错误码，不返回 Task ID。
- 查询只暴露四种公开状态和安全错误；图片 `result.data` 复用 OpenAI 数据数组。
- 下载用 `c.DataFromReader` 写二进制和 Range Header，不重定向到上游。
- 非法或不可满足 Range 返回 416；内容暂时不可用返回 502 和稳定错误码，响应中不包含内部引用。
- 不实现 `Cancel` 方法，也不注册 DELETE 路由。

只在 `handler.Handlers` 增加 `MediaTask *MediaTaskHandler` 并在 Wire Provider 构造；本 Task 不修改 `backend/internal/server/routes/gateway.go`，因此现有生产 `/v1/images/*` 仍由 `OpenAIGatewayHandler` 处理，视频路由也尚未对外开放。测试用 `newStandaloneMediaRouter` 显式挂载六个方法验证未来契约。

- [ ] **Step 7: 运行内容与 Handler 测试**

Run: `cd backend && go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia(TaskHandler|Router|HTTPContent|Content)' -count=1`

Expected: PASS。

- [ ] **Step 8: 提交**

```bash
git add backend/internal/service/media_content.go backend/internal/service/media_content_test.go backend/internal/service/media_adapter.go backend/internal/repository/media_http_content.go backend/internal/repository/media_http_content_test.go backend/internal/handler/media_task_handler.go backend/internal/handler/media_task_handler_test.go backend/internal/handler/wire.go backend/internal/handler/handler.go
git commit -m "feat(media): define task and video content api contracts"
```

### Task 17: 接入部署配置、Wire 和 Worker 生命周期

**Files:**
- Modify: `backend/internal/config/config.go`
- Create: `backend/internal/config/media_task_config_test.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/cmd/server/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Modify: `deploy/config.example.yaml`

- [ ] **Step 1: 写部署默认值和 Worker 生命周期失败测试**

```go
func TestMediaTaskConfigDefaults(t *testing.T) {
	cfg := loadConfigWithYAML(t, "")
	require.True(t, cfg.MediaTasks.Enabled)
	require.Equal(t, 4, cfg.MediaTasks.WorkerCount)
	require.Equal(t, 7200, cfg.MediaTasks.TaskTimeoutSeconds)
	require.Equal(t, 120, cfg.MediaTasks.LeaseTTLSeconds)
	require.Equal(t, 30, cfg.MediaTasks.LeaseRenewIntervalSeconds)
	require.Equal(t, 2, cfg.MediaTasks.PollIntervalSeconds)
	require.Equal(t, 15, cfg.MediaTasks.RecoveryIntervalSeconds)
	require.Equal(t, 100, cfg.MediaTasks.RecoveryBatchSize)
}

func TestMediaTaskConfigRejectsRenewIntervalNotBelowLease(t *testing.T) {
	_, err := loadConfigWithYAMLError(t, `
media_tasks:
  lease_ttl_seconds: 30
  lease_renew_interval_seconds: 30
`)
	require.ErrorContains(t, err, "lease_renew_interval_seconds")
}

func TestMediaWorkerStartStopIsIdempotent(t *testing.T) {
	worker := newIdleMediaWorkerForLifecycleTest(t)
	require.NoError(t, worker.Start())
	require.NoError(t, worker.Start())
	worker.Stop()
	worker.Stop()
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/config ./internal/service -run 'TestMedia(TaskConfig|WorkerStartStop)' -count=1`

Expected: FAIL，提示 `Config.MediaTasks` 和 Worker 生命周期方法未定义。

- [ ] **Step 3: 增加仅部署可变的媒体任务配置**

```go
type MediaTaskConfig struct {
	Enabled                   bool  `mapstructure:"enabled"`
	WorkerCount               int   `mapstructure:"worker_count"`
	TaskTimeoutSeconds        int   `mapstructure:"task_timeout_seconds"`
	LeaseTTLSeconds           int   `mapstructure:"lease_ttl_seconds"`
	LeaseRenewIntervalSeconds int   `mapstructure:"lease_renew_interval_seconds"`
	PollIntervalSeconds       int   `mapstructure:"poll_interval_seconds"`
	RecoveryIntervalSeconds   int   `mapstructure:"recovery_interval_seconds"`
	RecoveryBatchSize         int   `mapstructure:"recovery_batch_size"`
	StreamBlockMilliseconds   int   `mapstructure:"stream_block_milliseconds"`
	ContentProxyTimeoutSeconds int  `mapstructure:"content_proxy_timeout_seconds"`
	MaxContentBytes           int64 `mapstructure:"max_content_bytes"`
}

type Config struct {
	// 保留现有字段
	MediaTasks MediaTaskConfig `mapstructure:"media_tasks"`
}
```

默认值为测试所列值，另设 `stream_block_milliseconds=1000`、`content_proxy_timeout_seconds=90`、`max_content_bytes=2147483648`。校验：Worker 数量、任务超时、租约、续租、Poll、恢复间隔、批量、内容代理超时和内容上限均大于 0；续租必须严格小于租约。

在 `deploy/config.example.yaml` 增加独立 `media_tasks:` 区块，并注明这些是部署运行参数，不属于系统设置页热更新项。

- [ ] **Step 4: 增加 Provider 并启动 Worker**

Repository Provider 提供：

```go
func ProvideMediaTaskQueue(rdb *redis.Client, cfg *config.Config) service.MediaTaskQueue {
	consumer := mediaWorkerConsumerName()
	return NewMediaTaskStream(rdb, consumer, time.Duration(cfg.MediaTasks.LeaseTTLSeconds)*time.Second)
}

func ProvideMediaHTTPContentReader(upstream service.HTTPUpstream, cfg *config.Config) service.MediaHTTPContentReader {
	return NewMediaHTTPContentReader(upstream, cfg)
}
```

Service Provider 提供 Registry、Scheduler、禁用 Billing、Orchestrator、Content Service 和 Worker。Model Registry 在返回前 `Refresh`；Adapter Registry 本阶段不注册任何真实 Adapter，也不把 Fake Adapter 注入生产容器：

```go
func ProvideMediaAdapterRegistry() *MediaAdapterRegistry {
	return NewMediaAdapterRegistry()
}

func ProvideMediaBilling() MediaBillingPort {
	return DisabledMediaBilling{}
}

func ProvideMediaSettlementCoordinator(tasks MediaTaskRepository, billing MediaBillingPort) *MediaBillingCoordinator {
	return NewMediaBillingCoordinator(tasks, billing)
}

func ProvideMediaContentPolicy() MediaContentPolicy {
	return AllowAllMediaContentPolicy{}
}

func ProvideMediaPricing() MediaPricingPort {
	return ZeroMediaPricing{}
}

func ProvideMediaArtifactObjectStore() MediaArtifactObjectStore {
	return DisabledMediaArtifactObjectStore{}
}

func ProvideMediaTaskMetrics() MediaTaskMetrics {
	return NewMediaTaskMetrics()
}

// ProviderSet 同时绑定：
// wire.Bind(new(MediaInputStager), new(*MediaContentService))
// wire.Bind(new(MediaArtifactWriter), new(*MediaContentService))
// wire.Bind(new(MediaExecutionController), new(*MediaWorker))

func ProvideMediaWorker(
	queue MediaTaskQueue,
	tasks MediaTaskRepository,
	artifacts MediaArtifactRepository,
	artifactWriter MediaArtifactWriter,
	scheduler *MediaScheduler,
	adapters *MediaAdapterRegistry,
	settlements MediaSettlementCoordinator,
	metrics MediaTaskMetrics,
	cfg *config.Config,
) (*MediaWorker, error) {
	worker := NewMediaWorker(queue, tasks, artifacts, artifactWriter, scheduler, adapters, settlements, metrics, mediaWorkerConfigFrom(cfg))
	if !cfg.MediaTasks.Enabled {
		return worker, nil
	}
	if err := worker.Start(); err != nil {
		return nil, err
	}
	return worker, nil
}
```

`MediaWorker.Start` 先 `EnsureGroups`，再启动固定数量 Consumer 和一个恢复扫描协程；任何协程 Panic 都记录并退出该循环，由 Supervisor 重启，不能带崩进程。`Stop` 取消根 Context，等待所有协程退出，但不把 in-progress 任务写成失败。

- [ ] **Step 5: 接入清理顺序和生成 Wire**

`provideCleanup` 增加 `mediaWorker *service.MediaWorker`，在关闭 Redis/Ent 之前的应用层并行步骤调用 `mediaWorker.Stop()`。`handler.ProviderSet` 构造 `MediaTaskHandler` 并写入 `Handlers.MediaTask`，但 `server/routes/gateway.go` 不消费该字段。

`handler.ProviderSet` 使用 `wire.Bind(new(MediaTaskApplication), new(*service.MediaOrchestrator))` 和 `wire.Bind(new(MediaVideoContentOpener), new(*service.MediaContentService))` 满足小端口，避免 Handler 测试依赖完整 Service 图。

Run: `cd backend && go generate ./cmd/server`

Expected: `wire_gen.go` 更新成功，`MediaWorker` 出现在构造与 Cleanup 依赖图中，没有修改生产路由。

- [ ] **Step 6: 运行配置、生命周期和 Wire 编译测试**

Run: `cd backend && go test ./internal/config ./internal/service ./internal/repository ./internal/handler ./cmd/server -run 'TestMedia|^$' -count=1`

Expected: PASS；`./cmd/server` 在无测试时仍完成编译。

- [ ] **Step 7: 提交**

```bash
git add backend/internal/config/config.go backend/internal/config/media_task_config_test.go backend/internal/repository/wire.go backend/internal/service/wire.go backend/internal/handler/wire.go backend/cmd/server/wire.go backend/cmd/server/wire_gen.go deploy/config.example.yaml
git commit -m "feat(media): wire task workers and deployment config"
```

### Task 18: 全量生成、回归验证和收口

**Files:**
- Generated: `backend/ent/`
- Generated: `backend/cmd/server/wire_gen.go`
- Verify only: `backend/internal/server/routes/gateway.go`
- Verify only: repository and frontend files changed by Tasks 1–17

- [ ] **Step 1: 格式化所有新增和修改代码**

Run: `cd backend && gofmt -w ent/schema/media_task.go ent/schema/media_artifact.go ent/schema/media_model_definition.go internal/service/media_*.go internal/service/account_candidate_selector*.go internal/repository/media_*.go internal/handler/media_task_handler*.go internal/config/media_task_config_test.go`

Expected: 命令退出码为 0。

Run: `cd frontend && pnpm exec eslint src/components/admin/settings/MediaGenerationSettingsCard.vue src/components/account/MediaConfigEditor.vue src/components/admin/group/GroupMediaSettings.vue src/components/common/GroupSelector.vue src/views/admin/SettingsView.vue src/views/admin/GroupsView.vue src/api/admin/settings.ts src/types/index.ts --fix`

Expected: 命令退出码为 0，仅格式化本计划涉及的前端文件。

- [ ] **Step 2: 重新生成 Ent 和 Wire 并确认无漂移**

Run: `cd backend && go generate ./ent ./cmd/server`

Expected: 命令退出码为 0；第二次运行相同命令后 `git diff --exit-code -- ent cmd/server/wire_gen.go` 退出码为 0。

- [ ] **Step 3: 运行后端单元测试**

Run: `cd backend && go test ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin ./internal/config ./migrations -count=1`

Expected: PASS。

Run: `cd backend && go test ./... -count=1`

Expected: PASS，所有未带 integration Build Tag 的后端包完成编译和回归。

- [ ] **Step 4: 运行 PostgreSQL/Redis 故障恢复集成测试**

Run: `cd backend && go test -tags=integration ./internal/repository -run 'TestMedia(TaskRepository|TaskStream|WorkerIntegration)' -count=1`

Expected: Docker 可用时 PASS；CI 中 Docker 不可用按现有 TestMain 规则明确失败，本地无 Docker 时明确跳过。

- [ ] **Step 5: 运行前端单元测试、类型检查和生产构建**

Run: `cd frontend && pnpm test:run src/components/admin/settings/__tests__/MediaGenerationSettingsCard.spec.ts src/components/account/__tests__/MediaConfigEditor.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/components/admin/group/__tests__/GroupMediaSettings.spec.ts src/components/common/__tests__/GroupSelector.media.spec.ts src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts src/views/admin/__tests__/settingsFormState.spec.ts`

Expected: PASS。

Run: `cd frontend && pnpm lint:check && pnpm typecheck && pnpm build`

Expected: PASS，ESLint、Vue TypeScript 和 Vite 生产构建成功。

- [ ] **Step 6: 运行生产链路回归测试**

Run: `cd backend && go test ./internal/handler ./internal/service -run 'Test(OpenAIGatewayHandler|OpenAIImages|Gateway.*Messages|GeminiMessages)' -count=1`

Expected: PASS；现有 OpenAI Images、OpenAI/Anthropic/Gemini 文本链路行为不变。

- [ ] **Step 7: 审核路由、取消接口和敏感字段边界**

Run: `git diff --exit-code f0d6eac3f -- backend/internal/server/routes/gateway.go`

Expected: 退出码为 0，第一阶段没有切换或新增生产媒体路由。

Run: `rg -n 'CancelMedia|CancelTask|func .*Cancel|DELETE.+/v1/(videos|images)' backend/internal/service/media_*.go backend/internal/handler/media_task_handler.go backend/internal/server/routes/gateway.go frontend/src/components/admin/settings frontend/src/components/account/MediaConfigEditor.vue frontend/src/components/admin/group/GroupMediaSettings.vue`

Expected: 无输出；用户端和管理员端都没有媒体任务取消能力。

Run: `rg -n 'upstream_task_id|upstream_reference|authorization' backend/internal/handler/media_task_handler.go`

Expected: 无公开 DTO JSON Tag 或响应映射命中；若命中仅允许出现在明确的内部禁止透传注释，需人工复核后再继续。

- [ ] **Step 8: 检查变更质量并提交最终生成产物**

Run: `git diff --check && git status --short`

Expected: `git diff --check` 无输出；状态只包含本计划涉及文件。

```bash
git add backend/ent backend/cmd/server/wire_gen.go
git commit -m "chore(media): refresh generated clients"
```

如果生成文件已在之前提交且没有漂移，则跳过空提交。最后用 `git log --oneline --max-count=20` 核对 Tasks 1–17 每项均有独立提交。
