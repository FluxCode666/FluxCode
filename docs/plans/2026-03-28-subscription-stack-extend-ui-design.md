# 叠卡/延长：兑换示例、预览时间线与订阅展示对齐旧系统（方案 B）

## 背景

当前仓库 `/Volumes/T7/project/new/FluxCode` 已具备订阅叠卡/延长的核心后端能力（`subscription_grants` + `subscription_mode`），但前端与部分后端展示/一致性能力尚未对齐旧系统 `/Volumes/T7/project/FluxCode`：

1. **兑换页缺少“叠卡/延长示例（mode guide）”与“兑换前/后额度时间线预览（timeline）”**，用户难以理解两种兑换方式区别。
2. **我的订阅页 / 订阅管理页无法正确展示叠加后的额度（×N）**：列表接口未按当前时刻动态计算 `quota_multiplier`，导致前端只能显示默认值（通常为 1）。
3. **窗口重置与 grants 用量存在一致性风险**：当前仅重置 `user_subscriptions` 的日/周/月用量窗口，但未同步清零 `subscription_grants` 的用量，可能导致计费资格检查/展示出现误判或不一致。

本方案采用：**对齐旧系统的交互与展示（方案 B）**，在新系统补齐缺失的接口、组件与 i18n 文案，并修复后端列表快照与窗口重置一致性。

## 目标

1. 在兑换页面提供**静态示例**，帮助用户理解：
   - `extend`：延长时长，额度倍数不变
   - `stack`：从现在开始叠加额度，额度倍数 +1（持续本次有效期）
2. 在兑换弹窗中提供**兑换前/后预览**：
   - 兑换前：当前订阅卡片 + 额度时间分段（timeline）
   - 兑换后：结果卡片（到期时间/额度倍数变化）+ 兑换后 timeline
3. 在“我的订阅”页展示叠加倍率（`×quota_multiplier`），并支持点击订阅进入**详情展开**查看 timeline。
4. 在“订阅管理（管理端）”页展示叠加倍率（`×quota_multiplier`），用量/限额按倍率放大显示，便于运营识别“已叠加”的订阅。
5. 确保**计费语义正确**：
   - 叠卡时按多张卡叠加后的额度进行限额判断
   - 消耗按 grants 分摊（优先最早到期）
   - 窗口重置时 `user_subscriptions` 与 `subscription_grants` 的用量同步清零

## 语义对齐（与旧系统一致）

- `extend`：
  - 若存在未过期订阅：延长 **tail grant** 的 `expires_at += validity_days`
  - 聚合订阅 `user_subscriptions.expires_at` 同步更新为新的 tail 过期时间
  - `quota_multiplier` 不变
- `stack`：
  - 若存在未过期订阅：新增一条 grant（`starts_at=now`, `expires_at=now+validity_days`）
  - 聚合订阅 `user_subscriptions.expires_at` 取 max（原到期、grant 到期）
  - 同一时刻 active grants 数量增加 → `quota_multiplier` +1
- 消耗分摊：
  - 以 active grants（按最早到期优先）为单位分摊用量
  - 聚合用量用于展示/资格检查时取 active grants 用量之和

## 后端改动设计

### 1) 订阅列表动态填充 quota 快照（修复“×N 不显示”）

在 `SubscriptionService` 的列表方法中对每条订阅调用 `populateQuotaSnapshot()`：

- `ListUserSubscriptions`
- `ListActiveUserSubscriptions`
- `ListGroupSubscriptions`
- `List`（管理端列表）

使前端拿到的 `quota_multiplier` 与 `daily/weekly/monthly_usage_usd` 反映“当前时刻 active grants”的真实值。

### 2) 窗口重置同步清零 grants 用量（修复一致性风险）

在以下逻辑中，同时重置 `user_subscriptions` 与 `subscription_grants` 的窗口用量：

- `CheckAndResetWindows`（异步维护）
- `AdminResetQuota`（管理端手动重置）

