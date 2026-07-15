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

Task 12 新测试共有 15 个顶层测试、22 个通过事件（包含子测试）；`-count=10` 连续运行通过。

## 实现结果

### AccountCandidateSelector

- 只在调用方传入的候选 ID 中工作；复制排序切片且不修改排除 map。
- 候选集内 sticky 优先；候选外或被排除的 sticky 主动清除；选中账号用现有 `stickySessionTTL` 写回。
- 常规路径按 Priority、LoadRate、WaitingCount、LRU 排序；批量负载读取失败时复用 `sortAccountsByPriorityAndLastUsed` 降级。
- 所有成功返回前调用现有原子 `AcquireAccountSlot`；release 使用 `atomic.Bool` 幂等保护，不在锁内执行外部回调。
- 所有槽位占满时返回排序后的最佳 `AccountWaitPlan`。
- `Wait` 先占等待计数，退出时必定递减；使用有 owner 的 timer/ticker，无 goroutine；区分调用方 context 取消和内部 plan timeout，后者稳定返回 `ErrAccountConcurrencySaturated`。

### MediaScheduler

- `SnapshotCandidates` 只接受分组 repo 返回且实时可调度、支持模型、媒体配置可解析、Registry 存在、Adapter 方法集满足 native mode 的账号。
- 方法集规则为：`unsupported = Generate`，`required = Submit + Poll`，`optional = Generate + Submit + Poll`。
- 快照冻结 `ResolvedMediaAccountModel`；任务创建后实时账号 mapping/media config 改变不影响快照。
- `Select` 只在快照 ID 中实时重读账号，并再次拒绝排除、禁用、冷却、临时不可调度、平台改变、Registry 缺失或方法集不符的账号。
- 重复/非法快照、重复实时账号、nil selector 结果、候选外结果及不一致的 acquired/wait 结果都安全返回可匹配 `ErrNoAvailableAccounts`，不会 panic；若恶意 selector 已占槽则先释放。
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
