# 媒体 Adapter 自动解析实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use the available `superpowers:executing-plans` skill to implement this plan task-by-task; the primary agent may still delegate isolated tasks through its normal collaboration tools. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 移除管理员对媒体 Adapter 和全局默认异步模式的配置权，让系统按规范模型的 `vendor + model_id/family` 从代码注册表唯一解析 Adapter，并在不破坏旧任务快照的前提下完成调度、管理 API 和前端闭环。

**Architecture:** 新增代码级 `MediaAdapterRegistration`、`MediaAdapterResolver` 和历史 key alias，模型 Registry 在刷新时把可路由模型与 unavailable tombstone 原子发布。管理 API 只写模型业务元数据并返回只读 `adapter_resolution`；Scheduler 只消费解析结果和账号绑定能力，Worker 保持现有 v1 候选数组及两阶段冻结语义。

**Tech Stack:** Go 1.26、Gin、Ent、Google Wire、PostgreSQL、Vue 3、TypeScript、Vitest、pnpm。

---

## 范围与约束

- 依据 `docs/superpowers/specs/2026-07-21-media-adapter-auto-resolution-design.md`。
- 本计划是一个统一契约变更：后端解析状态、管理 API 和前端只读展示必须一起交付，不拆成互相独立的子项目。
- 不实现 Grok、Seedance、Nano Banana、Veo、Z-Image、Agens Video 或其他真实 Adapter；生产 Registry 为空时，模型应得到明确 unavailable 状态。
- 不修改 Chat、Responses、Anthropic Messages、Gemini 文本链路。
- 不删除 Ent 字段、数据库列或迁移文件；`default_adapter/default_async_mode` 只保留数据库和 v1 快照兼容。
- 不改变候选快照顶层 JSON 数组 wire format。
- 所有代码步骤遵循 TDD；每个任务完成后单独提交。

## 文件职责映射

### 新建

- `backend/internal/service/media_adapter_resolver.go`：代码注册描述、exact/family 解析、能力交集、解析状态和兼容异步模式换算。
- `backend/internal/service/media_adapter_resolver_test.go`：Resolver、规则歧义、能力校验、历史 key alias 单元测试及跨测试 fixture。
- `backend/internal/service/media_candidate_snapshot_codec.go`：显式 v1 候选快照编解码，固定历史 wire key。
- `backend/internal/service/media_candidate_snapshot_codec_test.go`：硬编码历史 JSON、严格解码和 durable rewrite 兼容测试。
- `frontend/src/components/admin/media/MediaAdapterResolutionPanel.vue`：只读系统适配状态和能力展示。
- `frontend/src/components/admin/media/__tests__/MediaAdapterResolutionPanel.spec.ts`：状态面板渲染测试。
- `frontend/src/api/admin/__tests__/mediaModels.spec.ts`：媒体模型 API 请求体和 ready 过滤测试。
- `frontend/src/views/admin/__tests__/MediaModelsView.spec.ts`：注册表列表、启停与只读解析状态测试。

### 修改

- `backend/internal/service/media_adapter.go`、`media_adapter_test.go`：Registry 规范 key、模型注册、历史 alias 和实现能力检查。
- `backend/internal/service/media_model_registry.go`、`media_model_registry_test.go`：基础校验、Resolver 注入、ready 索引、tombstone 和 typed unavailable error。
- `backend/internal/service/media_scheduler.go`、`media_scheduler_test.go`：使用只读解析结果和规则能力过滤账号，冻结派生兼容字段。
- `backend/internal/service/media_worker.go`、`media_worker_test.go`、`media_worker_lifecycle_test.go`：v1 快照只做基础模型校验，继续信任冻结候选和任务列。
- `backend/internal/service/media_orchestrator.go`、`media_orchestrator_test.go`：所有候选快照读写改走同一 v1 codec，验证两阶段冻结。
- `backend/internal/service/media_model_admin.go`、`media_model_admin_test.go`：管理记录富化、启用前原子校验、ready 分组校验和只读 preflight。
- `backend/internal/repository/media_model_repo.go`、`media_model_repo_test.go`：新写入停止更新旧列，读取仅保留发布诊断值。
- `backend/internal/handler/admin/media_model_handler.go`、`media_model_handler_test.go`：废弃输入槽、只读响应和 preflight Handler。
- `backend/internal/handler/media_task_handler.go`、`media_task_handler_test.go`：unavailable tombstone 映射为 503。
- `backend/internal/server/routes/admin.go`：注册静态 preflight 路由。
- `backend/internal/service/wire.go`、`backend/cmd/server/wire_gen.go`：先构造 Adapter Registry/Resolver，再刷新模型 Registry，并注入管理服务。
- `frontend/src/types/index.ts`、`frontend/src/api/admin/mediaModels.ts`：新只读契约和 ready 模型过滤。
- `frontend/src/components/admin/media/MediaModelEditor.vue`、`MediaModelEditor.spec.ts`：删除 Adapter/默认异步输入并嵌入状态面板。
- `frontend/src/views/admin/MediaModelsView.vue`：列表、搜索、编辑和启停只使用 `adapter_resolution`。
- `frontend/src/components/account/MediaConfigEditor.vue`、`MediaConfigEditor.spec.ts`：展示解析能力并限制账号上游模式。
- `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`、`EditAccountModal.spec.ts`：更新模型 fixture，保留旧账号迁移语义。
- `frontend/src/components/admin/group/GroupMediaSettings.vue`、`GroupMediaSettings.spec.ts`：仅新增 ready 模型并可移除历史失效授权。
- `frontend/src/views/admin/GroupsView.vue`：区分模型列表加载状态，并允许安全清理仅由历史失效模型构成的分组授权。
- `frontend/src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts`：更新媒体模型响应 fixture。
- `frontend/src/i18n/locales/zh.ts`、`en.ts`：系统解析状态、能力、错误和历史不可用文案。
- `deploy/README_CLUSTER.md`：preflight、写入冻结和多实例发布顺序。

### 明确不改

- `backend/ent/schema/media_model_definition.go`
- `backend/migrations/131_media_unified_routing_storage.sql`
- `backend/ent/**`

旧列本期继续存在，因此不运行 Ent 生成。

---

### Task 1：建立代码级 Adapter 注册、解析和历史 key alias

**Files:**
- Create: `backend/internal/service/media_adapter_resolver.go`
- Create: `backend/internal/service/media_adapter_resolver_test.go`
- Modify: `backend/internal/service/media_adapter.go`
- Modify: `backend/internal/service/media_adapter_test.go`
- Modify: `backend/internal/service/media_fake_adapter.go`
- Modify: `backend/internal/service/media_metrics.go`
- Modify: `backend/internal/service/media_metrics_test.go`
- Modify: `backend/internal/service/media_scheduler_test.go`
- Modify: `backend/internal/service/media_worker_test.go`
- Modify: `backend/internal/service/media_content_test.go`
- Modify: `backend/internal/repository/media_worker_integration_test.go`

- [ ] **Step 1：写 Resolver 与 Registry 失败测试**

在 `media_adapter_resolver_test.go` 写表驱动测试，固定 exact 覆盖 family、family 唯一命中、无命中、family 多匹配、操作不兼容、规则没有执行路径，以及账号服务商和上游模型不参与解析。测试名和关键断言必须包含：

- `TestMediaAdapterRegistryRejectsDuplicateExactKey`：第二个相同 `vendor+model_id` 注册失败且 Registry 快照不变；
- `TestMediaAdapterRegistryRejectsCapabilitiesBeyondImplementation`：规则声明的 sync/async 不得超过 Go interface；content-fetch 不允许配置，只从 `MediaContentFetcher` 派生；
- `TestMediaAdapterRegistryRejectsDefinitionWithoutExecutionPath`：sync/native 都为 false 时拒绝；
- `TestMediaAdapterRegistrationOperationsAreSourceOfTruth`：规则 operations 只能是 registration `SupportedOperations` 子集；
- `TestMediaAdapterRegistryRejectsNameKeyMismatchWithoutMutation`：key/`Name()` 不一致时零写入；
- `TestMediaAdapterRegistryRejectsAliasOverwriteDuplicateChainAndCycle`：覆盖规范 key、重复、链、环全部失败；
- `TestMediaAdapterAliasResolvesSameImplementationAndCapabilities`：历史 alias 只影响实现查找，不改变规范 registration 能力；注入 `AtomicMediaTaskMetrics` 和内存 JSON logger 后只解析一次 alias，断言 `HistoricalAdapterAliasResolutions()==1`，日志仅含 `legacy_adapter_key/adapter_key`；
- `TestMediaAdapterRegistryCanonicalKeyPreservesHistoricalLookup`：`CanonicalKey("legacy-image")` 返回规范 key 和 `aliased=true`，未知 key 原样返回且不修改 Registry；
- `TestMediaAdapterResolverReportsImplementationMissingWithMatchedKey`：防御性状态保留 matched key/type，capabilities 为 nil；
- `TestMediaAdapterResolverCapabilityMismatchKeepsResolvedMetadata`：保留 resolved key、match 和实际 capabilities；
- `TestMediaAdapterResolverDerivesContentFetchFromInterface`：`ContentFetch` 与 `MediaContentFetcher` 实现一致。

```go
func TestMediaAdapterResolverUsesExactBeforeFamily(t *testing.T) {
	t.Parallel()
	registry, resolver := newTestMediaAdapterResolver(t,
		MediaAdapterRegistration{
			Key: "family-image",
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "family-image", NativeAsyncMode: NativeAsyncUnsupported}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			FamilyRules: []MediaAdapterFamilyRule{{
				Vendor: "xai", FamilyID: "grok-image",
				Match: func(modelID string) bool { return strings.HasPrefix(modelID, "grok-") },
				Capabilities: MediaAdapterRuleCapabilities{Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true},
			}},
		},
		MediaAdapterRegistration{
			Key: "exact-image",
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "exact-image", NativeAsyncMode: NativeAsyncRequired}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			ExactRules: []MediaAdapterExactRule{{
				Vendor: "xai", ModelID: "grok-2-image",
				Capabilities: MediaAdapterRuleCapabilities{Operations: []MediaOperation{MediaOperationTextToImage}, NativeAsyncUpstream: true},
			}},
		},
	)
	t.Cleanup(func() { require.NotNil(t, registry) })

	resolution := resolver.Resolve(" XAI ", " Grok-2-Image ", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.Equal(t, "exact-image", resolution.ResolvedAdapter)
	require.Equal(t, MediaAdapterMatchedExact, resolution.MatchedBy)
	require.True(t, resolution.Capabilities.NativeAsyncUpstream)
}

func TestMediaAdapterRegistryResolvesHistoricalAlias(t *testing.T) {
	registry := NewMediaAdapterRegistry()
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "canonical-image", NativeAsyncMode: NativeAsyncUnsupported})
	require.NoError(t, registry.Register("canonical-image", adapter))
	require.NoError(t, registry.RegisterAlias("legacy-image", "canonical-image"))
	resolved, err := registry.Resolve("legacy-image")
	require.NoError(t, err)
	require.Same(t, adapter, resolved)
	require.Error(t, registry.RegisterAlias("alias-chain", "legacy-image"))
}

func newTestMediaAdapterResolver(
	t *testing.T,
	registrations ...MediaAdapterRegistration,
) (*MediaAdapterRegistry, *MediaAdapterResolver) {
	t.Helper()
	registry := NewMediaAdapterRegistry()
	for _, registration := range registrations {
		require.NoError(t, registry.RegisterDefinition(registration))
	}
	return registry, NewMediaAdapterResolver(registry)
}

func exactImageRegistration(key, vendor, modelID string) MediaAdapterRegistration {
	return MediaAdapterRegistration{
		Key:                 key,
		Adapter:             NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: key, NativeAsyncMode: NativeAsyncUnsupported}),
		SupportedOperations: []MediaOperation{MediaOperationTextToImage},
		ExactRules: []MediaAdapterExactRule{{
			Vendor: vendor,
			ModelID: modelID,
			Capabilities: MediaAdapterRuleCapabilities{
				Operations:   []MediaOperation{MediaOperationTextToImage},
				SyncUpstream: true,
			},
		}},
	}
}

func readyResolution(syncUpstream, nativeAsyncUpstream bool) MediaAdapterResolution {
	return MediaAdapterResolution{
		Status:          MediaAdapterResolutionReady,
		ResolvedAdapter: "test-adapter",
		MatchedBy:       MediaAdapterMatchedExact,
		Capabilities: &MediaAdapterCapabilities{
			Operations:          []MediaOperation{MediaOperationTextToImage},
			SyncUpstream:        syncUpstream,
			NativeAsyncUpstream: nativeAsyncUpstream,
		},
	}
}
```

同时把 `media_adapter_test.go` 中名称不一致的旧断言改成失败断言，并给 `fake-sync/fake-async/fake-optional` 显式传入相同 `Name`。

- [ ] **Step 2：运行测试并确认按预期失败**

Run:

```bash
cd backend
go test ./internal/service -run 'MediaAdapter(Resolver|Registry|Registration|Alias)' -count=1
```

Expected: FAIL；首次失败应明确表明 `MediaAdapterRegistration`/`RegisterDefinition` 尚未定义，或 `MediaAdapterRegistry.Register` 尚未返回 `error`。

- [ ] **Step 3：增加解析状态、能力与规则类型**

在 `media_adapter_resolver.go` 增加以下契约；后续任务只使用这些名字，不另造同义类型。

```go
type MediaAdapterResolutionStatus string

const (
	MediaAdapterResolutionReady                 MediaAdapterResolutionStatus = "ready"
	MediaAdapterResolutionInvalidDefinition     MediaAdapterResolutionStatus = "invalid_definition"
	MediaAdapterResolutionUnresolved            MediaAdapterResolutionStatus = "unresolved"
	MediaAdapterResolutionAmbiguous             MediaAdapterResolutionStatus = "ambiguous"
	MediaAdapterResolutionImplementationMissing MediaAdapterResolutionStatus = "implementation_missing"
	MediaAdapterResolutionCapabilityMismatch    MediaAdapterResolutionStatus = "capability_mismatch"
)

type MediaAdapterMatchType string

const (
	MediaAdapterMatchedExact  MediaAdapterMatchType = "exact"
	MediaAdapterMatchedFamily MediaAdapterMatchType = "family"
)

type MediaAdapterCapabilities struct {
	Operations          []MediaOperation
	SyncUpstream        bool
	NativeAsyncUpstream bool
	ContentFetch        bool
}

type MediaAdapterRuleCapabilities struct {
	Operations          []MediaOperation
	SyncUpstream        bool
	NativeAsyncUpstream bool
}

type MediaAdapterExactRule struct {
	Vendor       string
	ModelID      string
	Capabilities MediaAdapterRuleCapabilities
}

type MediaAdapterFamilyRule struct {
	Vendor       string
	FamilyID     string
	Match        func(modelID string) bool
	Capabilities MediaAdapterRuleCapabilities
}

type MediaAdapterRegistration struct {
	Key                 string
	Adapter             MediaAdapter
	SupportedOperations []MediaOperation
	ExactRules          []MediaAdapterExactRule
	FamilyRules         []MediaAdapterFamilyRule
}

type MediaAdapterResolution struct {
	Status          MediaAdapterResolutionStatus
	ResolvedAdapter string
	MatchedBy       MediaAdapterMatchType
	MatchedFamily   string
	Capabilities    *MediaAdapterCapabilities
	ReasonCode      string
}

func (r MediaAdapterResolution) IsReady() bool {
	return r.Status == MediaAdapterResolutionReady && r.Capabilities != nil
}

func (r MediaAdapterResolution) CompatibilityAsyncMode() NativeAsyncMode {
	if !r.IsReady() {
		return NativeAsyncUnsupported
	}
	switch {
	case r.Capabilities.SyncUpstream && r.Capabilities.NativeAsyncUpstream:
		return NativeAsyncOptional
	case r.Capabilities.NativeAsyncUpstream:
		return NativeAsyncRequired
	default:
		return NativeAsyncUnsupported
	}
}
```

