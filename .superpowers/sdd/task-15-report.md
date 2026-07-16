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

## 9. 首轮复审修复（2026-07-16）

### 9.1 范围与基线

- 修复前 HEAD：`0edc017cc85b0189a8304c26c767683a24893d40`。
- 复审提出的 5 个 Important 已逐项按 RED→GREEN 修复。
- 生产改动仍仅位于：
  - `backend/internal/service/media_orchestrator.go`
  - `backend/internal/repository/media_task_repo.go`
- 回归测试位于：
  - `backend/internal/service/media_orchestrator_test.go`
  - `backend/internal/service/media_worker_test.go`
  - `backend/internal/repository/media_task_repo_test.go`
  - `backend/internal/service/media_model_registry_test.go`
- `media_model_registry_test.go` 仅扩展共享测试 fixture：image definition 增加 `image_edit`，video definition 增加 `reference_to_video`、`video_extend`、`video_remix`；Orchestrator fixture 同时加载 image/video definition，使各 operation 测试能够到达输入校验。生产 Registry 无改动。
- 未修改 progress ledger、路由、Handler、Provider Adapter、UI、schema/migration、`go.mod` 或 `go.sum`；未扩展到 Task 16。

### 9.2 Important 1：初始化可见性与 ready 屏障

实现：

- 新任务以 `billing_status=pending` 和 5 分钟初始化 `lease_until` 创建。
- ready 发布通过同一个 `UpdateQueued` version CAS 原子写入 `billing_status=precharged`、`precharged_amount` 并清空 `lease_until`。
- Repository `Claim` 和 `ListRecoverable` 仅允许账单已预扣且初始化 lease 为空/过期的 queued task；in-progress 仍按执行 lease 恢复。
- active 初始化任务的幂等重放返回 `ErrMediaTaskInitializing`，不再返回 `accepted`。
- 初始化 lease 过期后，重放方以 `UpdateQueued` version CAS 接管；复用持久化 BillingSnapshot 和固定预扣幂等键继续初始化。并发 loser 不能越过 ready 屏障。

RED 直接证据：

- Repository：`lease_until` 不在 queued 更新白名单；pending queued 可被 Claim；恢复扫描包含 pending queued。
- Service：新任务无初始化 lease；active loser 被返回 `accepted`；expired 初始化不能接管；Worker 会执行 pending queued task。

GREEN：

- Repository 精确回归：PASS，`ok .../internal/repository 0.831s`。
- Service 精确回归：PASS，`ok .../internal/service 0.963s`。
- 直接测试：`TestMediaTaskRepositoryReadyCASClearsInitializationLease`、`TestMediaTaskRepositoryQueuedClaimRequiresReadyBillingAndExpiredLease`、`TestMediaWorkerDoesNotExecuteQueuedTaskBeforeReady`、`TestMediaOrchestratorPublishesReadyOnlyAfterPrechargeAndClearsInitializationLease`、`TestMediaOrchestratorIdempotencyRetryDoesNotAcceptActiveInitialization`、`TestMediaOrchestratorIdempotencyRetryTakesOverExpiredInitialization`、`TestMediaOrchestratorConcurrentIdempotencyLoserCannotBypassReadiness`。

### 9.3 Important 2：parent cancel 与 timeout 同时 ready

RED：

- `TestMediaOrchestratorParentCancelWinsWhenTimeoutAlsoReady` 使用 64 次迭代，fake timer 已 ready，并在首次 DB read hook 中同步 cancel parent context。
- 修复前第 0 次迭代即随机进入 timeout 路径并返回 nil error。

GREEN：

- timeout case 在进入 `context.WithoutCancel` 前再次检查 `ctx.Err()`。
- 精确测试 PASS，`ok .../internal/service 0.854s`；64 次迭代均返回 `context.Canceled`，0 Stop、0 失败结算。

### 9.4 Important 3：timeout 终态与 settlement recovery 原子持久化

RED：

- `go test ./internal/repository -run '^TestMediaTaskRepositoryTransitionPersistsSettlementRecoveryForPendingScan$' -count=1`
  - FAIL，`media task Transition field "settlement_recovery" is not allowed`，0.828s。
