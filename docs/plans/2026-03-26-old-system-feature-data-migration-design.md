# 旧系统功能与数据迁移技术方案

## 背景

当前仓库 `/Volumes/T7/project/new/FluxCode` 与旧仓库 `/Volumes/T7/project/FluxCode` 属于同源系统，但功能演进已出现明显分叉。当前仓库已经补入了部分定价与号池监控能力，也存在一批尚未完全收口的相关改动；与此同时，旧仓库中仍保留了当前系统尚未完整承接的叠卡订阅能力、相关 dashboard 展示、多语言文案，以及与这些保留功能绑定的历史业务数据和历史统计数据。

用户已明确本次迁移边界如下：

- 需要迁移：定价相关功能
- 需要迁移：号池监控相关功能
- 需要迁移：叠卡订阅能力
- 需要迁移：与上述保留功能相关的旧系统数据
- 不需要迁移：独立工具
- 不需要迁移：补丁类能力

用户同时确认：

- 不做 worktree 隔离
- 执行阶段每完成一步都需要维护“已完成 / 进行中 / 待做”记录
- 数据迁移采用可重复预演后切换，而不是一次性不可回放导入

## 目标

在不误迁独立工具与补丁、不破坏当前仓库已有收口工作的前提下，将旧仓库中与保留业务相关的代码能力与数据能力完整迁移到当前仓库，最终形成以下闭环：

1. 定价系统功能完整可见、可配、可用
2. 号池监控功能、配置与 dashboard 展示完整可用
3. 叠卡订阅能力从后端模型到前端交互完整对齐旧系统
4. 旧系统中与上述功能相关的业务数据与历史统计数据迁移到当前系统
5. 切换后历史视图尽量不断档，计费与订阅消耗语义保持一致

## 方案对比

### 方案 A：功能迁移与数据迁移分离推进，功能先落地，数据后补

优点：

- 代码收口节奏更清晰
- 前后端功能可以先行联调

缺点：

- 迁移期较长
- 容易出现“功能已可见但历史数据为空”的中间态
- 叠卡这类强依赖数据模型的能力难以真正分离

### 方案 B：以旧系统为准，按业务域做功能与数据一体化迁移

优点：

- 功能语义、数据结构、历史结果更容易保持一致
- 更适合叠卡、号池监控、dashboard 这类强耦合功能
- 切换验收口径清晰

缺点：

- 需要在执行前把数据迁移边界与重建策略定义清楚
- 对迁移器设计要求更高

### 方案 C：数据库导出导入为主，代码只做最少补丁

优点：

- 表面推进速度快
- 对已有数据库工具链依赖较小

缺点：

- 不适合本次非完全同构迁移
- 叠卡新增表、衍生聚合表回填、多阶段对账都会变得笨重
- 难以支撑 dry-run、重复预演与阶段恢复

## 结论

采用方案 B。

本次迁移把旧系统视为保留业务域的事实源，把当前仓库视为最终承接系统。迁移工作不是简单搬文件，也不是单纯搬库，而是一次“功能迁移 + 数据迁移 + 计费语义迁移 + 历史统计延续”的系统级收口。

## 已确认的关键决策

### 切换方式

采用：可重复预演后切换。

含义：

- 建立可重复执行的迁移方案
- 支持 dry-run 与预演环境多次验证
- 正式切换时冻结旧系统写入
- 最后一次导入完成后切流

### 数据策略

采用：旧系统为准整体接管。

含义：

- 正式迁移时，目标系统相关业务域数据以旧系统为事实源
- 尽量保留旧系统主键与外键关系
- 不做复杂的并库去重与长期双写

### 数据保留范围

采用：只迁保留功能相关数据。

包括：

- 用户、分组、账号、代理、API Key
- 用户属性、允许分组关系、系统设置
- 兑换码、订阅、叠卡 grant
- 使用日志、计费辅助数据
- 定价配置
- 号池监控配置与历史统计
- dashboard 相关历史统计

排除：

- 独立工具
- 补丁类功能
- 与排除模块强绑定的专属数据
- 纯运行态缓存、锁、幂等键、临时任务结果等非业务事实数据

### 历史数据要求

采用：业务数据 + 历史统计全保留。

目标：

- 切换后不仅保留当前有效状态
- 还尽量保留 dashboard、号池监控等历史视图连续性
- 对旧系统没有同构来源的新系统衍生聚合表，允许在导入原始数据后重建

