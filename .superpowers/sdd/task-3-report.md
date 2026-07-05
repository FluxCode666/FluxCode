# Task 3 Report: Frontend Admin Form Wiring

## 实现了什么

- 在 `frontend/src/api/admin/channelMonitor.ts` 为 `ChannelMonitor` 和 `CreateParams` 接入 `jitter_seconds` 字段。
- 在 `frontend/src/components/admin/monitor/MonitorFormDialog.vue` 为 admin 监控创建/编辑表单新增 `jitter_seconds` 输入框，并为 `interval_seconds` / `jitter_seconds` 增加 `data-testid`。
- 在表单状态中加入 `form.jitter_seconds`，默认新建值为 `0`，编辑时从现有 monitor 加载。
- 新增 `maxJitterSeconds` 计算值和 interval watcher，保持 `jitter_seconds >= 0` 且 `interval_seconds - jitter_seconds >= 15` 的前端约束。
- 在 `buildPayload()` 中提交 `jitter_seconds` 到 create/update payload。
- 在 `frontend/src/i18n/locales/zh.ts` 与 `frontend/src/i18n/locales/en.ts` 增加 `admin.channelMonitor.form.jitterSeconds` 与 `admin.channelMonitor.form.jitterSecondsHint`。
- 新增 `frontend/src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts`，覆盖默认值、编辑回填、提交 payload、interval 降低后的 jitter clamp。

## 测试命令和结果

### RED

命令：

```bash
cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts
```

结果：失败，符合预期。

关键失败输出：

```text
FAIL  src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts
Error: Unable to get [data-testid="monitor-jitter-input"] within: <div ...>
Tests  4 failed (4)
```

为什么预期失败：

- brief 要求先写表单 jitter 测试。
- 运行 RED 时，`MonitorFormDialog.vue` 里还没有 `monitor-jitter-input` 和对应表单字段，所以测试正确地因为缺少 jitter 输入控件而失败。

### GREEN

命令：

```bash
cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts src/views/admin/__tests__/ChannelMonitorView.spec.ts
```

结果：通过。

关键通过输出：

```text
✓ src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts (4 tests)
✓ src/views/admin/__tests__/ChannelMonitorView.spec.ts (1 test)
Test Files  2 passed (2)
Tests  5 passed (5)
```

## TDD Evidence

- RED 命令：`cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts`
- RED 关键失败：`Unable to get [data-testid="monitor-jitter-input"]`
- RED 失败原因：实现前表单没有 jitter 控件/状态/payload 接线
- GREEN 命令：`cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts src/views/admin/__tests__/ChannelMonitorView.spec.ts`
- GREEN 关键通过：`Test Files  2 passed (2)`，`Tests  5 passed (5)`

## 修改文件

- `frontend/src/api/admin/channelMonitor.ts`
- `frontend/src/components/admin/monitor/MonitorFormDialog.vue`
- `frontend/src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts`
- `frontend/src/i18n/locales/zh.ts`
- `frontend/src/i18n/locales/en.ts`

## 自审发现

- 已确认只修改了任务允许的前端文件与本报告文件。
- 未修改用户端渠道状态页。
- 前端 clamp 逻辑通过 watcher 保持 `interval_seconds - jitter_seconds >= 15`，与 brief 语义一致。
- payload 通过 `buildPayload()` 统一输出 `jitter_seconds`，不会影响现有 template、headers、body override 逻辑。
- `ChannelMonitorView.spec.ts` 复跑通过，说明 admin 页面基础壳子未被本次接线破坏。

## 疑问或担忧

- 测试输出包含 `Browserslist: browsers data (caniuse-lite) is 7 months old` 警告；这不是本任务引入的问题，也不影响本次结果。
