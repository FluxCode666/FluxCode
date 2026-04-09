# 旧系统功能与数据迁移 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将旧系统中的叠卡订阅能力、相关 dashboard/多语言收口，以及保留业务域的数据迁移能力完整补入当前系统，并保留历史业务与统计结果。

**Architecture:** 先补齐叠卡底层 schema、repository、service 与计费归因链，再收口前端兑换/订阅/dashboard/i18n，最后落离线双库迁移器与对账流程。所有行为以旧系统为参考源，但接线、路由和现有半成品实现以当前仓库为基线增量收口。

**Tech Stack:** Go, Ent, PostgreSQL migrations, Gin, Wire, Vue 3, TypeScript, pnpm/vitest

---

### Task 1: 叠卡 schema、port 与 repository 基础

**Files:**
- Create: `backend/ent/schema/subscription_grant.go`
- Create: `backend/internal/service/subscription_grant_port.go`
- Create: `backend/internal/repository/subscription_grant_repo.go`
- Create: `backend/internal/repository/subscription_grant_repo_integration_test.go`
- Modify: `backend/ent/schema/user_subscription.go`
- Modify: `backend/ent/schema/redeem_code.go`
- Modify: `backend/internal/repository/redeem_code_repo.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/migrations/migrations.go`
- Create: `backend/migrations/081_subscription_grants_stack_mode.sql`

**Step 1: Write the failing tests**

- 新增 `backend/internal/repository/subscription_grant_repo_integration_test.go`
- 覆盖以下最小行为：
  - 创建 grant 后可按 subscription 查询活动 grant
  - `GetTailGrantBySubscriptionID()` 返回最大 `expires_at`
  - `AllocateUsageToActiveGrants()` 按最早到期优先分配 usage
  - `redeemRepo.Use()` 可记录 `subscription_mode`
- 补一个 migration schema 断言，验证 `subscription_grants` 表和 `redeem_codes.subscription_mode` 字段存在

**Step 2: Run tests to verify they fail**

Run:

```bash
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestSubscriptionGrantRepository_|TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate' -count=1
```

Expected:

- FAIL，提示缺少 `subscription_grants` schema / repository / `subscription_mode`

**Step 3: Write minimal implementation**

- 新增 `subscription_grants` ent schema 与 service port
- 在 `user_subscription` schema 上增加 `grants` edge
- 在 `redeem_code` schema 上增加 `subscription_mode` 可选字段
- 实现 grant repository 的最小能力：
  - `Create`
  - `ListActiveBySubscriptionID`
  - `ListUnexpiredBySubscriptionID`
  - `ListActiveForUpdate`
  - `GetTailGrantBySubscriptionID`
  - `CountActiveBySubscriptionID`
  - `SumActiveUsageBySubscriptionID`
  - `MinActiveExpiresAtBySubscriptionID`
  - `UpdateExpiresAt`
  - `AllocateUsageToActiveGrants`
  - `ResetDailyUsageBySubscriptionID`
  - `ResetWeeklyUsageBySubscriptionID`
  - `ResetMonthlyUsageBySubscriptionID`
- 新增 `081_subscription_grants_stack_mode.sql`
  - 创建 `subscription_grants`
  - 增加 grant usage 字段
  - 为现有 `user_subscriptions` 回填单 grant
  - 为 `redeem_codes` 增加 `subscription_mode`
- 更新 `redeem_code_repo.Use()` 以支持写入 `subscription_mode`
- 更新 wire provider

**Step 4: Run tests to verify they pass**

Run:

```bash
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestSubscriptionGrantRepository_|TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate' -count=1
go test ./internal/repository -run 'TestRedeemCodeRepository|TestBuildRedisOptions' -count=1
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add backend/ent/schema/subscription_grant.go backend/ent/schema/user_subscription.go backend/ent/schema/redeem_code.go backend/internal/service/subscription_grant_port.go backend/internal/repository/subscription_grant_repo.go backend/internal/repository/subscription_grant_repo_integration_test.go backend/internal/repository/redeem_code_repo.go backend/internal/repository/wire.go backend/migrations/081_subscription_grants_stack_mode.sql backend/migrations/migrations.go
git commit -m "feat: add subscription grant foundation"
```

### Task 2: 订阅服务、兑换服务与后台接口补齐叠卡语义

