# 叠卡/延长：兑换示例、预览时间线与订阅展示 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在新系统对齐旧系统的叠卡/延长交互与展示：兑换页示例 + 兑换弹窗时间线预览 + 我的订阅详情展开，并修复后端 `quota_multiplier` 列表快照与窗口重置一致性，确保叠卡计费语义正确。

**Architecture:** 后端以 `subscription_grants` 为事实源动态计算 `quota_multiplier` 与窗口用量快照；前端通过新增 `GET /subscriptions/:id/grants` 获取 grants 详情渲染 timeline，并在兑换弹窗里做兑换前/后预览（含 fallback）。

**Tech Stack:** Go（gin + ent + testify）、Vue3（script setup + vue-i18n + vitest）、pnpm。

---

## 约束与已知基线

- 运行全量 `make test` 可能因宿主机缺少 `golangci-lint` 失败；后端验证优先使用 `cd backend && go test ./...`。
- 本计划按 **TDD** 执行：每个行为变更先写测试并确认失败，再写最小实现让其通过。

---

### Task 1: 后端列表返回 quota 快照（修复“×N 不展示”）

**Files:**
- Modify: `backend/internal/service/subscription_service.go`
- Test: `backend/internal/service/subscription_list_quota_snapshot_test.go`

**Step 1: 写 failing unit test（ListUserSubscriptions 应填充 quota 快照）**

新增测试：stub `UserSubscriptionRepository.ListByUserID` 返回 1 条订阅；stub `SubscriptionGrantRepository.CountActiveBySubscriptionID/SumActiveUsageBySubscriptionID` 返回 multiplier=2、usage=合计值；断言返回的订阅 `QuotaMultiplier==2` 且用量被覆盖为 grants 汇总值。

Run: `cd backend && go test -tags=unit ./internal/service -run TestSubscriptionService_ListUserSubscriptions_PopulatesQuotaSnapshot -v`
Expected: FAIL（当前 ListUserSubscriptions 未填充快照）。

**Step 2: 最小实现**

在 `ListUserSubscriptions / ListActiveUserSubscriptions / ListGroupSubscriptions / List(管理端)` 内部对每条订阅调用 `populateQuotaSnapshot(ctx, &subs[i], time.Now())`，并保持 `normalizeExpiredWindows/normalizeSubscriptionStatus` 的调用顺序与语义一致。

**Step 3: 重新跑测试**

Run: 同 Step 1
Expected: PASS

---

### Task 2: 窗口重置同步清零 grants 用量（修复计费一致性）

**Files:**
- Modify: `backend/internal/service/subscription_service.go`
- Test: `backend/internal/service/subscription_reset_quota_test.go`
- Test: `backend/internal/service/subscription_check_and_reset_windows_test.go`

**Step 1: 写 failing unit test（AdminResetQuota 需要调用 grantRepo.Reset*）**

在 `subscription_reset_quota_test.go` 增加 stub grantRepo，断言当 resetDaily/resetWeekly/resetMonthly=true 时分别调用 `ResetDaily/Weekly/MonthlyUsageBySubscriptionID(subscriptionID)`。

Run: `cd backend && go test -tags=unit ./internal/service -run TestAdminResetQuota_ResetsGrantUsage -v`
Expected: FAIL（当前 AdminResetQuota 未触发 grants reset）。

**Step 2: 最小实现**

在 `AdminResetQuota` 中：在每个 `userSubRepo.Reset*Usage` 成功后，若 `s.grantRepo != nil` 则调用对应 `s.grantRepo.Reset*UsageBySubscriptionID(ctx, sub.ID)`；失败应直接返回错误并中止后续 reset（与现有错误处理保持一致）。

**Step 3: 写 failing unit test（CheckAndResetWindows 需要调用 grantRepo.Reset*）**

新增 `subscription_check_and_reset_windows_test.go`：

- 构造 `UserSubscription{DailyWindowStart: now-25h, DailyUsageUSD: ...}` 触发 `NeedsDailyReset()==true`
- stub userSubRepo.ResetDailyUsage 与 grantRepo.ResetDailyUsageBySubscriptionID
- 调用 `svc.CheckAndResetWindows(ctx, sub)` 后断言两个 reset 都被调用

Run: `cd backend && go test -tags=unit ./internal/service -run TestSubscriptionService_CheckAndResetWindows_ResetsGrantUsage -v`
Expected: FAIL（当前 CheckAndResetWindows 未触发 grants reset）。

**Step 4: 最小实现**

在 `CheckAndResetWindows` 的 daily/weekly/monthly 分支中，在重置 `user_subscriptions` 后同步重置 grants（同旧系统语义）。

**Step 5: 重新跑 unit tests**

Run: `cd backend && go test -tags=unit ./internal/service -v`
Expected: PASS

---

### Task 3: 新增用户 grants 预览接口（GET /subscriptions/:id/grants）

**Files:**
- Modify: `backend/internal/server/routes/user.go`
- Modify: `backend/internal/handler/subscription_handler.go`
- Create/Modify: `backend/internal/handler/dto/types.go`
- Create/Modify: `backend/internal/handler/dto/mappers.go`
- Modify/Create: `backend/internal/service/subscription_service.go`（或拆新文件）
- Test: `backend/internal/server/routes/user_subscription_grants_test.go`（建议新增）

