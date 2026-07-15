# Task 12 实现报告：跨平台媒体候选调度

## 基线与范围

- 固定起点：`619d30cef7a0b6aad16a2941cdfde139f70c46f6`（`feat(media): define provider adapter contracts`）
- 生产代码仅新增：
  - `backend/internal/service/account_candidate_selector.go`
  - `backend/internal/service/media_scheduler.go`
- 测试仅新增：
  - `backend/internal/service/account_candidate_selector_test.go`
  - `backend/internal/service/media_scheduler_test.go`
- 未修改现有 Gateway/OpenAI/Anthropic/Gemini 文本调度、handler/adapter、路由、数据库、Wire、配置或 Task 13+ 文件。

## Go 环境

```text
go version go1.26.5 darwin/arm64
GOVERSION=go1.26.5
GOTOOLCHAIN=auto
GOOS=darwin
GOARCH=arm64
CGO_ENABLED=1
GOMOD=/Users/duegin/.codex/worktrees/9a51/FluxCode/backend/go.mod
GOWORK=
GOMEMLIMIT=
GOGC=
```

模块声明为 `go 1.26.2`。

## TDD 证据

### RED

先只创建两份行为测试，再运行：

```bash
cd backend
go test ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector)' -count=1
```

按预期编译失败，核心错误为：

```text
undefined: AccountCandidateSelectionRequest
undefined: NewAccountCandidateSelector
```

此时尚无生产实现。

### GREEN

最小实现加入后，同一命令通过。之后补齐边界覆盖并反复运行：

```text
ok github.com/Wei-Shaw/sub2api/internal/service
```

Task 12 新测试共有 27 个顶层测试、82 个通过事件（包含子测试）；`-count=10` 连续运行通过。

## 实现结果

### AccountCandidateSelector

- 只在调用方传入的候选 ID 中工作；复制排序切片且不修改排除 map。
- 候选集内 sticky 优先；候选外或被排除的 sticky 主动清除；选中账号用现有 `stickySessionTTL` 写回。
- 常规路径按 Priority、LoadRate、WaitingCount、LRU 排序；批量负载读取失败时复用 `sortAccountsByPriorityAndLastUsed` 降级。
- 所有成功返回前调用现有原子 `AcquireAccountSlot`；release 使用 `atomic.Bool` 幂等保护，不在锁内执行外部回调。
- 只有原子 Acquire 明确返回 busy 时才允许生成 `AccountWaitPlan`，并选择排序中第一个明确 busy 的候选；全为基础设施错误时用 `errors.Join` 保留全部错误链。
- `Wait` 的 plan context 从函数入口覆盖等待计数和首次/后续 Acquire；成功递增后所有出口使用不受原 context 取消影响的 cleanup context 恰好递减一次。实现只有可收口 ticker，无 goroutine；调用方 context 取消原样返回，内部 plan timeout 稳定返回 `ErrAccountConcurrencySaturated`。

### MediaScheduler

- `SnapshotCandidates` 只接受分组 repo 返回且实时可调度、支持模型、媒体配置可解析、Registry 存在、Adapter 方法集满足 native mode 的账号。
- 方法集规则为：`unsupported = Generate`，`required = Submit + Poll`，`optional = Generate + Submit + Poll`。
- 快照冻结 `ResolvedMediaAccountModel`；任务创建后实时账号 mapping/media config 改变不影响快照。
- `Select` 只在快照 ID 中实时重读账号，并再次拒绝排除、禁用、冷却、临时不可调度、平台改变、Registry 缺失或方法集不符的账号。
- 重复/非法快照、重复实时账号、nil selector 结果、候选外结果及不一致的 acquired/wait 结果都安全返回可匹配 `ErrNoAvailableAccounts`，不会 panic；`(result, error)`、缺账号、候选外及畸形等待结果携带的槽位均恰好释放一次。
- 多平台同模型候选统一交给 `AccountCandidateSelector`，返回的 Account、ResolvedModel、Acquired、ReleaseFunc、WaitPlan 保持同一账号语义。
- `WaitForSlot` 仅包装 selector `Wait`；`MarkUsed` 调用 `UpdateLastUsed`；`GetFixedAccount` 直接调用 `GetByID`，不因禁用/冷却改选，错误均用 `%w` 保链。

## 覆盖点

- 跨 OpenAI/Gemini 平台同模型快照和选择。
- 缺 adapter、缺 Registry、错误 adapter 方法集、禁用、冷却、unsupported model。
- 排除失败账号、实时 failover、冻结 upstream/adapter/mode、固定账号恢复边界。
- 重复 snapshot ID、空 resolved model、selector nil/越界。
- sticky 命中、sticky 清理、sticky 等待计划、TTL 写回。
- load failure 的 Priority + LRU 降级、Priority/Load/Waiting/LRU 排序。
- 并发 acquire race、release 幂等、最佳 WaitPlan。
- wait 成功/超时/取消/队列满、Increment/Decrement 清理。
- MarkUsed、GetFixedAccount 与 repo 错误链。
- sticky/普通路径的基础设施 Acquire 错误、全错误聚合、错误+busy、错误+success。
- selector 直接过滤 disabled/banned/error、不可调度、过期、过载、限流和临时冷却账号。
- plan timeout 覆盖阻塞 Increment、首次 Acquire、ticker Acquire；caller cancel 优先且 cleanup context 未取消。
- Scheduler 拒绝 selector result+error、错误 concurrency、零 timeout/max waiting、零 canonical concurrency；`WaitForSlot` 独立重复校验。
- `LoadBatchQueryCap`、nil concurrency service、零并发立即获取以及 sticky cache 读/写/删失败 fail-open。

