# 定价与号池监控迁移执行记录

## 当前状态

### 已完成

1. Task 1 定价服务层契约收口
   - 增加 `backend/internal/service/pricing_plan_service_test.go`
   - 修正 `PricingPlanService.ListPublicGroups()` 的公开分组过滤逻辑
   - 验证通过：`go test ./internal/service -run 'TestPricingPlanService_' -count=1`
   - 已提交：`e31d5731`

2. Task 2 定价仓储与迁移脚本对齐
   - 增加 `backend/internal/repository/pricing_plan_repo_integration_test.go`
   - 补充 migration schema 断言
   - 验证通过：
     - `GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestPricingPlanRepository_' -count=1`
     - `GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestPricingPlanRepository_|TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate' -count=1`
   - 已提交：`379dc31e`

3. Task 3 定价 handler、DTO、路由与 wiring 收口
   - 增加 handler 与 route 测试
   - 重生成 `backend/cmd/server/wire_gen.go`
   - 验证通过：
     - `go test ./internal/handler/... ./internal/server/routes -run 'TestPricingPlanHandler_|TestPublicPricingPlanHandler_|TestPricingPlanRoutes_' -count=1`
     - `go test ./cmd/server/...`
   - 已提交：`762f1834`

4. Task 4 定价前端 API、类型、页面与路由闭环
   - 增加 `PricingView` / `PricingPlansView` 测试
   - 补齐后台定价页路由文案元信息
   - 验证通过：
     - `pnpm --dir frontend exec vitest --run src/views/__tests__/PricingView.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts`
     - `pnpm --dir frontend run typecheck`
   - 说明：共享文件仍会被后续 Task 8/9 继续修改，暂未单独提交

5. Task 5 号池监控配置仓储与服务层收口
   - 迁移 `backend/internal/service/pool_monitor_service_test.go`
   - 新增 `backend/internal/repository/pool_alert_config_cache_test.go`
   - 新增 `backend/internal/repository/pool_alert_config_repo_integration_test.go`
   - 扩展 `backend/internal/repository/migrations_schema_integration_test.go` 覆盖 `080_pool_monitor_tables.sql`
   - 修复旧缓存 payload 兼容逻辑：当缺失 `proxy_active_probe_enabled` 时回填默认值 `true`
   - 验证通过：
     - `go test ./internal/repository -run 'TestPoolAlertConfigCacheBackwardCompatibility_' -count=1`
     - `go test ./internal/service -run 'TestPoolMonitorService_' -count=1`
     - `GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestPoolAlertConfigRepository_|TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate' -count=1`

6. Task 6 号池监控 handler、worker 与 wiring 收口
   - 新增 `backend/internal/handler/admin/pool_monitor_handler_test.go`
   - 新增 `backend/internal/server/routes/pool_monitor_test.go`
   - 新增 `backend/internal/service/openai_pool_monitor_worker_test.go`
   - 新增 `backend/internal/repository/alert_cooldown_store.go`
   - 新增 `backend/internal/repository/proxy_connectivity_snapshot_store.go`
   - 补齐 `AlertService`、`OpenAIPoolMonitorWorker`、cleanup 链与 `wire` 注入
   - 成功重生成 `backend/cmd/server/wire_gen.go`
   - 验证通过：
     - `go generate ./cmd/server`
     - `go test ./internal/repository -run 'TestPoolAlertConfigCacheBackwardCompatibility_|TestBuildRedisOptions' -count=1`
     - `go test ./internal/handler/admin ./internal/server/routes ./internal/service -run 'TestPoolMonitorHandler_|TestPoolMonitorRoutes_|TestOpenAIPoolMonitorWorker_' -count=1`
     - `go test ./cmd/server/... -count=1`
   - 说明：涉及 `admin.go` / `handler.wire.go` / `service.wire.go` / `repository.wire.go` 等共享文件，等待后续任务一起收口提交，避免误切无关 diff

