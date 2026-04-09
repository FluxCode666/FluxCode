# 旧系统功能与数据迁移执行记录

## 当前状态

当前处于“迁移器已具备可安全重复执行的最小闭环，并已开始扩充 phase 覆盖面；下一步进入真实 DSN 演练 + 切换/回滚验收清单收口”的阶段。

### 已完成

1. 迁移范围与边界确认
   - 明确迁移目标包含：定价、号池监控、叠卡、相关 dashboard、多语言与相关数据
   - 明确不迁移：独立工具、补丁类功能及其专属数据

2. 迁移策略确认
   - 切换方式确定为“可重复预演后切换”
   - 数据策略确定为“旧系统为准整体接管”
   - 历史范围确定为“业务数据 + 历史统计全保留”
   - 实现形态确定为“离线双库迁移器”

3. 现状调研完成
   - 已确认当前仓库定价与号池监控存在部分基础能力和半成品改动
   - 已确认旧系统存在完整叠卡后端、前端和 grant 级消耗逻辑
   - 已确认当前仓库尚未完整承接 `subscription_grants` 与 `subscription_mode`
   - 已确认当前系统已有账号/代理导入能力不足以覆盖本次全量业务迁移

4. 技术方案文档已保存
   - 新增 `docs/plans/2026-03-26-old-system-feature-data-migration-design.md`

5. 执行台账已初始化
   - 新增本文件，作为后续每一步实施的统一记录入口

6. 文档跟踪规则已收口
   - 更新 `.gitignore`，放行 `docs/plans/*.md`
   - 已确认本次新增设计文档与进度台账可被 Git 正常识别

7. 详细实施计划文档已保存
   - 新增 `docs/plans/2026-03-26-old-system-feature-data-migration.md`
   - 后续实现统一以该计划为执行基线

8. Task 1 红灯测试已补齐并验证失败原因正确
   - 新增 `backend/internal/repository/subscription_grant_repo_integration_test.go`
   - 扩展 `backend/internal/repository/migrations_schema_integration_test.go`
   - 已确认当前缺口集中在 `subscription_grants` repository、`redeem_codes.subscription_mode` 写入链路与相关生成代码

9. Task 1 差异复核已完成
   - 已对照旧系统实现、当前仓库状态和实施计划逐项确认最小闭环范围
   - 当前仅推进 Task 1 所需的 schema / migration / repository / wire，不提前扩散到 Task 2/3

10. Task 1 叠卡 schema、port 与 repository 基础已完成
   - 新增 `backend/internal/repository/subscription_grant_repo.go`
   - 新增 `backend/migrations/081_subscription_grants_stack_mode.sql`
   - 已补齐 `redeemRepo.Use()` 的 `subscriptionMode` 写入链路，并同步更新 `auth/redeem` 调用方与测试桩
   - 已补齐 `redeem_code` repository 对 `subscription_mode` 的 create/update/read 映射
   - 已完成 `go generate ./ent`
   - 验证通过：
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestSubscriptionGrantRepoSuite|TestRedeemCodeRepository_UseRecordsSubscriptionMode|TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test ./internal/repository -run 'TestRedeemCodeRepoSuite|TestBuildRedisOptions' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test ./internal/service -run 'TestRedeem|TestAuth' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=unit ./internal/server -run TestDoesNotExist -count=1`

11. Task 2 订阅服务、兑换服务与后台接口补齐叠卡语义已完成
   - 新增 `backend/internal/repository/subscription_service_grants_integration_test.go`
   - 新增 `backend/internal/repository/redeem_subscription_choice_integration_test.go`
   - 已补齐 `SubscriptionService` 的 `AssignSubscriptionWithMode`、`ApplyRedeemSubscription`、初始 grant 创建、`QuotaMultiplier` 快照与活动 grant 用量聚合
   - 已补齐 `RedeemService.Redeem(..., subscriptionMode)`，支持订阅兑换 `choice required`、`extend`、`stack` 与 `subscription_mode` 落库
   - 已补齐用户兑换接口、管理员创建并兑换接口、管理员单个分配订阅接口对 `subscription_mode` 的透传
   - 已补齐 DTO / mapper 中的 `quota_multiplier` 与 `subscription_mode` 输出
   - 已重生成 `backend/cmd/server/wire_gen.go`
   - 验证通过：
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestSubscriptionServiceGrantsSuite|TestRedeemSubscriptionChoiceSuite' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test ./internal/service -run 'TestAssignSubscription|TestAdminResetQuota|TestRedeem|TestAuth' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test ./internal/handler/admin ./internal/handler -run 'TestSubscription|TestRedeem' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=unit ./internal/server -run TestDoesNotExist -count=1`