- `go test ./internal/service -run '^TestMediaOrchestratorSyncTimeoutTransitionPersistsRecoveryBeforeSettlementPlan$' -count=1`
  - FAIL，首次 `settlement_plan` 写入失败后 `SettlementRecovery` 为空，1.177s。

GREEN：

- 普通 `Transition` 白名单最小增加 `settlement_recovery`。
- timeout 在状态 CAS 前计算并编码 `MediaSettlementPlan{type=failure}`；同一个 `Transition` 原子写入 `failed/sync_timeout`、finished fields 和 `settlement_recovery`。
- CAS 成功后再 Stop 和调用 Coordinator。即使 Coordinator 首次正式计划写失败，任务仍保持 `billing_status=precharged`、终态 recovery 非空，并可被 `ListSettlementPending` 找到恢复。
- Repository 精确测试 PASS，0.877s；Service 精确测试 PASS，1.435s。Coordinator 对 recovery/plan 一致性校验无冲突。

### 9.5 Important 4：sync_timeout 幂等重放语义

RED：

- `go test ./internal/service -run '^TestMediaOrchestratorIdempotentRetryAfterSyncTimeoutKeepsGatewayTimeoutPrivate$' -count=1`
- FAIL：第二次相同 sync IdempotencyKey 重放返回 disposition `failed`，预期 `gateway_timeout`，0.911s。

GREEN：

- `terminalResult` 对 `status=failed && error_code=sync_timeout` 始终返回 `gateway_timeout` 的任务副本并清空 `PublicID`。
- 首次 timeout 和第二 waiter/幂等重放保持相同私有超时语义，且预扣和入队调用均仍为 1 次。
- 与 timeout CAS loss/recovery 测试合并执行 PASS，`ok .../internal/service 0.951s`。

### 9.6 Important 5：operation-specific 输入与 MIME

RED：

- `go test ./internal/service -run '^(TestMediaOrchestratorEnforcesOperationSpecificInputContractsBeforeTaskAndCharge|TestMediaOrchestratorVideoInputMappingUsesSortedSourceAndImageReferences)$' -count=1`
- FAIL，现实现误接受：6 类必须输入为空、图片/视频角色错配、声明 MediaType 与 MIME 前缀不一致、`image/`、`image/*`、非 image/video 顶级类型、排序后首个 source 为图片、video reference 等情况；0.957s。

GREEN：

- `text_to_image` / `text_to_video` 强制 0 输入。
- `image_to_image` / `image_edit` / `image_to_video` / `reference_to_video` 至少 1 输入且全部为 image。
- `video_extend` / `video_remix` 至少 1 输入；按 Position 排序后首个必须为 video source，其余必须为 image reference。
- 使用标准库 `mime.ParseMediaType`；拒绝不可解析、空/通配 subtype、顶级类型与声明 MediaType 不一致的 Content-Type，同时接受合法参数。
- source/reference Artifact ID 映射保持排序后首个 ID 为 `SourceArtifactID`，其余为 `ReferenceArtifactIDs`。
- 精确输入回归 PASS，`ok .../internal/service 0.876s`。
- 原 raw/nonrecoverable 测试切换到 `image_to_image` 并提供合法 `image/png`，确保失败原因仍直接覆盖 raw Data/持久引用问题。

### 9.7 扩大回归、race 与最终门禁

定向非 race：

- `go test ./internal/service -run '^(TestMediaOrchestrator|TestMediaBilling|TestMediaWorkerDoesNotExecuteQueuedTaskBeforeReady)' -count=1`：PASS，1.236s。
- `go test ./internal/service -run '^TestMediaModel' -count=1`：PASS，1.929s。
- `go test ./internal/repository -run '^TestMediaTaskRepository' -count=1`：最终 PASS，1.114s。
- 扩大仓储回归首次发现 3 个跨域白名单测试仍断言旧 fixture 初值 `pending`；通用可 Claim fixture 已因 ready 契约改为 `precharged`，因此只将“不应被更新改变”的预期同步为 `MediaBillingStatusPrecharged`，未修改生产行为。

精确 race：