7. Task 7 账号管理增强后端
   - 新增 `backend/internal/handler/admin/account_handler_list_filters_test.go`
   - 新增 `backend/internal/handler/admin/account_handler_summary_test.go`
   - 新增 `backend/internal/repository/account_scheduling_strategy_test.go`
   - 新增 `backend/internal/repository/account_repo_integration_test.go` 中高级筛选与汇总集成覆盖
   - 新增 `backend/internal/service/account_scheduling.go`
   - 新增 `backend/internal/service/account_summary.go`
   - 新增 `backend/internal/repository/account_scheduling_strategy.go`
   - 扩展 `backend/internal/repository/account_repo.go`，补齐高级筛选与账号汇总查询
   - 扩展 `backend/internal/service/admin_service.go`，补齐 `ListAccountsAdvanced()` 与 `GetAccountSummary()`
   - 扩展 `backend/internal/handler/admin/account_handler.go`，补齐 `schedulable_status` / `group_id` / `proxy_ids` / `created_start_date` / `created_end_date` / `sort_by` / `sort_order` 解析与 `GET /summary`
   - 扩展 `backend/internal/server/routes/admin.go`，注册 `GET /api/v1/admin/accounts/summary`
   - 对齐当前系统状态语义：
     - `inactive` 查询兼容旧 `inactive` 与当前 `disabled`
     - `banned` 不再依赖旧 `status=banned`，改为基于 `status=error` + `error_message` 中 `violation / terms of service` 识别
   - 验证通过：
     - `go test ./internal/handler/admin ./internal/repository -run 'TestAccountHandler|TestResolveAccountSchedulingStrategy|TestBuildAccountSchedulingBucketCaseSQLIncludesPlatformBranches' -count=1`
     - `go test ./internal/service -run 'Test.*ListAccounts|Test.*AccountSummary' -count=1`
     - `go test ./internal/handler/admin ./internal/repository ./internal/service -run 'TestAccountHandler|TestResolveAccountSchedulingStrategy|TestBuildAccountSchedulingBucketCaseSQLIncludesPlatformBranches|Test.*ListAccounts|Test.*AccountSummary' -count=1`
     - `GOPROXY=https://goproxy.cn,direct go test -tags=integration ./internal/repository -run 'TestAccountRepoSuite/(TestListWithAdvancedFilters_FilterBySchedulingStatus|TestGetAccountSummary)' -count=1`
   - 说明：与 Task 6 一样仍涉及共享文件，等待后续前端任务完成后统一切分提交

8. Task 8 账号管理增强前端
   - 新增 `frontend/src/views/admin/__tests__/AccountsView.spec.ts`
   - 新增 `frontend/src/components/common/ProxyMultiSelectFilter.vue`
   - 扩展 `frontend/src/api/admin/accounts.ts`，补齐 `schedulable_status` / `proxy_ids` / `created_start_date` / `created_end_date` / `sort_by` / `sort_order` 参数与 `proxy_ids` 序列化
   - 扩展 `frontend/src/types/index.ts`，补齐 `AccountSchedulingState`
   - 扩展 `frontend/src/components/admin/account/AccountTableFilters.vue`，补齐调度状态、代理、多日期区间筛选控件
   - 扩展 `frontend/src/views/admin/AccountsView.vue`，补齐高级筛选参数透传、服务端排序接线、本地筛选匹配
   - 扩展 `frontend/src/i18n/locales/en.ts` 与 `frontend/src/i18n/locales/zh.ts`，补齐账号增强筛选文案
   - 验证通过：
     - `corepack pnpm --dir frontend exec vitest --run src/views/admin/__tests__/AccountsView.spec.ts`
     - `corepack pnpm --dir frontend run typecheck`
   - 说明：当前账号管理前端已与 Task 7 后端接口字段对齐，后续继续推进 Task 9 号池监控前端

9. Task 9 号池监控前端
   - 新增 `frontend/src/views/admin/__tests__/PoolMonitorView.spec.ts`
   - 新增 `frontend/src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts`
   - 对照旧仓库复核当前 `frontend/src/views/admin/PoolMonitorView.vue`、`frontend/src/views/admin/PoolMonitorDashboardView.vue`、`frontend/src/router/index.ts`、`frontend/src/api/admin/poolMonitor.ts`、`frontend/src/components/charts/ProxyUsageSummaryChart.vue`、`frontend/src/components/charts/RequestCountTrend.vue`，确认本轮主要缺口为测试收口
   - 修正 `PoolMonitorView` 测试中的 `Toggle` stub 运行时模板语法，使配置页完整渲染后可验证保存行为
   - 修正 `PoolMonitorDashboardView` 测试中的平台 tab 选择逻辑，改为精确切换到 `openai` 标签页验证汇总切换
   - 验证通过：
     - `corepack pnpm --dir frontend exec vitest --run src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts`
     - `corepack pnpm --dir frontend run typecheck`
   - 说明：当前号池监控前端页面、图表与路由链路已完成闭环，进入 Task 10 统一验证阶段

