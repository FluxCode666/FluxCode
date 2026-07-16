# Task 13 Redis integration follow-up 报告

## 状态与范围

- 固定基线：`a172dbb44af5602d016b81d2990f4054fa43b702`；开始时 `HEAD`、`merge-base` 均为该 SHA，工作树 clean。
- 仅修改 Task 13 Redis queue：
  - `backend/internal/repository/media_task_stream.go`
  - `backend/internal/repository/media_task_stream_test.go`
  - 本报告
- 未修改 Task 14 Worker/scheduler/billing、路由、schema、adapter、frontend 或 Task 15。
- 未运行需求明确禁止的已知 fatal 完整 service 执行套件；以全仓无测试编译代替执行级全量测试。

## 环境证据

- 时间：2026-07-16，Asia/Shanghai；integration harness 使用 UTC。
- Go：`go version go1.26.5 darwin/arm64`；`go.mod` 为 `go 1.26.2`。
- `GOTOOLCHAIN=auto`、`GOOS=darwin`、`GOARCH=arm64`、`CGO_ENABLED=1`。
- `GOMOD=/Users/duegin/.codex/worktrees/9a51/FluxCode/backend/go.mod`；`GOWORK` 为空。
- Docker Client/Server：`29.6.1/29.6.1`，Docker Desktop。
- testcontainers-go：`v0.40.0`；真实 integration 使用 `redis:8.4-alpine` 与 `postgres:18.1-alpine3.23`。
- 独立边界诊断容器报告 Redis `8.4.4`。

## Phase 1：稳定复现与边界取证

### RED 1：fresh PEL window 后的 XAUTOCLAIM cursor

基线 exact 命令：

```bash
cd backend && go test -tags=integration ./internal/repository \
  -run '^TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshPendingWindow$' \
  -count=1 -v
```

关键 RED：

```text
=== RUN   TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshPendingWindow
Received unexpected error:
    receive media tasks: read tcp 127.0.0.1:61810->127.0.0.1:55002: i/o timeout
--- FAIL: TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshPendingWindow (0.70s)
```

真实 Redis 命令语义复现：321 条消息进入 PEL，租约过期后用 `XCLAIM` 刷新前 320 条，保留最后一条 stale。随后：

```text
XAUTOCLAIM media:tasks:async media-workers cursor-worker 500 0-0 COUNT 32 JUSTID
=> 空消息，next=1784205239738-0（示例），约 53ms（含 docker exec 开销）

XAUTOCLAIM media:tasks:async media-workers cursor-worker 500 1784205239738-0 COUNT 32 JUSTID
=> next=0-0，message=1784205239738-0，约 54ms（含 docker exec 开销）
```

这确认 Redis 的 `COUNT 32` 最多扫描 `COUNT*10=320` 条 PEL；第一页可合法返回“空消息 + 非零 cursor”，第二页才找到 stale 消息。

同一 Redis 容器对空队列执行：

```text
XREADGROUP GROUP media-workers cursor-worker COUNT 32 BLOCK 50 STREAMS media:tasks:async >
=> nil；5 次墙钟约 156-202ms（包含 docker exec/连接开销）
```

生产调用链证据：

1. exact test 的 `Receive(context.Background(), 2*time.Second)` 由内部 block 产生 effective deadline，来源为 `mediaReceiveDeadlineInternal`，不是 parent deadline。
2. `recoverPriority` 基线每次只执行一页 XAUTOCLAIM；空页后会进入 non-blocking XREAD，再进入 `BLOCK 50`。
3. `receiveRedisCommand` 基线对包括 blocking XREAD 在内的所有命令统一使用 100ms hard I/O timeout。
4. Redis blocking reply 的合法唤醒可越过该 100ms socket deadline；此时 effective deadline 仍剩约 1.9s。
5. `normalizeMediaReceiveError` 只在 effective deadline 剩余不超过 `mediaTaskMaxBlock=50ms` 时把 network timeout 映射为 receive timeout，所以该中途 socket timeout 原样泄露。

### RED 2：投递后 Stream 被删除的 ACK

基线 exact 命令：