### 实现形态

采用：离线双库迁移器。

说明：

- 不采用纯 SQL / `pg_dump` 主导方案
- 不采用应用内管理员导入任务作为主实现
- 迁移器需支持阶段执行、dry-run、正式 apply、失败恢复与对账报告

## 现状结论

### 叠卡能力现状

已确认旧系统存在完整叠卡实现，而当前系统尚未完整承接。

旧系统已具备：

- `subscription_grants` 表
- `redeem_codes.subscription_mode`
- `extend | stack` 两种订阅模式
- 兑换时冲突二选一
- 管理端分配时冲突二选一
- grant 级别额度消耗
- 最早到期优先消耗
- grant 到期后的额度回落逻辑

当前系统缺口：

- 缺少 `subscription_grants`
- 缺少 `subscription_mode`
- 缺少完整后端服务链
- 缺少完整前端交互
- 缺少相关多语言文案的一致接入

### 定价与号池监控现状

当前系统已经存在部分迁移痕迹：

- `backend/migrations/079_pricing_plan_tables.sql`
- `backend/migrations/080_pool_monitor_tables.sql`

说明：

- 定价与号池监控已有基础 schema
- 但不能据此认定功能已完整迁移
- 仍需继续核对页面、接口、dashboard 接线、多语言与历史数据路径

### Dashboard 现状

已确认当前系统存在：

- `/api/v1/admin/dashboard/proxies-usage` 路由注册
- 前端 dashboard 代理使用图接口调用
- `DashboardView` 中代理使用统计模块

这说明 dashboard 已不是完全空白，但仍需在最终实施中纳入专项验收，防止局部有代码、整体体验仍不完整。

### 多语言现状

当前系统已存在部分相关文案，但不能认定已全部完整。实施中需要专项审计以下区域：

- dashboard
- 号池监控
- 调度状态下拉
- 用户兑换页叠卡提示
- 管理端订阅分配页叠卡提示
- 相关错误提示、空状态、按钮文案、说明文案

### 数据导入现状

当前系统已有“账号与代理数据导入/导出”能力，但只覆盖：

- 账号
- 代理

它不能直接承担本次迁移任务，因为本次还需要迁：

- 用户
- 分组
- API Key
- 设置
- 兑换码
- 订阅与叠卡 grant
- 使用日志
- 定价
- 号池监控配置
- 历史统计

因此必须单独建设“旧库到新库”的离线双库迁移器。

## 模块边界

### 后端主线

#### 叠卡订阅主线

需要补齐：

- 叠卡 migration
- schema / repository
- subscription service
- redeem service
- admin subscription handler
- user redeem handler
- gateway usage / billing 归因链

#### 定价主线

继续以当前仓库已存在的 pricing 能力为基线，对齐旧系统行为与历史数据，而不是整块覆盖当前实现。

#### 号池监控主线

继续以当前仓库已存在的 `pool monitor`、dashboard 代理使用统计链路为基线，对齐旧系统行为、配置项与历史统计保留。

### 前端主线

#### 用户侧

补齐：

- 兑换页 `extend / stack` 二选一
- 叠卡预览与提示
- 我的订阅页 grant/额度可视化

#### 管理侧

补齐：

- 订阅分配页 `extend / stack` 二选一
- dashboard 缺失模块与历史数据显示
- 号池监控页面与配置页面
- 多语言缺口

## 数据迁移范围设计

### 直接迁移的共享业务表

以下数据优先按“保留原 ID + 保留关联关系”的原则迁移：

- `users`
- `groups`
- `user_allowed_groups`
- `user_attribute_definitions`
- `user_attribute_values`
- `settings`
- `proxies`
- `accounts`
- `account_groups`
- `api_keys`
- `redeem_codes`
- `user_subscriptions`
- `pricing_plan_groups`
- `pricing_plans`
- `account_pool_alert_configs`
- `usage_logs`
- `billing_usage_entries`
- `proxy_usage_metrics_hourly`
- `ops_metrics_hourly`
- `ops_metrics_daily`

### 需要新增承接的旧系统数据表

旧系统有、当前系统业务上需要但结构未完整承接的表：

- `subscription_grants`

对应策略：

- 在当前系统补齐 schema、迁移脚本、仓储、服务与消费链
- 再把旧系统中的 grant 历史数据导入