12. Task 3 差异复核与最小测试面确认已完成
   - 已对照旧系统 `billing_cache_service.go`、`gateway_service.go`、`openai_gateway_service.go` 与当前仓库实现逐项确认差异
   - 已确认当前仓库与旧系统的关键结构差异：
     - 当前仓库 `BillingCacheService` 仍未承接 `grantRepo`，订阅缓存缺少 `quota_multiplier` 与 `next_grant_expires_at`
     - 当前仓库 `GatewayService` / `OpenAIGatewayService` 已改为通过 `usageBillingRepo` 做幂等扣费，不再沿用旧系统“usage_log + 扣费同事务”的原路径
     - 旧系统的 `usage_queue_service.go` / `BilledAt` 归属时刻逻辑在当前仓库未完整保留，Task 3 需要按现有 `usageBilling` 架构增量承接，而不是原样硬搬
   - 已确认 Task 3 的最小红灯测试入口调整为：
     - `billing cache` 返回 `QuotaMultiplier` 并按 multiplier 放大订阅限额
     - `usageBilling` 链路驱动的 gateway / openai gateway 订阅记账能够把 usage 分摊到 active grants，并保持 `user_subscriptions` 聚合字段兼容更新
     - grant usage 仍需满足“最早到期优先”与幂等不重复扣费

13. Task 3 红灯测试已补齐并验证失败点准确
   - 新增 `backend/internal/service/billing_cache_service_quota_multiplier_test.go`
   - 新增 `backend/internal/repository/gateway_record_usage_grants_integration_test.go`
   - 已验证 `billing cache` 红灯失败点集中在：
     - `SubscriptionCacheData` / `subscriptionCacheData` 尚未承接 `QuotaMultiplier`、`NextGrantExpiresAt`
     - `NewBillingCacheService(...)` 仍未接入 `grantRepo`
   - 已验证 `gateway/openai gateway` 红灯失败点集中在：
     - `RecordUsageInput` / `OpenAIRecordUsageInput` 尚未承接 `BilledAt`
     - `BillingCacheService` 仍未接入 `grantRepo`
     - 当前集成链路尚未具备 grant 级 usage 分摊入口
   - 红灯验证命令：
     - `cd backend && /opt/homebrew/bin/go test -tags=unit ./internal/service -run 'TestBillingCacheService_CheckBillingEligibility_SubscriptionQuotaMultiplier|TestBillingCacheService_GetSubscriptionStatus_CacheMissLoadsGrantSnapshot' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestGatewayRecordUsageGrantsSuite' -count=1`

14. Task 3 计费缓存、网关 usage 归因与 grant 级扣费已完成
   - 已补齐 `billing cache` 对 grant 快照的承接：
     - `backend/internal/service/billing_cache_port.go`
     - `backend/internal/service/billing_cache_service.go`
     - `backend/internal/repository/billing_cache.go`
   - 已补齐 `usageBilling` 原子扣费链路对 grant usage 分摊的承接：
     - `backend/internal/service/usage_billing.go`
     - `backend/internal/repository/usage_billing_repo.go`
   - 已补齐 gateway / openai gateway 的 `BilledAt` 归属时刻透传与订阅 usage 归因：
     - `backend/internal/service/gateway_service.go`
     - `backend/internal/service/openai_gateway_service.go`
   - 已同步更新 `NewBillingCacheService(...)` 的调用点与生成代码：
     - `backend/cmd/server/wire_gen.go`
     - `backend/cmd/server/wire_gen_test.go`
     - `backend/internal/handler/gateway_handler_warmup_intercept_unit_test.go`
     - `backend/internal/handler/sora_gateway_handler_test.go`
     - `backend/internal/service/billing_cache_service_test.go`
     - `backend/internal/service/billing_cache_service_singleflight_test.go`
   - 已确认当前行为：
     - `billing cache` 可返回 `quota_multiplier` 与 `next_grant_expires_at`
     - 订阅限额检查按 multiplier 放大
     - `usageBillingRepo` 在幂等扣费时同步更新 `subscription_grants` 与 `user_subscriptions`
     - gateway / openai gateway 的订阅 usage 按最早到期 grant 优先分摊，重复请求不会重复扣费
   - 已重生成：
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go generate ./cmd/server`
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test -tags=unit ./internal/service -run 'TestBillingCacheService_CheckBillingEligibility_SubscriptionQuotaMultiplier|TestBillingCacheService_GetSubscriptionStatus_CacheMissLoadsGrantSnapshot' -count=1`
     - `cd backend && /opt/homebrew/bin/go test -tags=unit ./internal/service -run 'TestBillingCacheService|TestGatewayServiceRecordUsage|TestOpenAIGatewayServiceRecordUsage' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestGatewayRecordUsageGrantsSuite|TestUsageBillingRepositoryApply_DeduplicatesSubscriptionBilling' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test ./cmd/server/... -count=1`

