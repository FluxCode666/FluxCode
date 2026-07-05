# 渠道监控上游对齐设计

## 背景

当前分支已确认 `upstream/main` 指向远端最新提交 `b650bdd68d25bad3e502b2e34efe775555da2eba`。本仓库 `main` 与上游分叉较大，直接合并上游主干会带来大量非渠道监控冲突和行为变化。

本地已经移植了渠道监控主体能力，包括管理端监控项、用户端渠道状态、OpenAI `chat_completions` 与 `responses` 检测、请求模板快照、错误体保留、API key 脱敏、SSRF 防护、历史与聚合查询。与上游最新渠道监控相比，明确缺口是 `jitter_seconds` 调度抖动链路；另外上游把渠道监控默认开关改为开启，但本次明确保留本地默认关闭策略。

## 目标

1. 在新分支 `codex/channel-monitor-upstream-align` 上定向移植渠道监控上游差异。
2. 补齐 `jitter_seconds`，让每个监控项可配置周期检测的正负随机抖动。
3. 保留本地 `channel_monitor_enabled=false` 默认关闭策略。
4. 保留本地已有渠道监控能力，不重写无关页面或全量合并上游主干。
5. 通过渠道监控专项测试验证后端调度、校验、持久化和前端表单行为。

## 非目标

1. 不全量 merge `upstream/main`。
2. 不把上游默认开启策略同步到本地。
3. 不改造网关调度、计费、账号状态或模型映射逻辑。
4. 不重写用户端渠道状态页，只做必要兼容和字段展示。
5. 不处理渠道监控以外的上游功能差异。

## 方案选择

采用定向移植，而不是全量合并或直接 cherry-pick 一串上游提交。

定向移植的优点是冲突面小，容易审查，并且能保留当前仓库的销售、支付、系统提示词、推广、OpenAI Codex image bridge 等本地定制。cherry-pick 虽然可追溯性更强，但上游相关提交夹带周边重构和格式化，冲突成本更高。全量 merge 风险最大，不符合本次只对齐渠道监控的目标。

## 后端设计

### 数据模型

在 `channel_monitors` 增加 `jitter_seconds` 字段：

- 类型：整数秒。
- 默认值：`0`。
- 含义：每轮调度在 `interval_seconds` 基础上增加 `±[0, jitter_seconds]` 的均匀随机偏移。
- 约束：`jitter_seconds >= 0`，并且 `interval_seconds - jitter_seconds >= 15`，避免随机偏移后检测频率过高。

需要同步更新 ent schema、生成代码和迁移。迁移编号从当前仓库最新序列继续，不复用上游编号，避免和本地迁移历史冲突。

### Service 与校验

`ChannelMonitorCreateParams` 和 `ChannelMonitorUpdateParams` 增加 `JitterSeconds`。

创建与更新时校验：

- `interval_seconds` 仍保持 `15` 到 `3600`。
- `jitter_seconds` 不允许为负数。
- `interval_seconds - jitter_seconds` 不得小于最小检测间隔。
- 更新时如果 `interval_seconds` 或 `jitter_seconds` 任一变化，使用更新后的组合重新校验。

校验失败返回现有错误体系中的 Bad Request 错误，新增错误码命名为 `CHANNEL_MONITOR_INVALID_JITTER`。

### Repository 与 DTO

repository 在创建、更新和 ent 转 service 模型时读写 `jitter_seconds`。

管理端 handler 请求与响应增加 `jitter_seconds`：

- 创建请求：可选，未传时为 `0`。
- 更新请求：可选，未传时不更新。
- 响应：始终返回当前值。

用户端只读接口不需要暴露 `jitter_seconds`，因为抖动是管理端调度配置，不影响用户状态展示。

### Runner 调度

`ChannelMonitorRunner` 的单个监控任务从固定 `ticker` 改为可重置 `timer`：