### 新系统独有但不从旧系统直接搬运的表

这类表不作为旧库事实数据直接导入目标，而是在基础数据导入后进行回填或重建：

- `usage_dashboard_hourly`
- `usage_dashboard_daily`
- `usage_dashboard_hourly_users`
- `usage_dashboard_daily_users`
- `usage_dashboard_aggregation_watermark`
- `usage_billing_dedup`
- `usage_billing_dedup_archive`

原因：

- 这些表属于聚合、衍生或一致性辅助层
- 旧系统没有完全同构来源
- 直接伪造迁移值风险较高
- 更适合通过原始 `usage` 数据重建

### 字段差异审计（2026-04-03 更新）

对老项目与新项目所有迁移范围内表的 schema 进行逐字段对比，结论如下。

#### ⚠️ 需处理：老项目有而新项目缺失的字段

| 表 | 字段 | 老项目定义 | 新项目状态 | 风险 | 处理建议 |
|---|---|---|---|---|---|
| `redeem_codes` | `welfare_no` | `VARCHAR(64) NULLABLE` | **不存在** | **高 — 数据丢失** | 新项目需补齐该列及 `(used_by, welfare_no)` 唯一索引，否则迁移时列交集机制会丢弃此字段数据 |

#### ✅ 安全：新项目有而老项目没有的字段（列交集跳过，使用默认值）

以下字段在迁移时因列交集机制而不会从老库读取，目标端将使用 DB 默认值。已确认默认值语义正确，不影响业务。

**accounts 表（4 个新增字段）：**

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `notes` | `text` | `NULL` | 管理员备注，迁移后为空 |
| `load_factor` | `int` | `NULL` | 负载因子，新功能字段 |
| `rate_multiplier` | `decimal(10,4)` | `1.0` | 账号计费倍率，默认 1.0 与老系统行为一致 |
| `auto_pause_on_expired` | `bool` | `true` | 过期自动暂停，新功能字段 |

> 注：`temp_unschedulable_until` 和 `temp_unschedulable_reason` 两个项目均有（老项目 migration 020），列交集会正确迁移。

**groups 表（15+ 新增字段）：**

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `image_price_1k/2k/4k` | `decimal(20,8)` | `NULL` | 图片定价，新功能 |
| `sora_image_price_360/540` | `decimal(20,8)` | `NULL` | Sora 图片定价 |
| `sora_video_price_per_request(_hd)` | `decimal(20,8)` | `NULL` | Sora 视频定价 |
| `sora_storage_quota_bytes` | `bigint` | `0` | Sora 存储配额 |
| `claude_code_only` | `bool` | `false` | Claude Code 限制 |
| `fallback_group_id` | `bigint` | `NULL` | 降级分组 ID |
| `fallback_group_id_on_invalid_request` | `bigint` | `NULL` | 无效请求兜底分组 |
| `model_routing` | `jsonb` | `NULL` | 模型路由配置 |
| `model_routing_enabled` | `bool` | `false` | 模型路由开关 |
| `mcp_xml_inject` | `bool` | `true` | MCP XML 注入 |
| `supported_model_scopes` | `jsonb` | `["claude","gemini_text","gemini_image"]` | 支持的模型系列 |
| `sort_order` | `int` | `0` | 分组排序 |
| `allow_messages_dispatch` | `bool` | `false` | Messages 调度 |
| `default_mapped_model` | `varchar(100)` | `""` | 默认映射模型 |

**users 表（5 个新增字段）：**

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `totp_secret_encrypted` | `text` | `NULL` | TOTP 密钥，老系统无 TOTP |
| `totp_enabled` | `bool` | `false` | TOTP 启用状态 |
| `totp_enabled_at` | `timestamptz` | `NULL` | TOTP 启用时间 |
| `sora_storage_quota_bytes` | `bigint` | `0` | Sora 存储配额 |
| `sora_storage_used_bytes` | `bigint` | `0` | Sora 存储使用量 |

**api_keys 表（14 个新增字段）：**

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `last_used_at` | `timestamptz` | `NULL` | 最后使用时间 |
| `ip_whitelist` / `ip_blacklist` | `jsonb` | `NULL` | IP 限制 |
| `quota` / `quota_used` | `decimal(20,8)` | `0` | 配额（0=无限） |
| `expires_at` | `timestamptz` | `NULL` | 过期时间 |
| `rate_limit_5h/1d/7d` | `decimal(20,8)` | `0` | 速率限制（0=无限） |
| `usage_5h/1d/7d` | `decimal(20,8)` | `0` | 窗口内使用量 |
| `window_5h/1d/7d_start` | `timestamptz` | `NULL` | 限速窗口起始 |

