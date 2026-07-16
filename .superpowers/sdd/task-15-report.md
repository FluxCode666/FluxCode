# Task 15 实施报告：Billing Port、Media Orchestrator、同步等待与超时策略

## 1. 结论

- 状态：完成。
- 代码审计结论：通过；无阻断项。
- 原固定基线：`3592aff2587c409b9130287d659f7ebf9312cc4e`。
- 实施时 review base：`b461ab8ef227f21e2899a70500df388f075332a1`。该提交仅修正文档范围；按总任务指示未 reset/rebase。
- 范围保持：未注册生产媒体路由，未实现真实 Provider Adapter，未修改文本链路，未增加取消 API/UI；`StopForSyncTimeout` 仅作为 Orchestrator 内部依赖调用。

## 2. 实现内容与文件

### 2.1 Brief 指定文件

- `backend/internal/service/media_billing.go`
  - 增加安全启动用 `DisabledMediaBilling`。
  - 固定 `task.PublicID + settlement_type` 计费幂等键生成规则。
  - 对失败结算强制校验 `RefundRatio + PenaltyRatio == 1`、有限数及 `[0,1]` 范围；首次结算和持久计划重试均校验。
- `backend/internal/service/media_billing_test.go`
  - 覆盖禁用计费、幂等键、Recording Port 去重、非法比例及非法持久计划。
- `backend/internal/service/media_orchestrator.go`
  - 定义 Create DTO/结果、运行时端口、Clock/Timer 和 Orchestrator。
  - 按固定顺序实现规格/Registry、Group 权限、可恢复输入、内容策略、规范化指纹、幂等查询、候选快照、定价、任务创建、输入 Artifact、`RequestSpec` CAS、预扣、预扣状态 CAS、持久入队。
  - 实现同步订阅等待、0 秒无应用 timer、fallback、关闭 fallback 后的超时失败/停止/结算、完成与超时 CAS 竞争处理。
  - 实现 `GetForUser` 归属查询和内部字段清理。
- `backend/internal/service/media_orchestrator_test.go`
  - 覆盖异步、同步、fallback、refund/penalty、0 秒、幂等竞争、CAS 竞争、订阅关闭、预扣/入队失败补偿、结算重试和用户安全读取。

### 2.2 经 NEEDS_CONTEXT 授权的必要跨文件扩展

- `backend/internal/service/media_adapter.go` / `media_adapter_test.go`
  - `MediaArtifactInput` 增加独立 `ObjectKey string`；不复用 `UpstreamReference`。
  - 原因：Create 前的 Task 16 暂存结果需要无歧义映射到 `MediaArtifact.ObjectKey`，且 Orchestrator 明确拒绝非空 `Data`。
  - 调用方搜索：当前仅 fake/worker/test struct literal；字段为追加字段，不破坏现有调用方。
- `backend/internal/repository/media_task_repo.go` / `media_task_repo_test.go`
  - `UpdateQueued` 白名单仅增加 `request_spec`，要求严格 `json.RawMessage` 且 JSON 有效；未扩大 Claimed/Billing 白名单。
  - `GetByPublicIDForUser`、`GetByIdempotencyKey` 仅把 Ent NotFound 包装为 `ErrMediaTaskNotFound`，其他 context/DB 错误保留原链。
  - 原因：输入 Artifact ID 只能在任务行创建后获得，固定流程要求用 queued/version CAS 写回 `RequestSpec`。
  - 数据契约：沿用既有 `media_tasks.request_spec JSONB` 和既有 `(user_id, api_key_id, idempotency_key)` 部分唯一索引；无 schema/migration 变化。

## 3. 核心执行链

### 3.1 Create

1. 校验 `MediaSpec`、Registry 模型/Operation/媒体类型/约束。
2. 读取 Group 并检查图片/视频权限；拒绝 RequestSpec 中预置 Artifact ID。
3. 规范化 `Inputs`：拒绝非空 `Data`，要求且仅允许一种持久引用（`ObjectKey`、已由上层验证的 `ExternalURL` 或 `UpstreamReference`），校验位置、类型、Content-Type 和引用格式。
4. 执行内容策略；计算 SHA-256 规范化请求指纹。
5. 幂等键初查；同指纹复用，不同指纹返回 `ErrMediaIdempotencyConflict`。
6. 固化 Scheduler 候选账号、Adapter、上游模型和 `NativeAsyncMode`；生成定价快照和随机 `task_` 公共 ID。
7. 以 `billing_status=pending` 创建任务；输入落 `direction=input` Artifact；queued/version CAS 写回 Artifact ID。
8. 调用幂等预扣；queued/version CAS 写入 `precharged`；按 client async 选择 sync/async 优先级持久入队。
9. 显式异步在入队成功后返回 `accepted`；同步进入订阅等待。

### 3.2 幂等竞争与补偿