- [ ] **Step 4：扩展 Registry 并实现 Resolver**

把 `MediaAdapterRegistry.Register` 改为返回 `error`。规范 key 必须等于 `adapter.Name()`；新增 `RegisterDefinition`、`RegisterAlias` 和只读 registration 快照。简单 `Register` 只注册可供历史任务按 key 解析的实现，不产生模型规则；只有 `RegisterDefinition` 能进入 Resolver 目录。

```go
type MediaAdapterRegistry struct {
	mu            sync.RWMutex
	adapters      map[string]MediaAdapter
	aliases       map[string]string
	registrations []MediaAdapterRegistration
	routingMetrics MediaRoutingMetrics
	logger         *slog.Logger
}

func NewMediaAdapterRegistry() *MediaAdapterRegistry {
	return &MediaAdapterRegistry{
		adapters: make(map[string]MediaAdapter),
		aliases:  make(map[string]string),
		logger:   slog.Default(),
	}
}

func (r *MediaAdapterRegistry) SetRoutingMetrics(metrics MediaRoutingMetrics) {
	r.mu.Lock()
	r.routingMetrics = metrics
	r.mu.Unlock()
}

func (r *MediaAdapterRegistry) SetLogger(logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	r.mu.Lock()
	r.logger = logger
	r.mu.Unlock()
}

func (r *MediaAdapterRegistry) Register(name string, adapter MediaAdapter) error {
	key := normalizeMediaAdapterName(name)
	if key == "" || isNilMediaAdapter(adapter) {
		return errors.New("media adapter name and implementation are required")
	}
	if normalizeMediaAdapterName(adapter.Name()) != key {
		return fmt.Errorf("media adapter key %q does not match implementation name %q", key, adapter.Name())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = make(map[string]MediaAdapter)
	}
	if _, exists := r.adapters[key]; exists {
		return fmt.Errorf("duplicate media adapter: %s", key)
	}
	r.adapters[key] = adapter
	return nil
}

func (r *MediaAdapterRegistry) RegisterAlias(oldKey, canonicalKey string) error {
	oldKey = normalizeMediaAdapterName(oldKey)
	canonicalKey = normalizeMediaAdapterName(canonicalKey)
	r.mu.Lock()
	defer r.mu.Unlock()
	if oldKey == "" || canonicalKey == "" || oldKey == canonicalKey {
		return errors.New("media adapter alias and canonical key are invalid")
	}
	if _, exists := r.adapters[oldKey]; exists {
		return fmt.Errorf("media adapter alias %q conflicts with a canonical key", oldKey)
	}
	if _, exists := r.aliases[oldKey]; exists {
		return fmt.Errorf("duplicate media adapter alias: %s", oldKey)
	}
	if _, aliasTarget := r.aliases[canonicalKey]; aliasTarget {
		return errors.New("media adapter alias chains are not allowed")
	}
	if _, exists := r.adapters[canonicalKey]; !exists {
		return fmt.Errorf("canonical media adapter %q is not registered", canonicalKey)
	}
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	r.aliases[oldKey] = canonicalKey
	return nil
}

func (r *MediaAdapterRegistry) Resolve(name string) (MediaAdapter, error) {
	requestedKey := normalizeMediaAdapterName(name)
	r.mu.RLock()
	canonicalKey := requestedKey
	if target, ok := r.aliases[requestedKey]; ok {
		canonicalKey = target
	}
	adapter, ok := r.adapters[canonicalKey]
	metrics, logger := r.routingMetrics, r.logger
	r.mu.RUnlock()
	if !ok {
		return nil, ErrMediaAdapterNotFound
	}
	if canonicalKey != requestedKey {
		if logger == nil {
			logger = slog.Default()
		}
		if metrics != nil {
			metrics.IncrementHistoricalAdapterAliasResolution()
		}
		logger.Debug(
			"media_adapter_historical_alias_resolved",
			"legacy_adapter_key", requestedKey,
			"adapter_key", canonicalKey,
		)
	}
	return adapter, nil
}
```

`RegisterDefinition` 必须先在局部变量中完成 vendor/model/family、重复 exact、操作子集、至少一个执行路径，以及规则协议不超过 Go interface 能力的全部验证，再在一次写锁内检查冲突并同时提交 Adapter 与 registration；任何失败都不能修改 `adapters`、`aliases` 或 `registrations`。

签名固定为 `func (r *MediaAdapterRegistry) RegisterDefinition(registration MediaAdapterRegistration) error`；它在同一把写锁内验证并写入规范实现与 registration，不能先调用已加锁的 `Register` 造成自锁。`Registrations()` 返回深拷贝，Resolver 构造时据此建立不可变 exact/family 索引。再增加 `func (r *MediaAdapterRegistry) Validate() error`，逐项复核规范 key、实现和规则索引；生产 Provider 必须传播该错误，不能忽略 registration 失败。

同一任务增加只读 `CanonicalKey(name) (canonical string, aliased bool)`：命中历史 alias 时返回规范 key 和 true；规范 key 或未知 key 返回规范化后的输入和 false。该方法只供 Worker 记录冻结旧 key，不改变任务列、快照或 `Resolve` 行为。

`Register` 改为返回 `error` 后，同一个提交内必须更新全仓媒体调用点，避免 Task 1 自身无法编译：普通测试使用 `require.NoError(t, registry.Register(key, adapter))`；并发 goroutine 把返回错误写入现有 `errCh`；原 `require.PanicsWithValue` 改为 `require.Error`/`require.ErrorContains`。`media_adapter_test.go` 的前三个 Fake Adapter 必须分别设置 `Name: "fake-sync"`、`Name: "fake-async"`、`Name: "fake-optional"`，其余注册的 key 也必须与 `adapter.Name()` 规范化后完全相等。

在 `media_metrics.go` 增加独立的固定维度接口，避免用 model ID 创建无界内存标签；`AtomicMediaTaskMetrics` 实现它：

```go
type MediaRoutingMetrics interface {
	IncrementAdapterResolutionFailure(status MediaAdapterResolutionStatus)
	IncrementCandidateCapabilityMismatch()
	IncrementHistoricalAdapterAliasResolution()
}

type mediaRoutingMetricCounters struct {
	resolutionFailures          [5]atomic.Int64
	candidateCapabilityMismatch atomic.Int64
	historicalAliasResolution   atomic.Int64
}

func mediaAdapterResolutionFailureIndex(status MediaAdapterResolutionStatus) (int, bool) {
	switch status {
	case MediaAdapterResolutionInvalidDefinition:
		return 0, true
	case MediaAdapterResolutionUnresolved:
		return 1, true
	case MediaAdapterResolutionAmbiguous:
		return 2, true
	case MediaAdapterResolutionImplementationMissing:
		return 3, true
	case MediaAdapterResolutionCapabilityMismatch:
		return 4, true
	default:
		return 0, false
	}
}

func (m *AtomicMediaTaskMetrics) IncrementAdapterResolutionFailure(status MediaAdapterResolutionStatus) {
	if index, ok := mediaAdapterResolutionFailureIndex(status); m != nil && ok {
		m.routing.resolutionFailures[index].Add(1)
	}
}

func (m *AtomicMediaTaskMetrics) IncrementCandidateCapabilityMismatch() {
	if m != nil {
		m.routing.candidateCapabilityMismatch.Add(1)
	}
}

func (m *AtomicMediaTaskMetrics) IncrementHistoricalAdapterAliasResolution() {
	if m != nil {
		m.routing.historicalAliasResolution.Add(1)
	}
}

func (m *AtomicMediaTaskMetrics) AdapterResolutionFailures(status MediaAdapterResolutionStatus) int64 {
	if index, ok := mediaAdapterResolutionFailureIndex(status); m != nil && ok {
		return m.routing.resolutionFailures[index].Load()
	}
	return 0
}

func (m *AtomicMediaTaskMetrics) CandidateCapabilityMismatches() int64 {
	if m == nil {
		return 0
	}
	return m.routing.candidateCapabilityMismatch.Load()
}

func (m *AtomicMediaTaskMetrics) HistoricalAdapterAliasResolutions() int64 {
	if m == nil {
		return 0
	}
	return m.routing.historicalAliasResolution.Load()
}
```

把 `routing mediaRoutingMetricCounters` 加入现有 `AtomicMediaTaskMetrics`。resolution counter 只接受五个非 ready 状态，未知值忽略；另两个 counter 使用 `atomic.Int64`。固定维度 counter 按解析状态聚合，规范模型维度由 Registry 的结构化日志字段 `canonical_model_id` 提供，避免为任意 model ID 创建无界指标标签；运维需要“状态 + 规范模型”时按该日志聚合。`MediaAdapterRegistry` 增加 `SetRoutingMetrics(MediaRoutingMetrics)`，命中 alias 时递增历史恢复计数并写结构化 debug 日志，字段固定为 `legacy_adapter_key`、`adapter_key`。该 counter 表示 alias 解析事件；Worker 另在冻结旧 key 的执行边界写带 `task_id` 的结构化日志，运维按 distinct `task_id` 统计“使用冻结旧 Adapter key 的任务数”，避免把内容回源/重复 Resolve 误报成唯一任务数。`media_metrics_test.go` 覆盖所有状态、未知状态忽略和并发安全。

Resolver 固定按以下算法执行：

```go
func (r *MediaAdapterResolver) Resolve(vendor, modelID string, operations []MediaOperation) MediaAdapterResolution {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	modelID = normalizeMediaModelID(modelID)
	if exact, ok := r.exact[mediaAdapterExactKey(vendor, modelID)]; ok {
		return r.resolveRule(exact.adapterKey, MediaAdapterMatchedExact, "", exact.capabilities, operations)
	}
	matches := make([]mediaAdapterFamilyEntry, 0, 1)
	for _, family := range r.families[vendor] {
		if family.match(modelID) {
			matches = append(matches, family)
		}
	}
	if len(matches) == 0 {
		return mediaAdapterResolutionFailure(MediaAdapterResolutionUnresolved, "MEDIA_ADAPTER_UNRESOLVED")
	}
	if len(matches) != 1 {
		return mediaAdapterResolutionFailure(MediaAdapterResolutionAmbiguous, "MEDIA_ADAPTER_AMBIGUOUS")
	}
	match := matches[0]
	return r.resolveRule(match.adapterKey, MediaAdapterMatchedFamily, match.familyID, match.capabilities, operations)
}
```

`resolveRule` 检查实现存在、请求 operations 是规则与实现操作交集的子集，并保证最终 `SyncUpstream || NativeAsyncUpstream`；失败返回稳定 `ReasonCode`。

非 ready 结果不能一律清空字段。`implementation_missing` 保留规则得到的 `ResolvedAdapter`、`MatchedBy`、`MatchedFamily`，但 `Capabilities=nil`；`capability_mismatch` 保留相同匹配字段和已经按 registration/实现求交集的实际 capabilities。只有 `unresolved`/`ambiguous` 没有唯一 key。上述字段形状由 Step 1 的两个专用测试锁定。

- [ ] **Step 5：运行 Resolver/Registry 测试并确认通过**

Run:

```bash
cd backend
gofmt -w internal/service/media_adapter.go internal/service/media_adapter_resolver.go internal/service/media_adapter_test.go internal/service/media_adapter_resolver_test.go internal/service/media_fake_adapter.go internal/service/media_metrics.go internal/service/media_metrics_test.go internal/service/media_scheduler_test.go internal/service/media_worker_test.go internal/service/media_content_test.go internal/repository/media_worker_integration_test.go
go test ./internal/service -run 'MediaAdapter(Resolver|Registry|Registration|Alias)|AtomicMediaTaskMetrics' -count=1
go test -race ./internal/service -run 'MediaAdapterRegistrySupportsConcurrent|AtomicMediaTaskMetrics' -count=1
go test ./internal/service ./internal/repository -run '^$'
```

Expected: 全部 PASS；race 测试无数据竞争；两个包的仅编译检查通过，证明所有 `Register` 调用点已适配新签名。

- [ ] **Step 6：提交纯代码解析内核**

```bash
git add backend/internal/service/media_adapter.go backend/internal/service/media_adapter_resolver.go backend/internal/service/media_adapter_test.go backend/internal/service/media_adapter_resolver_test.go backend/internal/service/media_fake_adapter.go backend/internal/service/media_metrics.go backend/internal/service/media_metrics_test.go backend/internal/service/media_scheduler_test.go backend/internal/service/media_worker_test.go backend/internal/service/media_content_test.go backend/internal/repository/media_worker_integration_test.go
git commit -m "feat(media): add automatic adapter resolver"
```

---

### Task 2：拆分模型基础校验并发布 ready/tombstone Registry 快照

**Files:**
- Modify: `backend/internal/service/media_model_registry.go`
- Modify: `backend/internal/service/media_model_registry_test.go`
- Modify: `backend/internal/repository/media_model_repo_test.go`

- [ ] **Step 1：写单模型故障关闭和 tombstone 失败测试**

新增核心测试：一个 ready 模型和一个 unresolved 模型同时刷新时，ready 模型仍可解析，unresolved 规范 ID 及别名返回 typed unavailable，旧快照中该模型不能继续服务。

```go
func TestMediaModelRegistryPublishesValidModelsAndUnavailableTombstones(t *testing.T) {
	ready := validImageModelDefinition()
	ready.ID, ready.ModelID, ready.Vendor = 1, "grok-2-image", "xai"
	ready.Operations = []MediaOperation{MediaOperationTextToImage}
	unavailable := validImageModelDefinition()
	unavailable.ID, unavailable.ModelID, unavailable.Vendor = 2, "unknown-image", "unknown"
	unavailable.Operations = []MediaOperation{MediaOperationTextToImage}
	repo := &mediaModelRepositoryWithAliasesStub{
		mediaModelRepoStub: mediaModelRepoStub{items: []MediaModelDefinition{ready, unavailable}},
		mediaModelAliasRepoStub: mediaModelAliasRepoStub{items: []MediaModelAlias{{RequestedModelID: "unknown-alias", ModelDefinitionID: 2}}},
	}
	_, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("xai-image", "xai", "grok-2-image"))
	registry := NewMediaModelRegistryWithResolver(repo, resolver)
	require.NoError(t, registry.Refresh(context.Background()))

	definition, err := registry.Resolve("grok-2-image", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "xai-image", definition.AdapterResolution.ResolvedAdapter)

	_, err = registry.Resolve("unknown-alias", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
	require.Equal(t, "MEDIA_MODEL_ADAPTER_UNAVAILABLE", infraerrors.Reason(err))
}

func testMediaAdapterRegistrationForDefinitions(
	key string,
	mode NativeAsyncMode,
	definitions ...MediaModelDefinition,
) MediaAdapterRegistration {
	operationSet := map[MediaOperation]struct{}{}
	exactRules := make([]MediaAdapterExactRule, 0, len(definitions))
	for _, definition := range definitions {
		operations := append([]MediaOperation(nil), definition.Operations...)
		for _, operation := range operations {
			operationSet[operation] = struct{}{}
		}
		exactRules = append(exactRules, MediaAdapterExactRule{
			Vendor: definition.Vendor,
			ModelID: definition.ModelID,
			Capabilities: MediaAdapterRuleCapabilities{
				Operations:          operations,
				SyncUpstream:        mode != NativeAsyncRequired,
				NativeAsyncUpstream: mode != NativeAsyncUnsupported,
			},
		})
	}
	operations := make([]MediaOperation, 0, len(operationSet))
	for operation := range operationSet {
		operations = append(operations, operation)
	}
	slices.Sort(operations)
	return MediaAdapterRegistration{
		Key:                 key,
		Adapter:             NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: key, NativeAsyncMode: mode}),
		SupportedOperations: operations,
		ExactRules:          exactRules,
	}
}
```