**usage_logs 表（9 个新增字段）：**

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `requested_model` | `varchar(100)` | `NULL` | 客户请求模型 |
| `upstream_model` | `varchar(100)` | `NULL` | 实际上游模型 |
| `account_rate_multiplier` | `decimal(10,4)` | `NULL` | 账号倍率快照 |
| `user_agent` | `varchar(512)` | `NULL` | 用户代理 |
| `ip_address` | `varchar(45)` | `NULL` | IP 地址 |
| `image_count` | `int` | `0` | 图片数量 |
| `image_size` | `varchar(10)` | `NULL` | 图片尺寸 |
| `media_type` | `varchar(16)` | `NULL` | 媒体类型 |
| `cache_ttl_overridden` | `bool` | `false` | 缓存 TTL 覆盖标记 |

#### ✅ 字段完全一致的表

以下表在老项目与新项目之间字段定义完全一致（仅 import 路径和常量引用不同），迁移无风险：

- `user_subscriptions`
- `subscription_grants`
- `user_allowed_groups`
- `account_groups`
- `user_attribute_definitions`
- `user_attribute_values`
- `proxies`
- `settings`

#### ✅ SQL-only 表结构一致性

以下纯 SQL 定义的表在两个项目间结构一致（新项目为合并后的建表语句）：

- `pricing_plan_groups` — 一致
- `pricing_plans` — 新项目已合并 029/030/032 迁移，结构一致
- `account_pool_alert_configs` — 新项目已合并 037-048 迁移，结构一致
- `proxy_usage_metrics_hourly` — 一致
- `ops_metrics_hourly` — 一致
- `ops_metrics_daily` — 一致
- `billing_usage_entries` — 一致
- `orphan_allowed_groups_audit` — 一致

#### 🔧 所需修复动作

1. **[必须] 新项目补齐 `redeem_codes.welfare_no` 列**
   - 在 `backend/ent/schema/redeem_code.go` 中添加 `welfare_no` 字段
   - 新建 SQL 迁移文件添加 `ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS welfare_no VARCHAR(64)`
   - 创建 `(used_by, welfare_no)` 唯一索引
   - 否则老系统福利码数据将在迁移时被截断

2. **[建议] 确认 `accounts.rate_multiplier` 默认值 1.0 的语义**
   - 老系统账号没有 `rate_multiplier` 概念，迁移后全部默认 1.0
   - 需确认这不会改变原有计费行为（1.0 = 不加倍 = 与老系统一致）

3. **[建议] 确认 `groups.mcp_xml_inject` 默认值 `true` 的影响**
   - 老系统无此字段，迁移后全部分组默认开启 MCP XML 注入
   - 若 antigravity 平台以外的分组也会触发注入逻辑，需确认是否有条件保护

### 明确排除的数据

以下数据不纳入本次“旧系统数据迁移”范围：

1. 独立工具相关表
2. 补丁/临时增强模块数据
3. 幂等记录
4. 调度外发队列
5. 清理任务结果
6. 系统运行缓存
7. 纯临时审计与运行态表

原则：

- 若某数据不是保留功能的业务事实源，则不迁
- 若迁过去只会增加复杂度而不改善业务结果，则不迁

## 技术设计

### 叠卡后端设计

当前系统需补齐旧系统叠卡核心能力，目标行为与旧系统保持一致。

核心模型：

- `user_subscriptions`：保留订阅聚合视图与兼容字段
- `subscription_grants`：每次 `extend / stack` 形成的额度窗实体
- `redeem_codes.subscription_mode`：记录最终选择的 `extend / stack`

核心语义：

- `extend`：延长当前订阅到期
- `stack`：从当前时刻新增一段独立额度窗，可并行存在多个 grant

核心规则：

- 同组已有未过期订阅时，若未指定模式，返回 `choice required`
- 消耗时优先扣最早到期 grant
- grant 到期后不再参与可用额度计算
- 聚合展示值由 grant 状态汇总得出

### 前端设计

#### 用户端

补齐：

