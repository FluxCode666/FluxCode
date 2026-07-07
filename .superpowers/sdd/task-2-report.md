# Task 2 Report: Runner Timer Jitter Scheduling

## 实现了什么

- 在 `backend/internal/service/channel_monitor_runner.go` 中为 `scheduledMonitor` 增加了 `jitter time.Duration`。
- 新增 `(*scheduledMonitor).nextDelay() time.Duration`，按 brief 要求基于 `interval` 和 `jitter` 计算随机 delay，并在低于 `monitorMinIntervalSeconds` 时进行 clamp。
- 调整 `ChannelMonitorRunner.Start()`：即使 `channel_monitor_enabled=false`，startup 仍会加载并注册 enabled monitors；实际是否执行检查仍由 `fire()` 内的 `canRun()` gate 控制。
- 在 `Schedule()` 中读取 `ChannelMonitor.JitterSeconds`，转换为 `time.Duration`，并对负值做保护性归零。
- 将 `runScheduled()` 从固定 `ticker` 改为每次触发后重新计算 delay 的 `timer` 调度。
- 在 `backend/internal/service/channel_monitor_runner_test.go` 中新增/更新测试，覆盖：
  - 默认关闭时 startup 仍会 preload/schedule，但不会实际 `RunCheck`
  - `nextDelay()` 在无 jitter、有 jitter、以及需要最小值 clamp 时的行为
  - 默认关闭 startup 的既有测试语义已更新为新要求

## 测试命令和结果

- RED:
  - 命令：`cd backend && go test ./internal/service -run 'TestChannelMonitorRunnerStartDefaultDisabledSchedulesButSkipsChecks|TestScheduledMonitorNextDelay' -count=1`
  - 结果：`FAIL`
- GREEN:
  - 命令：`cd backend && go test ./internal/service -run 'TestChannelMonitorRunner|TestScheduledMonitorNextDelay' -count=1`
  - 结果：`PASS`

## TDD Evidence

### RED

- 命令：

```bash
cd backend && go test ./internal/service -run 'TestChannelMonitorRunnerStartDefaultDisabledSchedulesButSkipsChecks|TestScheduledMonitorNextDelay' -count=1
```

- 关键失败输出：

```text
internal/service/channel_monitor_runner_test.go:43:51: runner.tasks[10].jitter undefined (type *scheduledMonitor has no field or method jitter)
internal/service/channel_monitor_runner_test.go:49:3: unknown field jitter in struct literal of type scheduledMonitor
internal/service/channel_monitor_runner_test.go:52:40: task.nextDelay undefined (type *scheduledMonitor has no field or method nextDelay)
```

- 为什么预期失败：
  - 这是新增行为对应的首轮测试，旧实现还没有 `jitter` 字段，也没有 `nextDelay()`，并且 startup disabled 语义还未迁移到“会 schedule 但不 fire checks”。

### GREEN

- 命令：

```bash
cd backend && go test ./internal/service -run 'TestChannelMonitorRunner|TestScheduledMonitorNextDelay' -count=1
```

- 关键通过输出：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	0.059s
```

## 修改文件

- `/Volumes/T7/project/new/FluxCode/backend/internal/service/channel_monitor_runner.go`
- `/Volumes/T7/project/new/FluxCode/backend/internal/service/channel_monitor_runner_test.go`

## 自审发现

- 改动范围保持在 runner 与对应测试文件，没有触碰其他 service、handler、repo、frontend 或默认配置。
- `fire()` 的 `canRun()` gate 保持不变，因此本地默认 `channel_monitor_enabled=false` 时仍不会实际执行 `RunCheck`。
- `nextDelay()` 的下限 clamp 使用现有 `monitorMinIntervalSeconds`，与 `interval_seconds - jitter_seconds >= 15` 的语义一致。
- 定时器在首次 `fire()` 之后才创建，并在每轮触发后重新 `Reset(task.nextDelay())`，符合“每次调度重新计算 jitter delay”的需求。
- 测试里为 startup disabled 场景增加了 `runCalls` 断言，确认 monitor 被注册但检查未执行。

## 疑问或担忧

- 当前只运行了 brief 指定的 runner tests，没有额外全量执行 `./internal/service` 全部测试；从任务要求看这已足够，但如果后续该模块还有依赖 startup list-call 次数的测试，建议在集成阶段再跑一遍更大范围测试。

## Fix after review

- reviewer 的 Important finding 已修复：`TestChannelMonitorRunnerStartsWhenSettingEnabledAfterDefaultDisabledStartup` 现在重新覆盖“startup 时 disabled、后续切换 enabled 后，已注册 task 会在下一轮调度真正触发 `RunCheck`，且不会重新 reload monitors”。
- 测试改为使用轻量 `channelMonitorRunnerSvcStub` 和可变的 `channelMonitorRuntimeStub`，避免引入真实 `ChannelMonitorService.RunCheck` 依赖。
- 为避免并发读写不稳定，runtime 与 runner service stub 都增加了 `sync.Mutex` 保护；等待调度触发使用 `require.Eventually`，不再依赖裸 `time.Sleep`。

- 聚焦验证命令：

```bash
cd backend && go test ./internal/service -run 'TestChannelMonitorRunnerStartsWhenSettingEnabledAfterDefaultDisabledStartup' -count=1
```

- 关键输出：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	1.060s
```

- 任务要求验证命令：

```bash
cd backend && go test ./internal/service -run 'TestChannelMonitorRunner|TestScheduledMonitorNextDelay' -count=1
```

- 关键输出：

```text
ok  	github.com/Wei-Shaw/sub2api/internal/service	1.092s
```