15. Task 4 前端叠卡兑换 / 分配选择链路与多语言最小闭环已完成
   - 已新增前端红灯测试：
     - `frontend/src/views/user/__tests__/RedeemView.spec.ts`
     - `frontend/src/views/admin/__tests__/SubscriptionsView.spec.ts`
     - 扩展 `frontend/src/i18n/__tests__/migrationLocales.spec.ts`
   - 已补齐前端最小实现：
     - `frontend/src/api/client.ts` 现可透传后端错误的 `reason` / `metadata`
     - `frontend/src/types/index.ts` 已补 `subscription_mode`、`quota_multiplier` 与 API 响应错误元数据类型
     - `frontend/src/api/redeem.ts` 已支持 `redeem(code, subscriptionMode?)`，并补齐兑换历史的 `subscription_mode`
     - `frontend/src/views/user/RedeemView.vue` 已补订阅兑换 choice required 弹窗、extend/stack 二次提交与最小预览
     - `frontend/src/views/admin/SubscriptionsView.vue` 已补管理员分配订阅 choice required 弹窗、二次按 mode 分配与 `quota_multiplier` 限额展示
     - `frontend/src/i18n/locales/zh.ts` / `frontend/src/i18n/locales/en.ts` 已补叠卡兑换与分配相关文案
   - 红绿验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend exec vitest --run src/views/user/__tests__/RedeemView.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts src/i18n/__tests__/migrationLocales.spec.ts`

16. Task 4 前端叠卡 / dashboard / 多语言收口已完成
   - 额外验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend run typecheck`
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend run build`
   - 结果确认：
     - Task 4 定向测试、类型检查、生产构建均已通过
     - `vite build` 仍有既有的大 chunk 警告，但未阻塞本次迁移收口

17. Task 5 红灯测试已补齐并验证失败点准确
   - 已新增：
     - `backend/internal/service/data_migration_service_test.go`
     - `backend/internal/repository/data_migration_repo_integration_test.go`
   - 已确认当前缺口集中在：
     - `DataMigrationService` / `DataMigrationReport` / `DataMigrationRunRequest` 等服务层类型与执行入口缺失
     - `DataMigrationRepository` 与 `NewDataMigrationRepository()` 缺失
     - `backend/cmd/data-migrator/main.go` 尚不存在
   - 红灯验证命令：
     - `cd backend && /opt/homebrew/bin/go test ./internal/service -run 'TestDataMigrationService_' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1`

18. Task 5 离线双库迁移器与 JSON 对账报告最小闭环已完成
   - 已新增：
     - `backend/internal/service/data_migration_service.go`
     - `backend/internal/repository/data_migration_repo.go`
     - `backend/cmd/data-migrator/main.go`
   - 当前实现能力：
     - 支持 `mode=dry-run|apply`
     - 支持 `phase=all|core|subscriptions|pricing|pool_monitor`
     - 支持 `source_dsn` / `target_dsn` / `reset_target` / `report_file`
     - `dry-run` 可输出各 phase 的源库/目标库行数差异
     - `apply` 可按 phase 搬运表数据，并输出 JSON 报告
     - 当前最小 phase 表族已覆盖：
       - `core`: `groups`、`users`
       - `subscriptions`: `user_subscriptions`、`subscription_grants`、`redeem_codes`
       - `pricing`: `pricing_plan_groups`、`pricing_plans`
       - `pool_monitor`: `account_pool_alert_configs`、`proxy_usage_metrics_hourly`
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test ./internal/service -run 'TestDataMigrationService_' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1`
     - `cd backend && /opt/homebrew/bin/go test ./cmd/data-migrator -count=1`

