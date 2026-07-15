# Task 11 实现报告：Media Adapter 契约、Registry 与 Fake Adapter

## 范围与基线

- 固定起点：`7c7007f1a2a0b08ce891087a09569d10643c73c8`
- 目标包：`github.com/Wei-Shaw/sub2api/internal/service`
- 生产代码范围：`media_adapter.go`、`media_fake_adapter.go`
- 测试范围：`media_adapter_test.go`
- 未修改路由、Wire、文本调用链、数据库、配置或 Task 12+。

## Go 环境

- `go version go1.26.5 darwin/arm64`
- `GOVERSION=go1.26.5`
- `GOTOOLCHAIN=auto`
- `GOOS=darwin`
- `GOARCH=arm64`
- `CGO_ENABLED=1`
- `GOMOD=/Users/duegin/.codex/worktrees/9a51/FluxCode/backend/go.mod`
- `GOWORK` 为空
- `GOMEMLIMIT`、`GOGC` 未显式设置
- `backend/go.mod` 声明 `go 1.26.2`

## TDD 证据

### RED

测试先写入 `backend/internal/service/media_adapter_test.go`，随后运行：

```bash
cd backend
go test ./internal/service -run 'TestMediaAdapterRegistry|TestMediaFakeNativeAsyncAdapter' -count=1
```

结果：失败（退出码 1）。编译器首先报告：

```text
undefined: NewMediaAdapterRegistry
undefined: NewFakeMediaAdapter
undefined: FakeMediaAdapterOptions
undefined: MediaAdapter
undefined: MediaSyncGenerator
```

这与计划中“Adapter 接口和 Registry 未定义”的预期一致。

### GREEN

最小实现落在两个生产文件：

- `media_adapter.go`：协议无关 DTO、五个小能力接口、`MediaAdapterError`、并发安全 Registry。
- `media_fake_adapter.go`：同步/异步/可选异步三个私有 wrapper、确定性执行结果、原子调用计数、加锁轮询状态和深拷贝。

实现后运行：

```bash
cd backend
go test ./internal/service -run 'TestMedia(Adapter|Fake)' -count=1
```

结果：通过。

随后补跑计划中的原始验收名称及全部 Task 11 测试：

```bash
go test ./internal/service -run 'TestMediaAdapterRegistry|TestFakeNativeAsyncAdapter' -count=1
go test ./internal/service -run 'TestMedia(Adapter|Fake)' -count=1
```

结果：两条命令均通过。Task 11 共 `13` 个顶层测试，另含 `3` 个 `PollsBeforeDone=0/1/2` 子测试。

## 验证

全部命令均在 `backend/` 下运行：

| 验证项 | 命令 | 结果 |
| --- | --- | --- |
| 计划定向测试 | `go test ./internal/service -run 'TestMediaAdapterRegistry\|TestFakeNativeAsyncAdapter' -count=1` | 通过，约 1.18 秒 |
| Task 11 定向测试 | `go test ./internal/service -run 'TestMedia(Adapter\|Fake)' -count=1` | 通过，约 1.10 秒 |
| 相邻媒体回归 | `go test ./internal/service -run 'Test.*Media' -count=1` | 通过，`54` 个顶层测试，约 0.66 秒 |
| Race | `go test -race ./internal/service -run 'TestMedia(Adapter\|Fake)\|TestFakeNativeAsyncAdapter' -count=1` | 通过，约 2.23 秒 |
| Vet | `go vet ./internal/service` | 通过，无输出 |
| 格式化 | `gofmt -w internal/service/media_adapter.go internal/service/media_fake_adapter.go internal/service/media_adapter_test.go` | 完成 |
| 差异检查 | `git diff --check`，提交前另跑 staged diff check | 当前无空白错误 |

未运行整个 `backend` 全套测试：本任务按计划运行目标包定向与所有相邻媒体测试；仓库已知全套测试可能在 `openai_images_official_params_test.go:576` 因既存测试 stub 并发 map 访问 fatal，该问题不属于本任务范围。本次所有实际运行的命令均通过，没有需要归类的新失败。

## 自审与风险

- `MediaAdapterError.Error()` 只返回安全 `Message` 或固定兜底文本；内部 `Cause` 保留 `errors.Is/errors.As` 链并通过 `json:"-"` 防止 JSON 暴露。测试验证了稳定 `Code` 和五个分类字段。
- Adapter DTO/接口未导入 Gin、Redis、Ent、余额或计费能力；没有真实 provider 实现，也没有修改生产文本链。
- Registry 对名称执行 `TrimSpace + ToLower`，拒绝空名、nil interface、typed nil 与归一化重复名；普通 map 由 `RWMutex` 保护，锁内只读写 map，不调用 Adapter。
- 三个 Fake wrapper 使用命名字段而非嵌入，精确暴露：`unsupported=Generate`、`required=Submit+Poll`、`optional=Generate+Submit+Poll`；都不意外实现 Idempotent Submit 或 Abort。
- Fake 共享任务进度使用 mutex，调用/成功轮询计数使用 typed atomic；错误轮询增加调用数但不推进成功进度。生产实现没有 goroutine。
- `GenerateResult`、Artifacts 中的 `[]byte`、Poll 结果和 `json.RawMessage` 在构造及返回边界深拷贝；测试会修改原始配置和返回值验证隔离。
- 取消的 Context 在任何 Fake 状态或计数变化前原样返回 `ctx.Err()`；配置错误不重新拼接，保留原始错误链。
- `PollsBeforeDone=0` 与 `1` 都在首次成功 Poll 完成，`2` 为 running 后 completed；未知任务返回可由 `errors.Is` 匹配的 Fake sentinel。
- 残余风险：这是基础契约与测试 Fake，尚未接入真实 provider、Worker、路由或计费；这些明确属于 Task 12+，本任务未越界实现。
