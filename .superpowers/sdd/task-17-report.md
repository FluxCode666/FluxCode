# Task 17 实施报告

## 状态

DONE

- 基线：`addb37c98 fix(media): persist upload integrity metadata`
- 目标：接入媒体任务部署配置、生产 Wire 图和 Worker 生命周期，同时保持生产 gateway 不开放图片/视频路由。

## 实现内容

### 部署配置

- 新增 `Config.MediaTasks` / `MediaTaskConfig`，覆盖：启用开关、固定 Worker 数、任务超时、租约/续租、上游轮询、恢复扫描、恢复批量、Stream block、内容代理超时与内容大小上限。
- 默认值：`enabled=true`、`worker_count=4`、`task_timeout_seconds=7200`、`lease_ttl_seconds=120`、`lease_renew_interval_seconds=30`、`poll_interval_seconds=2`、`recovery_interval_seconds=15`、`recovery_batch_size=100`、`stream_block_milliseconds=1000`、`content_proxy_timeout_seconds=90`、`max_content_bytes=2147483648`。
- 对所有数值字段执行严格正数校验；续租间隔必须严格小于租约。
- `deploy/config.example.yaml` 增加独立 `media_tasks:` 区块，并明确它是重启生效的部署参数，不属于系统设置页热更新项。

### Worker 生命周期

- `MediaWorkerConfig` 增加独立 `StreamBlock`，避免把上游 Poll interval 与 Redis Stream block 混用；旧测试未显式配置时仍回退至 Poll interval。
- `Start` 在 `EnsureGroups` 成功后才启动固定数量消费者与一个恢复循环；失败时不创建运行协程。
- 每个消费者/恢复循环由独立 supervisor 托管；panic 被恢复、写入安全日志/错误通道，只重启对应循环。
- `Start`/`Stop` 保持锁保护、幂等和并发安全；`Stop` 取消根 Context、取消活跃执行并等待全部 supervisor 收口，既有取消路径保证不把 in-progress 任务主动标失败。
- `Enabled=false` 时 Provider 仍构造 Worker，但不调用 `Start`。

### Repository / Service / Handler / Wire

- `ProvideMediaTaskQueue` 使用部署租约并生成唯一 consumer name。
- `ProvideMediaHTTPContentReader` 使用部署内容代理超时与内容上限，同时保留直接构造器对旧零值测试配置的兼容回退。
- Service Provider 新增并接入：Model Registry（返回前 `Refresh`）、空 Adapter Registry、Disabled Billing、Settlement Coordinator、AllowAll Content Policy、Zero Pricing、Disabled Artifact Object Store、固定维度 Metrics、Scheduler、Content Service、Worker、Orchestrator。
- 生产 Adapter Registry 不注册真实 Adapter，也未注入 Fake Adapter；对象存储与 Billing 均使用禁用实现。
- 同一个 `ProvideMediaBilling` 结果同时注入 Orchestrator、Settlement Coordinator 与 Worker Precharger；生成代码可见复用同一 `mediaBillingPort`。
- Service 媒体 Set 绑定 `MediaSettlementCoordinator`、`MediaInputStager`、`MediaArtifactWriter`、`MediaExecutionController`；Handler 媒体 Set 绑定 `MediaTaskApplication`、`MediaVideoContentOpener`、`MediaInputLifecycle`。
- Handler 生产聚合改为 `ProvideHandlersWithMedia`，写入 `Handlers.MediaTask`；`backend/internal/server/routes/gateway.go` 未消费该字段、零 diff。
- cleanup 在 Redis/Ent 基础设施关闭前的应用层并行阶段调用 `mediaWorker.Stop()`，并等待其结束后才关闭基础设施。

### 已确认的 brief 示例适配