19. 修复 Admin Dashboard 在代理用量接口异常时整页空白的问题
   - 问题表现：访问 `/admin/dashboard` 时，若 `/api/v1/admin/dashboard/proxies-usage` 返回 404，会导致整页不渲染（`stats` 不落地，模板 `v-else-if="stats"` 不成立）
   - 根因：`loadDashboardSnapshot()` 里使用 `Promise.all([getSnapshotV2, getProxyUsageSummary])`，任何一个请求失败都会进入 catch，从而导致 `stats` 为空
   - 修复：
     - 改为 `Promise.allSettled`，把 `proxies-usage` 作为“可降级”依赖；失败时仅降级为空数据，不再阻塞 dashboard 主渲染
     - 补齐无 `stats` 时的错误占位（避免空白）
     - 增加用例覆盖 `getProxyUsageSummary` 失败仍可渲染的场景
   - 关键文件：
     - `frontend/src/views/admin/DashboardView.vue`
     - `frontend/src/views/admin/__tests__/DashboardView.spec.ts`
   - 验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend exec vitest --run src/views/admin/__tests__/DashboardView.spec.ts`

20. 补齐账号列表筛选区的多语言（平台/账号类型）
   - 背景：用户反馈“调度状态下拉等多个地方多语言未设置”，排查发现账号列表筛选区存在硬编码英文标签（平台、账号类型）
   - 修复：把平台与账号类型下拉的硬编码文案替换为 i18n key
   - 关键文件：
     - `frontend/src/components/admin/account/AccountTableFilters.vue`
   - 验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend run typecheck`

21. 修复迁移器 `reset_target` 被忽略导致无条件 TRUNCATE 的高风险问题
   - 问题：`apply` 模式下即使 `reset_target=false` 也会对目标表执行 `TRUNCATE ... RESTART IDENTITY CASCADE`，存在误清库风险
   - 修复：
     - `reset_target=true` 时才 TRUNCATE
     - `reset_target=false` 时改为 `INSERT ... ON CONFLICT DO NOTHING`，尽量做到可重复执行且不破坏目标库既有数据
     - 增加集成用例覆盖“非 reset 不应清空目标表”的行为
   - 关键文件：
     - `backend/internal/repository/data_migration_repo.go`
     - `backend/internal/repository/data_migration_repo_integration_test.go`
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test ./internal/service -run 'TestDataMigrationService_' -count=1`
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1`

22. 前端增加 preinstall 防呆：阻止 npm 安装导致的诡异错误
   - 背景：用户反馈 `npm i` 报 `Cannot read properties of null (reading 'matches')`，且本项目前端约定必须使用 pnpm
   - 修复：在 `frontend/package.json` 增加 `preinstall`，检测非 pnpm 安装直接退出并提示正确命令
   - 关键文件：
     - `frontend/package.json`
   - 验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend run -s preinstall`

23. 增强迁移器健壮性：列交集插入 + 自动回填 serial/identity 序列
   - 背景：
     - 新旧系统可能存在“目标表新增列”的情况；如果直接按目标列全量插入，源库缺失列会被写入 NULL，可能破坏默认值或触发 NOT NULL 约束
     - TRUNCATE RESTART IDENTITY 后再插入显式 id，序列可能停留在 1，导致后续业务写入出现主键冲突
   - 修复：
     - apply 时改为“源列 ∩ 目标列”的交集列插入，目标新增列交由默认值/NULL 处理
     - 每表导入后尝试用 `pg_get_serial_sequence + setval(MAX(id))` 回填 `id` 序列，避免后续自增冲突
     - 增加集成用例覆盖序列回填行为
   - 关键文件：
     - `backend/internal/repository/data_migration_repo.go`
     - `backend/internal/repository/data_migration_repo_integration_test.go`
   - 验证通过：
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1`