另加测试：数据库读取错误与 duplicate canonical/alias 仍保留整个旧快照；单模型 capability mismatch 发布新快照并剔除旧 route；`implementation_missing` 返回刷新错误并保留整个旧快照；内部 `AdapterResolution` JSON 不出现在 v1 定义。测试 helper 使用 `slices.Sort` 固定 operation 顺序，因此补充标准库 `slices` import。

- [ ] **Step 2：运行 Registry 测试并确认失败**

Run:

```bash
cd backend
go test ./internal/service -run 'MediaModelRegistry' -count=1
```

Expected: FAIL，错误包含 `undefined: NewMediaModelRegistryWithResolver` 或 `MediaModelDefinition has no field or method AdapterResolution`。

- [ ] **Step 3：保留旧快照字段并新增不序列化的解析结果**

修改 `MediaModelDefinition`。旧字段只作为兼容载体，新的解析结果必须 `json:"-"`。

```go
type MediaModelDefinition struct {
	ID                int64
	ModelID           string
	Vendor            string
	MediaType         MediaType
	Operations        []MediaOperation
	Constraints       json.RawMessage
	BillingUnit       string
	DefaultAdapter    string
	DefaultAsyncMode  NativeAsyncMode
	AdapterResolution MediaAdapterResolution `json:"-"`
	Enabled           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
```

拆分校验：`validateMediaModelDefinitionBase` 不检查 enabled、旧 Adapter 或旧 async，但必须完整校验规范模型 ID 格式、vendor 简单标识符、media type、operations 非空/唯一/与媒体类型匹配、constraints、billing unit 简单标识符；不能只检查非空。Worker 和 Admin 后续复用基础校验。

```go
func validateMediaModelDefinitionBase(definition MediaModelDefinition) error {
	if !isValidMediaModelIdentifier(definition.ModelID) {
		return errors.New("media model id has invalid format")
	}
	if !isValidMediaSimpleIdentifier(definition.Vendor, 64) {
		return errors.New("media model vendor has invalid format")
	}
	if !isValidMediaSimpleIdentifier(definition.BillingUnit, 32) {
		return errors.New("media model billing unit has invalid format")
	}
	return validateMediaModelShapeAndConstraints(definition)
}

func validateMediaModelShapeAndConstraints(definition MediaModelDefinition) error {
	if definition.MediaType != MediaTypeImage && definition.MediaType != MediaTypeVideo {
		return fmt.Errorf("unsupported media type %q", definition.MediaType)
	}
	if len(definition.Operations) == 0 {
		return errors.New("operations are empty")
	}
	seen := make(map[MediaOperation]struct{}, len(definition.Operations))
	for _, operation := range definition.Operations {
		mediaType, ok := mediaTypeForOperation(operation)
		if !ok {
			return fmt.Errorf("unsupported media operation %q", operation)
		}
		if mediaType != definition.MediaType {
			return fmt.Errorf("media operation %q does not match media type %q", operation, definition.MediaType)
		}
		if _, exists := seen[operation]; exists {
			return fmt.Errorf("duplicate media operation %q", operation)
		}
		seen[operation] = struct{}{}
	}
	constraints, err := decodeMediaModelConstraints(definition.Constraints)
	if err != nil {
		return err
	}
	return validateMediaModelConstraints(definition.MediaType, constraints)
}

func validateEnabledMediaModelDefinition(definition MediaModelDefinition) error {
	if !definition.Enabled {
		return errors.New("disabled model returned by enabled model repository")
	}
	return validateMediaModelDefinitionBase(definition)
}

// Task 2 期间保留旧名字作为显式 wrapper，确保尚未在 Task 3/4 迁移的调用点可编译；
// 最终调用点迁移完成后可删除该 wrapper。
func validateMediaModelDefinition(definition MediaModelDefinition) error {
	return validateEnabledMediaModelDefinition(definition)
}

func (r *MediaAdapterResolver) ResolveDefinition(definition MediaModelDefinition) MediaAdapterResolution {
	if err := validateMediaModelDefinitionBase(definition); err != nil {
		return mediaAdapterResolutionFailure(
			MediaAdapterResolutionInvalidDefinition,
			"MEDIA_MODEL_DEFINITION_INVALID",
		)
	}
	return r.Resolve(definition.Vendor, definition.ModelID, definition.Operations)
}
```

`validateMediaModelShapeAndConstraints` 是从现有 `validateMediaModelDefinition` 原样提取的 media type、operations 和 constraints 校验体，不新增另一套规则。增加 model ID 含空格、vendor 含点号、billing unit 为空/含空格的表驱动测试，均发布 `invalid_definition` tombstone；旧 DB Adapter/async 任意值不影响结果。

`ResolveDefinition` 是 Registry 与 Admin 的唯一模型解析入口；不得在其他路径重复拼接 `vendor/model_id/operations` 调三参数 `Resolve`。它不要求 `Enabled=true`，所以禁用模型同样能得到诊断状态。

- [ ] **Step 4：实现原子 ready 索引和 unavailable tombstone**

给快照增加互不混用的索引：

```go
var ErrMediaModelAdapterUnavailable = infraerrors.ServiceUnavailable(
	"MEDIA_MODEL_ADAPTER_UNAVAILABLE",
	"media model adapter is unavailable",
)

type mediaModelUnavailableTombstone struct {
	CanonicalModelID string
	Resolution       MediaAdapterResolution
}

type mediaModelRegistrySnapshot struct {
	models             map[string]MediaModelDefinition
	aliases            map[string]string
	unavailableModels  map[string]mediaModelUnavailableTombstone
	unavailableAliases map[string]string
}

type MediaModelRegistry struct {
	repo           MediaModelDefinitionRepository
	aliasRepo      MediaModelAliasRepository
	resolver       *MediaAdapterResolver
	routingMetrics MediaRoutingMetrics
	logger         *slog.Logger
	snapshot       atomic.Value // mediaModelRegistrySnapshot
	refreshMu      sync.Mutex
	startOnce      sync.Once
}
```

新增 `NewMediaModelRegistryWithResolver(repo, resolver, aliasRepos...)`；Resolver 为 nil 时只构造一个空代码 Resolver，严禁回退读取 DB Adapter。旧 `NewMediaModelRegistry` 仅作为测试/兼容构造 wrapper：

```go
func NewMediaModelRegistry(repo MediaModelDefinitionRepository, aliasRepos ...MediaModelAliasRepository) *MediaModelRegistry {
	return NewMediaModelRegistryWithResolver(repo, NewMediaAdapterResolver(NewMediaAdapterRegistry()), aliasRepos...)
}

func NewMediaModelRegistryWithResolver(
	repo MediaModelDefinitionRepository,
	resolver *MediaAdapterResolver,
	aliasRepos ...MediaModelAliasRepository,
) *MediaModelRegistry {
	if resolver == nil {
		resolver = NewMediaAdapterResolver(NewMediaAdapterRegistry())
	}
	registry := &MediaModelRegistry{repo: repo, resolver: resolver, logger: slog.Default()}
	if len(aliasRepos) > 0 {
		registry.aliasRepo = aliasRepos[0]
	} else if aliasRepo, ok := repo.(MediaModelAliasRepository); ok {
		registry.aliasRepo = aliasRepo
	}
	registry.snapshot.Store(mediaModelRegistrySnapshot{
		models: map[string]MediaModelDefinition{}, aliases: map[string]string{},
		unavailableModels: map[string]mediaModelUnavailableTombstone{}, unavailableAliases: map[string]string{},
	})
	return registry
}
```

生产 Wire 不得调用旧 wrapper。

`Refresh` 按以下顺序构建完整新快照：

1. 读取 definitions 和 aliases；读取失败直接返回，不 Store。
2. 规范化 identity 并检查重复 canonical ID/definition ID。
3. 每个定义先检查 `definition.Enabled`；`ListEnabled` 若错误返回 disabled definition，视为 Repository 全局契约错误，立即返回并保留旧快照。随后只调用 `resolver.ResolveDefinition`；ready 写 `models`；`invalid_definition/unresolved/ambiguous/capability_mismatch` 写单模型 tombstone。
4. `implementation_missing` 属于实例代码注册完整性错误：立即返回错误、不发布半成品并保留整个旧快照；首次启动由 Wire Provider 直接失败。
5. 全局检查 alias 重复及与全部 canonical ID 的冲突。
6. 指向 ready 定义的 alias 写 `aliases`；指向 tombstone 的 alias 写 `unavailableAliases`。
7. 只有全局检查通过后一次 `snapshot.Store(next)`。

增加 `TestMediaModelRegistryRejectsDisabledDefinitionFromEnabledRepositoryWithoutPublishing`：先发布一个 ready 旧快照，再让 Repository 返回 `Enabled=false` 记录，断言 Refresh 返回包含 `disabled model returned by enabled model repository` 的错误，旧模型仍可解析，disabled 模型不进入 ready/tombstone 索引。

ready 定义写快照前使用代码派生兼容值：

```go
definition.AdapterResolution = resolution
definition.DefaultAdapter = resolution.ResolvedAdapter
definition.DefaultAsyncMode = resolution.CompatibilityAsyncMode()
```

`Resolve` 和 `CanonicalModelID` 先处理 ready alias/model，再检查 unavailable alias/model；命中 tombstone 时必须返回完整诊断 metadata，不得返回 definition：

```go
return ErrMediaModelAdapterUnavailable.WithMetadata(map[string]string{
	"model_id":          tombstone.CanonicalModelID,
	"resolution_status": string(tombstone.Resolution.Status),
	"reason_code":       tombstone.Resolution.ReasonCode,
})
```

`MediaModelRegistry` 增加 `routingMetrics MediaRoutingMetrics`、`logger *slog.Logger` 字段和 `SetRoutingMetrics`/`SetLogger`；构造函数默认 logger 为 `slog.Default()`，nil metrics 为 no-op。每个非 ready resolution 调用 `IncrementAdapterResolutionFailure(status)`，并写 `media_model_adapter_resolution_unavailable`，字段固定为 `canonical_model_id`、`vendor`、`adapter_resolution_status`、`adapter_key`、`matched_by`、`matched_family`、`reason_code`；`implementation_missing` 使用 error 级别，其余 tombstone 使用 warn。测试注入 `AtomicMediaTaskMetrics`，逐状态断言计数；日志测试用 `bytes.Buffer` + `slog.NewJSONHandler` 构造 logger 后调用 `SetLogger`，只断言上述安全字段且确认没有 constraints、请求体或凭证。

- [ ] **Step 5：补齐 clone 和快照 JSON 兼容断言**

`cloneMediaModelDefinition` 深拷贝解析能力中的 operations；测试 Marshal 后包含 `DefaultAdapter/DefaultAsyncMode` 且不包含 `AdapterResolution`。

```go
func cloneMediaAdapterResolution(input MediaAdapterResolution) MediaAdapterResolution {
	copy := input
	if input.Capabilities != nil {
		capabilities := *input.Capabilities
		capabilities.Operations = append([]MediaOperation(nil), input.Capabilities.Operations...)
		copy.Capabilities = &capabilities
	}
	return copy
}
```

在 `media_model_registry_test.go` 中，所有期望 definition 为 ready 的现有 `NewMediaModelRegistry(...)` 用例都改为显式创建 `testMediaAdapterRegistrationForDefinitions(...)`、Resolver 和 `NewMediaModelRegistryWithResolver(...)`；空仓库/not-found 以及故意验证 invalid/unresolved tombstone 的用例使用空 Resolver，不能拿非法 definition 去构造 registration。把旧 `TestMediaModelRegistryRefreshValidatesRoutingMetadata` 拆开：missing vendor 断言发布 `invalid_definition` tombstone；missing DB adapter 和非法 DB async mode 在代码 exact 规则存在时都必须 ready，且快照中的兼容字段被代码派生值覆盖。`media_model_repo_test.go` 的 ready Registry 用例同样显式注册 exact 规则。测试 helper 不能从 `DefaultAdapter` 自动决定生产路由，只能由测试调用者明确传入 key/mode。

- [ ] **Step 6：运行 Registry 与 Repository 回归测试**

Run:

```bash
cd backend
gofmt -w internal/service/media_model_registry.go internal/service/media_model_registry_test.go internal/repository/media_model_repo_test.go
go test ./internal/service -run 'MediaModelRegistry|MediaModelConstraints' -count=1
go test ./internal/repository -run 'MediaModel' -count=1
```

Expected: 全部 PASS；旧数据库列内容不再影响 Registry 解析结果。

- [ ] **Step 7：提交模型 Registry 故障关闭**

```bash
git add backend/internal/service/media_model_registry.go backend/internal/service/media_model_registry_test.go backend/internal/repository/media_model_repo_test.go
git commit -m "feat(media): resolve model adapters in registry"
```

---

### Task 3：让 Scheduler/Worker 只使用解析结果和冻结的 v1 候选

**Files:**
- Create: `backend/internal/service/media_candidate_snapshot_codec.go`
- Create: `backend/internal/service/media_candidate_snapshot_codec_test.go`
- Modify: `backend/internal/service/media_scheduler.go`
- Modify: `backend/internal/service/media_scheduler_test.go`
- Modify: `backend/internal/service/media_worker.go`
- Modify: `backend/internal/service/media_worker_test.go`
- Modify: `backend/internal/service/media_worker_lifecycle_test.go`
- Modify: `backend/internal/service/media_orchestrator.go`
- Modify: `backend/internal/service/media_orchestrator_test.go`
- Modify: `backend/internal/repository/media_worker_integration_test.go`

- [ ] **Step 1：写规则能力、optional 和两阶段冻结失败测试**

增加三个测试组：Scheduler 忽略 DB 旧值；历史 optional 语义不变；新快照仍是 v1 顶层数组。