- `go test -race ./internal/service -run '^(TestMediaOrchestrator|TestMediaBilling|TestMediaWorkerDoesNotExecuteQueuedTaskBeforeReady)' -count=1`：PASS，2.165s。
- `go test -race ./internal/repository -run '^TestMediaTaskRepository' -count=1`：PASS，8.963s。

静态、构建和模块：

- `gofmt`：已执行。
- `git diff --check`：PASS，exit 0。
- `go vet ./...`：PASS，exit 0。
- `go build ./...`：PASS，exit 0。
- `go test ./... -run '^$' -count=1`：PASS，exit 0；全仓包完成编译且不执行测试体。
- `go mod verify`：PASS，`all modules verified`。

未执行：

- 仍未执行完整 `go test ./internal/service` 测试体套件：既有 OpenAI stub 在并发运行时存在 concurrent map fatal，本轮禁止修改无关基线；精确 service/repository/Worker 测试、精确 race、全仓无测试编译替代该门禁。

### 9.8 复审后代码审计

- 审计深度：高。原因是变更涉及预扣可见性、幂等接管、DB CAS、队列执行资格、同步 timeout、失败结算恢复和外部媒体输入边界。
- 需求对账：5 个 Important 均有原失败直接测试、最小生产修复和 GREEN 证据。
- 调用链：`Create -> validateRequest/normalizeMediaInputs -> 初始化 lease -> 持久输入/预扣 -> ready CAS -> Queue/Worker Claim`；`waitSync -> handleSyncTimeout -> failed+recovery CAS -> Stop -> Coordinator -> ListSettlementPending`；`reuseOrResumeTask/terminalResult` 覆盖初始化重放和 sync_timeout 重放。
- 数据/契约：复用既有 `lease_until`、`settlement_recovery`、billing fields 和 Artifact 字段；无 schema、迁移、HTTP DTO、OpenAPI、SDK 或缓存/队列 topic 变化。
- 并发反例：active initializer、expired takeover、concurrent loser、cancel+timeout 同时 ready、timeout CAS loss、首次 settlement plan 写失败、第二 waiter/幂等重放均有直接测试。
- 输入反例：空输入、禁止输入、角色错配、排序错位、MediaType/MIME 不一致、非法 MIME、raw Data、无/多重持久引用均在任务创建和预扣前拒绝。
- 低级假通过排除：已检查实际 diff、生产方/调用方/消费方、仓储与 Worker fake、错误码/枚举/字段搜索、异常与并发路径；未把静态工具通过单独当作结论。
- 字段对账：未新增字段；仅扩大既有 `lease_until`/`settlement_recovery` 的受控更新路径。编辑/删除链路不涉及。
- 结论：通过；未发现新的阻断或非阻断代码风险。唯一保留的验证缺口是上述既有 OpenAI stub 限制导致未跑完整 service 测试体套件。

## 10. 第二轮复审修复（2026-07-16）

### 10.1 范围、协议与文件

- 第二轮 5 个 Important 已按 RED→GREEN 修复；修复起点为 `9b00562698ae3f2be6e88057da93f6ed62af092b`。
- 统一初始化协议：
  1. 创建 `queued + billing pending + finite init lease + owner version`。
  2. 当前 owner 持久输入后，以固定 Billing 幂等键 Precharge。
  3. `Queue.Enqueue` 成功后，才用同一 owner version CAS 原子发布 `precharged_amount + billing_status=precharged + lease=nil`。
  4. 预扣后的失败只允许当前 owner 用 `queued+expectedVersion` CAS 原子写 `failed + full-refund settlement_recovery + actual precharged_amount`，再调用 Coordinator；loser 只重读并服从 winner。
  5. 后台扫描 expired pending initializer，取得新 owner lease 后幂等建立/确认 Precharge，再原子终态并全退；Precharge 结果未知时保持 pending+finite lease，后续同键重试。
- 修改文件：
  - `backend/internal/service/media_task.go`
  - `backend/internal/repository/media_task_repo.go`
  - `backend/internal/repository/media_task_repo_test.go`
  - `backend/internal/service/media_orchestrator.go`
  - `backend/internal/service/media_orchestrator_test.go`
  - `backend/internal/service/media_worker.go`
  - `backend/internal/service/media_worker_test.go`
  - `backend/internal/repository/media_worker_integration_test.go`