- 兑换时 `extend / stack` 二选一
- 叠卡后有效期预览
- 当前订阅与叠加订阅效果说明
- 我的订阅页 grant/额度可视化

#### 管理端

补齐：

- 分配订阅时 `extend / stack` 二选一
- 对已有订阅用户的模式提示
- dashboard 中代理使用与相关监控视图验收

#### 多语言

统一补齐并校验：

- 中文
- 英文

要求：

- 不允许依赖 fallback 文案来“看起来能跑”
- 所有新增或迁移功能必须有正式 key

### 数据迁移器设计

输入：

- 旧库连接
- 新库连接
- 执行模式：`dry-run` / `apply`
- 迁移阶段：支持分阶段执行
- 是否清理目标域数据
- 报告输出路径

行为要求：

- 支持重复预演
- 支持正式执行
- 支持按阶段重跑
- 支持失败后从阶段恢复
- 支持生成对账报告

建议执行阶段：

1. 基础身份与配置
2. 资源与账号
3. 兑换码、订阅、叠卡
4. 使用日志与计费辅助数据
5. 定价与号池监控
6. 历史统计
7. 新系统衍生聚合表回填
8. 对账与验收报告

### 正式切换设计

正式切换采用固定流程：

1. 预演环境多轮 dry-run
2. 输出差异与对账报告
3. 确认冻结窗口
4. 冻结旧系统写入
5. 执行最终 apply
6. 回填新系统衍生聚合表
7. 做页面与业务验收
8. 切换流量
9. 保留回滚用快照与报告

## 风险与处理

### 叠卡不是简单 UI 迁移

风险：

- 如果只迁页面，不迁 grant 级扣费逻辑，会出现额度显示正确但实际扣费错误

处理：

- 叠卡按“后端模型 + 计费链路 + 前端交互”完整迁移
- 不接受只做表面 UI 对齐

### 历史统计连续性风险

风险：

- 如果只搬业务主表，不搬历史统计或不重建聚合，dashboard 和 pool monitor 图表会断档

处理：

- 共享历史统计表直接迁
- 新系统衍生聚合表通过原始数据回填

### 当前仓库存在部分未完成迁移痕迹

风险：

- 仓库已存在 pricing / pool monitor / dashboard / i18n 局部实现，若直接覆盖，容易打掉当前已有修复

处理：

- 先 diff 当前实现与旧系统行为
- 决定每个点是复用、补齐还是替换
- 在脏工作区里避免误回退用户已有改动

### 安全与加密差异

已确认：

- 用户密码为 `bcrypt` 哈希，可直接迁移
- 新系统存在 `TOTP` 与 `security_secrets` 体系

处理：

- 若旧系统无对应 TOTP 数据，则不伪造迁移
- 新系统安全密钥保持目标端现状
- 只迁移真实存在且业务需要承接的数据

## 测试与验收标准

### 功能验收

- 定价页可见、可编辑、可展示
- 号池监控页可见、可配置、可展示历史
- dashboard 可展示代理使用统计
- 用户兑换可选择 `extend / stack`
- 管理端分配订阅可选择 `extend / stack`
- 我的订阅页正确展示叠卡结果
- 多语言切换无缺 key、无回退漏网

### 数据验收

- 共享业务表行数对账通过
- 关键金额字段汇总对账通过
- 订阅有效期与 grant 对账通过
- 兑换码状态对账通过
- 历史 `usage` 与统计桶数量在允许误差范围内一致
- 新系统衍生聚合表回填成功

### 行为验收

- 随机抽样老用户可登录
- 订阅可查询
- 网关产生新消耗后 grant 扣减正确
- dashboard / pool monitor / pricing 页面结果与旧系统预期一致

## 执行记录要求

实施阶段必须维护固定格式执行台账：

- 已完成
- 进行中
- 待做

规则：

- 每完成一个子任务立即记录
- 每次开始新子任务前先刷新台账
- 每次交付前检查“待做”是否仍有遗漏项

## 结论

本次不是简单功能复制，而是一次“功能迁移 + 数据迁移 + 计费语义迁移 + 历史统计延续”的系统级迁移。

最核心的实施原则是：

- 旧系统为准
- 叠卡按完整后端能力迁移
- 数据迁移用离线双库迁移器完成
- 历史统计尽量保留，无法直接搬运的聚合表通过新系统回填
- 排除独立工具与补丁域
- 执行过程全程留痕，按步骤维护台账