```bash
cd backend && go test -tags=integration ./internal/repository \
  -run '^TestMediaTaskStreamRecoversMissingGroup$' -count=1 -v
```

关键 RED：

```text
=== RUN   TestMediaTaskStreamRecoversMissingGroup
media queue message is not pending: message ... is not pending in async
--- FAIL: TestMediaTaskStreamRecoversMissingGroup (0.00s)
```

真实 Redis 8.4.4 边界：

```text
DEL media:tasks:async
XACK media:tasks:async media-workers 1-0
=> 0（go-redis 为 acked=0, err=nil）
EXISTS media:tasks:async
=> 0
```

### RED 3：投递后仅 consumer group 被删除的 ACK

基线 exact 命令：

```bash
cd backend && go test -tags=integration ./internal/repository \
  -run '^TestMediaTaskStreamAckReportsDestroyedGroupDeliveryState$' \
  -count=1 -v
```

关键 RED：

```text
=== RUN   TestMediaTaskStreamAckReportsDestroyedGroupDeliveryState
expected: media queue delivery state is lost
in chain: media queue message is not pending: message ... is not pending in async
--- FAIL: TestMediaTaskStreamAckReportsDestroyedGroupDeliveryState (0.00s)
```

真实 Redis 8.4.4 边界：

```text
XGROUP DESTROY media:tasks:async media-workers
=> 1
XACK media:tasks:async media-workers <delivered-id>
=> 0（go-redis 为 acked=0, err=nil）
EXISTS media:tasks:async
=> 1
XINFO GROUPS media:tasks:async
=> 空 group 列表
```

Redis 8.4 的两个真实 ACK 缺失场景都没有返回显式 `NOGROUP`。基线代码仅在显式 `NOGROUP` 分支检查 Stream 并重建 group；`acked==0` 直接返回 `ErrMediaQueueMessageNotPending`，这是两个 ACK RED 的共同根因。

## Phase 2：工作模式对照

- 基线 fake cursor 测试为 GREEN，因为 fake `XREADGROUP` 立即返回 nil，`Receive` 可马上进入下一轮并用保存的 cursor 发第二次 XAUTOCLAIM；它没有建模真实 Redis blocking wake-up。
- 基线 `TestMediaTaskStreamValidatesInputsAndTimeouts` 的真实 Redis 50ms internal deadline 为 GREEN：raw timeout 发生时已经接近 effective deadline，能被归一化为 `ErrMediaQueueReceiveTimeout`。这与 2s exact test 中的中途 raw timeout不同。
- 基线 fake ACK 显式 NOGROUP controls 均为 GREEN：
  - Stream missing -> EnsureGroups + idempotent success；
  - Stream exists -> EnsureGroups + `ErrMediaQueueDeliveryStateLost`；
  - XACK 0 -> 不经 inspection 直接 NotPending。
- 引入 hard timeout 的原提交为 `fc737ccd8 fix(media): harden stream recovery and cancellation`：它把 `mediaTaskMaxBlock` 从 250ms 降为 50ms，并为 Redis receive 命令加入固定 100ms timeout，解释了真实 Redis 才暴露的边界差异。

## Phase 3：单一假设与 TDD RED

### Cursor / blocking RED

新增或增强的 fake tests：

- `TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshWindow`：第 1-3 页返回空消息与递增 cursor，第 4 页返回 stale 消息；若进入 blocking XREAD 则 stall。
- `TestMediaTaskStreamBoundsAutoClaimCursorProgressPerRecoveryPass`：要求每个恢复轮的命名 page budget 为 4。
- `TestMediaTaskStreamStopsAutoClaimWhenCursorDoesNotAdvance`：cursor 不前进时只发 1 条命令，防 busy loop。
- `TestMediaTaskStreamReceiveAllowsRedisBlockCompletionBeforeEffectiveDeadline`：合法 blocking reply 延迟 120ms，调用方 effective deadline 为 220ms，要求不丢连接且最终不泄露 raw timeout。
- `TestMediaTaskStreamReceivePreservesNonBlockingRedisTimeoutAsInfrastructureError`：non-blocking XAUTOCLAIM stall 仍必须返回基础设施 timeout，不能被误当成无消息。