10. Task 10 统一验证与记录收口
   - 验证通过：
     - `go test ./internal/service ./internal/handler/... ./internal/server/... -run 'TestPricingPlan|TestPoolMonitor|TestAccountHandler|TestAccountSchedulingStrategy' -count=1`
     - `corepack pnpm --dir frontend exec vitest --run src/views/__tests__/PricingView.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts src/views/admin/__tests__/AccountsView.spec.ts`
     - `corepack pnpm --dir frontend run typecheck`
     - `corepack pnpm --dir frontend run build`
     - `go test ./cmd/server/... -count=1`
   - 结果说明：
     - 前端 `build` 成功，存在既有 chunk size warning，但未阻断构建
     - 已同步更新迁移计划文档与设计文档中的完成范围、非目标与已知边界说明
   - 提交说明：
     - 当前工作区仍存在多组混合改动与共享文件变更，暂未自动切分提交，待后续人工确认提交策略

11. Task 11 前端入口补漏与号池监控路由纠偏
   - 新增 `frontend/src/components/layout/__tests__/AppSidebar.spec.ts`
   - 新增 `frontend/src/router/__tests__/admin-routes.spec.ts`
   - 新增 `frontend/src/i18n/__tests__/navigationLocales.spec.ts`
   - 扩展 `frontend/src/components/layout/AppSidebar.vue`
     - 补齐后台侧边栏中的 `定价方案` 与 `号池监控` 入口
     - 对齐旧仓库的后台导航顺序与简单模式显隐规则
   - 扩展 `frontend/src/router/index.ts`
     - 恢复旧仓库号池监控路由语义：`/admin/pool-monitor` 为 dashboard，`/admin/pool-monitor/config` 为配置页
     - 保留 `/admin/pool-monitor/dashboard -> /admin/pool-monitor` 的兼容跳转，避免当前错误入口失效
   - 扩展 `frontend/src/i18n/locales/en.ts` 与 `frontend/src/i18n/locales/zh.ts`
     - 补齐 `nav.pricingPlans` 与 `nav.poolMonitor`
   - 验证通过：
     - `corepack pnpm --dir frontend exec vitest --run src/components/layout/__tests__/AppSidebar.spec.ts src/router/__tests__/admin-routes.spec.ts src/i18n/__tests__/navigationLocales.spec.ts`
     - `corepack pnpm --dir frontend exec vitest --run src/components/layout/__tests__/AppSidebar.spec.ts src/router/__tests__/admin-routes.spec.ts src/i18n/__tests__/navigationLocales.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts`
     - `corepack pnpm --dir frontend run typecheck`
     - `corepack pnpm --dir frontend run build`
   - 结果说明：
     - 用户反馈的“定价系统 / 号池监控页面看不到”根因已修复，页面入口与号池监控 dashboard 路由已重新可达
     - 前端 `build` 成功，仍存在既有 chunk size warning，但未阻断构建

12. Task 12 Dashboard 补漏与多语言收口
   - 扩展 `frontend/src/views/admin/DashboardView.vue`
     - 补齐旧仓库缺失的 `RequestCountTrend` 与 `ProxyUsageSummaryChart`
     - 补齐代理使用汇总数据链路，接入 `adminAPI.dashboard.getProxyUsageSummary()`
   - 扩展 `frontend/src/api/admin/dashboard.ts`
     - 补齐 `getProxyUsageSummary()` 与 `ProxyUsageSummaryResponse`
   - 扩展 `frontend/src/i18n/locales/en.ts` 与 `frontend/src/i18n/locales/zh.ts`
     - 补齐 `admin.dashboard.requestCountTrend / proxyUsageSummary / metricLegend / proxyLegend / totalCount / successCount / failureCount`
     - 补齐 `admin.poolMonitorDashboard.*`、`admin.poolMonitorConfig.*`、`admin.poolMonitor.*`
     - 补齐 `admin.proxies.countStates.*`，修复调度状态下拉与号池监控页面 raw key 问题
   - 新增验证覆盖：
     - `frontend/src/i18n/__tests__/migrationLocales.spec.ts`
     - `frontend/src/views/admin/__tests__/DashboardView.spec.ts` 中 dashboard 缺图表与代理汇总调用断言
   - 验证通过：
     - `corepack pnpm --dir frontend exec vitest --run src/views/admin/__tests__/DashboardView.spec.ts`
     - `corepack pnpm --dir frontend exec vitest --run src/i18n/__tests__/migrationLocales.spec.ts`
     - `corepack pnpm --dir frontend exec vitest --run src/views/admin/__tests__/DashboardView.spec.ts src/i18n/__tests__/migrationLocales.spec.ts src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts src/router/__tests__/admin-routes.spec.ts src/i18n/__tests__/navigationLocales.spec.ts`
     - `corepack pnpm --dir frontend run typecheck`
     - `corepack pnpm --dir frontend run build`
   - 结果说明：
     - 用户反馈的“dashboard 未迁移完整、多语言未设置”问题已修复，当前 dashboard 已补齐缺失图表，号池监控页与调度状态筛选不再依赖缺失 key
     - 前端 `build` 成功，仍存在既有 Vite dynamic import / chunk size warning，但未阻断构建