- 未修改 progress ledger、路由、Handler、Provider Adapter、UI、schema/migration、依赖或 Task 16 文件。

### 10.2 Important 1：独立于客户端重试的 expired pending 收口

RED：

- `go test ./internal/repository -run '^TestMediaTaskRepositoryListsRecoverableAndSettlementPending$' -count=1`
  - FAIL，期望 recoverable IDs `[1,2,4]`，实际 `[1,4]`；expired pending initializer 未进入扫描，0.843s。
- `go test ./internal/service -run '^TestMediaWorkerRecoverOnceCleansExpiredPendingInitializerWithoutIdempotencyKey$' -count=1`
  - build FAIL：`MediaWorkerDependencies` 不存在 `Precharger`，后台无法对未知预扣状态做安全收敛。

GREEN：

- Repository `ListRecoverable` 对 queued 同时扫描 `pending/precharged`，仍要求 lease 为空或过期；Worker 按 billing status 显式分流，pending 不进入执行 Claim。
- Worker 新增必需 `Precharger MediaBillingPort` 依赖；现有 unit fixture 显式使用 `DisabledMediaBilling`，真实 integration fixture 将同一个 Billing port 同时传给 Precharger 和 Coordinator，未放宽依赖校验。
- pending cleanup 先 `UpdateQueued(version)` 获取 finite lease，再解码持久 `BillingSnapshot` 并以 `task.PublicID + precharge` 固定幂等键调用 Precharge；成功后 `TransitionQueued(ownerVersion)` 原子写 `failed/system_initialization_expired`、真实 EstimatedAmount、`billing_status=precharged`、full-refund recovery 和清 lease，最后由 Coordinator 结算。
- `TestMediaWorkerRecoverOnceCleansExpiredPendingInitializerWithoutIdempotencyKey`：PASS，1.383s；覆盖无 Idempotency Key 的 create 后崩溃和外部已预扣后/ready 前崩溃，两者均 `settled/refunded=2/final=0`，外部 precharge mutation 恰好 1 次、余额归零。
- `TestMediaWorkerRecoverOnceRetriesUnknownPrechargeResultBeforeCleanup`：PASS，0.869s；首次 mutation 已发生但响应失败时保持 pending+新 lease、余额 -2；lease 再过期后同键重试不重复扣，最终 settled/full refund/余额 0。
- Repository 扫描 GREEN：PASS，0.844s。

### 10.3 Important 2：初始化 owner version fence

RED：

- `go test ./internal/repository -run '^TestMediaTaskRepositoryTransitionQueuedFencesInitializationOwnerAndPersistsRefundIntent$' -count=1`
  - build FAIL：`MediaTaskRepository` 不存在 `TransitionQueued`。

GREEN：

- 新增受限 `TransitionQueued(ctx,id,expectedVersion,to,updates)`：只允许 queued→failed；同一 DB CAS 检查 status+version，更新终态字段并递增 version。
- 专用白名单仅包含 stage/error/finished、billing status、precharged amount、settlement recovery、lease；不扩大普通 Worker 更新路径。
- Repository 直接测试 PASS，0.841s；错误 version、非法 completed 目标均不更新，正确 owner 原子写 recovery/金额并被 `ListSettlementPending` 扫描。
- `TestMediaOrchestratorStaleInitializationOwnerCannotFailOrRefundTakeoverWinner` 覆盖 A Precharge 阻塞→lease 过期→B 接管 ready→A 恢复的 success/error 两条路径；winner 保持 queued+precharged+lease nil，0 失败结算。
- `TestMediaOrchestratorStaleEnqueueFailureCannotFailOrRefundTakeoverWinner` 覆盖 A 在 Enqueue 阻塞时 B 接管，A 返回 enqueue error 后 owner CAS 丢失且不终态化/退款 winner。
- 上述确定性 owner 竞态合并执行 PASS，0.903s。

### 10.4 Important 3：durable enqueue 先于 ready 发布

RED：