**Files:**
- Modify: `backend/internal/service/subscription_service.go`
- Modify: `backend/internal/service/redeem_service.go`
- Modify: `backend/internal/service/user_subscription.go`
- Modify: `backend/internal/service/user_subscription_port.go`
- Modify: `backend/internal/handler/redeem_handler.go`
- Modify: `backend/internal/handler/admin/subscription_handler.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Create: `backend/internal/repository/subscription_service_grants_integration_test.go`
- Create: `backend/internal/repository/redeem_subscription_choice_integration_test.go`

**Step 1: Write the failing tests**

- 从旧系统迁移并裁剪以下集成测试：
  - `AssignSubscriptionWithMode` 无旧订阅时创建初始 grant
  - 活跃订阅未指定 mode 时返回 choice required
  - `extend` 延长尾 grant
  - `stack` 创建新 grant 并提升 `QuotaMultiplier`
  - suspended / expired 的分支行为
  - 兑换码在订阅场景下可记录与校验 `subscription_mode`

**Step 2: Run tests to verify they fail**

Run:

```bash
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestSubscriptionServiceGrantsSuite|TestRedeemSubscriptionChoice' -count=1
```

Expected:

- FAIL，提示缺少 `AssignSubscriptionWithMode` / `ApplyRedeemSubscription` / `QuotaMultiplier` / `subscription_mode`

**Step 3: Write minimal implementation**

- 给 `SubscriptionService` 注入 `grantRepo`
- 新增：
  - `SubscriptionModeExtend`
  - `SubscriptionModeStack`
  - `ApplyRedeemSubscriptionInput`
  - `AssignSubscriptionWithModeInput`
  - `AssignSubscriptionWithMode()`
  - `ApplyRedeemSubscription()`
  - `populateQuotaSnapshot()`
- 给 `UserSubscription` 增加 `QuotaMultiplier`
- 将 `RedeemService.Redeem()` 改成接受 `subscriptionMode`
- 订阅兑换与后台分配统一走 extend/stack 语义
- 更新 DTO / mapper / handler 请求体与响应字段

**Step 4: Run tests to verify they pass**

Run:

```bash
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestSubscriptionServiceGrantsSuite|TestRedeemSubscriptionChoice' -count=1
go test ./internal/service ./internal/handler/admin ./internal/handler -run 'TestSubscription|TestRedeem' -count=1
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add backend/internal/service/subscription_service.go backend/internal/service/redeem_service.go backend/internal/service/user_subscription.go backend/internal/service/user_subscription_port.go backend/internal/handler/redeem_handler.go backend/internal/handler/admin/subscription_handler.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/repository/subscription_service_grants_integration_test.go backend/internal/repository/redeem_subscription_choice_integration_test.go
git commit -m "feat: add stacked subscription service flow"
```

### Task 3: 计费缓存、网关 usage 归因与 grant 级扣费

**Files:**
- Modify: `backend/internal/service/billing_cache_service.go`
- Modify: `backend/internal/service/billing_cache_port.go`
- Modify: `backend/internal/service/gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/usage_queue_service.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Create: `backend/internal/repository/gateway_record_usage_grants_integration_test.go`
- Create: `backend/internal/service/billing_cache_service_quota_multiplier_test.go`

**Step 1: Write the failing tests**

- 覆盖：
  - billing cache 预热能返回 `QuotaMultiplier`
  - grant 到期后有效 usage 会下降
  - gateway / openai gateway 使用 `billedAt` 对活动 grant 分配 usage
  - grant allocation 按最早到期优先

**Step 2: Run tests to verify they fail**

Run:

```bash
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestGatewayRecordUsageGrants' -count=1
go test ./internal/service -run 'TestBillingCacheService_CheckBillingEligibility_SubscriptionQuotaMultiplier' -count=1
```

Expected:

- FAIL，提示 grant repository 未注入或 usage 仍只写 `user_subscriptions`

**Step 3: Write minimal implementation**

- 给 billing cache 注入 grant snapshot 计算
- 给 gateway / openai gateway / usage queue 改成按 grant 分配 usage
- 保留 `user_subscriptions` 聚合字段，但由 grant usage 派生或同步
- 重生成 wire

**Step 4: Run tests to verify they pass**

Run:

```bash
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestGatewayRecordUsageGrants' -count=1
go test ./internal/service -run 'TestBillingCacheService_CheckBillingEligibility_SubscriptionQuotaMultiplier|TestOpenAIGatewayService_RecordProxyUsageMetric' -count=1
go test ./cmd/server/... -count=1
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add backend/internal/service/billing_cache_service.go backend/internal/service/billing_cache_port.go backend/internal/service/gateway_service.go backend/internal/service/openai_gateway_service.go backend/internal/service/usage_queue_service.go backend/internal/service/wire.go backend/cmd/server/wire_gen.go backend/internal/repository/gateway_record_usage_grants_integration_test.go backend/internal/service/billing_cache_service_quota_multiplier_test.go
git commit -m "feat: allocate subscription usage by grants"
```