13. Task 13 Dashboard 代理使用汇总后端补齐
   - 新增 `backend/internal/server/routes/dashboard_test.go`
   - 新增 `backend/internal/service/openai_gateway_service_proxy_usage_metrics_test.go`
   - 新增 `backend/internal/repository/proxy_usage_metrics_repo.go`
   - 扩展 `backend/internal/server/routes/admin.go`
     - 注册 `GET /api/v1/admin/dashboard/proxies-usage`
   - 扩展 `backend/internal/handler/admin/dashboard_handler.go`
     - 补齐 `GetProxyUsageSummary()`，返回 dashboard 代理使用汇总数据
   - 扩展 `backend/internal/service/dashboard_service.go`
     - 补齐 `GetProxyUsageSummary()` 聚合逻辑
     - 注入 `ProxyUsageMetricsRepository`
   - 扩展 `backend/internal/service/openai_gateway_service.go`
     - 补齐代理使用指标写入链路，在 OpenAI 请求成功/失败时记录 `proxy_usage_metrics_hourly`
     - 修正失败指标写入时机，避免将自动重试后成功的请求误记为失败
   - 扩展 `backend/internal/pkg/usagestats/usage_log_types.go`
     - 补齐 `ProxyUsageSummaryItem` 与 `ProxyUsageTimelinePoint`
   - 扩展 `backend/internal/service/wire.go`、`backend/internal/repository/wire.go`
     - 将代理使用指标仓储接入 `DashboardService` 与 `OpenAIGatewayService`
   - 生成更新：
     - `go generate ./cmd/server`
     - 更新 `backend/cmd/server/wire_gen.go`
   - 验证通过：
     - `go test ./internal/server/routes ./internal/service -run 'TestDashboardRoutes_AdminProxyUsageSummaryPathIsRegistered|TestOpenAIGatewayService_RecordProxyUsageMetric' -count=1`
     - `go test ./cmd/server/... -count=1`
   - 结果说明：
     - 用户反馈的 `GET /api/v1/admin/dashboard/proxies-usage` 404 已修复
     - dashboard 代理使用汇总接口已接回后端，且代理使用指标不再是“有查无写”的断链状态

14. Task 14 前端包管理器约束收口
   - 根因确认：
     - 用户在 `frontend` 目录执行 `npm i`，但当前前端工程使用 `pnpm-lock.yaml` 与 `pnpm` 风格 `node_modules`
     - `npm@10.8.2` 在解析 `.pnpm/.../node_modules/...` 链接树时触发内部异常：`Cannot read properties of null (reading 'matches')`
     - 该错误发生在 npm 构建依赖树阶段，早于 `preinstall` 脚本执行，因此不能依赖脚本拦截来修复
   - 收口动作：
     - 扩展 `frontend/package.json`，新增 `packageManager: pnpm@10.28.1`
     - 统一后续安装命令为 `corepack pnpm --dir frontend install --frozen-lockfile`
   - 验证通过：
     - `corepack pnpm --dir frontend install --frozen-lockfile`
   - 结果说明：
     - 当前问题根因不是依赖损坏，也不是 Node 版本列表异常，而是对 `pnpm` 工程误用了 `npm`
     - 前端安装链路已明确收口到 `pnpm`