1. 调度任务启动后立即执行一次检测，保持现有行为。
2. 每次检测后计算下一次延迟：`interval - jitter + random(0, 2*jitter)`。
3. 当 `jitter_seconds=0` 时退化为固定 `interval_seconds`。
4. 延迟下限兜底为最小检测间隔。
5. 保持现有 in-flight 防重入和 worker pool 满载保护。
6. 每次 fire 前仍读取 `channel_monitor_enabled`，关闭时不执行检测。

功能开关默认仍关闭。runner 可以启动并注册任务，但只有开关开启时才真正执行周期检测；管理端接口仍可配置监控项。

## 前端设计

### API 类型

`frontend/src/api/admin/channelMonitor.ts` 增加：

- `ChannelMonitor.jitter_seconds: number`
- `CreateParams.jitter_seconds?: number`
- `UpdateParams.jitter_seconds?: number`

### 管理端表单

`MonitorFormDialog.vue` 在检测间隔附近增加抖动秒数字段：

- 默认值：`0`。
- 最小值：`0`。
- 最大值：`interval_seconds - 15`。
- 编辑已有监控时回填 `jitter_seconds`。
- 提交创建和更新时带上当前值。

文案说明该字段用于避免多个监控固定同步触发。新增中英文 i18n 文案。

### 用户端页面

用户端渠道状态页不重写。若本地页面已经满足状态卡片、时间线、详情弹窗和自动刷新需求，只保持现状。除非新增字段导致类型错误，否则不修改用户端展示。

## 默认开关策略

本次保留本地默认关闭：

- 新安装或缺省配置中 `channel_monitor_enabled=false`。
- 前端 public settings fallback 也保持 `channel_monitor_enabled=false`，避免接口尚未返回时菜单短暂显示。
- 管理端可打开开关后，用户端菜单和状态页才可见，runner 才执行检测。

这与上游当前默认开启不同，是本地产品策略的显式差异，不作为未对齐问题。

## 错误处理与安全

1. 抖动配置错误返回明确的管理端校验错误。
2. endpoint SSRF 校验、API key 加密存储和响应脱敏保持现有实现。
3. 检测失败只写历史，不影响网关请求链路。
4. worker pool 满载时跳过本轮检测并释放 in-flight 标记，避免监控永久卡住。
5. 随机抖动不会突破最小检测间隔。

## 测试设计

后端测试：

- `validateJitter` 覆盖正常、负数、超过 interval 下限、边界值。
- 创建监控时写入 `jitter_seconds`。
- 更新 interval 或 jitter 时触发组合校验。
- repository 创建、更新、读取保留 `jitter_seconds`。
- runner `nextDelay` 在 `interval ± jitter` 范围内，并且不低于最小间隔。
- runner 在开关关闭时不执行检测，开关开启后按任务执行。

前端测试：

- 管理端监控表单默认 `jitter_seconds=0`。
- 编辑已有监控时回填字段。
- 表单提交 payload 包含 `jitter_seconds`。
- 最大值随 `interval_seconds` 变化。

验证命令：

- `cd backend && go test ./internal/service -run 'ChannelMonitor|Jitter'`
- `cd backend && go test ./internal/repository -run 'ChannelMonitor|migration'`
- `cd backend && go test ./internal/handler ./internal/server/routes -run 'ChannelMonitor'`
- `cd frontend && npm run test -- --run src/views/admin/__tests__/ChannelMonitorView.spec.ts`
- `cd frontend && npm run type-check`

## 验收标准

1. `channel_monitors` 表存在 `jitter_seconds`，默认 `0`。
2. 管理端可以创建和编辑抖动秒数。
3. 后端拒绝会导致实际间隔低于 15 秒的抖动配置。
4. runner 对配置了抖动的监控使用随机延迟，对未配置抖动的监控保持固定间隔。
5. `channel_monitor_enabled` 默认仍为 `false`。
6. 渠道监控专项后端测试和前端测试通过。
7. diff 聚焦渠道监控，不引入上游无关改动。