```go
func TestMediaSchedulerUsesResolutionInsteadOfStoredDefaultAdapter(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ID = 31
	definition.ModelID = "grok-2-image"
	definition.Vendor = "xai"
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	definition.DefaultAdapter = "malicious-db-value"
	definition.DefaultAsyncMode = NativeAsyncRequired

	adapters, resolver := newTestMediaAdapterResolver(t,
		exactImageRegistration("xai-image", "xai", "grok-2-image"),
	)
	models := NewMediaModelRegistryWithResolver(
		&mediaModelRepoStub{items: []MediaModelDefinition{definition}},
		resolver,
	)
	require.NoError(t, models.Refresh(context.Background()))
	account := Account{
		ID: 21, Platform: PlatformMedia, Priority: 1, Concurrency: 1,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"media_config": map[string]any{
			"version": 1, "provider": "xai", "models": map[string]any{
				"grok-2-image": map[string]any{
					"enabled": true, "upstream_model_id": "grok-upstream", "async_mode": "unsupported",
				},
			},
		}},
	}
	accountRepo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	metrics := NewAtomicMediaTaskMetrics()
	scheduler := NewMediaScheduler(
		accountRepo,
		&mediaSchedulerSelectorStub{selectedID: account.ID},
		adapters,
		&mediaSchedulerGroupRepoStub{group: &Group{ID: 9, Platform: PlatformMedia}},
		models,
		&mediaSchedulerScopeRepoStub{modelIDs: []string{definition.ModelID}},
		MediaRoutingMetrics(metrics),
	)
	candidates, err := scheduler.SnapshotCandidatesForOperation(
		context.Background(), 9, definition.ModelID, MediaOperationTextToImage,
	)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "xai-image", candidates[0].ResolvedModel.Adapter)

	account.Extra = map[string]any{"media_config": map[string]any{
		"version": 1, "provider": "xai", "models": map[string]any{
			"grok-2-image": map[string]any{
				"enabled": true, "upstream_model_id": "grok-upstream", "async_mode": "native",
			},
		},
	}}
	accountRepo.replaceAccounts([]Account{account})
	_, err = scheduler.SnapshotCandidatesForOperation(
		context.Background(), 9, definition.ModelID, MediaOperationTextToImage,
	)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, int64(1), metrics.CandidateCapabilityMismatches())
}

func TestMediaSchedulerKeepsLegacyOptionalSemantics(t *testing.T) {
	resolution := readyResolution(true, true)
	require.True(t, resolution.SupportsAccountMode(NativeAsyncOptional))
	require.Equal(t, MediaExecutionPathSync, chooseMediaExecutionPath(false, NativeAsyncOptional))
	require.Equal(t, MediaExecutionPathNativeAsync, chooseMediaExecutionPath(true, NativeAsyncOptional))
}

func TestMediaSchedulerPreservesUnavailableTombstoneError(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ID = 41
	definition.ModelID = "unknown-image"
	definition.Vendor = "unknown"
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	repo := &mediaModelRepositoryWithAliasesStub{
		mediaModelRepoStub: mediaModelRepoStub{items: []MediaModelDefinition{definition}},
		mediaModelAliasRepoStub: mediaModelAliasRepoStub{items: []MediaModelAlias{{
			RequestedModelID: "unknown-alias", ModelDefinitionID: definition.ID,
		}}},
	}
	_, resolver := newTestMediaAdapterResolver(t)
	models := NewMediaModelRegistryWithResolver(repo, resolver)
	require.NoError(t, models.Refresh(context.Background()))
	scheduler := NewMediaScheduler(
		&mediaSchedulerAccountRepoStub{},
		&mediaSchedulerSelectorStub{},
		NewMediaAdapterRegistry(),
		&mediaSchedulerGroupRepoStub{group: &Group{ID: 9, Platform: PlatformMedia}},
		models,
		&mediaSchedulerScopeRepoStub{modelIDs: []string{definition.ModelID}},
	)
	for _, requested := range []string{definition.ModelID, "unknown-alias"} {
		_, err := scheduler.SnapshotCandidatesForOperation(
			context.Background(), 9, requested, MediaOperationTextToImage,
		)
		require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
	}
}

func TestMediaOrchestratorRejectsUnavailableBeforeCreatePricingAndPrecharge(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	definition := validImageModelDefinition()
	definition.ID = 51
	definition.ModelID = "unknown-image"
	definition.Vendor = "unknown"
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	_, resolver := newTestMediaAdapterResolver(t)
	models := NewMediaModelRegistryWithResolver(
		&mediaModelRepoStub{items: []MediaModelDefinition{definition}}, resolver,
	)
	require.NoError(t, models.Refresh(context.Background()))
	fixture.orchestrator.deps.Registry = models
	req := validAsyncMediaCreateRequest()
	req.RequestedModel = definition.ModelID
	_, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
	require.Zero(t, fixture.repo.createCalls())
	require.Zero(t, fixture.scheduler.snapshotCalls())
	require.Zero(t, fixture.pricing.snapshotCalls())
	require.Zero(t, fixture.billing.prechargeCalls())
	require.Zero(t, fixture.queue.enqueueCalls())
	require.Zero(t, fixture.controller.stopCalls())
}
```

扩展现有 Worker 测试，证明安全提交前失败可在冻结候选集合内切换；一旦有 upstream task ID 或 `SubmissionUnknown=true`，账号与 Adapter 不再切换。

为该测试给现有 `orchestratorScheduler` 增加 `snapshots int` 与 `snapshotCalls() int`，在 `SnapshotCandidates` 递增；给 `orchestratorPricing` 增加 `snapshots int` 与 `snapshotCalls() int`，在 `Snapshot` 递增。这样断言覆盖真正的预扣前边界，而不是只测 Handler 错误映射。

```go
func (s *orchestratorScheduler) snapshotCalls() int { return s.snapshots }
func (p *orchestratorPricing) snapshotCalls() int   { return p.snapshots }
```

- [ ] **Step 2：写硬编码 v1 wire fixture 测试**

在 `media_candidate_snapshot_codec_test.go` 使用硬编码历史 JSON，而不是当前结构先 Marshal 再 Unmarshal：

```go
const historicalMediaCandidateSnapshotV1 = `[{"account_id":7,"platform":"media","resolved_model":{"Provider":"relay","Adapter":"legacy-image","UpstreamModel":"upstream-image","NativeAsyncMode":"optional","RequestMapping":{}},"model_definition":{"ID":9,"ModelID":"grok-2-image","Vendor":"xai","MediaType":"image","Operations":["text_to_image"],"Constraints":{},"BillingUnit":"image","DefaultAdapter":"legacy-image","DefaultAsyncMode":"optional","Enabled":true,"CreatedAt":"2026-07-21T00:00:00Z","UpdatedAt":"2026-07-21T00:00:00Z"}}]`

type legacyStrictResolvedModelV1 struct {
	Provider        string              `json:"Provider"`
	Adapter         string              `json:"Adapter"`
	UpstreamModel   string              `json:"UpstreamModel"`
	NativeAsyncMode NativeAsyncMode     `json:"NativeAsyncMode"`
	RequestMapping  MediaRequestMapping `json:"RequestMapping"`
}

type legacyStrictMediaModelDefinitionV1 struct {
	ID               int64            `json:"ID"`
	ModelID          string           `json:"ModelID"`
	Vendor           string           `json:"Vendor"`
	MediaType        MediaType        `json:"MediaType"`
	Operations       []MediaOperation `json:"Operations"`
	Constraints      json.RawMessage  `json:"Constraints"`
	BillingUnit      string           `json:"BillingUnit"`
	DefaultAdapter   string           `json:"DefaultAdapter"`
	DefaultAsyncMode NativeAsyncMode  `json:"DefaultAsyncMode"`
	Enabled          bool             `json:"Enabled"`
	CreatedAt        time.Time        `json:"CreatedAt"`
	UpdatedAt        time.Time        `json:"UpdatedAt"`
}

type legacyStrictMediaCandidateV1 struct {
	AccountID       int64                                `json:"account_id"`
	Platform        string                               `json:"platform"`
	ResolvedModel   legacyStrictResolvedModelV1          `json:"resolved_model"`
	ResolvedRequest json.RawMessage                      `json:"resolved_request,omitempty"`
	ModelDefinition *legacyStrictMediaModelDefinitionV1  `json:"model_definition,omitempty"`
}

func TestMediaCandidateSnapshotV1StrictlyDecodesHistoricalJSON(t *testing.T) {
	candidates, err := decodeMediaCandidateSnapshotV1(json.RawMessage(historicalMediaCandidateSnapshotV1))
	require.NoError(t, err)
	require.Equal(t, "legacy-image", candidates[0].ResolvedModel.Adapter)
	require.Equal(t, NativeAsyncOptional, candidates[0].ResolvedModel.NativeAsyncMode)

	raw, err := encodeMediaCandidateSnapshotV1(candidates)
	require.NoError(t, err)
	require.Equal(t, byte('['), bytes.TrimSpace(raw)[0])
	require.NotContains(t, string(raw), "AdapterResolution")
}

func TestMediaCandidateSnapshotV1NewWriterIsReadableByLegacyStrictReader(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ModelID = "grok-2-image"
	definition.Vendor = "xai"
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	definition.DefaultAdapter = "stale-db-key"
	definition.DefaultAsyncMode = NativeAsyncRequired
	definition.AdapterResolution = readyResolution(true, false)
	definition.AdapterResolution.ResolvedAdapter = "xai-image"
	candidates := []MediaAccountCandidateSnapshot{{
		AccountID: 7,
		Platform: PlatformMedia,
		ResolvedModel: ResolvedMediaAccountModel{
			Provider: "xai", Adapter: "xai-image", UpstreamModel: "upstream-image",
			NativeAsyncMode: NativeAsyncUnsupported, RequestMapping: MediaRequestMapping{},
		},
		ModelDefinition: &definition,
	}}
	raw, err := encodeMediaCandidateSnapshotV1(candidates)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var legacy []legacyStrictMediaCandidateV1
	require.NoError(t, decoder.Decode(&legacy))
	var trailing json.RawMessage
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
	require.Equal(t, "xai-image", legacy[0].ModelDefinition.DefaultAdapter)
	require.Equal(t, NativeAsyncUnsupported, legacy[0].ModelDefinition.DefaultAsyncMode)
	require.Equal(t, "xai-image", legacy[0].ResolvedModel.Adapter)
}
```

在测试文件定义独立的 `legacyStrictMediaCandidateV1`、`legacyStrictResolvedModelV1`、`legacyStrictMediaModelDefinitionV1`，字段和 JSON tag 逐项复制历史 v1 wire，不复用生产 codec DTO。再加：未知嵌套字段、额外顶层 JSON 值必须失败；故意让 `DefaultAdapter` 与 `resolved_model.Adapter` 不同，断言新 Worker 只使用后者。

writer 转换规则必须内聚在 `encodeMediaCandidateSnapshotV1`：当 `ModelDefinition.AdapterResolution.IsReady()` 时，无条件用 resolution 派生 `DefaultAdapter/DefaultAsyncMode`；当 resolution 为空（历史快照 decode 后 durable rewrite）时原样保留旧兼容字段。不能依赖 Registry 在调用 codec 前碰巧改写过 Definition。

- [ ] **Step 3：运行调度/快照/Worker 测试并确认失败**

Run:

```bash
cd backend
go test ./internal/service -run 'Media(Scheduler|Worker|Orchestrator|CandidateSnapshot)' -count=1
```

Expected: FAIL，原因包括 codec 未定义、Scheduler 仍采用 DB `DefaultAdapter`，或冻结模型仍检查旧默认字段。

- [ ] **Step 4：实现显式 v1 codec 并替换所有读写点**

在 `media_candidate_snapshot_codec.go` 固定当前 wire 名称。外层字段继续 snake_case，内层历史结构继续 PascalCase。

```go
type resolvedMediaAccountModelV1 struct {
	Provider        string              `json:"Provider"`
	Adapter         string              `json:"Adapter"`
	UpstreamModel   string              `json:"UpstreamModel"`
	NativeAsyncMode NativeAsyncMode     `json:"NativeAsyncMode"`
	RequestMapping  MediaRequestMapping `json:"RequestMapping"`
}

type mediaModelDefinitionV1 struct {
	ID               int64              `json:"ID"`
	ModelID          string             `json:"ModelID"`
	Vendor           string             `json:"Vendor"`
	MediaType        MediaType          `json:"MediaType"`
	Operations       []MediaOperation   `json:"Operations"`
	Constraints      json.RawMessage    `json:"Constraints"`
	BillingUnit      string             `json:"BillingUnit"`
	DefaultAdapter   string             `json:"DefaultAdapter"`
	DefaultAsyncMode NativeAsyncMode    `json:"DefaultAsyncMode"`
	Enabled          bool               `json:"Enabled"`
	CreatedAt        time.Time          `json:"CreatedAt"`
	UpdatedAt        time.Time          `json:"UpdatedAt"`
}

type mediaCandidateSnapshotV1 struct {
	AccountID       int64                         `json:"account_id"`
	Platform        string                        `json:"platform"`
	ResolvedModel   resolvedMediaAccountModelV1   `json:"resolved_model"`
	ResolvedRequest json.RawMessage               `json:"resolved_request,omitempty"`
	ModelDefinition *mediaModelDefinitionV1       `json:"model_definition,omitempty"`
}
```

实现 `encodeMediaCandidateSnapshotV1`、`decodeMediaCandidateSnapshotV1` 及双向转换；decode 使用 `DisallowUnknownFields` 并调用 `validateMediaCandidateSnapshot`。把以下三个路径全部改走 codec：

- `media_orchestrator.go:252` 首次写入；
- `media_orchestrator.go:900-907` durable input 重写；
- `media_worker.go:2009` 严格读取。

- [ ] **Step 5：按解析能力和账号模式过滤候选**

在 `MediaAdapterResolution` 增加唯一账号模式判定：

```go
func (r MediaAdapterResolution) SupportsAccountMode(mode NativeAsyncMode) bool {
	if !r.IsReady() {
		return false
	}
	switch mode {
	case NativeAsyncUnsupported:
		return r.Capabilities.SyncUpstream
	case NativeAsyncRequired:
		return r.Capabilities.NativeAsyncUpstream
	case NativeAsyncOptional:
		return r.Capabilities.SyncUpstream && r.Capabilities.NativeAsyncUpstream
	default:
		return false
	}
}
```

在 `snapshotCandidates` 中删除从 `registryDefinition.DefaultAdapter` 取路由的逻辑，改成：

```go
resolution := registryDefinition.AdapterResolution
if !resolution.IsReady() || !resolution.SupportsAccountMode(resolved.NativeAsyncMode) {
	continue
}
resolved.Adapter = resolution.ResolvedAdapter
adapter, adapterErr := s.adapters.Resolve(resolved.Adapter)
if adapterErr != nil || !adapterSupportsNativeMode(adapter, resolved.NativeAsyncMode) {
	continue
}
```

账号 `provider`、Base URL 和 `upstream_model_id` 只能继续写入 route target，不能传给 Resolver。

`MediaScheduler` 在现有 `dependencies ...any` 中识别 `MediaRoutingMetrics`，并默认使用 `slog.Default()`；测试可额外注入 `*slog.Logger`。因 `SupportsAccountMode` 或实现 interface 不兼容而排除候选时调用 `IncrementCandidateCapabilityMismatch()`，写 `media_candidate_adapter_capability_mismatch` debug 日志，字段只含 `canonical_model_id`、`account_id`、`adapter_key`、`native_async_mode`。上面的绕过前端用例断言 counter 恰好加一；另用内存 JSON logger 断言日志不包含账号 credentials、Base URL、request mapping 或上游模型 ID。

- [ ] **Step 6：拆掉 Worker 对旧默认字段的信任**

`SnapshotCandidatesForDefinition` 使用 `validateEnabledMediaModelDefinition`，并要求传入的运行时 `definition.AdapterResolution.IsReady()`；否则返回带 model/status/reason metadata 的 `ErrMediaModelAdapterUnavailable`，不能静默退化成无候选。`resolveWorkerMediaModelDefinition` 只使用 `validateMediaModelDefinitionBase`，因为 v1 DTO 不序列化运行时 resolution。新候选继续从 Registry 得到代码派生的兼容 `DefaultAdapter/DefaultAsyncMode`，但 Worker 的 Adapter 只信 `candidate.ResolvedModel.Adapter`。

Worker 首次绑定冻结候选或恢复任务列中的 Adapter 前调用 Task 1 已提供的 `MediaAdapterRegistry.CanonicalKey`；`aliased=true` 时写一次 `media_worker_frozen_adapter_alias` info 日志，字段固定为 `task_id`、`legacy_adapter_key`、`adapter_key`，但继续用历史 key 查询实现并保持任务列/快照不变。`media_worker_test.go` 注入内存 logger，断言一次执行只写一条、可按 distinct `task_id` 聚合，且日志不含请求、凭证或上游响应。

本任务把 `media_scheduler_test.go`、`media_orchestrator_test.go`、`media_worker_test.go` 和 `media_worker_integration_test.go` 中所有装载非空 definition 的旧 `NewMediaModelRegistry(...)` fixture 改为显式 registration + Resolver；空 Registry/not-found 用例可保留旧构造。不得让测试从数据库旧 `DefaultAdapter` 自动生成生产解析结果。