15. Task 15 前端国际化漏项静态收口
   - 根因确认：
     - 现有 `migrationLocales.spec.ts` / `navigationLocales.spec.ts` 只覆盖了少量迁移页面词条，无法发现项目内其他静态 `t('...')` / `$t('...')` 调用的缺失 key
     - 缺失主要集中在 `admin.pricingPlans.*`、`admin.ops.runtime.*`、`admin.accounts.*`、`admin.users.*`、`admin.settings.soraS3.*` 与 `common.*`
   - 收口动作：
     - 新增 `frontend/src/i18n/__tests__/staticLocaleCoverage.spec.ts`
       - 扫描 `frontend/src` 中所有静态国际化 key，并分别校验 `zh` / `en` locale 是否存在对应词条
       - 过滤拼接式动态 key，避免把 `t('foo.' + status)` 误报为缺失
     - 扩展 `frontend/src/i18n/locales/zh.ts` 与 `frontend/src/i18n/locales/en.ts`
       - 补齐定价方案页相关词条
       - 补齐号池监控、运维运行设置、用户编辑弹窗、Sora 存储列表等缺失词条
       - 补齐 `common.apply / clear / banned / creating / sending / purchase / recommended / currency.* / period.*`
   - 验证通过：
     - `corepack pnpm --dir frontend exec vitest --run src/i18n/__tests__/staticLocaleCoverage.spec.ts`
     - `corepack pnpm --dir frontend exec vitest --run src/i18n/__tests__/staticLocaleCoverage.spec.ts src/i18n/__tests__/migrationLocales.spec.ts src/i18n/__tests__/navigationLocales.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts`
     - `corepack pnpm --dir frontend run typecheck`
   - 结果说明：
     - 当前项目内静态字面量国际化 key 已完成 `zh` / `en` 双语覆盖
     - 本轮额外建立了自动化兜底，后续若再漏配静态词条，测试会直接报出缺失 key 列表

### 进行中

1. 提交切分与提交动作确认
   - 下一步：基于当前混合工作区确认哪些文件进入迁移提交，避免把无关半成品一并提交

### 待做

1. 迁移改动正式提交
   - 当前功能与验证已完成，仍需结合当前混合工作区确认提交切分边界

## 执行日志

### 2026-03-24

- 初始化执行记录文件，接续已有迁移进度，并确认后续每完成一步都同步更新。
- 完成 Task 5：补齐号池监控配置服务测试、仓储集成测试与 migration schema 断言，修复缓存兼容旧 payload 时遗漏 `proxy_active_probe_enabled` 默认值的问题。
- 完成 Task 6：补齐号池监控 handler、路由、worker 测试与 provider/wire/cleanup 接线，`go generate ./cmd/server` 与 `go test ./cmd/server/...` 均通过。
- 完成 Task 7：补齐账号管理高级筛选、调度状态汇总与 `/accounts/summary` 后端接口，并按当前系统语义收口 `inactive` / `banned` 状态兼容，新增仓储集成测试锁定行为。
- 完成 Task 8：补齐账号管理前端高级筛选、服务端排序与参数链路，新增 `AccountsView` 测试并通过前端 `typecheck`。

### 2026-03-25

