# Task 13 实现报告：双优先级 Redis Streams 队列与终态通知

## 基线与范围

- 固定起点：`4e3c1be33ec9e47adde037f4c3b588f6f9b1df88`（`fix(media): close sticky candidate edge cases`）。
- 生产代码仅涉及计划内文件：
  - `backend/internal/service/media_queue.go`
  - `backend/internal/repository/media_task_stream.go`
  - `backend/internal/repository/wire.go`
- 测试仅新增 `backend/internal/repository/media_task_stream_integration_test.go`。
- 另新增本唯一报告；未修改 integration harness、Worker、路由、数据库、文本链或 Task 14+ 文件。

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

模块声明为 `go 1.26.2`，go-redis 为 `github.com/redis/go-redis/v9 v9.17.2`。

## TDD 证据

### RED

先只新增 integration 行为测试，再运行：

```bash
cd backend
go test -tags=integration ./internal/repository -run TestMediaTaskStream -count=1
```

测试包按预期编译失败，核心错误为：

```text
undefined: service.MediaQueuePriorityAsync
undefined: service.MediaQueuePrioritySync
undefined: MediaTaskStream
```

此时尚未创建 service 端口和 Redis 生产实现。

### GREEN 与 integration 运行边界

实现后，同一命令可编译。当前 Docker daemon 不可用，verbose 运行由仓库既有 `TestMain` 明确跳过真实容器测试：

```text
docker is not available; skipping integration tests (start Docker to enable)
ok github.com/Wei-Shaw/sub2api/internal/repository
```

因此本报告只声明 integration 测试二进制编译成功，不声明真实 Redis integration 已执行通过。额外运行：

```bash
go test -c -tags=integration -o /tmp/fluxcode-media-repository.test ./internal/repository
```

命令成功且无编译输出。

## 实现结果

### Service 端口与稳定错误

- `MediaQueuePriority` 是严格 `sync`/`async` 枚举；消息包含 Redis message ID、task ID 和 priority。
- `MediaTaskQueue` 包含 `EnsureGroups`、`Enqueue`、`Receive`、`Ack`、`PublishTerminal`、`SubscribeTerminal`。
- 对 invalid priority、message、stream payload、Receive timeout 参数、Receive 无消息超时和 terminal payload 提供可用 `errors.Is` 匹配的稳定 sentinel。
- `Receive` 的 Redis 空结果在调用方等待时长耗尽后映射为 `ErrMediaQueueReceiveTimeout`；小于 1ms 的等待被拒绝；调用方 context 取消/超时原样返回。
- 接口文档明确 Pub/Sub 只是 wake-up hint：调用方必须先订阅再读 DB，并在通知、断线或取消后重新读取 DB，不能把通知负载当事实源。

### Redis Streams 队列

- 固定键为 `media:tasks:sync`、`media:tasks:async`，group 为 `media-workers`；group 从 `0-0` 幂等创建，`BUSYGROUP` 视为成功，避免 group 创建前消息丢失。
- `Enqueue` 拒绝非正 task ID/未知 priority，使用 `XADD MAXLEN ~ 100000`。
- `Receive` 在读取新消息前先对两条 Stream 执行 `XAUTOCLAIM`，领取 idle 超过 lease 的 pending；`NOGROUP` 时幂等重建并重试一次。
- 同步优先；实例连续返回 8 条同步任务后优先恢复/探测一次异步任务，存在异步压力时最迟第 9 次返回异步任务。
- 非阻塞优先级探测后，用最长 250ms 的短 `BLOCK` 同时读取两条 Stream；长调用等待由多个短阻塞组成，不进行空转 hot loop。
- 每次 `XREADGROUP`/`XAUTOCLAIM` 返回的全部消息均进入按优先级分离的受控内存 backlog，不会只返回第一条而遗失已经进入 PEL 的 extras；进程崩溃时 backlog 消息仍留在 Redis PEL，可由另一 consumer 租约后恢复。
- 实例级 Receive gate 串行化并发读取，公平计数、backlog、buffered/inflight ID 集合均受锁保护；同一实例不会把仍在 backlog 或未 ACK 的消息重复返回。
- `task_id` 解析兼容 go-redis 常见 `string`，并兼容 `[]byte`、`int64`/`int`；缺失、非数字或非正 payload 返回稳定错误且不 panic、不 ACK。

### ACK、恢复和终态通知

- ACK 拒绝 nil、非法 Redis message ID、非正 task ID 和未知 priority，严格按 message priority 选择 Stream；ACK 另一 Stream 的同名 ID 不会移除原 Stream pending。
- 未 ACK 消息可通过 `PendingCount` 观察，并由不同 consumer 在 lease 后 `XAUTOCLAIM`；ACK 只由调用方在 DB 推进后显式调用。
- ACK 遇到 `NOGROUP` 时重建 group 并返回幂等成功，因为原 PEL 已不存在；其它 Redis 错误使用 `%w` 保留错误链。
- 终态 channel 固定为 `media:task:<id>:terminal`；只允许正 task ID 和 `completed`/`failed`。
- `SubscribeTerminal` 等待 Redis subscription confirmation 后才返回，消除 subscribe→publish 竞态；输出 channel 有 1 个缓冲，忽略非法/非终态 payload，最多发送一个合法终态后关闭。
- 每个订阅只有一个 goroutine owner；它负责发送和关闭输出，具备 context/Redis 断线退出、panic 兜底和资源关闭。`unsubscribe` 用 `sync.Once` 幂等取消/关闭并等待 goroutine 完成，不会向已关闭 channel 发送。