- DB 唯一竞争：Create 失败后按同 scope 重读；找到 winner 时比较指纹并复用，竞争失败方不预扣、不入队；NotFound 返回原 Create 错误，其他基础设施错误与 Create 错误用 `errors.Join` 保链。
- 预扣失败：任务 CAS 为 `failed/billing_precharge`，不入队、不执行失败结算。
- 已预扣后入队失败：任务 CAS 为 `failed/system_queue`，Coordinator 持久化失败计划并全额退款。
- 客户端在补偿前取消：仅补偿边界使用 `context.WithoutCancel`，外加 30 秒 `WithTimeout`；返回错误仍保留原 `context.Canceled`。
- 补偿 CAS 丢失时重读真实状态；不会在其他终态上误做失败结算。

### 3.3 同步等待与超时

- `SubscribeTerminal` 严格发生在首次 `GetByID` 之前；通知只唤醒，DB 是事实源。
- 订阅 Channel 关闭后立即重读 DB；真实终态继续处理，否则返回 `ErrMediaTerminalSubscriptionClosed`，无无限空转。
- `MediaSyncWaitTimeoutSeconds == 0` 时 timeout Channel 为 nil，完全不创建应用 timer；客户端取消仅返回 context 错误，不 stop、不结算。
- timeout 先重读任务：
  - fallback 开启：`MarkSyncFallback` CAS；成功返回 `fallback_async`，不 stop、不重入队、不结算；CAS 丢给终态时返回真实终态。
  - fallback 关闭：状态 CAS 为 `failed/sync_timeout`；CAS 丢失重读真实终态；仅 CAS 胜方调用 `StopForSyncTimeout` 和失败结算。
- penalty 资格：必须 `SubmittedAt != nil` 且 timeout 前最新阶段为 `submitting/generating/polling`；其他阶段始终全退。
- refund 策略始终全退；penalty 使用配置比例和 `1-ratio`；设置的 NaN/Inf/越界和 duration 溢出在等待前拒绝。
- timeout/补偿脱钩路径均有 30 秒上界；无 goroutine、ticker 或未收口 subscription。
- 结算 Port 暂时失败时 Coordinator 写入 `billing_status=retry`，Orchestrator 仍返回 `gateway_timeout`；Gateway 结果清空公共任务 ID。

## 4. TDD RED / GREEN

### 4.1 有效 RED

命令：

`cd backend && go test ./internal/service -run 'TestMedia(Orchestrator|Billing)' -count=1`

结果：FAIL（预期）。编译错误明确指向未定义的 `MediaCreateRequest`、`MediaOrchestrator`、`MediaTimer`、`DisabledMediaBilling` 和 `MediaArtifactInput.ObjectKey`。

相邻仓储 RED：

`cd backend && go test ./internal/repository -run 'TestMediaTaskRepository(UpdateQueuedPersistsRequestSpecWithCAS|RequestSpecUpdateRequiresRawMessageAndQueuedPath|CreateAndLookupRoundTrip)' -count=1`

结果：FAIL（预期），`service.ErrMediaTaskNotFound` 未定义。

### 4.2 GREEN

- `go test ./internal/service -run 'TestMedia(Orchestrator|Billing)|TestMediaArtifactInputCarriesDurableObjectKey' -count=1`
  - PASS，`ok .../internal/service 0.868s`。
- `go test ./internal/repository -run '^TestMedia(Task|Artifact)Repository' -count=1`
  - PASS，最终相邻执行 `ok .../internal/repository 0.971s`。
- 所有命令均使用 `-count=1`，不是测试缓存命中。

## 5. 幂等、CAS 与竞态直接证据

- `TestMediaOrchestratorIdempotencyKeyReusesTaskWithoutSecondCharge`
  - 同请求复用同 PublicID，预扣调用数为 1；不同指纹冲突。
- `TestMediaOrchestratorIdempotencyCreateRaceLoserNeverPrecharges`
  - 模拟唯一约束竞争 winner；失败方预扣 0 次、入队 0 次。
- `TestMediaOrchestratorQueueFailureMarksFailedAndRefunds`
  - `failed/system_queue`，失败计划 `RefundRatio=1`。
- `TestMediaOrchestratorQueueFailureCompensatesAfterClientCancellation`
  - 原 context 取消后仍完成有界补偿，错误链保持 `context.Canceled`。
- `TestMediaOrchestratorPrechargeFailureMarksTaskFailedWithoutEnqueue`
  - `failed/billing_precharge`，0 入队、0 失败结算。
- `TestMediaOrchestratorSyncSubscribePrecedesFirstDBRead`
  - 事件顺序严格为 `subscribe -> get`。
- `TestMediaOrchestratorTimeoutCASLossReturnsRealCompletion`
  - timeout 失败 CAS 丢给 Worker 完成时返回 `completed`，0 stop、0 失败结算。
- `TestMediaOrchestratorFallbackCASLossReturnsRealCompletion`
  - fallback CAS 丢给完成时返回真实完成状态，0 stop、0 失败结算。
- `TestMediaOrchestratorTimeoutBillingUsesFreshReReadState`
  - 计费资格使用 timeout handler 的最新 DB 重读阶段，不使用 wait 循环旧快照。
- `TestMediaOrchestratorSettlementFailureDoesNotChangeGatewayTimeoutDecision`
  - 返回 `gateway_timeout`，公共 ID 为空，DB `billing_status=retry`。