- `go test ./internal/service -run '^TestMediaOrchestratorEnqueuesBeforePublishingReadyAndClearsInitializationLease$' -count=1`
  - FAIL：Enqueue hook 观察到 `billing_status=precharged`，预期 `pending`，0.872s。
- `go test ./internal/service -run '^TestMediaWorkerAcksEarlyInitializationMessageAndRecoveryRequeuesAfterReady$' -count=1`
  - FAIL：早到消息返回 `ErrMediaTaskNotClaimed` 且未 ACK，0.859s。

GREEN：

- 主链顺序改为 Precharge→durable Enqueue→ready version CAS；Enqueue 时 DB 仍是 pending+active init lease，Claim 必须拒绝。
- Worker 仅对 `ErrMediaTaskNotClaimed` ACK 早到/重复消息；其他基础设施、执行、lease/slot 错误仍不 ACK。ready 后 `RecoverOnce` 通过 DB 扫描重新入队，保证 eventual requeue。
- Enqueue-before-ready 测试 PASS，0.943s；早到 ACK+恢复测试 PASS，0.864s。
- Enqueue failure 在任务仍不可执行时走 owner-fenced full-refund terminal；stale enqueue loser 不影响新 owner，见 10.3。

### 10.5 DB 写结果不确定性

RED：

- `go test ./internal/service -run '^TestMediaOrchestratorReadyPublishWriteAppliedButReturnedErrorReusesReadyTask$' -count=1`
  - FAIL：DB 已发布 ready 但返回 error，Orchestrator 仍报 `persist media precharge state`，0.892s。
- `go test ./internal/service -run '^TestMediaOrchestratorCompensationWriteAppliedButReturnedErrorStillSettles$' -count=1`
  - FAIL：owner-fenced failed+recovery 已写入但返回 error，Billing 留在 precharged 未继续结算，0.869s。

GREEN：

- ready `UpdateQueued` 无论返回 false 还是 error 都先 `GetByID`；仅当 fresh 为 `billing_status=precharged && lease=nil` 才按已成功 ready 复用，绝不补偿；否则进入 owner-fenced 补偿，CAS loser 不覆盖 winner。
- ready applied-error 测试 PASS，0.969s。
- failure CAS 返回 applied+error 时，以 applied 状态为事实继续 Coordinator；测试 PASS，0.871s。
- Worker 获取 cleanup lease、终态 CAS 同样对 error/false 重读：仅匹配本次 version+lease 或完整 recovery 才继续；其他状态服从 winner。

### 10.6 Important 4/5：所有初始化失败的原子 recovery 与真实金额

RED：

- `go test ./internal/service -run '^TestMediaOrchestratorPrechargedInitializationFailurePersistsRecoveryBeforePlanWrite$' -count=1`
  - FAIL：`system_queue` 与 `system_billing_state` 均保留 `billing_status=pending`，没有原子 recovery，0.873s。
- `go test ./internal/service -run '^TestMediaOrchestratorPrechargedInitializationFailureRefundsActualAmount$' -count=1`
  - FAIL：两条失败链的 `PrechargedAmount/RefundedAmount` 实际为 0，预期 3.25，0.721s。
- 增加 `system_input` 相邻资金反例：takeover 可能发生在旧 owner 已外部预扣之后，新 owner 的 input persistence 再失败；修复前仍终态 `pending`。plan-failure RED 1.664s，实际退款 RED 1.145s。

GREEN：

- `failAfterPrecharge` 在同一个 `TransitionQueued(ownerVersion)` 写入 failed fields、`billing_status=precharged`、不可变 BillingSnapshot 的 EstimatedAmount、full-refund `settlement_recovery`、清 lease；只有 CAS 胜方调用 Coordinator。
- `system_queue`、ready publish `system_billing_state`、`system_input` 三类均覆盖首次 `settlement_plan` 写失败：终态 recovery/真实金额仍持久化，Billing 保持 precharged，`ListSettlementPending` 可见。完整表测试 PASS，0.870s。
- 三类正常 Coordinator 路径均 `settled`、`precharged_amount=3.25`、`refunded_amount=3.25`、`final_amount=0`；完整表测试 PASS，2.005s。
- input/spec 的当前 owner 在失败时先以固定幂等键建立/确认 Precharge，再 owner-fenced 全退；若 Precharge 响应失败/结果未知，则保留 pending+finite lease 给后台继续收敛，避免 takeover 前已有扣款变成 terminal pending 孤儿。
- 普通 Precharge 错误行为新增 RED：修复前立即 failed，1.227s；GREEN 后保持 queued+pending+lease，0 入队/0 退款，待后台同键恢复，相关 stale-owner 合并测试 PASS，0.856s。