1. brief 的 `ProvideMediaWorker` 示例签名与 Tasks 1–16 已完成的基线接口不一致：基线 `NewMediaWorker(cfg, MediaWorkerDependencies)` 强制需要 `*MediaModelRegistry` 和 `MediaBillingPort Precharger`，且不使用示例中的 `MediaArtifactRepository`。经主任务确认，采用最小适配：Provider 增加 models/billing，移除未使用 artifacts，保持既有构造器与依赖验证不变。
2. Wire 要求 `wire.Bind` 的 concrete provider 位于同一 ProviderSet。为了既保持 `service.ProviderSet` 完整聚合语义，又避免生产重复 provider/bind，将 Service 拆为 `CoreProviderSet` 与单一 `MediaTaskProviderSet`，`ProviderSet` 聚合两者；生产 Build 使用 `CoreProviderSet + handler.ProviderSet`，Handler 媒体 Set 嵌入 Service 媒体 Set，媒体图只构造一次。

## TDD RED 证据

### 配置与生命周期 RED

命令：

```bash
cd backend && go test ./internal/config ./internal/service -run 'TestMedia(TaskConfig|WorkerStartStop|WorkerStartRestarts|WorkerStartEnsureGroups)' -count=1
```

关键失败：

```text
cfg.MediaTasks undefined (type *Config has no field or method MediaTasks)
--- FAIL: TestMediaWorkerStartRestartsConsumerAfterPanic
    Condition never satisfied
--- FAIL: TestMediaWorkerStartRestartsRecoveryAfterPanic
    Condition never satisfied
```

符合预期：部署配置尚不存在；原 Worker 虽已有基础 Start/Stop，但 panic 后只退出循环，没有 supervisor 重启。

### Provider 与生产 Handler 图 RED

命令：

```bash
cd backend && go test ./internal/repository ./internal/service ./internal/handler -run 'TestMedia(TaskQueueProvider|HTTPContentReaderProvider|WorkerProvider|WorkerConfig|TaskWire)' -count=1
```

关键失败：

```text
too many arguments in call to ProvideMediaTaskQueue
undefined: ProvideMediaHTTPContentReader
undefined: ProvideMediaWorker
undefined: mediaWorkerConfigFrom
TestMediaTaskWireProvidersJoinProductionGraph: ProviderSet should not contain "ProvideHandlers"
```

符合预期：Repository/Service Provider、部署参数映射和生产 Handler 聚合尚未接入。

### cleanup RED

命令：

```bash
cd backend && go test ./cmd/server -run 'TestProvideCleanup' -count=1
```

关键失败：

```text
too many arguments in call to provideCleanup
```

符合预期：cleanup 尚未接收 MediaWorker，无法在基础设施关闭前停止它。

### Wire 约束 RED

命令：

```bash
cd backend && go generate ./cmd/server
```

第一次关键失败：

```text
wire.Bind of concrete type "*...MediaOrchestrator" ... but MediaTaskProviderSet does not include a provider
wire.Bind of concrete type "*...MediaContentService" ... but MediaTaskProviderSet does not include a provider
```

将 Service 媒体 Set 嵌入 Handler 后，第二次关键失败：

```text
multiple bindings for ... MediaContentPolicy
multiple bindings for ... MediaArtifactWriter
multiple bindings for *...MediaWorker
```

符合预期：证明 Wire 的 Bind 同组约束以及生产同时引入完整 Service Set/Handler 媒体 Set 会重复绑定；随后按已确认的 Core/Media 分层做最小修复。

## GREEN 与扩大验证

### 聚焦 GREEN

```bash
cd backend && go test ./internal/config ./internal/service -run 'TestMedia(TaskConfig|WorkerStartStop|WorkerStartRestarts|WorkerStartEnsureGroups)' -count=1
```

```text
ok github.com/Wei-Shaw/sub2api/internal/config
ok github.com/Wei-Shaw/sub2api/internal/service
```

```bash
cd backend && go test ./internal/repository ./internal/service -run 'TestMedia(TaskQueueProvider|HTTPContentReaderProvider|WorkerProvider|WorkerConfig|WorkerStart)' -count=1
cd backend && go test ./internal/handler -run 'TestMediaTaskWire|TestProvideHandlersWithMedia' -count=1
```

均 PASS。

brief 指定聚焦命令：

```bash
cd backend && go test ./internal/config ./internal/service ./internal/repository ./internal/handler ./cmd/server -run 'TestMedia|^$' -count=1
```

五个 package 均 PASS；`cmd/server` 完成编译并报告 `[no tests to run]`。

### 扩大媒体与 race