## 6. 验证矩阵

### 6.1 精确测试与 race

- Service 精确测试：PASS，0.868s。
- Service 精确 race：
  - `go test -race ./internal/service -run 'TestMedia(Orchestrator|Billing)|TestMediaArtifactInputCarriesDurableObjectKey' -count=1`
  - PASS，2.181s。
- Repository 精确 race：
  - `go test -race ./internal/repository -run 'TestMediaTaskRepository(UpdateQueuedPersistsRequestSpecWithCAS|RequestSpecUpdateRequiresRawMessageAndQueuedPath|CreateAndLookupRoundTrip|MediaNotFoundMappingPreservesContextErrors)' -count=1`
  - PASS，3.340s。
- `git diff --check`：PASS。

### 6.2 静态、构建、全仓编译与模块

- `go vet ./...`：PASS，exit 0。
- `go build ./...`：PASS，exit 0。
- `go test ./... -run '^$' -count=1`：PASS，所有包完成无测试编译，exit 0。
- `go mod verify`：PASS，`all modules verified`。
- 未改 `go.mod` / `go.sum`，无新增依赖。

### 6.3 未执行项

- 未执行完整 `go test ./internal/service` 测试运行：任务已明确其可能触发既有 OpenAI stub 并发 map fatal，且禁止修改无关基线；本任务使用精确 service/repository 测试、精确 race 和全仓无测试编译替代。
- 未新增 Docker/Redis/Postgres integration：本任务没有新增真实依赖要求；仓储 CAS 使用现有 SQLite Ent harness，队列协议由 Task 13 既有实现承担。
- 未运行 `golangci-lint`、`govulncheck`：任务验收未要求；已运行指定 `vet/build/mod verify`。

## 7. Go 环境

- Go：`go version go1.26.5 darwin/arm64`
- Module `go` 版本：`1.26.2`
- `GOTOOLCHAIN=auto`
- `GOOS=darwin`，`GOARCH=arm64`
- `CGO_ENABLED=1`
- `GOMOD=/Users/duegin/.codex/worktrees/9a51/FluxCode/backend/go.mod`
- `GOWORK`：空（单 module 环境）
- `GOPROXY=https://proxy.golang.org,direct`
- `GOSUMDB=sum.golang.org`
- `GOPRIVATE/GONOSUMDB/GONOPROXY`：空
- `GOMEMLIMIT/GOGC`：未显式设置

## 8. 最终代码审计

### 8.1 深度与影响面

- 审计深度：高。原因是资金预扣/退款、唯一幂等、DB CAS、Redis 队列副作用、同步 timeout 和输入引用均属高风险路径。
- 生产方：新增 Orchestrator/Billing 能力及仓储最小支持。
- 调用方/消费方全量搜索：`MediaOrchestrator` 当前仅测试可达；无 Handler、route、Wire/DI 注册，符合 Task 15 非目标。Task 14 Worker 继续只依赖既有 `MediaSettlementCoordinator` 和 `MediaBillingPort`。
- 数据：仅复用既有 JSONB、Artifact ObjectKey 和部分唯一索引；无迁移、回填、缓存 key、队列 topic 或依赖变化。
- 字段对账：新增内部 `MediaArtifactInput.ObjectKey` 映射既有 `MediaArtifact.ObjectKey`；不涉及 HTTP JSON DTO/OpenAPI/SDK 字段。`request_spec` 是既有 DB 字段，仅扩大 queued/version CAS 写路径。
- 权限/隐私：Group 媒体权限在任务创建前检查；`GetForUser` 使用 `(public_id,user_id)` 仓储过滤并清理 Account、候选快照、上游任务、Poll、Billing、ObjectKey 和 UpstreamReference。
- 外部 URL：Create 仅保存引用而不发起网络访问；SSRF 解析/暂存仍由 Task 16 Handler/Stager 负责，符合 brief 边界。

### 8.2 反例检查

- 重复扣款：唯一索引竞争失败方重读且不预扣；Billing Port 幂等键按操作隔离。
- 部分失败：预扣前失败终态化；预扣后入队失败终态化并全退；取消不会截断补偿。
- 乱序终态：timeout/fallback CAS 失败均重读真实状态，不覆盖 Worker 终态。
- 订阅竞态/泄漏：先订阅后读 DB；单一 unsubscribe；timer Stop；关闭 Channel 不空转。
- 敏感输出：Gateway timeout 清空 PublicID；GetForUser 清理内部字段。
- 低级假通过排除：已检查实际 diff、生产/测试调用链、DB 索引/字段、异常/权限/并发/补偿分支；未把工具绿灯单独当成审计结论。

### 8.3 残余问题

- 无阻断问题。
- 生产路由、Handler/InputStager、真实 Pricing/Billing/Provider Adapter 和 DI 属于后续任务，本阶段不可达是设计结果，不是遗漏。
- Coordinator 的 Port 暂时失败恢复已有直接测试；真实 DB/进程故障演练未在本任务执行，发布前应由后续 integration/observability 收口。