- 完成 Task 9：复核当前号池监控前端实现已基本对齐旧仓库，补齐 `PoolMonitorView` / `PoolMonitorDashboardView` 测试并修正测试 stub 与 tab 选择逻辑，相关 Vitest 与前端 `typecheck` 均通过。
- 完成 Task 10 验证与记录收口：focused backend/frontend 测试、前端 `typecheck` / `build`、后端 `go test ./cmd/server/...` 全部通过，并将迁移范围、非目标、已知边界同步回计划文档。
- 完成提交流水风险评估：确认当前工作区含多组混合改动与共享文件变化，暂缓自动提交，后续需按文件边界谨慎切分。
- 启动 Task 11：根据最新页面反馈重新核查可见入口，定位到后台侧边栏缺少 `定价管理` / `号池监控` 导航项，且号池监控 dashboard/config 路由语义与旧仓库相反，导致页面虽已实现但无法按预期入口访问。
- 完成 Task 11：补齐后台侧边栏定价/号池监控入口，恢复旧仓库的号池监控 dashboard/config 路由语义，并补充导航与路由回归测试、`typecheck`、`build` 验证。
- 启动 Task 12：根据最新用户反馈复核 `DashboardView` 与多语言配置，确认当前 dashboard 仍缺旧仓库的请求数趋势图、代理使用汇总图及其数据接口调用，且号池监控页与调度状态下拉依赖的 locale key 尚未迁移；已用现有红测稳定锁定范围，准备进入最小实现修复。
- 完成 Task 12-1：补齐 `frontend/src/views/admin/DashboardView.vue` 与 `frontend/src/api/admin/dashboard.ts` 中缺失的请求数趋势图、代理使用汇总图与代理汇总接口调用链；`corepack pnpm --dir frontend exec vitest --run src/views/admin/__tests__/DashboardView.spec.ts` 已通过。
- 完成 Task 12-2：补齐 `frontend/src/i18n/locales/en.ts` 与 `frontend/src/i18n/locales/zh.ts` 中 dashboard、号池监控、调度状态相关缺失词条，`migrationLocales.spec.ts` 与号池监控 / 导航 / 路由回归测试全部通过。
- 完成 Task 12-3：补跑 dashboard + locale + 号池监控 + 导航相关组合回归，并通过前端 `typecheck` 与 `build`；构建仍有既有 Vite dynamic import / chunk size warning，但未阻断产物生成。
- 启动 Task 13：用户反馈 dashboard 请求 `/api/v1/admin/dashboard/proxies-usage` 返回 404，经定位确认是前端先接入了代理使用汇总接口，但当前后端仍缺对应路由、handler、service 与 repository 实现；已新增 focused 路由红测稳定复现该问题，准备开始补齐后端链路。
- 完成 Task 13-1：新增 `backend/internal/server/routes/dashboard_test.go` 并用 `go test ./internal/server/routes -run 'TestDashboardRoutes_AdminProxyUsageSummaryPathIsRegistered' -count=1` 稳定复现 `404`，确认根因是后端路由未注册。
- 完成 Task 13-2：补齐 dashboard 代理使用汇总的后端路由、handler、service、repository 与响应结构，并把 `ProxyUsageMetricsRepository` 接入依赖注入。
- 完成 Task 13-3：补齐 OpenAI 网关代理使用指标写入链路，新增 `openai_gateway_service_proxy_usage_metrics_test.go`，并修正失败指标写入时机，避免自动重试成功的请求被误记为失败。
- 完成 Task 13-4：执行 `go generate ./cmd/server` 更新 `wire_gen.go`，并通过 `go test ./internal/server/routes ./internal/service -run 'TestDashboardRoutes_AdminProxyUsageSummaryPathIsRegistered|TestOpenAIGatewayService_RecordProxyUsageMetric' -count=1` 与 `go test ./cmd/server/... -count=1` 验证。
- 启动 Task 14：根据用户提供的 `frontend % npm i` 报错与 npm debug 日志复核根因，确认当前前端是 `pnpm` 工程，而 `npm@10.8.2` 在解析 `.pnpm` 链接树时于 arborist 内部抛出 `Cannot read properties of null (reading 'matches')`，问题不在依赖本身。
- 完成 Task 14-1：扩展 `frontend/package.json`，新增 `packageManager: pnpm@10.28.1`，把工程元数据与现有 `pnpm-lock.yaml` / 文档约束对齐，避免后续继续沿着 `npm` 方向排查。
- 完成 Task 14-2：执行 `corepack pnpm --dir frontend install --frozen-lockfile` 验证前端安装链路，结果为 `Lockfile is up to date, Already up to date`；当前仅剩 `pnpm approve-builds` 的既有提示，不阻断依赖安装。
- 启动 Task 15：根据用户反馈继续复核前端国际化缺口，确认现有 locale 回归测试只覆盖少量迁移词条，无法发现项目内其他静态 `t('...')` / `$t('...')` 调用的缺失 key。
- 完成 Task 15-1：新增 `frontend/src/i18n/__tests__/staticLocaleCoverage.spec.ts`，通过静态扫描 `frontend/src` 中的字面量国际化 key，对 `zh` / `en` locale 做全量存在性校验，并过滤掉拼接式动态 key 的误报。
- 完成 Task 15-2：补齐 `frontend/src/i18n/locales/zh.ts` 与 `frontend/src/i18n/locales/en.ts` 中缺失的定价方案、运维运行设置、Sora 存储、用户编辑与通用词条，覆盖 `admin.pricingPlans.*`、`admin.ops.runtime.*`、`admin.accounts.*`、`admin.users.*`、`admin.settings.soraS3.columns.bucket` 与 `common.*`。
- 完成 Task 15-3：执行 `corepack pnpm --dir frontend exec vitest --run src/i18n/__tests__/staticLocaleCoverage.spec.ts src/i18n/__tests__/migrationLocales.spec.ts src/i18n/__tests__/navigationLocales.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts` 与 `corepack pnpm --dir frontend run typecheck`，全部通过。