## 验证

以下命令均通过：

```bash
cd backend
go test ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector|SelectAccountWithLoadAwareness)' -count=1
go test ./internal/service -run 'Test.*SelectAccountWithLoadAwareness' -count=1
go test ./internal/service -run 'Test(Media|AccountCandidateSelector|.*SelectAccountWithLoadAwareness|ConcurrencyService)' -count=1
go test ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector)' -count=10
go test -race ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector|.*SelectAccountWithLoadAwareness)' -count=1
go vet ./internal/service
gofmt -w internal/service/account_candidate_selector.go internal/service/account_candidate_selector_test.go internal/service/media_scheduler.go internal/service/media_scheduler_test.go
git diff --check
```

现有文本调度关键文件的范围 diff 为空。

## 全套基线与残余风险

- 本任务未运行后端全套 `go test ./...`；仓库已有已知基线问题：全套可能在 `openai_images_official_params_test.go` 的 repo stub 并发 map 读写处触发 fatal。Task 12 的定向、相邻、回归和 race 范围全部通过。
- Docker 在当前环境不可用；本任务没有数据库或容器集成变更。
- 等待轮询复用现有 100ms 初始等待量级；缓存 sticky 读写继续采用当前调度的 fail-open 语义。
- 本阶段没有生产路由、Wire 或真实媒体 Adapter，实际接线属于后续任务。

## 自审

- 错误：repo/等待/固定账号错误均保留 `errors.Is` 链；context 取消原样返回。
- 并发：生产代码不创建 goroutine；timer/ticker 均 `Stop`；共享 release 状态使用 typed atomic；race 通过。
- 输入所有权：排序使用切片副本，排除 map 传给 selector 前复制，快照转值 map，无调用方 map/slice 写入。
- 范围：仅四个计划文件和本唯一报告，无文本链、路由、DB、Wire、Task 13+ 改动。

## 独立复审修复追加记录

独立复审在 `c6fac7f87` 上指出 4 个 Important 与 2 个 Minor。修复严格先补测试并取得 RED：

```bash
cd backend
go test ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector)' -count=1
```

RED 证据包括：sticky Acquire 错误被回退为其它账号、全 Acquire 错误被伪装成 WaitPlan、不可调度候选被选中、LoadBatch 请求长度未受 cap 限制、阻塞 Increment/Acquire 只能被外层 400ms 安全 deadline 打断、取消后的 context 被传给 decrement、零并发 Wait 被当作无限制成功、selector result+error 未释放，以及 Scheduler/WaitForSlot 接受畸形 WaitPlan。

最小修复后，以下追加验证均通过：

```bash
go test ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector)' -count=10
go test ./internal/service -run 'Test(Media|AccountCandidateSelector|.*SelectAccountWithLoadAwareness|ConcurrencyService)' -count=1
go test ./internal/service -run 'Test.*SelectAccountWithLoadAwareness' -count=1
go test -race ./internal/service -run 'Test(MediaScheduler|AccountCandidateSelector|.*SelectAccountWithLoadAwareness)' -count=1
go vet ./internal/service
gofmt -w internal/service/account_candidate_selector.go internal/service/account_candidate_selector_test.go internal/service/media_scheduler.go internal/service/media_scheduler_test.go
git diff --check
```

修复后的关键不变量：

- sticky Acquire 非 context 错误立即保链返回；常规候选允许错误后继续，但只有明确 busy 才能排队。
- `candidateAccounts` 本身执行 `IsSchedulable`，不依赖 MediaScheduler 的前置过滤。
- selector 的异常 result 无论同时带 error、缺 Account、越界还是 WaitPlan 畸形，均不会泄漏已取得的槽位。
- `WaitForSlot` 不信任调用方构造的 selection，不会把非正或不匹配的并发上限传给并发服务。
- plan timeout 从 Wait 入口开始计时；成功进入等待队列后任意退出路径恰好 cleanup 一次。
- LoadBatch 使用现有 `LoadBatchQueryCap` 保护大候选池；sticky cache 失败保持当前 fail-open 语义。

## 第二轮独立复审修复追加记录

第二轮复审在 `bc6c36159` 上发现两个 sticky 边界。仍然先只补行为测试并取得 RED：

```bash
cd backend
go test ./internal/service -run 'TestAccountCandidateSelector(ClearsStickyWhenFilteringLeavesNoCandidates|StickyBusyWaitCountContext)' -count=1
```

RED 结果显示：唯一 sticky 账号被排除、禁用或临时冷却后，过滤得到空候选会直接返回而没有清理绑定；sticky Acquire 明确 busy 后，阻塞等待数查询返回的 caller cancel/deadline 以及合成 context 错误都会被吞掉并错误生成 WaitPlan。

修复后的行为：

- 过滤后为空时，对非空 session + 非 nil cache 直接执行一次幂等 Delete，不依赖 sticky read；无 session/cache 不调用，Delete 错误继续 fail-open。
- sticky busy 在等待数查询前后检查 parent context；调用期间取消或 deadline 原样返回且不生成 WaitPlan。
- parent 尚未置错但 wait-count error 可匹配 `context.Canceled`/`context.DeadlineExceeded` 时保留该错误；仅普通 Redis 错误继续 fail-open。

新增 sticky 用例 `-count=50` 稳定通过，并重新通过 Task 12 `-count=10`、相邻 Media/Concurrency、全部 `SelectAccountWithLoadAwareness`、目标 race、vet、gofmt 与 diff check。