**Step 1: 写 failing route test**

新增路由测试：未登录返回 401；登录后访问自身订阅 id 返回 200；访问非自身订阅返回 404/403（按当前错误约定）。

Run: `cd backend && go test ./internal/server/routes -run Subscription.*Grants -v`
Expected: FAIL（路由/handler/service 还不存在）。

**Step 2: 最小实现（路由 + handler）**

- `routes/user.go`：在 `subscriptions` 组内新增 `GET("/:id/grants", h.Subscription.GetMyGrants)`
- `SubscriptionHandler`：新增 `GetMyGrants`（parse id + auth subject userID + 调 service）

**Step 3: 最小实现（service + dto）**

service 新增：

- `ActiveSubscriptionGrantUsageResponse` / `SubscriptionGrantUsageDetail` / `SubscriptionGrantUsageWindow`
- `GetUserGrantUsageDetails(ctx, userID, subscriptionID)`：
  - 校验订阅属主
  - `grantRepo.ListUnexpiredBySubscriptionID(subID, now)`（包含未来排期）
  - 组合并返回 grants + 每窗口进度

dto 新增：

- `ActiveSubscriptionGrantUsageResponseFromService`

**Step 4: 重新跑 route tests**

Expected: PASS

---

### Task 4: 迁移前端组件 + 补 types/API/i18n

**Files:**
- Create: `frontend/src/components/common/SubscriptionUsageCard.vue`
- Create: `frontend/src/components/common/SubscriptionQuotaTimeline.vue`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/subscriptions.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/components/common/__tests__/SubscriptionQuotaTimeline.spec.ts`（可选）

**Step 1: 写 failing typecheck（先不实现组件，先补 types）**

先在 `types/index.ts` 增加 grants 详情 response 类型（对齐后端 JSON），并在 `api/subscriptions.ts` 暴露 `getSubscriptionGrants` 方法签名。

Run: `pnpm --dir frontend run typecheck`
Expected: FAIL（还未实现 API/或组件引用报错）。

**Step 2: 最小实现（API + types + i18n）**

- `api/subscriptions.ts`：实现 `getSubscriptionGrants(id)` → `GET /subscriptions/${id}/grants`
- i18n：迁移旧系统所需 key（mode guide、choice dialog、userSubscriptions timeline/detail）

**Step 3: 迁移组件**

直接从旧系统迁移两组件到新系统目录，按新系统工具函数/prop 命名做最小适配（日期格式化可在组件内部用 `formatDateTime` + options 生成）。

**Step 4: 重新跑 typecheck**

Expected: PASS

---

### Task 5: 兑换页（RedeemView）搬运示例 + 弹窗时间线预览

**Files:**
- Modify: `frontend/src/views/user/RedeemView.vue`
- Test: `frontend/src/views/user/__tests__/RedeemView.spec.ts`

**Step 1: 写 failing view test**

新增测试覆盖：

- 页面渲染 mode guide（包含 `redeem.modeGuideTitle`）
- 当 redeem API 返回 `SUBSCRIPTION_REDEEM_CHOICE_REQUIRED` 时弹窗出现
- 弹窗内切换 `stack/extend` 时，预览区域（到期/倍率/stack until）更新

Run: `pnpm --dir frontend run test:run frontend/src/views/user/__tests__/RedeemView.spec.ts`
Expected: FAIL（当前无 mode guide/无预览组件）。

**Step 2: 最小实现**

将旧系统 RedeemView 的以下逻辑迁入新系统：

- mode guide UI 与计算逻辑
- choice dialog：before/after `SubscriptionUsageCard` + `SubscriptionQuotaTimeline`
- 异步加载订阅与 grants（grants 失败走 fallback）

**Step 3: 重新跑测试**

Expected: PASS

---

### Task 6: 我的订阅页展示 ×N + 点击展开详情 timeline

**Files:**
- Modify: `frontend/src/views/user/SubscriptionsView.vue`
- Test: `frontend/src/views/user/__tests__/SubscriptionsView.spec.ts`

**Step 1: 写 failing view test**

- 列表卡片在 `quota_multiplier=2` 时显示 `×2`
- 点击订阅卡片进入详情，并触发 `getSubscriptionGrants`，渲染 timeline 标题（例如 `userSubscriptions.timeline.title`）

Run: `pnpm --dir frontend run test:run frontend/src/views/user/__tests__/SubscriptionsView.spec.ts`
Expected: FAIL（当前没有详情视图、也未显示倍率）。

**Step 2: 最小实现**

把旧系统的 SubscriptionsView 详情展开逻辑搬过来，并在列表上补倍率显示与“有效限额 = baseLimit * quota_multiplier”展示逻辑。

**Step 3: 重新跑测试**

Expected: PASS

---

## 验收（建议顺序）

1. 后端：`cd backend && go test ./...`（以及 unit tests）
2. 前端：`pnpm --dir frontend run lint:check && pnpm --dir frontend run typecheck`
3. 前端：`pnpm --dir frontend run test:run`
4. 手工：进入兑换页，输入订阅兑换码触发 choice required，验证 preview timeline 与提交后订阅倍率/到期变化

