# 渠道状态与渠道监控局部移植设计

## 背景

当前仓库只存在「渠道管理」能力，主要负责渠道、分组、模型定价、模型映射和渠道启停。用户提出的「渠道状态」来自原始上游 `Wei-Shaw/sub2api` 的 `Channel Monitor` 功能，并不是当前 `ChannelsView.vue` 中的渠道状态开关。

上游功能包含用户端「渠道状态」页面、管理端「渠道监控」配置页、后台定时检测、请求模板、检测历史、日聚合与功能开关。由于当前仓库已经包含销售、支付、系统提示词、推广等本地定制，全量合并上游 `main` 会引入大规模冲突和无关行为变化。因此本次采用局部移植：只移植 `Channel Monitor` 闭环能力，并保持默认关闭。

## 目标

1. 移植用户端「渠道状态」菜单页 `/monitor`。
2. 移植管理端「渠道监控」配置页 `/admin/channels/monitor`。
3. 移植后端渠道监控 CRUD、立即检测、周期检测、历史查询、用户视图聚合和请求模板能力。
4. 新增功能开关，默认关闭；关闭时用户端菜单和管理端子菜单隐藏，后台 runner 不执行检测。
5. 保留当前仓库的本地定制，不全量合并上游主干，不引入无关功能改动。

## 非目标

1. 不迁移上游主干中的全部近期改动。
2. 不替换当前「渠道管理」页面的既有渠道定价/映射逻辑。
3. 不改变网关主链路调度、计费、账号状态标记逻辑。
4. 不将检测结果直接用于请求调度或熔断；本次仅展示状态与可用性。

## 方案选择

采用完整功能局部移植，默认关闭。

备选方案包括只移植用户端展示页，或只加菜单占位。前者没有管理端配置和后台 runner，无法产生真实状态数据；后者不算实际迁移。完整局部移植虽然接线面更广，但功能闭环完整，并且默认关闭可以降低上线后的可见风险。

## 后端设计

新增渠道监控领域模型与服务，核心职责如下：

- `ChannelMonitorService`：负责监控项 CRUD、参数校验、API key 加密解密、立即检测、历史查询、用户视图聚合。
- `ChannelMonitorRunner`：负责后台周期检测。读取 `channel_monitor_enabled`，默认关闭时不调度；开启后按每个监控项的 `interval_seconds` 执行。
- `ChannelMonitorRepository`：负责监控项、历史记录、日聚合和聚合水位持久化。
- `ChannelMonitorRequestTemplateService`：负责请求模板 CRUD 与应用到监控项的快照复制。

支持的 provider：

- `openai`
- `anthropic`
- `gemini`

OpenAI 检测支持两种协议：

- `chat_completions`
- `responses`

检测状态枚举沿用上游：

- `operational`
- `degraded`
- `failed`
- `error`

API key 使用当前仓库已有的 `SecretEncryptor` 能力存储密文。handler 层只返回脱敏 key。解密失败时管理端列表展示需要重新填写 key，runner 与立即检测跳过该监控并返回明确错误。

## 数据模型

迁移文件不直接沿用上游编号，而是从当前仓库最新迁移之后继续编号，避免与当前分支后续迁移冲突。

新增表：

1. `channel_monitors`
   - 保存监控配置：名称、provider、OpenAI 协议模式、endpoint、加密 API key、主模型、附加模型、分组名、启用状态、检测间隔、模板快照字段、创建人和时间戳。

2. `channel_monitor_histories`
   - 保存单次检测历史：monitor、model、状态、请求延迟、ping 延迟、消息、检测时间。
   - monitor 删除时级联删除历史。

3. `channel_monitor_daily_rollups`
   - 保存日聚合结果，用于 7/15/30 天可用率和平均延迟查询。

4. `channel_monitor_aggregation_watermark`
   - 保存日聚合水位，避免重复全表聚合。

5. `channel_monitor_request_templates`
   - 保存可复用请求模板。模板应用到监控项时采用快照复制，模板后续变动不会自动影响已有监控，除非管理员主动应用。

新增设置：

- `channel_monitor_enabled`：默认 `false`。
- `channel_monitor_default_interval_seconds`：默认 `60`，范围 `15` 到 `3600`。

## API 设计

用户端只读接口：

- `GET /api/v1/channel-monitors`
- `GET /api/v1/channel-monitors/:id/status`

管理端接口：

- `GET /api/v1/admin/channel-monitors`
- `POST /api/v1/admin/channel-monitors`
- `GET /api/v1/admin/channel-monitors/:id`
- `PUT /api/v1/admin/channel-monitors/:id`
- `DELETE /api/v1/admin/channel-monitors/:id`
- `POST /api/v1/admin/channel-monitors/:id/run`
- `GET /api/v1/admin/channel-monitors/:id/history`
- `GET /api/v1/admin/channel-monitor-templates`
- `POST /api/v1/admin/channel-monitor-templates`
- `GET /api/v1/admin/channel-monitor-templates/:id`
- `PUT /api/v1/admin/channel-monitor-templates/:id`
- `DELETE /api/v1/admin/channel-monitor-templates/:id`
- `GET /api/v1/admin/channel-monitor-templates/:id/monitors`
- `POST /api/v1/admin/channel-monitor-templates/:id/apply`