精确 RED：

```bash
go test ./internal/repository \
  -run '^TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshWindow$' \
  -count=1 -v
```

```text
Received unexpected error: media queue receive timeout
--- FAIL ... (0.10s)
```

```bash
go test ./internal/repository \
  -run '^TestMediaTaskStreamBoundsAutoClaimCursorProgressPerRecoveryPass$' \
  -count=1 -v
```

```text
expected: 4
actual  : 1
--- FAIL
```

```bash
go test ./internal/repository \
  -run '^TestMediaTaskStreamReceiveAllowsRedisBlockCompletionBeforeEffectiveDeadline$' \
  -count=1 -v
```

```text
expected: media queue receive timeout
in chain: receive media tasks: read tcp ...: i/o timeout
--- FAIL ... (0.12s)
```

no-progress cursor 与 non-blocking Redis timeout controls 在基线保持 PASS，证明修复边界不是“吞掉所有 timeout”或“无界追 cursor”。

### ACK RED

新增 fake tests 锁定 `(0,nil)` 的四条边界：

- Stream missing -> idempotent success；
- Stream exists、target group missing -> `ErrMediaQueueDeliveryStateLost`；
- Stream 与 target group 都存在、message 不在 PEL -> `ErrMediaQueueMessageNotPending`；
- EXISTS/XINFO Redis error 或 inspection context deadline -> 保留基础设施/context error chain。

命令：

```bash
go test ./internal/repository \
  -run '^(TestMediaTaskStreamAckTreatsMissingStreamAfterZeroAckAsIdempotent|TestMediaTaskStreamAckReportsLostGroupAfterZeroAck|TestMediaTaskStreamAckPreservesInspectionErrorAfterZeroAck|TestMediaTaskStreamAckPreservesInspectionContextErrorAfterZeroAck)$' \
  -count=1 -v
```

基线四项全部 FAIL，分别实际得到 NotPending；失败原因与真实 Redis RED 一致。

## Phase 4：最小实现

### 有界 XAUTOCLAIM cursor progression

- 新增命名预算 `mediaTaskAutoClaimPageBudget=4`。
- 同一 `recoverPriority` 调用中，仅当本页为空、`next` 非 `0-0` 且确实前进时继续。
- 找到任何消息、cursor 回到 `0-0`、cursor 不前进或耗尽 4 页预算时立即停止。
- Redis 的每页最多扫描 `COUNT*10=320` 条，因此每个 priority 每个恢复轮最多扫描 1280 条 PEL；找到首个有消息页即停止并最多缓存 `COUNT=32` 条。更长 fresh window 依靠已保存 cursor 在后续 Receive 恢复轮继续，避免单 priority 垄断与无界循环。

### Blocking XREAD hard timeout

- non-blocking 命令继续使用固定 100ms hard I/O timeout，timeout 仍作为基础设施错误返回。
- blocking XREAD 的 hard timeout 为 `block + 100ms`，但 `receiveRedisCommand` 仍以 effective Receive context 的剩余时间取最小值。
- 若合法 blocking 路径仍先触发 timeout 且 outer Receive context 尚有效，只把它视为本轮无消息并继续；每轮至少等待有界 hard timeout，保持 caller cancellation、parent/internal deadline 分类且不会 busy loop。
- go-redis client clone 仍共享原 pool，不修改应用 client options；每个 command context 都显式 cancel。

### ACK inspection

- 显式 `NOGROUP` 与 `(acked=0, err=nil)` 统一进入 `inspectAckState`。
- `EXISTS=0`：Stream missing，`EnsureGroups` 后幂等成功。
- Stream 存在且 `XINFO GROUPS` 没有 target group：`EnsureGroups` 后返回 `ErrMediaQueueDeliveryStateLost`。
- Stream 与 target group 存在，且未观察到显式 NOGROUP：返回 `ErrMediaQueueMessageNotPending`。
- 显式 NOGROUP 后即使 group 被并发重建，也保守返回 delivery-state-lost，因为 XACK 时刻的 PEL 已丢失。
- EXISTS/XINFO/EnsureGroups/context 错误均使用 `%w` 或原 context 错误返回；不会把暂时 Redis 故障变成成功。
- EXISTS 与 XINFO 之间 Stream 被删除时，Redis 的 `no such key` 被识别为 Stream missing，随后重建并按幂等成功处理。