### Task 4: 前端兑换页、订阅页、dashboard 与多语言收口

**Files:**
- Modify: `frontend/src/views/user/RedeemView.vue`
- Modify: `frontend/src/views/admin/SubscriptionsView.vue`
- Modify: `frontend/src/views/admin/DashboardView.vue`
- Modify: `frontend/src/api/admin/dashboard.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Create: `frontend/src/views/user/__tests__/RedeemView.spec.ts`
- Create: `frontend/src/views/admin/__tests__/SubscriptionsView.spec.ts`
- Modify: `frontend/src/views/admin/__tests__/DashboardView.spec.ts`
- Modify: `frontend/src/i18n/__tests__/migrationLocales.spec.ts`

**Step 1: Write the failing tests**

- 兑换页测试：
  - 已有订阅时出现 extend/stack 选择
  - 显示叠加后的有效期预览
- 管理端订阅页测试：
  - 分配订阅时可选择 extend/stack
- dashboard / i18n 测试：
  - 代理使用统计与相关 key 完整存在
  - 调度状态、号池监控、叠卡文案不再出现 raw key

**Step 2: Run tests to verify they fail**

Run:

```bash
corepack pnpm --dir frontend exec vitest --run src/views/user/__tests__/RedeemView.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts src/views/admin/__tests__/DashboardView.spec.ts src/i18n/__tests__/migrationLocales.spec.ts
```

Expected:

- FAIL，提示缺少 `subscription_mode` UI、预览文案或 i18n key

**Step 3: Write minimal implementation**

- 对齐旧系统兑换页的 extend/stack 交互与预览
- 对齐管理端订阅分配页的 extend/stack 交互
- 补齐 `frontend/src/types/index.ts` 中相关类型
- 补齐 `zh.ts` / `en.ts` 文案
- 保持当前仓库 dashboard 结构，仅补缺口

**Step 4: Run tests to verify they pass**

Run:

```bash
corepack pnpm --dir frontend exec vitest --run src/views/user/__tests__/RedeemView.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts src/views/admin/__tests__/DashboardView.spec.ts src/i18n/__tests__/migrationLocales.spec.ts
corepack pnpm --dir frontend run typecheck
corepack pnpm --dir frontend run build
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add frontend/src/views/user/RedeemView.vue frontend/src/views/admin/SubscriptionsView.vue frontend/src/views/admin/DashboardView.vue frontend/src/api/admin/dashboard.ts frontend/src/types/index.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/views/user/__tests__/RedeemView.spec.ts frontend/src/views/admin/__tests__/SubscriptionsView.spec.ts frontend/src/views/admin/__tests__/DashboardView.spec.ts frontend/src/i18n/__tests__/migrationLocales.spec.ts
git commit -m "feat: add stacked subscription frontend flows"
```

### Task 5: 离线双库迁移器与对账报告

**Files:**
- Create: `backend/cmd/data-migrator/main.go`
- Create: `backend/internal/service/data_migration_service.go`
- Create: `backend/internal/service/data_migration_service_test.go`
- Create: `backend/internal/repository/data_migration_repo.go`
- Create: `backend/internal/repository/data_migration_repo_integration_test.go`
- Modify: `docs/plans/2026-03-26-old-system-feature-data-migration-progress.md`

**Step 1: Write the failing tests**

- 覆盖：
  - `dry-run` 能输出阶段计划但不写目标库
  - `apply` 能按阶段导入共享业务表
  - grant 数据迁移后与 `user_subscriptions` / `redeem_codes` 一致
  - 对账报告能输出行数与关键汇总差异

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/service -run 'TestDataMigrationService_' -count=1
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1
```

Expected:

- FAIL，提示迁移器与对账实现缺失

**Step 3: Write minimal implementation**

- 实现离线双库迁移器 CLI：
  - `source_dsn`
  - `target_dsn`
  - `mode=dry-run|apply`
  - `phase`
  - `reset_target`
  - `report_file`
- 实现按表族迁移的 repository / service
- 实现阶段性对账报告
- 在进度台账中记录正式迁移演练与结果

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/service -run 'TestDataMigrationService_' -count=1
GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add backend/cmd/data-migrator/main.go backend/internal/service/data_migration_service.go backend/internal/service/data_migration_service_test.go backend/internal/repository/data_migration_repo.go backend/internal/repository/data_migration_repo_integration_test.go docs/plans/2026-03-26-old-system-feature-data-migration-progress.md
git commit -m "feat: add old system data migrator"
```