保持当前两阶段状态机：queued 只能从 CandidateSnapshot 选；安全提交前失败可从同一集合换候选；一旦取得上游 ID 或提交状态未知，就固定任务列中的 account/adapter/upstream/native mode。

- [ ] **Step 7：运行定向、race 和 Repository 集成测试**

Run:

```bash
cd backend
gofmt -w internal/service/media_candidate_snapshot_codec.go internal/service/media_candidate_snapshot_codec_test.go internal/service/media_scheduler.go internal/service/media_scheduler_test.go internal/service/media_worker.go internal/service/media_worker_test.go internal/service/media_worker_lifecycle_test.go internal/service/media_orchestrator.go internal/service/media_orchestrator_test.go internal/repository/media_worker_integration_test.go
go test ./internal/service -run 'Media(Scheduler|Worker|Orchestrator|CandidateSnapshot)' -count=1
go test -race ./internal/service -run 'Media(Scheduler|Worker)' -count=1
go test ./internal/repository -run 'MediaWorker' -count=1
```

Expected: 全部 PASS；race 无告警；旧 v1 快照恢复测试继续通过。

- [ ] **Step 8：提交调度与任务冻结改造**

```bash
git add backend/internal/service/media_candidate_snapshot_codec.go backend/internal/service/media_candidate_snapshot_codec_test.go backend/internal/service/media_scheduler.go backend/internal/service/media_scheduler_test.go backend/internal/service/media_worker.go backend/internal/service/media_worker_test.go backend/internal/service/media_worker_lifecycle_test.go backend/internal/service/media_orchestrator.go backend/internal/service/media_orchestrator_test.go backend/internal/repository/media_worker_integration_test.go
git commit -m "feat(media): schedule with resolved adapters"
```

---

### Task 4：改造管理 Service、Repository、分组校验和发布 preflight

**Files:**
- Modify: `backend/internal/service/media_model_admin.go`
- Modify: `backend/internal/service/media_model_admin_test.go`
- Modify: `backend/internal/repository/media_model_repo.go`
- Modify: `backend/internal/repository/media_model_repo_test.go`
- Modify: `backend/internal/repository/group_media_model_scope_repo_test.go`
- Modify: `backend/internal/handler/admin/media_model_handler_test.go` (仅同步构造 fixture 与初始 ready 数据，API DTO 在 Task 5 完成)
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/service/wire_test.go`
- Modify: `backend/cmd/server/wire_gen.go`

- [ ] **Step 1：写启用原子校验、旧列隔离和 preflight 失败测试**

覆盖以下行为：disabled unresolved 可保存；enabled unresolved 在 Repository 调用前返回稳定 4xx；List/Get/Create/Update 都带实时解析结果；新建使用 DB 默认旧列；更新保留已有旧列；preflight 不写数据并比较规范 key。

```go
func TestMediaModelAdminRejectsEnabledUnresolvedBeforeWrite(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	resolver := NewMediaAdapterResolver(NewMediaAdapterRegistry())
	registry := NewMediaModelRegistryWithResolver(store, resolver)
	svc := NewMediaModelAdminService(store, nil, nil, registry, resolver)
	record := validMediaModelAdminServiceRecord()
	record.Definition.Enabled = true

	_, err := svc.Create(context.Background(), record)
	require.Error(t, err)
	require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", infraerrors.Reason(err))
	require.Nil(t, store.record)
	require.Zero(t, store.writeCount)
}

func TestMediaModelPreflightIsReadOnly(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	record := validMediaModelAdminServiceRecord()
	record.Definition.ModelID = "grok-2-image"
	record.Definition.Vendor = "xai"
	record.LegacyDefaultAdapter = "xai-image"
	record.LegacyDefaultAsyncMode = NativeAsyncUnsupported
	store.record = &record
	_, resolver := newTestMediaAdapterResolver(t,
		exactImageRegistration("xai-image", "xai", "grok-2-image"),
	)
	svc := NewMediaModelAdminService(store, nil, nil, nil, resolver)
	report, err := svc.Preflight(context.Background())
	require.NoError(t, err)
	require.True(t, report.Safe)
	require.Len(t, report.Items, 1)
	require.True(t, report.Items[0].LegacyCheckApplicable)
	require.Equal(t, 0, store.writeCount)
}
```

在现有 `mediaModelAdminServiceStore` 增加 `writeCount int`；`CreateAdmin`、`UpdateAdmin`、`DeleteAdmin` 进入方法后先递增它。不要新增第二套 store。fixture 的 List/Get/Create/Update 返回副本时同时深拷贝 `Definition.Operations`、`Definition.Constraints` 和 `Aliases`；`AdapterResolution` 含 capabilities 指针与 operations slice，必须调用 `cloneMediaAdapterResolution`，不能按标量浅拷贝；`LegacyDefault*` 原样复制。另加 disabled unresolved 用例，断言可保存但返回的 `AdapterResolution.Status == MediaAdapterResolutionUnresolved`。

同步修改现有 `TestMediaModelAdminServiceStrictValidation`：删除 `DefaultAdapter="bad adapter"` 和 `DefaultAsyncMode="sometimes"` 两个“应失败”表项；新增独立用例把这两个兼容字段设为任意旧值，断言基础校验结果不受影响且最终 resolution 只来自代码。模型 ID、vendor、media type、operations、constraints 和 aliases 的严格校验继续保留。

Repository 测试直接查 Ent entity：Create 得到 `default_adapter=""`、`default_async_mode="unsupported"`；Update 业务字段后，预先写入的旧列值保持不变。

再加 preflight 用例：ready enabled 记录的 `LegacyDefaultAdapter=""` 时 `LegacyCheckApplicable=false`、`RolloutSafe=true`；相同记录旧 key 非空但不匹配时 `LegacyCheckApplicable=true`、`RolloutSafe=false`、`BlockingCount=1`；旧值为 `" OpenAI-Images "`、解析 key 为 `openai-images` 时按旧 Registry 的规范化语义判定匹配，同时报告仍返回未改写的原始旧值。

- [ ] **Step 2：运行管理 Service/Repository 测试并确认失败**

Run:

```bash
cd backend
go test ./internal/service -run 'MediaModelAdmin|MediaModelPreflight' -count=1
go test ./internal/repository -run 'MediaModel' -count=1
```

Expected: FAIL，原因是构造函数尚无 Resolver、Preflight 不存在，或 Repository 仍写旧列。

- [ ] **Step 3：让 Admin 记录携带解析结果与隔离的旧列诊断值**

扩展管理记录和 preflight 类型：

```go
type MediaModelAdminRecord struct {
	Definition             MediaModelDefinition
	Aliases                []string
	AdapterResolution      MediaAdapterResolution
	LegacyDefaultAdapter   string
	LegacyDefaultAsyncMode NativeAsyncMode
}

type MediaAdapterPreflightItem struct {
	ModelID                 string
	Enabled                 bool
	Status                  MediaAdapterResolutionStatus
	ResolvedAdapter         string
	LegacyDefaultAdapter    string
	LegacyCheckApplicable   bool
	AdapterKeyMatches       bool
	LegacyDefaultAsyncMode  NativeAsyncMode
	LegacyAsyncModeReadable bool
	ReasonCode              string
	RolloutSafe             bool
}

type MediaAdapterPreflightReport struct {
	Safe          bool
	BlockingCount int
	Items         []MediaAdapterPreflightItem
}
```

给 `MediaModelAdminService` 注入 `resolver *MediaAdapterResolver`。`normalizeAndValidateMediaModelAdminRecord` 只做完整基础校验与 alias 校验；Create/Update 在写库前调用 Resolver，enabled 且非 ready 时返回 `infraerrors.BadRequest(resolution.ReasonCode, "media adapter is not ready")`。disabled 仍携带解析状态但允许写入。

Create/Update 的顺序固定为：基础 normalize/alias 校验 → `resolver.ResolveDefinition` → 若 `Enabled && !resolution.IsReady()` 立即返回（此时不得调用 `CreateAdmin`/`UpdateAdmin`）→ 将 resolution 只写入返回 read model 的内存字段 → Repository 事务写业务字段。Update 先读取现有记录检查 model ID 不可变，再执行上述 resolution，任何失败都不落库。

构造函数签名固定为 `NewMediaModelAdminService(models, scopes, groups, registry, resolver)`；同一任务内更新 `media_model_admin_test.go` 的两个既有调用点、Handler fixture 和生产 Wire 的调用点，不能保留隐式创建第二个 Resolver 的兼容构造分支。

List/Get/Create/Update 的返回值统一经过：

```go
func (s *MediaModelAdminService) enrichAdapterResolution(record MediaModelAdminRecord) MediaModelAdminRecord {
	definition := cloneMediaModelDefinition(record.Definition)
	resolution := s.resolver.ResolveDefinition(definition)
	definition.AdapterResolution = cloneMediaAdapterResolution(resolution)
	record.Definition = definition
	record.AdapterResolution = cloneMediaAdapterResolution(resolution)
	return record
}
```

真值规则固定为：`MediaModelDefinition.AdapterResolution` 只服务运行时 Registry/Scheduler，`MediaModelAdminRecord.AdapterResolution` 是管理 API read model；二者必须由同一次 `ResolveDefinition` 结果赋值，Repository 不持久化任何一个。增加 List/Get/Create/Update 表驱动测试，逐项断言两份状态、key、match、capabilities 和 reason 完全一致，防止漂移。

- [ ] **Step 4：停止新写入更新旧数据库列**

在 `normalizeAndValidateMediaModelAdminRecord` 删除旧 Adapter/async 的 normalize 与格式校验；只规范基础业务字段并调用 `validateMediaModelDefinitionBase`。在 `createMediaModelDefinition` 删除 `SetDefaultAdapter/SetDefaultAsyncMode`；在 `UpdateAdmin` 删除同名两个 setter。`ListEnabled` 不把实体旧列映射到运行时 definition；`mediaModelAdminRecordFromEntity` 只把旧值放入 `LegacyDefaultAdapter/LegacyDefaultAsyncMode`。

```go
// toServiceRecord 的 Definition.DefaultAdapter/DefaultAsyncMode 保持零值，
// 兼容输入槽不会进入这里；Repository 不从 AdapterResolution 回写旧列。
record := MediaModelAdminRecord{Definition: definition, Aliases: aliases}
```

Ent schema、生成代码和迁移文件保持不变。

- [ ] **Step 5：实现 ready 分组校验**

`ReplaceGroupScopes` 在 Repository 写入前逐个调用 Registry：

```go
for _, modelID := range normalized {
	canonical, resolveErr := s.registry.CanonicalModelID(modelID)
	if resolveErr != nil || canonical != modelID {
		return nil, ErrMediaModelScopeModelNotFound
	}
}
```

`GetGroupScopes` 改用现有 `ListMediaModelIDs` 返回全部持久化授权，包括 disabled/tombstone 授权，这样前端才能展示并删除它们；已被数据库删除的模型因外键级联不会出现在响应中。Scheduler 继续只消费 `ListEnabledMediaModelIDs` 并经过 Registry ready 校验。`ReplaceGroupScopes` 只验证请求中仍保留或新增的 ID 必须是 canonical+ready，管理员从提交数组中移除历史失效 ID 时 Repository 的全量替换会删除旧记录。测试必须先 seed 一个 disabled/tombstone 授权，断言 GET 可见、提交不含它后数据库已删除；不得通过 GET 静默过滤造成不可管理的幽灵授权。

- [ ] **Step 6：实现只读 preflight 报告**

`Preflight` 只调用 `ListAdmin`，不调用 Create/Update/Delete 或 Registry.Refresh。报告收录 enabled 和 disabled 模型，便于提前修复历史记录；只有 enabled 且不满足发布条件的项计入阻断。`mediaAdapterPreflightItem` 令 `LegacyCheckApplicable = strings.TrimSpace(LegacyDefaultAdapter) != ""`；不适用时 `AdapterKeyMatches` 与 `LegacyAsyncModeReadable` 都置 true，新切换后按数据库默认空旧 key 创建的模型不会永久阻断后续发布。适用时先执行 `normalizedLegacy := normalizeMediaAdapterName(LegacyDefaultAdapter)`，再与 `resolution.ResolvedAdapter` 严格比较，复刻旧 Registry 的 trim/lower 查找语义；报告中的 `LegacyDefaultAdapter` 仍保留原始数据库值。legacy async 只接受 trim/lower 后的 `unsupported|optional|required`。

空旧 key 表示旧版本没有可保留的有效 Adapter 路由，不视为滚动兼容对象。`LegacyCheckApplicable=false` 主要覆盖切换后新写入的记录；首次迁移时若 enabled 旧记录本身为空 key，preflight 的 safe 只表示“没有一条原本可工作的旧 Adapter 路由会被破坏”，不表示该记录能在旧版本实例上参与媒体路由。

```go
func (s *MediaModelAdminService) Preflight(ctx context.Context) (*MediaAdapterPreflightReport, error) {
	records, err := s.models.ListAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("list media models for preflight: %w", err)
	}
	report := &MediaAdapterPreflightReport{Safe: true, Items: []MediaAdapterPreflightItem{}}
	for _, record := range records {
		enriched := s.enrichAdapterResolution(record)
		item := mediaAdapterPreflightItem(enriched)
		legacyCompatible := !item.LegacyCheckApplicable || (item.AdapterKeyMatches && item.LegacyAsyncModeReadable)
		compatible := item.Status == MediaAdapterResolutionReady && legacyCompatible
		item.RolloutSafe = !item.Enabled || compatible
		if item.Enabled && !compatible {
			report.Safe = false
			report.BlockingCount++
		}
		report.Items = append(report.Items, item)
	}
	return report, nil
}
```

- [ ] **Step 7：同步所有构造调用点并生成可启动 Wire**

在 `media_model_handler_test.go` 增加本包默认 registration，保证现有不传 variadic 参数的 enabled `gpt-image-2` 用例继续命中代码规则：

```go
func defaultMediaModelHandlerRegistration() service.MediaAdapterRegistration {
	return service.MediaAdapterRegistration{
		Key:                 "openai-images",
		Adapter:             service.NewFakeMediaAdapter(service.FakeMediaAdapterOptions{Name: "openai-images", NativeAsyncMode: service.NativeAsyncOptional}),
		SupportedOperations: []service.MediaOperation{service.MediaOperationTextToImage},
		ExactRules: []service.MediaAdapterExactRule{{
			Vendor: "openai", ModelID: "gpt-image-2",
			Capabilities: service.MediaAdapterRuleCapabilities{
				Operations: []service.MediaOperation{service.MediaOperationTextToImage},
				SyncUpstream: true, NativeAsyncUpstream: true,
			},
		}},
	}
}
```

fixture 签名改为 `newMediaModelHandlerFixture(t, platform, registrations...)`；`len(registrations)==0` 时使用上面的默认 registration，随后构造 Adapter Registry、Resolver、Model Registry，再把 Resolver 作为第五个参数传给 Admin Service。`cloneHandlerMediaRecord` 同步深拷贝 `Definition.AdapterResolution` 和 `record.AdapterResolution` 的 capabilities/operations，不能共享指针。分组 scope 既有测试先向 store seed `image-one/image-two` 两个 enabled definition，给 fixture 传覆盖这两个 ID 的 exact registration，执行 `registry.Refresh` 后再 PUT；重新按实际刷新次数断言，不能对空 Registry 写 scope。

同时把生产构造改成返回错误的 `buildMediaAdapterRegistry`/`ProvideMediaAdapterRegistry`、`ProvideMediaAdapterResolver`、带 Resolver/`MediaRoutingMetrics` 的 `ProvideMediaModelRegistry`；`ProvideMediaTaskMetrics` 返回 `*AtomicMediaTaskMetrics`，ProviderSet 同时 bind 为 `MediaTaskMetrics` 和 `MediaRoutingMetrics`，`ProvideMediaScheduler` 接收并注入 routing metrics。`wire_test.go` 用非法 registration 断言 builder 返回 error。运行 `go generate ./cmd/server`，确保生成代码使用五参数 Admin Service；这一步与 Service 签名必须在同一个提交内完成。

在 Scheduler、Worker、Admin Service 的调用点全部迁移到 `validateMediaModelDefinitionBase`/`validateEnabledMediaModelDefinition` 后，执行 `rg -n 'validateMediaModelDefinition\(' backend/internal/service`，只允许找到测试迁移期间的变更；删除兼容 wrapper，最终代码不再有旧的“默认 Adapter/默认 async”校验入口。

生产函数体固定如下（`ProvideMediaModelRegistry` 还要 `SetRoutingMetrics`、刷新并启动 5 秒周期刷新）：

```go
func buildMediaAdapterRegistry(registrations []MediaAdapterRegistration, metrics MediaRoutingMetrics) (*MediaAdapterRegistry, error) {
	registry := NewMediaAdapterRegistry()
	registry.SetRoutingMetrics(metrics)
	for _, registration := range registrations {
		if err := registry.RegisterDefinition(registration); err != nil {
			return nil, fmt.Errorf("register media adapter %q: %w", registration.Key, err)
		}
	}
	if err := registry.Validate(); err != nil {
		return nil, fmt.Errorf("validate media adapter registry: %w", err)
	}
	return registry, nil
}