调用 `SubscriptionGrantRepository.ResetDaily/Weekly/MonthlyUsageBySubscriptionID(subscriptionID)`，确保后续计费资格检查与 UI 展示不会继续读取旧 grants 用量。

### 3) 用户 grants 详情接口（用于兑换预览与订阅详情）

新增用户接口：

- `GET /api/v1/subscriptions/:id/grants` → `SubscriptionHandler.GetMyGrants`

返回结构对齐旧系统（`ActiveSubscriptionGrantUsageResponse`）：

- `subscription_id / group_id / group_name`
- `grants[]`：每条 grant 的 `starts_at / expires_at / daily/weekly/monthly_usage_usd`，以及每窗口的使用进度（limit/used/percentage/unlimited）。

接口需校验订阅归属：仅允许订阅所属用户读取。

## 前端改动设计

### 1) 组件迁移

从旧系统迁移到新系统：

- `SubscriptionUsageCard.vue`：用于展示订阅卡片（支持倍率/到期 override）。
- `SubscriptionQuotaTimeline.vue`：用于渲染额度时间分段（hover 提示 + 列表明细）。

### 2) 兑换页（RedeemView）

在兑换页中补齐两块内容：

1. **Redeem Mode Guide（静态示例）**：
   - tab：`stack / extend`
   - `stack` 下包含 `case1 / case2`
   - 彩色段条 + 文案提示，帮助用户理解叠卡/延长的差异
2. **订阅兑换弹窗增强**：
   - 在 `SUBSCRIPTION_REDEEM_CHOICE_REQUIRED` 时弹出二选一弹窗
   - 弹窗中加载当前订阅 + `GET /subscriptions/:id/grants`，渲染兑换前/后卡片与 timeline
   - grants 拉取失败时：保留“基础预览”（到期 + 倍率变化）并提示预览加载失败，但不阻塞提交

### 3) 我的订阅页（SubscriptionsView）

- 列表卡片展示：
  - `×quota_multiplier`（当 >1）
  - 用量上限按倍率放大（`limit * quota_multiplier`）
- 点击某条订阅进入详情视图：
  - 加载 `GET /subscriptions/:id/grants`
  - 渲染 daily/weekly/monthly 的 timeline（仅在对应 limit 配置存在时显示）

### 4) 订阅管理（管理端 SubscriptionsView）

- 列表展示 `×quota_multiplier`（当 >1）
- 用量上限按倍率放大（保持与用户侧一致）
- 暂不引入管理端详情 timeline（除非后续明确需要）

### 5) i18n 与 types/API

- i18n：补齐旧系统中与 mode guide、兑换预览、订阅详情 timeline 相关的 key（`zh.ts` / `en.ts`）。
- types：补齐 grants 详情响应类型（`UserSubscriptionGrantUsageResponse` 等）。
- API：补齐 `subscriptions.getSubscriptionGrants(subscriptionId)`。

## 测试与验收口径

后端（最小集）：

1. `GET /api/v1/subscriptions` 返回的订阅在存在多张 active grants 时 `quota_multiplier>1`。
2. 日/周/月窗口重置后，`subscription_grants` 的对应 usage 也被清零，避免误判超限。
3. `GET /api/v1/subscriptions/:id/grants`：
   - 本人可读
   - 非本人不可读（返回 not found/forbidden 等一致错误）

前端（最小集）：

1. 兑换页渲染 mode guide，并可切换 tab/case。
2. 出现 “订阅兑换方式” 弹窗后能渲染兑换前/后预览（grants 接口成功与失败两种分支）。
3. 我的订阅页列表展示 `×N`，并可展开详情渲染 timeline。

## 风险与降级策略

- grants 详情接口失败：兑换弹窗仍允许提交兑换，仅展示基础预览并提示“预览加载失败”。
- 老数据无 grants：timeline 走 fallback（按 `expires_at + quota_multiplier` 渲染单段），不阻塞主流程。