```bash
cd backend && go test ./internal/config -count=1
cd backend && go test ./internal/service ./internal/repository ./internal/handler -run 'TestMedia' -count=1
cd backend && go test -race ./internal/config ./internal/service ./internal/repository ./internal/handler -run 'TestMedia' -count=1
```

全部 PASS；race 覆盖配置、Worker 并发生命周期、媒体 Service/Repository/Handler 测试，无数据竞争报告。

### 全仓编译、vet 与构建

```bash
cd backend && go test ./... -run '^$' -count=1
cd backend && go vet ./...
cd backend && go build -o /tmp/fluxcode-task17-server ./cmd/server
```

三条命令均 exit 0。按 brief 已知基线风险，未运行无过滤完整 service 功能套件，避免触发既有 OpenAI stub concurrent map fatal；未修改该无关基线。

### Wire 生成与幂等

```bash
cd backend && go generate ./cmd/server
```

成功生成 `cmd/server/wire_gen.go`；生成图包含 Model Registry Refresh、共享 Disabled Billing、Content Service、Media Worker、Orchestrator、MediaTask Handler 与 cleanup Worker 依赖。

第二次生成前后使用 `git hash-object cmd/server/wire_gen.go` 比较，hash 相同，命令 exit 0，证明可重复生成零 diff。`wire_gen.go` 未手改。

### 保护文件

```bash
git diff --exit-code -- \
  backend/internal/server/routes/gateway.go \
  backend/go.mod backend/go.sum \
  backend/internal/handler/gateway_handler.go \
  backend/internal/handler/openai_gateway_handler.go
```

exit 0、无输出；生产媒体路由未开放，依赖文件与文本 Handler 无意外改动。`git diff --check` 通过。

## 文件列表

- `.superpowers/sdd/task-17-report.md`
- `backend/internal/config/config.go`
- `backend/internal/config/media_task_config_test.go`
- `backend/internal/repository/wire.go`
- `backend/internal/repository/media_task_stream.go`
- `backend/internal/repository/media_task_stream_integration_test.go`
- `backend/internal/repository/media_http_content.go`
- `backend/internal/repository/media_provider_test.go`
- `backend/internal/service/wire.go`
- `backend/internal/service/media_worker.go`
- `backend/internal/service/media_worker_lifecycle_test.go`
- `backend/internal/handler/wire.go`
- `backend/internal/handler/media_task_wire_test.go`
- `backend/cmd/server/wire.go`
- `backend/cmd/server/wire_gen.go`（仅由 Wire 生成）
- `backend/cmd/server/wire_gen_test.go`
- `deploy/config.example.yaml`

## 自审

- [x] 配置默认值完整，十个正数边界及续租 `<` 租约均有测试。
- [x] `Enabled=false` 构造但不启动；`Enabled=true` 固定消费者 + 恢复扫描。
- [x] Start/Stop 双调用、并发调用与 `-race` 通过。
- [x] `EnsureGroups` 失败不启动循环、不留下 started 状态。
- [x] Consumer/Recovery panic 只由各自 supervisor 恢复；Stop 可在 restart backoff 中取消并收口。
- [x] Stop 复用既有取消语义，不主动把 in-progress 任务标失败。
- [x] cleanup 等待应用层 MediaWorker Stop 后才顺序关闭 Redis/Ent。
- [x] Model Registry 在 Provider 返回前 Refresh。
- [x] Adapter Registry 为空；无真实/Fake Adapter 注入生产。
- [x] Billing/Object Store 使用禁用实现，Pricing 为 Zero，Content Policy 为 AllowAll。
- [x] ProviderSet 无重复绑定和循环；Wire 编译/生成成功。
- [x] Wire 二次生成内容 hash 不变。
- [x] gateway 路由、go.mod/go.sum、文本 Handler 零 diff。
- [x] 未增加取消 API、未修改文本链路、未开放生产图片/视频路由。

## Concerns

- 无 Task 17 范围内未解决问题。
- 已知且按 brief 明确规避：无过滤完整 service 功能套件可能触发既有 OpenAI stub concurrent map fatal；本任务未修改该基线，已用聚焦/扩大媒体测试、race、全仓编译型测试、vet、build 与 Wire 幂等验证替代。