func ProvideMediaAdapterRegistry(metrics MediaRoutingMetrics) (*MediaAdapterRegistry, error) {
	return buildMediaAdapterRegistry([]MediaAdapterRegistration{}, metrics)
}

func ProvideMediaAdapterResolver(registry *MediaAdapterRegistry) *MediaAdapterResolver {
	return NewMediaAdapterResolver(registry)
}

func ProvideMediaModelRegistry(repo MediaModelDefinitionRepository, resolver *MediaAdapterResolver, metrics MediaRoutingMetrics) (*MediaModelRegistry, error) {
	registry := NewMediaModelRegistryWithResolver(repo, resolver)
	registry.SetRoutingMetrics(metrics)
	if err := registry.Refresh(context.Background()); err != nil {
		return nil, err
	}
	registry.StartPeriodicRefresh(context.Background(), 5*time.Second)
	return registry, nil
}

func ProvideMediaTaskMetrics() *AtomicMediaTaskMetrics { return NewAtomicMediaTaskMetrics() }
```

- [ ] **Step 8：运行 Service/Repository 与跨包编译测试并确认通过**

Run:

```bash
cd backend
gofmt -w internal/service/media_model_admin.go internal/service/media_model_admin_test.go internal/repository/media_model_repo.go internal/repository/media_model_repo_test.go internal/repository/group_media_model_scope_repo_test.go internal/handler/admin/media_model_handler_test.go internal/service/wire.go internal/service/wire_test.go
go generate ./cmd/server
go test ./internal/service -run 'MediaModelAdmin|MediaModelPreflight' -count=1
go test ./internal/repository -run 'MediaModel|GroupMediaModelScope' -count=1
go test ./internal/handler/admin ./cmd/server -run '^$'
```

Expected: 全部 PASS；Repository 测试确认旧列没有被新写入覆盖；Handler 与 server 包编译通过，不存在四参数旧调用点。

- [ ] **Step 9：提交管理域、持久化隔离和构造切换**

```bash
git add backend/internal/service/media_model_admin.go backend/internal/service/media_model_admin_test.go backend/internal/repository/media_model_repo.go backend/internal/repository/media_model_repo_test.go backend/internal/repository/group_media_model_scope_repo_test.go backend/internal/handler/admin/media_model_handler_test.go backend/internal/service/wire.go backend/internal/service/wire_test.go backend/cmd/server/wire_gen.go
git commit -m "feat(media): validate resolved adapters before enable"
```

---

### Task 5：发布管理 API、preflight 和媒体 503

**Files:**
- Modify: `backend/internal/handler/admin/media_model_handler.go`
- Modify: `backend/internal/handler/admin/media_model_handler_test.go`
- Modify: `backend/internal/handler/media_task_handler.go`
- Modify: `backend/internal/handler/media_task_handler_test.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/routes/admin_media_model_routes_test.go`

- [ ] **Step 1：写管理 API 契约和 503 失败测试**

复用 Task 4 已改好的 variadic Handler fixture；无参数调用固定得到 `openai/gpt-image-2 -> openai-images` 默认 registration，所以现有 enabled Create/Update 测试继续成功。测试必须覆盖：旧字段传任意字符串或 null 都被接受但忽略；未知第三字段仍 400；四个读取响应都有相同 `adapter_resolution`；compat response 只由解析能力派生；preflight 为只读；tombstone 媒体创建返回 503。现有严格非法请求表删除 `invalid adapter`、`invalid async mode` 两项并改为成功/忽略用例，其他未知字段仍返回 400。

该测试文件补充 `github.com/tidwall/gjson` import；`newMediaModelHandlerFixture` 的媒体模型路由同时注册 `GET /admin/media-models/preflight`，并把 `handler.Preflight` 放在 `/:id` 之前。`admin_media_model_routes_test.go` 既断言静态 route 存在，也实际请求 `/api/v1/admin/media-models/preflight`，确认不会落入 `/:id` 的整数解析。

```go
func TestMediaModelHandlerIgnoresDeprecatedAdapterInputs(t *testing.T) {
	registration := service.MediaAdapterRegistration{
		Key:                 "xai-image",
		Adapter:             service.NewFakeMediaAdapter(service.FakeMediaAdapterOptions{Name: "xai-image", NativeAsyncMode: service.NativeAsyncUnsupported}),
		SupportedOperations: []service.MediaOperation{service.MediaOperationTextToImage},
		ExactRules: []service.MediaAdapterExactRule{{
			Vendor: "xai", ModelID: "grok-2-image",
			Capabilities: service.MediaAdapterRuleCapabilities{
				Operations: []service.MediaOperation{service.MediaOperationTextToImage}, SyncUpstream: true,
			},
		}},
	}
	router, store, _ := newMediaModelHandlerFixture(t, service.PlatformMedia, registration)
	body := `{"model_id":"grok-2-image","vendor":"xai","media_type":"image","operations":["text_to_image"],"constraints":{},"billing_unit":"image","enabled":true,"aliases":[],"default_adapter":"client-value","default_async_mode":null}`
	recorder := performMediaModelHandlerRequest(router, http.MethodPost, "/admin/media-models", body)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, store.records, 1)
	require.Empty(t, store.records[0].LegacyDefaultAdapter)
	require.JSONEq(t, `{"status":"ready","resolved_adapter":"xai-image","matched_by":"exact","matched_family":"","capabilities":{"operations":["text_to_image"],"sync_upstream":true,"native_async_upstream":false,"content_fetch":false},"reason_code":""}`, gjson.Get(recorder.Body.String(), "data.adapter_resolution").Raw)
}

func TestMediaTaskHandlerReturnsUnavailableForTombstone(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = nil
	app.createErr = service.ErrMediaModelAdapterUnavailable.WithMetadata(map[string]string{
		"model_id": "grok-2-image", "resolution_status": "unresolved", "reason_code": "MEDIA_ADAPTER_UNRESOLVED",
	})
	recorder := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"grok-2-image","prompt":"cat"}`, 42)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"MEDIA_MODEL_ADAPTER_UNAVAILABLE","message":"The media model adapter is temporarily unavailable","type":"server_error"}}`, recorder.Body.String())
}
```

- [ ] **Step 2：运行 Handler/Route 测试并确认失败**

Run:

```bash
cd backend
go test ./internal/handler/admin ./internal/handler ./internal/server/routes -run 'Media(Model|Task)' -count=1
```

Expected: FAIL，响应仍回显 DB 字段、preflight 路由不存在或 unavailable 被映射成 400。

- [ ] **Step 3：改写管理 DTO，保留废弃输入槽但不传 Service**

`mediaModelWriteRequest` 使用 `json.RawMessage` 接住两个废弃字段，使旧客户端的字符串、null 或其他 JSON 值都不会影响业务；`toServiceRecord` 完全不读取它们。

```go
type mediaModelWriteRequest struct {
	ModelID                 string                   `json:"model_id"`
	Vendor                  string                   `json:"vendor"`
	MediaType               service.MediaType        `json:"media_type"`
	Operations              []service.MediaOperation `json:"operations"`
	Constraints             json.RawMessage          `json:"constraints"`
	BillingUnit             string                   `json:"billing_unit"`
	DeprecatedDefaultAdapter json.RawMessage          `json:"default_adapter"`
	DeprecatedDefaultAsync   json.RawMessage          `json:"default_async_mode"`
	Enabled                 *bool                    `json:"enabled"`
	Aliases                 []string                 `json:"aliases"`
}
```

其余未知字段继续由 `DisallowUnknownFields` 拒绝。

- [ ] **Step 4：增加统一只读 response 和 compat 字段**

增加以下 Handler DTO；List/Get/Create/Update 全部只调用一个 `mediaModelRecordToResponse`。

```go
type mediaAdapterCapabilitiesResponse struct {
	Operations          []service.MediaOperation `json:"operations"`
	SyncUpstream        bool                     `json:"sync_upstream"`
	NativeAsyncUpstream bool                     `json:"native_async_upstream"`
	ContentFetch        bool                     `json:"content_fetch"`
}

type mediaAdapterResolutionResponse struct {
	Status          service.MediaAdapterResolutionStatus `json:"status"`
	ResolvedAdapter string                               `json:"resolved_adapter"`
	MatchedBy       service.MediaAdapterMatchType         `json:"matched_by"`
	MatchedFamily   string                               `json:"matched_family"`
	Capabilities    *mediaAdapterCapabilitiesResponse     `json:"capabilities"`
	ReasonCode      string                               `json:"reason_code"`
}
```

`mediaModelResponse` 保留一版 deprecated `default_adapter/default_async_mode`，但值只能是 `record.AdapterResolution.ResolvedAdapter` 和 `CompatibilityAsyncMode()`；严禁使用 `record.Legacy*`。

- [ ] **Step 5：增加只读 preflight Handler 与静态路由**

实现：

```go
// GET /api/v1/admin/media-models/preflight
func (h *MediaModelAdminHandler) Preflight(c *gin.Context) {
	report, err := h.service.Preflight(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, mediaAdapterPreflightResponseFromService(report))
}

type mediaAdapterPreflightItemResponse struct {
	ModelID                 string                               `json:"model_id"`
	Enabled                 bool                                 `json:"enabled"`
	ResolutionStatus        service.MediaAdapterResolutionStatus `json:"resolution_status"`
	ResolvedAdapter         string                               `json:"resolved_adapter"`
	LegacyDefaultAdapter    string                               `json:"legacy_default_adapter"`
	LegacyCheckApplicable   bool                                 `json:"legacy_check_applicable"`
	AdapterKeyMatches       bool                                 `json:"adapter_key_matches"`
	LegacyDefaultAsyncMode  service.NativeAsyncMode              `json:"legacy_default_async_mode"`
	LegacyAsyncModeReadable bool                                 `json:"legacy_async_mode_readable"`
	ReasonCode              string                               `json:"reason_code"`
	RolloutSafe             bool                                 `json:"rollout_safe"`
}

type mediaAdapterPreflightResponse struct {
	Safe          bool                                `json:"safe"`
	BlockingCount int                                 `json:"blocking_count"`
	Items         []mediaAdapterPreflightItemResponse `json:"items"`
}
```

`mediaAdapterPreflightResponseFromService` 逐字段复制上述 DTO，不读取 `Definition.Default*`。在 `registerMediaModelRoutes` 中把 `models.GET("/preflight", h.Admin.MediaModel.Preflight)` 放在 `models.GET("/:id", h.Admin.MediaModel.GetByID)` 前。诊断成功时无论 `safe` 是否为 true 都返回 HTTP 200，发布脚本只以 `data.safe` 和 `data.blocking_count` 判定。

- [ ] **Step 6：让 tombstone 错误保留为 503**

在 `media_task_handler.go` 的 `writeServiceError` 中、普通 model-not-found 之前处理。媒体端点使用自己的 OpenAI 风格错误 envelope，不能改用管理 API 的 `response.ErrorFrom`：

```go
case errors.Is(err, service.ErrMediaModelAdapterUnavailable):
	writeMediaErrorType(
		c,
		http.StatusServiceUnavailable,
		"MEDIA_MODEL_ADAPTER_UNAVAILABLE",
		"The media model adapter is temporarily unavailable",
		"server_error",
	)
```

将现有 `writeMediaError` 保留为 `writeMediaErrorType(c, status, code, message, "invalid_request_error")` 的薄封装；新增 `writeMediaErrorType` 的 `type` 参数。503 分支固定返回 OpenAI 风格 envelope：`{"error":{"code":"MEDIA_MODEL_ADAPTER_UNAVAILABLE","message":"The media model adapter is temporarily unavailable","type":"server_error"}}`，测试同时断言 HTTP 503、`error.code`、`error.type`，不读取根级 `reason`。

Scheduler 收到 typed unavailable 时不得无条件改写为 `ErrNoAvailableAccounts`；直接向 Orchestrator 返回原错误，确保发生在预扣和上游调用之前。

- [ ] **Step 7：运行后端 API 定向测试**

Run:

```bash
cd backend
gofmt -w internal/handler/admin/media_model_handler.go internal/handler/admin/media_model_handler_test.go internal/handler/media_task_handler.go internal/handler/media_task_handler_test.go internal/server/routes/admin.go internal/server/routes/admin_media_model_routes_test.go
go test ./internal/handler/admin ./internal/handler ./internal/server/routes -run 'Media(Model|Task)' -count=1
go test ./internal/service -run 'Media(Model|Adapter|Scheduler)' -count=1
```

Expected: 全部 PASS；preflight 静态路由不落入 `/:id`，媒体 503 envelope 与管理响应契约稳定。

- [ ] **Step 8：提交管理 API 与媒体错误契约**

```bash
git add backend/internal/handler/admin/media_model_handler.go backend/internal/handler/admin/media_model_handler_test.go backend/internal/handler/media_task_handler.go backend/internal/handler/media_task_handler_test.go backend/internal/server/routes/admin.go backend/internal/server/routes/admin_media_model_routes_test.go
git commit -m "feat(media): expose resolved adapter diagnostics"
```

---

### Task 6：切换前端媒体模型契约和只读系统适配 UI

**Files:**
- Create: `frontend/src/api/admin/__tests__/mediaModels.spec.ts`
- Create: `frontend/src/components/admin/media/MediaAdapterResolutionPanel.vue`
- Create: `frontend/src/components/admin/media/__tests__/MediaAdapterResolutionPanel.spec.ts`
- Create: `frontend/src/views/admin/__tests__/MediaModelsView.spec.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/admin/mediaModels.ts`
- Modify: `frontend/src/components/admin/media/MediaModelEditor.vue`
- Modify: `frontend/src/components/admin/media/__tests__/MediaModelEditor.spec.ts`
- Modify: `frontend/src/views/admin/MediaModelsView.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1：写 TypeScript API 和组件失败测试**

API 测试从 `@/types` import `MediaModelDefinition`/`MediaModelDefinitionInput`，从被测模块 import `create/listEnabled/remove/update`，使用 hoisted `vi.fn()` 真正 mock `@/api/client`，断言 `listEnabled()` 只返回 enabled+ready，create/update body 不包含旧字段；不要 import 真实 `apiClient` 后只调用 `vi.mocked()`。