关闭行为：

- 用户端列表接口返回空列表。
- 用户端详情接口返回 not found。
- 后台 runner 不执行检测。
- 管理端接口保留，便于管理员先配置监控，再开启用户可见入口。

## 前端设计

新增用户端页面：

- `frontend/src/views/user/ChannelStatusView.vue`
- `frontend/src/components/user/monitor/*`
- `frontend/src/api/channelMonitor.ts`

用户页功能：

- 展示渠道状态卡片、provider、主模型、附加模型、7 天可用率、延迟和近期时间线。
- 支持 7/15/30 天窗口切换。
- 支持详情弹窗。
- 支持自动刷新。
- 无数据时展示空状态。

新增管理端页面：

- `frontend/src/views/admin/ChannelMonitorView.vue`
- `frontend/src/components/admin/monitor/*`
- `frontend/src/api/admin/channelMonitor.ts`
- `frontend/src/api/admin/channelMonitorTemplate.ts`

管理页功能：

- 监控项列表、provider 筛选、启用状态筛选、搜索。
- 创建、编辑、删除、启停监控项。
- 立即检测并展示检测结果。
- 请求模板管理与应用。
- API key 选择和脱敏展示。

路由与菜单：

- 用户端新增 `/monitor`，菜单文案「渠道状态」。
- 管理端新增 `/admin/channels/monitor`，挂在「渠道管理」下，菜单文案「渠道监控」。
- 菜单显示受 `channel_monitor_enabled` 控制，默认关闭时不显示。

系统设置：

- public settings、admin settings、前端类型和 store 均补齐 `channel_monitor_enabled` 与 `channel_monitor_default_interval_seconds`。
- 系统设置页在功能开关区域新增「渠道监控」配置块。

## 错误处理与安全

1. endpoint 校验拒绝非法协议、loopback、内网和本地地址，降低 SSRF 风险。
2. API key 仅密文落库，响应只返回脱敏值。
3. API key 解密失败时不执行检测，并提示管理员重新填写。
4. 检测失败只写历史，不影响现有网关请求、计费、账号调度或渠道定价。
5. 功能默认关闭，避免迁移后用户端立即暴露未配置页面。

## 测试设计

后端测试重点：

- `ChannelMonitorService` 创建、更新、删除、校验、加密和解密失败。
- checker 请求构造和响应状态判断：OpenAI `chat_completions`、OpenAI `responses`、Anthropic、Gemini。
- runner 默认关闭不调度，开启后按 interval 执行，停用 monitor 后取消任务。
- repository 聚合：最近状态、7/15/30 天可用率、模板应用。
- handler/routes：用户端关闭返回空列表或 not found，管理端 CRUD 正常。

前端测试重点：

- router 注册 `/monitor` 与 `/admin/channels/monitor`。
- sidebar 默认关闭隐藏「渠道状态」和「渠道监控」，开启后显示。
- 用户页渲染空状态、状态卡和详情弹窗。
- 管理页能打开创建、编辑、模板、立即检测弹窗，并调用对应 API。

验证命令预期：

- `cd backend && go generate ./ent`
- `cd backend && go generate ./cmd/server`
- `cd backend && go test ./internal/service ./internal/handler ./internal/server/routes -run 'ChannelMonitor|channel monitor'`
- `cd frontend && npm run type-check`
- `cd frontend && npm run test -- --run` 或运行相关 vitest 子集。

## 验收标准

1. 默认 `channel_monitor_enabled=false`。
2. 默认关闭时用户端和管理端菜单不显示渠道监控入口。
3. 管理员开启后，用户端可看到「渠道状态」，管理端可看到「渠道监控」。
4. 管理员能创建监控项、运行立即检测、查看最近状态和历史。
5. 后台 runner 在开关开启时执行周期检测，关闭时停止检测。
6. 用户端能查看渠道可用性、延迟、时间线和详情。
7. 新迁移可幂等执行，并与当前仓库迁移序列兼容。
8. 代码 diff 聚焦渠道监控、settings、路由菜单、迁移和生成文件，不引入上游无关大规模改动。

## 实施顺序

1. 添加后端 schema、迁移与 ent 生成代码。
2. 添加 channel monitor service、repository、checker、runner、template service。
3. 接入 handler、routes、wire、setting service 与 public settings。
4. 添加前端 API、类型、页面、组件和 i18n。
5. 接入 router、sidebar、系统设置开关。
6. 运行后端目标测试、前端 typecheck 和相关 vitest。
7. 审查 diff，确认没有引入无关上游改动。