### 10.7 扩大回归、race 与最终门禁

扩大回归：

- 第一次扩大 Orchestrator 回归发现旧 timeout fixture 在 Enqueue hook 直接把 pending task 改成 in-progress/terminal，绕过了新 Claim 屏障，导致 ready CAS 冲突。测试模拟点改为 ready CAS 成功后的 repository hook；无生产回退。
- `go test ./internal/service -run '^TestMediaOrchestrator' -count=1`：PASS，1.164s。
- `go test ./internal/service -run '^(TestMediaOrchestrator|TestMediaBilling)' -count=1`：PASS，2.250s。
- `go test ./internal/service -run '^TestMediaWorker' -count=1`：PASS，2.475s。
- `go test ./internal/repository -run '^TestMediaTaskRepository' -count=1`：PASS，0.967s。

精确 race：

- `go test -race ./internal/service -run '^(TestMediaOrchestrator|TestMediaBilling|TestMediaWorker)' -count=1`：PASS，3.138s。
- `go test -race ./internal/repository -run '^TestMediaTaskRepository' -count=1`：PASS，8.358s。

最终门禁：

- `gofmt`：已执行。
- `git diff --check`：PASS，exit 0。
- `go vet ./...`：PASS，exit 0。
- `go build ./...`：PASS，exit 0。
- `go test ./... -run '^$' -count=1`：PASS，单独轮询确认 exit 0。
- `go mod verify`：PASS，`all modules verified`。
- 仍不执行完整 `go test ./internal/service` 测试体套件：既有 OpenAI stub concurrent map fatal 不属于 Task 15，使用精确 service/repository tests、精确 race 和全仓无测试编译替代。

### 10.8 第二轮高风险代码审计

- 审计深度：高；涉及余额预扣/退款、DB owner CAS、Redis 消息先后、后台恢复、幂等和不确定写结果。
- 资金状态证明：
  - 外部未预扣：后台首次同键 Precharge 成功后原子终态，再全退。
  - 外部已预扣：后台同键 Precharge 去重，原子终态后全退。
  - 外部 mutation 已发生但响应未知：保持 pending+finite lease；后续同键重试不重复扣，成功后全退。
  - owner CAS 丢失：不结算、不覆写金额，winner 继续 ready 或自己的补偿链。
- 队列状态证明：Enqueue 前后 DB 均不可 Claim，直到 durable Enqueue 成功后的 ready CAS；早到消息 ACK 后由 ready DB 扫描重新投递；Enqueue error 的消息即使实际已写入也只能看到 pending/terminal，不会执行生成。
- 数据/契约：复用既有 `version`、`lease_until`、BillingSnapshot、`settlement_recovery` 和 billing amount 字段；无 schema/migration、HTTP DTO、OpenAPI、SDK、cache key 或 queue topic 变化。
- 接口影响：`MediaTaskRepository` 新增 `TransitionQueued`，真实 Ent repository 与 Orchestrator/Worker fakes 均同步；Worker 新增必需 `Precharger`，所有现有构造点显式补齐并由全仓编译证明无漏改。
- 异常/并发直接证据：expired pending 无 Key、已扣/未扣/未知、A/B takeover、stale precharge success/error、stale enqueue error、早到 ACK、ready applied-error、terminal applied-error、plan 首写失败均有直接回归。
- 结论：第二轮 Important 通过；未发现新的阻断风险。
- 第二轮 Minor（`media_orchestrator.go` 初始化职责较多）未在本轮拆文件：此轮状态机/资金时序改动面较大，额外物理重构会提高审查风险；不影响行为正确性，后续可在独立纯重构任务中移动输入规范化/初始化状态机并保持现有回归全绿。