```ts
const { getMock, postMock, putMock, deleteMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  putMock: vi.fn(),
  deleteMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get: getMock, post: postMock, put: putMock, delete: deleteMock },
}))

const readyModel: MediaModelDefinition = {
  id: 1,
  model_id: 'grok-2-image',
  vendor: 'xai',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
  adapter_resolution: {
    status: 'ready',
    resolved_adapter: 'xai-image',
    matched_by: 'exact',
    matched_family: '',
    capabilities: {
      operations: ['text_to_image'],
      sync_upstream: true,
      native_async_upstream: false,
      content_fetch: false,
    },
    reason_code: '',
  },
}

const unresolvedModel: MediaModelDefinition = {
  ...readyModel,
  id: 2,
  model_id: 'unknown-image',
  adapter_resolution: {
    status: 'unresolved',
    resolved_adapter: '',
    matched_by: '',
    matched_family: '',
    capabilities: null,
    reason_code: 'MEDIA_ADAPTER_UNRESOLVED',
  },
}

const disabledModel: MediaModelDefinition = {
  ...readyModel,
  id: 3,
  model_id: 'disabled-image',
  enabled: false,
}

const modelInput: MediaModelDefinitionInput = {
  model_id: 'grok-2-image',
  vendor: 'xai',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
}

it('returns only enabled ready media models', async () => {
  getMock.mockResolvedValue({ data: { items: [readyModel, unresolvedModel, disabledModel] } })
  await expect(listEnabled()).resolves.toEqual([readyModel])
})

it('sends only business fields when creating a model', async () => {
  postMock.mockResolvedValue({ data: readyModel })
  await create(modelInput)
  expect(postMock).toHaveBeenCalledWith('/admin/media-models', modelInput)
  expect(postMock.mock.calls[0][1]).toEqual(modelInput)
})

it('sends only business fields when updating a model', async () => {
  putMock.mockResolvedValue({ data: readyModel })
  await update(readyModel.id, modelInput)
  expect(putMock).toHaveBeenCalledWith(`/admin/media-models/${readyModel.id}`, modelInput)
  expect(putMock.mock.calls[0][1]).toEqual(modelInput)
})

it('removes a model through the canonical endpoint', async () => {
  deleteMock.mockResolvedValue({ data: undefined })
  await remove(readyModel.id)
  expect(deleteMock).toHaveBeenCalledWith(`/admin/media-models/${readyModel.id}`)
})
```

组件测试断言：不存在 `media-registry-adapter`/`media-registry-async-mode`；新建显示“保存后由系统解析”；编辑 ready/family 显示 key 和能力；unresolved/capability mismatch 显示部署或定义错误文案；emitted input 无旧字段。

`MediaAdapterResolutionPanel.spec.ts` 和 `MediaModelsView.spec.ts` 各自定义本地 fixture，不能跨测试文件引用上面的 `readyModel`。Panel 测试先 mock `vue-i18n`，再直接挂载：

```ts
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const panelModel = { status: 'capability_mismatch', resolved_adapter: 'xai-image', matched_by: 'exact', matched_family: '', capabilities: { operations: ['text_to_image'], sync_upstream: true, native_async_upstream: false, content_fetch: false }, reason_code: 'MEDIA_ADAPTER_CAPABILITY_MISMATCH' } satisfies MediaAdapterResolution
const wrapper = mount(MediaAdapterResolutionPanel, { props: { resolution: panelModel } })
expect(wrapper.get('[data-test="media-adapter-resolution"]').text()).toContain('xai-image')
expect(wrapper.find('[data-status="capability_mismatch"]').exists()).toBe(true)
expect(wrapper.find('li').exists()).toBe(true)
```

View 测试使用独立 `readyViewModel()` fixture，并用 hoisted mocks 替换 `@/api/admin`、`@/stores/app`；`adminAPI.mediaModels.list/create/update/remove` 均为 `vi.fn()`。挂载时 stub `AppLayout`、`TablePageLayout`、`BaseDialog`、`ConfirmDialog`、`DataTable`、`MediaModelEditor`、`EmptyState`、`Icon`，并在 `beforeEach` 提供 `window.matchMedia = () => ({ matches:false, addListener:vi.fn(), removeListener:vi.fn(), addEventListener:vi.fn(), removeEventListener:vi.fn(), dispatchEvent:vi.fn() })`。测试加载后点击 create/edit/toggle，逐次断言 `create.mock.calls[0][0]`、`update.mock.calls[*][1]` 和 toggle payload 都没有 `default_adapter/default_async_mode`，同时断言 `adapter_resolution.resolved_adapter`、family、capabilities 在表格/编辑器中显示。

- [ ] **Step 2：运行前端定向测试并确认失败**

Run:

```bash
cd frontend
pnpm test:run src/api/admin/__tests__/mediaModels.spec.ts src/components/admin/media/__tests__/MediaAdapterResolutionPanel.spec.ts src/components/admin/media/__tests__/MediaModelEditor.spec.ts src/views/admin/__tests__/MediaModelsView.spec.ts
```

Expected: FAIL，原因是新类型/组件不存在，旧输入框仍渲染，或 `listEnabled` 仍只看 enabled。

- [ ] **Step 3：定义唯一前端只读契约**

在 `types/index.ts` 增加：

```ts
export type MediaAdapterResolutionStatus =
  | 'ready'
  | 'invalid_definition'
  | 'unresolved'
  | 'ambiguous'
  | 'implementation_missing'
  | 'capability_mismatch'

export type MediaAdapterMatchType = '' | 'exact' | 'family'

export interface MediaAdapterCapabilities {
  operations: MediaOperation[]
  sync_upstream: boolean
  native_async_upstream: boolean
  content_fetch: boolean
}

export interface MediaAdapterResolution {
  status: MediaAdapterResolutionStatus
  resolved_adapter: string
  matched_by: MediaAdapterMatchType
  matched_family: string
  capabilities: MediaAdapterCapabilities | null
  reason_code: string
}
```

`MediaModelDefinition` 必须有 `adapter_resolution`。兼容响应字段暂时保留为 `/** @deprecated */ default_adapter?: string` 与 `default_async_mode?: NativeAsyncMode`，使 Task 6 的阶段提交仍能通过当前账号组件类型检查；媒体模型管理 API/View/Editor 从本任务起不得读取它们。Task 7 把账号组件最后两处读取切到 `adapter_resolution`，最终静态门禁再要求业务代码零读取。`MediaModelDefinitionInput` 彻底删除两个旧字段。保留全局 `NativeAsyncMode`，因为旧账号兼容仍使用。

- [ ] **Step 4：让 API 只暴露 ready 选择列表**

```ts
export async function listEnabled(): Promise<MediaModelDefinition[]> {
  const items = await list()
  return items.filter((item) => item.enabled && item.adapter_resolution.status === 'ready')
}
```

`list()` 继续返回全部模型，供注册表诊断。

- [ ] **Step 5：实现只读 MediaAdapterResolutionPanel**

组件只接收服务端结果，禁止发出 Adapter 修改事件：

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MediaAdapterResolution } from '@/types'

const props = defineProps<{ resolution?: MediaAdapterResolution | null }>()
const { t } = useI18n()
const ready = computed(() => props.resolution?.status === 'ready')
</script>

<template>
  <section data-test="media-adapter-resolution" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
    <h4 class="text-sm font-semibold">{{ t('admin.mediaModels.resolution.title') }}</h4>
    <p v-if="!resolution" data-test="media-adapter-resolution-pending" class="mt-2 text-xs text-gray-500">
      {{ t('admin.mediaModels.resolution.pending') }}
    </p>
    <template v-else>
      <p :data-status="resolution.status" class="mt-2 text-xs">
        {{ t(`admin.mediaModels.resolution.status.${resolution.status}`) }}
      </p>
      <code v-if="resolution.resolved_adapter" class="mt-2 block text-xs">{{ resolution.resolved_adapter }}</code>
      <p v-if="resolution.matched_by" class="mt-1 text-xs text-gray-500">
        {{ t(`admin.mediaModels.resolution.matchedBy.${resolution.matched_by}`) }}
        <span v-if="resolution.matched_family"> · {{ resolution.matched_family }}</span>
      </p>
	  <ul v-if="resolution.capabilities" class="mt-3 text-xs">
        <li>{{ t('admin.mediaModels.resolution.capabilities.sync') }}: {{ resolution.capabilities.sync_upstream ? t('common.yes') : t('common.no') }}</li>
        <li>{{ t('admin.mediaModels.resolution.capabilities.nativeAsync') }}: {{ resolution.capabilities.native_async_upstream ? t('common.yes') : t('common.no') }}</li>
        <li>{{ t('admin.mediaModels.resolution.capabilities.contentFetch') }}: {{ resolution.capabilities.content_fetch ? t('common.yes') : t('common.no') }}</li>
      </ul>
      <p v-if="!ready" class="mt-2 text-xs text-amber-700" role="status">
        {{ t(`admin.mediaModels.resolution.reason.${resolution.reason_code}`) }}
      </p>
    </template>
  </section>
</template>
```

- [ ] **Step 6：删除模型编辑器的 Adapter/默认异步输入**

`MediaModelEditor` 删除 `NativeAsyncMode` import、旧字段校验、normalize 和两段表单控件；新增 `adapterResolution?: MediaAdapterResolution | null` prop，并在业务字段之后渲染 Panel。新建时传 null，编辑时展示当前服务端结果；vendor/operations 修改后提示“保存时重新解析”，本期不请求预览 API。

- [ ] **Step 7：改造 MediaModelsView 列表、搜索和启停 payload**

`createDefault()` 和 `toInput()` 只构造业务字段。系统适配列 key 改为 `adapter_resolution`；搜索使用：

```ts
const resolutionSearchValues = (model: MediaModelDefinition): string[] => [
  model.adapter_resolution.resolved_adapter,
  model.adapter_resolution.matched_family,
  model.adapter_resolution.reason_code,
]
```

表格显示 status/key/match；编辑器传 `:adapter-resolution="editing?.adapter_resolution ?? null"`。`toggleEnabled` 调用 `{ ...toInput(model), enabled: !model.enabled }`。错误展示只对五个已知解析码建立本地化 key 集合；收到 `INVALID_MEDIA_MODEL`、`MEDIA_MODEL_ALIAS_CONFLICT`、`MEDIA_MODEL_ID_CONFLICT` 或其他未知 reason 时回退 `error.message || t('admin.mediaModels.messages.saveFailed')`，不能把任意 reason 直接拼成 i18n key。

- [ ] **Step 8：更新中英文媒体模型文案**

删除“管理员填写 Adapter/默认异步”的描述，增加 status 六态、exact/family、三项能力、pending 和五个稳定 reason code 文案：`MEDIA_MODEL_DEFINITION_INVALID`、`MEDIA_ADAPTER_UNRESOLVED`、`MEDIA_ADAPTER_AMBIGUOUS`、`MEDIA_ADAPTER_IMPLEMENTATION_MISSING`、`MEDIA_ADAPTER_CAPABILITY_MISMATCH`。中文必须明确“需要部署对应代码适配”，英文表达同一含义；capability mismatch 同时展示实际能力与错误原因。

- [ ] **Step 9：运行前端测试、类型检查和 lint**

Run:

```bash
cd frontend
pnpm test:run src/api/admin/__tests__/mediaModels.spec.ts src/components/admin/media/__tests__/MediaAdapterResolutionPanel.spec.ts src/components/admin/media/__tests__/MediaModelEditor.spec.ts src/views/admin/__tests__/MediaModelsView.spec.ts
pnpm typecheck
pnpm lint:check
```

Expected: 全部 PASS，ESLint 无错误，TypeScript 不再要求 input 旧字段。

- [ ] **Step 10：提交媒体模型管理前端**

```bash
git add frontend/src/types/index.ts frontend/src/api/admin/mediaModels.ts frontend/src/api/admin/__tests__/mediaModels.spec.ts frontend/src/components/admin/media/MediaAdapterResolutionPanel.vue frontend/src/components/admin/media/__tests__/MediaAdapterResolutionPanel.spec.ts frontend/src/components/admin/media/MediaModelEditor.vue frontend/src/components/admin/media/__tests__/MediaModelEditor.spec.ts frontend/src/views/admin/MediaModelsView.vue frontend/src/views/admin/__tests__/MediaModelsView.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(media): show automatic adapter resolution"
```

---

### Task 7：在账号绑定与分组白名单中执行解析能力约束

**Files:**
- Modify: `frontend/src/components/account/MediaConfigEditor.vue`
- Modify: `frontend/src/components/account/__tests__/MediaConfigEditor.spec.ts`
- Modify: `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`
- Modify: `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`
- Modify: `frontend/src/components/admin/group/GroupMediaSettings.vue`
- Modify: `frontend/src/components/admin/group/__tests__/GroupMediaSettings.spec.ts`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

- [ ] **Step 1：写账号能力和历史绑定失败测试**

更新所有媒体模型 fixture 使用 `adapter_resolution`，新增断言：sync-only 模型禁用 native option；native-only 模型禁用 unsupported option；当前模式不兼容时 `update:valid=false`；历史/非 ready 模型保留只读 option、显示警告并可移除；发布的 `media_config` 仍不含 Adapter。`MediaConfigEditor.spec.ts` 增加 `beforeEach` 和 `MediaModelDefinition` import，删除现有 `registryModelIDs = vi.hoisted(...)` 及其整段 `vi.mock('@/api/admin', ...)`，再用下面唯一一套 hoisted mock 取代，不能让两套 mock 并存；继续复用文件中已有的 `mountHarness`：

```ts
const listEnabledMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  adminAPI: { mediaModels: { listEnabled: listEnabledMock } },
}))

function readyRegistryModel(
  modelID: string,
  capabilities: { sync_upstream: boolean; native_async_upstream: boolean } = {
    sync_upstream: true,
    native_async_upstream: true,
  },
  id = 1,
): MediaModelDefinition {
  return {
    id,
    model_id: modelID,
    vendor: 'xai',
    media_type: 'image',
    operations: ['text_to_image'],
    constraints: {},
    billing_unit: 'image',
    enabled: true,
    aliases: [],
    adapter_resolution: {
      status: 'ready',
      resolved_adapter: 'xai-image',
      matched_by: 'exact',
      matched_family: '',
      capabilities: {
        operations: ['text_to_image'],
        content_fetch: false,
        ...capabilities,
      },
      reason_code: '',
    },
  }
}