24. 扩充迁移器 phase 覆盖面：纳入旧系统核心业务表族（为“全量业务数据迁移”铺路）
   - 背景：用户要求旧系统数据迁移，原先 phase 仅覆盖少量表（groups/users/subscriptions/pricing/pool_monitor）
   - 调整：扩充 phase 与表族映射（仍保持按依赖顺序分阶段）
     - `core`: `groups`、`users`、`settings`
     - `access_control`: `user_allowed_groups`、`user_attribute_definitions`、`user_attribute_values`
     - `infrastructure`: `proxies`、`accounts`、`account_groups`
     - `api_keys`: `api_keys`
     - `usage`: `usage_logs`
     - 保留原有：`subscriptions`、`pricing`、`pool_monitor`
   - 关键文件：
     - `backend/internal/repository/data_migration_repo.go`
     - `backend/internal/repository/data_migration_repo_integration_test.go`
   - 验证通过：
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1`

25. 同步更新 data-migrator CLI 的 phase 帮助文案
   - 背景：phase 扩充后，CLI `--phase` 的 help 仍停留在旧列表，容易误导
   - 修复：更新 `--phase` 说明为最新可用 phase 集合
   - 关键文件：
     - `backend/cmd/data-migrator/main.go`
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test ./cmd/data-migrator -count=1`

26. 优化迁移器写入性能：批量插入（jsonb_populate_recordset）替代逐行 INSERT
   - 背景：`usage_logs` 等大表逐行 `INSERT` 会产生大量 round trip，迁移时间不可控
   - 修复：
     - apply 阶段改为按批次组装 JSON 数组，使用 `jsonb_populate_recordset` 一次插入多行
     - 保持 `reset_target=false` 时的 `ON CONFLICT DO NOTHING` 幂等语义
     - 序列回填逻辑改为仅当表包含 `id` 列时执行，避免对无 `id` 的表触发不必要的序列探测
   - 关键文件：
     - `backend/internal/repository/data_migration_repo.go`
   - 验证通过：
     - `cd backend && GOPROXY=https://goproxy.cn,direct /opt/homebrew/bin/go test -tags=integration ./internal/repository -run 'TestDataMigrationRepository_' -count=1`

27. 输出“切换与验收清单（执行版）”
   - 说明：补齐可直接执行的切换前检查、dry-run 演练、apply 导入、回填重建、验收口径与回滚预案
   - 新增文件：
     - `docs/plans/2026-03-26-old-system-feature-data-migration-cutover-checklist.md`

28. 清理多处硬编码平台下拉文案，补齐 i18n（避免中英混杂）
   - 背景：用户反馈多语言配置不完整，排查发现多处平台下拉仍硬编码英文（Anthropic/OpenAI/Gemini/Antigravity/Sora）
   - 修复：统一替换为 i18n key（`admin.groups.platforms.*` / `admin.accounts.platforms.*`）
   - 关键文件：
     - `frontend/src/views/admin/SubscriptionsView.vue`
     - `frontend/src/views/admin/GroupsView.vue`
     - `frontend/src/views/admin/ops/components/OpsDashboardHeader.vue`
     - `frontend/src/components/admin/ErrorPassthroughRulesModal.vue`
   - 验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend run typecheck`

29. 复核定价/号池监控前端页与侧边栏入口：定向用例验证通过
   - 说明：针对用户反馈“页面上没看到”，补做一轮定向测试，确保路由与侧边栏入口存在且页面可渲染
   - 验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend exec vitest --run src/views/admin/__tests__/PricingPlansView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts src/views/admin/__tests__/PoolMonitorView.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts`

30. 后端路由注册复核：dashboard / 定价 / 号池监控相关 routes 用例通过
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test ./internal/server/routes -count=1`

31. 补齐迁移器对“旧系统全部表”的覆盖：纳入 billing 与 ops metrics
   - 背景：旧系统 migrations 包含 `billing_usage_entries`、`ops_metrics_hourly/daily`、`orphan_allowed_groups_audit`，此前迁移器 `phase=all` 未覆盖，会导致“旧系统所有数据迁移”不完整
   - 调整：
     - `access_control` phase 增加：`orphan_allowed_groups_audit`
     - 新增 `ops_metrics` phase：`ops_metrics_hourly`、`ops_metrics_daily`
     - 新增 `billing` phase：`billing_usage_entries`
     - 同步更新 data-migrator CLI `--phase` help
   - 关键文件：
     - `backend/internal/repository/data_migration_repo.go`
     - `backend/cmd/data-migrator/main.go`
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test ./internal/service -run 'TestDataMigrationService_' -count=1`
     - `cd backend && /opt/homebrew/bin/go test ./internal/repository -run 'TestDataMigrationRepository_' -count=1`
     - `cd backend && /opt/homebrew/bin/go test ./cmd/data-migrator -count=1`