## GREEN 与真实 Redis RUN/PASS

### 三个 exact（逐个真实运行）

```text
=== RUN   TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshPendingWindow
--- PASS: TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshPendingWindow (0.60s)
PASS
```

```text
=== RUN   TestMediaTaskStreamRecoversMissingGroup
--- PASS: TestMediaTaskStreamRecoversMissingGroup (0.00s)
PASS
```

```text
=== RUN   TestMediaTaskStreamAckReportsDestroyedGroupDeliveryState
--- PASS: TestMediaTaskStreamAckReportsDestroyedGroupDeliveryState (0.00s)
PASS
```

### 目标 unit/integration/race

```bash
go test ./internal/repository -run '^TestMediaTaskStream' -count=1 -v
# PASS

go test -tags=integration ./internal/repository \
  -run '^TestMediaTaskStream' -count=1 -v
# 所有 MediaTaskStream unit + 真实 Redis integration PASS

go test -race ./internal/repository -run '^TestMediaTaskStream' -count=1
# ok github.com/Wei-Shaw/sub2api/internal/repository 2.832s

go test -race -tags=integration ./internal/repository \
  -run '^TestMediaTaskStream' -count=1
# ok github.com/Wei-Shaw/sub2api/internal/repository 5.790s
```

race 未报告 data race；新增代码没有启动生产 goroutine、ticker 或长期 timer，command context 均 cancel。fake Redis delay 在已有连接 goroutine 内有界执行，测试 cleanup 等待其退出。

### Task 14 Worker integration 回归

```bash
go test -tags=integration ./internal/repository \
  -run '^(TestMediaWorkerIntegrationDuplicateDeliverySettlesOnce|TestMediaWorkerIntegrationResumesPollWithoutResubmit)$' \
  -count=1 -v
```

```text
=== RUN   TestMediaWorkerIntegrationDuplicateDeliverySettlesOnce
--- PASS: TestMediaWorkerIntegrationDuplicateDeliverySettlesOnce (0.03s)
=== RUN   TestMediaWorkerIntegrationResumesPollWithoutResubmit
--- PASS: TestMediaWorkerIntegrationResumesPollWithoutResubmit (0.02s)
PASS
```

## 工具链门禁

均在 `backend` module 中执行：

```bash
go vet ./...                                      # exit 0
go build ./...                                    # exit 0
go test ./... -run '^$' -count=0                 # exit 0；全仓仅编译，不执行测试
go mod verify                                     # all modules verified
gofmt -d internal/repository/media_task_stream.go \
  internal/repository/media_task_stream_test.go   # 无输出
git diff --check                                  # 无输出
```

## 自审与残余问题

- 错误语义：`errors.Is` 对 receive timeout、context、NotPending 与 DeliveryStateLost 均由 unit/integration tests 覆盖；Redis inspection error 保持根因链。
- cancellation/deadline：parent cancellation、parent deadline、internal deadline、non-blocking hard timeout、blocking grace 均有 focused tests；blocking timeout fallback 仍被 effective deadline 封顶。
- fairness/边界：page budget、`next=0`、no-progress 与多页 cursor 均有 focused tests；单恢复轮不会无界扫描或无界增长 backlog。
- at-least-once：真实 Redis PEL 恢复、重复投递、未返回 buffered extras、malformed payload 不 ACK、双优先级公平性与 Worker duplicate settlement 回归均为绿。
- 资源：unit race 与 integration race 为绿；没有新增生产 goroutine/client/pool/timer owner。
- 有意残余：超过 1280 个连续 fresh PEL entry 的窗口不会在单次 priority 恢复中扫描到底，而是通过保存 cursor 在后续恢复轮继续。这是保持双优先级公平和单次命令预算的设计，不是消息丢失。
- 未执行项：按需求未运行已知 fatal 的完整 service 执行套件；已完成 `go test ./... -run '^$' -count=0` 的全仓无测试编译、vet 与 build。