it('rejects an account mode outside resolved adapter capabilities', async () => {
  listEnabledMock.mockResolvedValue([
    readyRegistryModel('grok-2-image', { sync_upstream: true, native_async_upstream: false }),
  ])
  const wrapper = mountHarness({
    version: 1,
    provider: 'xai',
    models: {
      'grok-2-image': {
        enabled: true,
        upstream_model_id: 'upstream-image',
        async_mode: 'native',
        request_mapping: {},
      },
    },
  })
  await flushPromises()
  expect(wrapper.get('[data-test="media-async-mode-0"] option[value="native"]').attributes('disabled')).toBeDefined()
  expect(wrapper.get('[data-test="valid"]').text()).toBe('false')
})
```

在该文件增加 `const defaultRegistryModelIDs = ['seedance', 'seedance-lite', 'grok-image', 'grok_image', '__proto__']`。`beforeEach` 中执行 `listEnabledMock.mockReset()`，再设置 `listEnabledMock.mockResolvedValue(defaultRegistryModelIDs.map((modelID, index) => readyRegistryModel(modelID, undefined, index + 1)))`，既保留全部既有选项、避免重复 Vue key，又防止测试之间共享 mock 状态。

- [ ] **Step 2：写分组失效授权失败测试**

```ts
it('shows and removes selected models that are no longer ready', async () => {
  const wrapper = mount(GroupMediaSettings, {
    props: {
      modelValue: {
        allow_image_generation: true,
        allow_video_generation: false,
        media_cross_platform_enabled: false,
      },
      platform: 'media',
      availableModels: [{
        id: 1,
        model_id: 'ready-image',
        vendor: 'xai',
        media_type: 'image',
        operations: ['text_to_image'],
        constraints: {},
        billing_unit: 'image',
        enabled: true,
        aliases: [],
        adapter_resolution: {
          status: 'ready',
          resolved_adapter: 'xai-image',
          matched_by: 'exact',
          matched_family: '',
          capabilities: {
            operations: ['text_to_image'],
            sync_upstream: true,
            native_async_upstream: false,
            content_fetch: false,
          },
          reason_code: '',
        },
      }],
      selectedModelIds: ['ready-image', 'removed-image'],
      modelsLoaded: true,
      modelsLoadFailed: false,
    },
  })
  expect(wrapper.get('[data-test="unavailable-media-model-removed-image"]').exists()).toBe(true)
  await wrapper.get('[data-test="remove-unavailable-media-model-removed-image"]').trigger('click')
  expect(wrapper.emitted('update:selectedModelIds')?.at(-1)).toEqual([['ready-image']])
})
```

- [ ] **Step 3：运行账号与分组测试并确认失败**

Run:

```bash
cd frontend
pnpm test:run src/components/account/__tests__/MediaConfigEditor.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/components/admin/group/__tests__/GroupMediaSettings.spec.ts src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts
```

Expected: FAIL，旧 hint 仍读取 `default_*`、async options 未禁用或失效授权没有删除入口。

- [ ] **Step 4：让账号编辑器验证最终解析能力**

增加唯一判定函数，并在 `refreshValidity` 中使用：

```ts
function rowSupportsSelectedMode(row: ModelRow): boolean {
  if (!row.enabled) return true
  const capabilities = selectedDefinition(row)?.adapter_resolution.capabilities
  if (!capabilities) return false
  return row.asyncMode === 'native'
    ? capabilities.native_async_upstream
    : capabilities.sync_upstream
}
```

`loadRegistryModels()` 完成后调用 `refreshValidity()`。option 禁用规则与函数一致；hint 显示 `resolved_adapter`、match 和 sync/native/content 能力。row.model 不在 ready Registry 时保留 legacy option，enabled row 显示不可用警告并判 invalid；管理员可以禁用，或在仍保留至少一个合法绑定时删除其他历史行后保存。当前 v1 契约要求至少一个模型绑定，最后一个历史绑定不能直接删除为空，只能先禁用后保存。

这里的 UI 校验不是安全边界：Task 3 的 `TestMediaSchedulerUsesResolutionInsteadOfStoredDefaultAdapter` 已用直接构造的 `async_mode=native` 账号绕过前端，断言 sync-only Adapter 返回 `ErrNoAvailableAccounts` 且不产生候选；保留该后端回归测试。

- [ ] **Step 5：保留账号级 async 与旧 optional 迁移语义**

不要删除 `MediaBindingAsyncMode` 或旧账号 `NativeAsyncMode`。Create/Edit Account 继续写：

```ts
{
  version: 1,
  provider,
  models: {
    [canonicalModelId]: {
      enabled,
      upstream_model_id: upstreamModelId,
      async_mode: 'unsupported' as const,
      request_mapping,
    },
  },
}
```

旧 account config 的 `optional|required` 保存迁移仍映射为 v1 `native`；本任务不重新开放 optional 选择。

- [ ] **Step 6：给分组组件增加失效授权删除入口**

```ts
const unavailableSelectedIds = computed(() => {
	if (!props.modelsLoaded || props.modelsLoadFailed) return []
  const available = new Set(props.availableModels.map((model) => model.model_id))
  return props.selectedModelIds.filter((modelID) => !available.has(modelID))
})

const availableSelectedCount = computed(() => {
	const available = new Set(props.availableModels.map((model) => model.model_id))
	return props.selectedModelIds.filter((modelID) => available.has(modelID)).length
})

function removeUnavailableModel(modelID: string) {
  emit('update:selectedModelIds', props.selectedModelIds.filter((candidate) => candidate !== modelID))
}
```

`GroupMediaSettings.vue` 把 Vue import 改为 `import { computed, ref, watch } from 'vue'`，增加 `modelsLoaded?: boolean`、`modelsLoadFailed?: boolean` props。新 checkbox 只遍历 `availableModels`，模板仅在模型列表成功加载后展示 `unavailableSelectedIds` 和删除按钮；加载失败时显示错误提示且禁止把任何历史 ID 判为失效。底部计数使用 `availableSelectedCount`，另行显示历史失效数量，不能把 tombstone 计为有效授权。

`GroupsView.vue` 增加以下状态并传给 create/edit 两处 `GroupMediaSettings`：

```ts
const mediaModelsLoaded = ref(false)
const mediaModelsLoadFailed = ref(false)
const editOriginalMediaModelIds = ref<string[]>([])

async function loadMediaModels() {
  mediaModelsLoading.value = true
  mediaModels.value = []
  mediaModelsLoaded.value = false
  mediaModelsLoadFailed.value = false
  try {
    mediaModels.value = await adminAPI.mediaModels.listEnabled()
    mediaModelsLoaded.value = true
  } catch (error) {
    mediaModels.value = []
    mediaModelsLoadFailed.value = true
    mediaModelsLoaded.value = false
    console.error('Error loading media models:', error)
  } finally {
    mediaModelsLoading.value = false
  }
}
```

`handleEdit` 开始加载 scopes 前、scopes 加载失败分支以及 `closeEditModal()` 都先执行 `editOriginalMediaModelIds.value = []`，避免沿用上一个分组的授权状态；仅在本次 scopes 加载成功后保存 `editOriginalMediaModelIds.value = [...editMediaModelIds.value]`。

创建媒体分组仍要求至少一个 ready scope。编辑时结果为空只允许清理“原始授权全部为当前不可用模型”的分组，不能因为混合授权中存在一个 tombstone 就放行：

```ts
const readyModelIDs = new Set(mediaModels.value.map((model) => model.model_id))
const canClearUnavailableOnlyScopes =
  mediaModelsLoaded.value &&
  !mediaModelsLoadFailed.value &&
  editOriginalMediaModelIds.value.length > 0 &&
  editOriginalMediaModelIds.value.every((id) => !readyModelIDs.has(id))
```

模型列表加载失败时保持 `mediaModels=[]`，不展示 checkbox、计数或删除入口，且空数组仍提示 `mediaModelScopeRequired`。`GroupsView.imageGeneration.spec.ts` 覆盖：最后一个历史失效授权可提交 `replaceGroupScopes(id, [])`；加载失败时不出现删除按钮且不会提交空数组；仅有 ready 授权时全部取消仍被拦截；原始授权同时含 ready 和 unavailable 时全部取消也必须被拦截。

- [ ] **Step 7：更新账号/分组中英文文案与 fixtures**

账号 hint 改成“Adapter 由系统按模型厂商和规范模型解析”；增加 async 能力不匹配、历史模型不可用文案。分组增加“历史授权当前不可用”和“移除授权”。更新 CreateAccount、EditAccount、GroupsView 测试 fixture，确保每个 ready 模型都有完整 `adapter_resolution`。`EditAccountModal.spec.ts` 的 Registry mock 必须同时返回现有 `duplicate` fixture 和 `seedance` ready fixture，且 `seedance.adapter_resolution.capabilities.native_async_upstream=true`，因为 `buildMediaAccount()` 默认绑定 `seedance`/`async_mode='native'`；否则重开编辑弹窗的既有有效性测试会被新能力校验误判。

- [ ] **Step 8：运行定向测试和前端门禁**

Run:

```bash
cd frontend
pnpm test:run src/components/account/__tests__/MediaConfigEditor.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/components/admin/group/__tests__/GroupMediaSettings.spec.ts src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts
pnpm typecheck
pnpm lint:check
pnpm build
```

Expected: 全部 PASS；账号 payload 中仍只有 provider/models/upstream/async/request_mapping。

- [ ] **Step 9：提交账号与分组能力 UI**

```bash
git add frontend/src/components/account/MediaConfigEditor.vue frontend/src/components/account/__tests__/MediaConfigEditor.spec.ts frontend/src/components/account/__tests__/CreateAccountModal.spec.ts frontend/src/components/account/__tests__/EditAccountModal.spec.ts frontend/src/components/admin/group/GroupMediaSettings.vue frontend/src/components/admin/group/__tests__/GroupMediaSettings.spec.ts frontend/src/views/admin/GroupsView.vue frontend/src/views/admin/__tests__/GroupsView.imageGeneration.spec.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat(media): enforce resolved adapter capabilities"
```

---

### Task 8：补齐发布文档、历史恢复回归和全仓门禁

**Files:**
- Modify: `backend/internal/service/media_content_test.go`
- Modify: `backend/internal/service/media_worker_test.go`
- Modify: `backend/internal/service/media_orchestrator_test.go`
- Modify: `backend/internal/service/media_scheduler_test.go`
- Modify: `deploy/README_CLUSTER.md`

- [ ] **Step 1：写历史 key 内容回源与恢复回归测试**

增加一条内容回源测试：任务/Artifact 保存旧 key，Registry 只注册规范实现和 alias，ContentService 仍解析到同一实现。给现有 `mediaContentFetcherAdapterStub` 增加 `calls atomic.Int64`（补 `sync/atomic` import），在 `OpenContent` 中递增，以证明请求确实经过 alias 找到 Adapter；数据库/任务 key 不改写由 Worker/Repository 恢复测试负责。

```go
func (s *mediaContentFetcherAdapterStub) OpenContent(
	context.Context,
	*Account,
	*MediaArtifact,
	string,
) (*MediaContent, error) {
	s.calls.Add(1)
	return s.content, s.err
}
```

```go
func TestMediaContentServiceUsesHistoricalAdapterAliasForContentFetch(t *testing.T) {
	accountID := int64(9)
	task := &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID,
		Adapter: "legacy-content", MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}
	tasks := &mediaContentTaskRepoStub{task: task}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: task.ID, Direction: "output", Position: 0, MediaType: MediaTypeVideo,
		ContentType: "video/mp4", UpstreamReference: "upstream-video-reference",
	}}}
	registry := NewMediaAdapterRegistry()
	adapter := &mediaContentFetcherAdapterStub{content: &MediaContent{
		Body: io.NopCloser(strings.NewReader("proxy")), StatusCode: http.StatusOK,
		ContentLength: 5, ContentType: "video/mp4",
	}}
	require.NoError(t, registry.Register("content-fetcher", adapter))
	require.NoError(t, registry.RegisterAlias("legacy-content", "content-fetcher"))
	svc := NewMediaContentService(
		tasks,
		artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{account: &Account{ID: accountID}},
		registry,
		mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	content, err := svc.OpenVideo(context.Background(), task.PublicID, task.UserID, "")
	require.NoError(t, err)
	defer content.Body.Close()
	require.Equal(t, int64(1), adapter.calls.Load())
	require.Equal(t, http.StatusOK, content.StatusCode)
}
```

复查 Worker/Orchestrator 测试包含：硬编码旧 v1 快照恢复、durable input rewrite 后仍为数组、当前 Resolver 改变不重解释候选、optional 四象限、安全提交前换候选、unknown submission 固定元组。

- [ ] **Step 2：运行历史恢复测试并修正遗漏**

Run:

```bash
cd backend
go test ./internal/service -run 'Media(Content|Worker|Orchestrator|Scheduler|CandidateSnapshot)' -count=1
```

Expected: 全部 PASS；任何失败都在本步骤修正，不能通过删除兼容 fixture 绕过。

- [ ] **Step 3：记录 preflight 与多实例冻结发布流程**

在 `deploy/README_CLUSTER.md` 增加可直接执行的预检：

```bash
curl -fsS \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$PREFLIGHT_BASE_URL/api/v1/admin/media-models/preflight" | \
  jq -e '.data.safe == true and .data.blocking_count == 0'
```

文档顺序必须与设计一致。由于旧版本没有 preflight 路由，先用新版本启动一个“预检候选实例”：设置 `MEDIA_TASKS_ENABLED=false`（对应 `media_tasks.enabled=false`）、不加入负载均衡、不接收公网媒体流量，仅开放受保护的 Admin 端口；把该实例直连地址赋给 `PREFLIGHT_BASE_URL`，调用 preflight 并保存结果。通过后在运维层禁用 blocking 模型，冻结媒体模型 CRUD/启停、账号新增绑定和分组新增授权；再部署全部 API/Worker，确认同一版本和 alias，解除冻结，最后启用新模型。

文档必须明确：`MEDIA_TASKS_ENABLED=false` 只阻止 `MediaWorker.Start()`，不会自动关闭完整 server 构造出的 ops、cleanup、监控、备份等其他后台组件。候选实例必须沿用正常多实例安全配置，并逐项关闭或隔离所有可能产生写入/外部副作用的组件；它不得承载真实请求。若无法创建这种只读隔离实例、无法冻结写入、无法确认 alias/key 一致，或无法关闭其他副作用组件，则不得运行候选 server，改用独立 preflight CLI（若届时提供）或停止旧实例后的受控切换。

- [ ] **Step 4：运行完整后端门禁**

Run:

```bash
cd backend
go test ./...
go test -race ./internal/service ./internal/repository
go vet ./...
golangci-lint run ./...
go test ./migrations -count=1
```

Expected: 全部 PASS；migration 测试确认没有删除旧列，race 无告警，vet 与 golangci-lint 无输出。

- [ ] **Step 5：运行完整前端门禁**

Run:

```bash
cd frontend
pnpm test:run
pnpm typecheck
pnpm lint:check
pnpm build
```

Expected: 全部 PASS；生产构建成功。

- [ ] **Step 6：执行静态范围检查**

Run:

```bash
rg -n 'default_adapter|default_async_mode' frontend/src --glob '!**/__tests__/**'
rg -n 'DefaultAdapter|DefaultAsyncMode' backend/internal/service backend/internal/repository --glob '*.go'
git diff --check
```

Expected:

- 前端业务代码只允许 deprecated optional 类型声明，不允许表单读取或写入旧字段；
- 后端命中只允许 Repository legacy 诊断、v1 codec 兼容、compat response 和测试，Resolver/Scheduler 不把旧值当路由真值；
- `git diff --check` 无输出。

- [ ] **Step 7：提交发布文档与回归收口**

```bash
git add backend/internal/service/media_content_test.go backend/internal/service/media_worker_test.go backend/internal/service/media_orchestrator_test.go backend/internal/service/media_scheduler_test.go deploy/README_CLUSTER.md
git commit -m "test(media): verify automatic adapter resolution"
```

- [ ] **Step 8：确认最终工作区和提交序列**

Run:

```bash
git status --short
git log --oneline -8
```

Expected: `git status --short` 无输出；最近提交按 Task 1 至 Task 8 顺序出现，且没有 Ent/migration 删除提交。

---

## 计划覆盖自检

- 设计第 7–8 节的 exact/family、能力交集和 alias：Task 1。
- 设计第 9 节的启用校验、单模型故障关闭、tombstone 和 typed 503：Task 2、4、5。
- 设计第 10 节的账号 async 与历史 optional：Task 3、7、8。
- 设计第 11–12 节的管理 API、只读 UI 和废弃输入兼容：Task 4、5、6。
- 设计第 13 节的旧数据库列保留且忽略：Task 4、8。
- 设计第 14 节的 v1 快照和两阶段冻结：Task 3、8。
- 设计第 15 节的 preflight 与多实例发布冻结：Task 4、5、8。
- 设计第 16 节的解析失败、能力排除、历史 alias 与注册完整性可观测性：Task 1、2、3、5、8。
- 真实媒体 Adapter、候选快照 v2、Ent 删列和文本路由重构均明确不在本计划。