### Provider

- `ProvideMediaTaskQueue` 返回 `service.MediaTaskQueue`，默认 lease 为 1 分钟，并加入 repository `ProviderSet`。
- consumer name 由 hostname、PID、加密随机后缀和进程内 atomic 序列组成；随机源失败时用时间戳加 atomic 序列安全降级，不记录随机错误或敏感数据。

## Integration 覆盖

计划内 integration 文件包含 15 个顶层 `TestMediaTaskStream...`，覆盖：

- sync 优先、terminal publish/subscribe 和订阅先于发布；
- EnsureGroups 幂等与 group 创建前消息保留；
- sync/async 各自 ACK/Pending、错误 Stream ACK 隔离；
- 未 ACK 消息和未返回 batch extras 的跨 consumer lease recovery；
- 持续 sync 压力下 8:1 公平性；
- batched extras backlog、同实例并发 Receive 唯一性、同实例 inflight 去重；
- string/`[]byte`/`int64` payload 与 malformed payload 不 ACK；
- invalid priority/message/ID/status/timeout、Receive timeout 和 caller cancel；
- NOGROUP Receive/ACK 恢复；
- invalid terminal payload、单次终态、重复 unsubscribe、context cancel；
- Provider consumer name 唯一性和默认 lease。

由于 `prefixHook` 不支持 `XAUTOCLAIM`/Pub/Sub，本测试没有修改计划外 harness，而是按任务简报直接使用共享 raw `integrationRedis`；所有测试保持串行，并在测试前后显式删除两条固定 Stream key（group 随 Stream key 一并清理）。

## 验证

以下命令通过：

```bash
cd backend
gofmt -w internal/service/media_queue.go internal/repository/media_task_stream.go internal/repository/media_task_stream_integration_test.go internal/repository/wire.go
go test ./internal/service ./internal/repository -run TestMedia -count=1
go test -race ./internal/service ./internal/repository -run TestMedia -count=1
go vet ./internal/service ./internal/repository
go vet -tags=integration ./internal/repository
go test -c -tags=integration -o /tmp/fluxcode-media-repository.test ./internal/repository
go test ./internal/setup -run '^$' -count=1
go test ./cmd/server -run '^$' -count=1
git diff --check
```

普通测试结果：

```text
ok github.com/Wei-Shaw/sub2api/internal/service
ok github.com/Wei-Shaw/sub2api/internal/repository
```

目标 race 结果同样为两个包 `ok`。`internal/setup` 与 `cmd/server` 均成功编译，证明当前 Wire 生产构建仍可编译。

尝试额外运行只读 Wire CLI 检查：

```bash
go run github.com/google/wire/cmd/wire check ./cmd/server
```

该命令因当前网络无法从 `proxy.golang.org` 下载尚未缓存的 `github.com/google/subcommands v1.2.0` 而失败，错误为 IPv6 连接 `i/o timeout`；没有把它记录为通过，也未生成/修改 Wire 产物。已有 `cmd/server` 和 `internal/setup` 编译验证通过。

## 残余风险与边界

- Docker 不可用导致 15 个真实 Redis integration 行为未在本环境执行；已用 `go test -c -tags=integration`、integration vet 和普通/race 包验证守住编译与静态边界。应在 Docker 可用的 CI 中执行同一 `TestMediaTaskStream` 命令。
- 本任务没有生产 Worker/路由，因此队列尚未被实际消费；DB lease、任务执行与计费协调属于 Task 14/15。
- 未运行后端全套 `go test ./...`；仓库已有全套测试可能在 `openai_images_official_params_test.go` 的并发 map stub 触发 fatal 的已知基线。本任务的目标、相邻、race、vet 和构建范围均已执行。
- 未修改现有文本调度链、数据库 schema、HTTP route 或真实媒体 Adapter。

## 自审

- 所有 Redis 非预期错误均以 `%w` 保留链；context 错误不翻译为普通基础设施错误。
- 共享 backlog、fairness、buffered/inflight 集合均受锁保护；Receive gate 可被 context/timeout 打断；race 目标范围通过。
- Pub/Sub goroutine 有 owner、退出条件、context、panic recover、PubSub close 和单一 channel close owner；unsubscribe 等待退出，不留无主 goroutine。
- 固定键、group、channel、stream max length、8:1 公平、1 分钟 lease 与 ProviderSet 均符合 Task 13 计划；范围仅四个计划文件和本唯一报告。