32. 补齐 i18n 平台文案：新增 `kiro`（避免号池监控/平台标签回退为原始值）
   - 背景：Pool Monitor 看板会根据平台动态生成标签，若平台 key 未配置会回退显示原始值（如 `kiro`）
   - 修复：为 `admin.groups.platforms.kiro`、`admin.accounts.platforms.kiro` 补齐中英文文案
   - 关键文件：
     - `frontend/src/i18n/locales/zh.ts`
     - `frontend/src/i18n/locales/en.ts`
   - 验证通过：
     - `PATH=/Users/duegin/.nvm/versions/node/v20.19.6/bin:$PATH /Users/duegin/.nvm/versions/node/v20.19.6/bin/pnpm --dir frontend run typecheck`

33. 同步更新“切换与验收清单（执行版）”的 phase 列表（对齐迁移器最新能力）
   - 背景：迁移器新增 `ops_metrics` / `billing` phase，并在 `access_control` 补齐审计表；清单若不更新会导致执行指令缺失
   - 修复：更新 phase 说明与分阶段 apply 示例命令
   - 关键文件：
     - `docs/plans/2026-03-26-old-system-feature-data-migration-cutover-checklist.md`

34. data-migrator 支持从后端 `config.yaml` 读取 DSN（降低执行门槛）
   - 背景：真实切换演练需要填写 `source_dsn/target_dsn`，手写 DSN 容易出错且不便管理
   - 修复：新增 `--source_config/--target_config`，当 `source_dsn/target_dsn` 为空时自动从配置文件生成 DSN（URL 形式，包含 sslmode）
   - 关键文件：
     - `backend/cmd/data-migrator/main.go`
     - `docs/plans/2026-03-26-old-system-feature-data-migration-cutover-checklist.md`
   - 验证通过：
     - `cd backend && /opt/homebrew/bin/go test ./cmd/data-migrator -count=1`

35. 本机演练：跑通 data-migrator dry-run（验证新增 phase 与缺表风险）
   - 说明：使用 `--source_config/--target_config` 在当前环境执行 dry-run（本机两套 config 当前指向同一 DB，因此 diff=0；该步骤主要验证“能连库 + phase 全量跑通 + 报告可落盘”）
   - 执行：
     - `cd backend && /opt/homebrew/bin/go run ./cmd/data-migrator --source_config /Volumes/T7/project/FluxCode/backend/config.yaml --target_config /Volumes/T7/project/new/FluxCode/backend/config.yaml --mode dry-run --phase core --report_file /tmp/fluxcode-migration-dry-run-core.json`
     - `cd backend && /opt/homebrew/bin/go run ./cmd/data-migrator --source_config /Volumes/T7/project/FluxCode/backend/config.yaml --target_config /Volumes/T7/project/new/FluxCode/backend/config.yaml --mode dry-run --phase all --report_file /tmp/fluxcode-migration-dry-run-all.json`
   - 产物：
     - `/tmp/fluxcode-migration-dry-run-core.json`
     - `/tmp/fluxcode-migration-dry-run-all.json`

### 进行中

1. 正式切换与回填验收清单
   - 需要输出一份可直接执行的切换前检查、迁移演练、校验对账、回填重建与回滚清单
   - 需要基于真实 `source_dsn/target_dsn` 跑通 dry-run 报告，并按报告补齐剩余表族与差异处理策略

### 待做

1. 用真实 `source_dsn/target_dsn` 跑一次 `dry-run --phase all` 输出 `report_file`，根据报告补齐剩余表族与差异处理
2. 针对大表（尤其 `usage_logs`）补齐性能方案（批量写入/COPY/分片导入）与可中断恢复策略
3. 演练一次 `apply --phase all --reset_target` 到 staging，按关键口径对账（用户、分组、订阅、余额、usage、代理用量等）
4. 输出最终切换步骤与回滚预案（冻结旧系统写入 -> final apply -> 切流 -> 回填/重建聚合表 -> 验收）

## 记录规则

从下一步开始，所有执行动作统一按以下格式持续维护：

- 已完成：记录结果、关键文件、验证方式
- 进行中：只保留当前唯一主任务
- 待做：按优先级刷新剩余任务

## 备注

- 当前已进入本轮业务代码实施，优先完成 Task 1 底层闭环
- 当前尚未开始旧库到新库的数据迁移实现
- 当前文档仅作为设计与执行记录基线，后续每完成一个任务必须即时更新
