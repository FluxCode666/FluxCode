# Pricing And Pool Monitor Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在当前仓库中续接并完成定价相关、号池监控相关以及其依赖的账号管理增强迁移，同时保持非目标功能不被误迁入。

**Architecture:** 以当前仓库的 wiring、迁移编号、前端路由和类型体系为基线，将旧仓库作为行为参考源，按“定价后端 -> 定价前端 -> 监控后端 -> 账号增强 -> 监控前端 -> 验证收口”的顺序增量补齐。所有实现按 TDD 推进，优先写失败测试，再做最小实现，最后做定向验证和小步提交。

**Tech Stack:** Go 1.26、Gin、Wire、PostgreSQL/SQL migration、Vue 3、TypeScript、Pinia、Vue Router、Vitest

---

### Task 1: 定价服务层契约收口

**Files:**
- Modify: `backend/internal/service/pricing_plan.go`
- Modify: `backend/internal/service/pricing_plan_service.go`
- Create: `backend/internal/service/pricing_plan_service_test.go`

**Step 1: Write the failing test**

```go
func TestPricingPlanService_ListPublicGroups_FiltersInactivePlans(t *testing.T) {
	repo := &pricingPlanRepoStub{
		groups: []PricingPlanGroup{{
			ID:     1,
			Name:   "Starter",
			Status: "active",
			Plans: []PricingPlan{
				{ID: 10, Name: "Monthly", Status: "active"},
				{ID: 11, Name: "Hidden", Status: "disabled"},
			},
		}},
	}

	svc := NewPricingPlanService(repo)
	groups, err := svc.ListPublicGroups(context.Background())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Plans, 1)
	require.Equal(t, int64(10), groups[0].Plans[0].ID)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/service -run 'TestPricingPlanService_' -count=1`

Expected: FAIL，提示 `PricingPlanService` 尚未正确过滤公开数据或测试桩接口不完整。

**Step 3: Write minimal implementation**

```go
func (s *PricingPlanService) ListPublicGroups(ctx context.Context) ([]PricingPlanGroup, error) {
	groups, err := s.repo.ListGroupsWithPlans(ctx, true)
	if err != nil {
		return nil, err
	}

	filtered := make([]PricingPlanGroup, 0, len(groups))
	for _, group := range groups {
		activePlans := make([]PricingPlan, 0, len(group.Plans))
		for _, plan := range group.Plans {
			if strings.EqualFold(plan.Status, "active") {
				activePlans = append(activePlans, plan)
			}
		}
		if len(activePlans) == 0 {
			continue
		}
		group.Plans = activePlans
		filtered = append(filtered, group)
	}
	return filtered, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/service -run 'TestPricingPlanService_' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/service/pricing_plan.go \
  backend/internal/service/pricing_plan_service.go \
  backend/internal/service/pricing_plan_service_test.go
git commit -m "feat: align pricing plan service behavior"
```

### Task 2: 定价仓储与迁移脚本对齐

**Files:**
- Modify: `backend/internal/repository/pricing_plan_repo.go`
- Modify: `backend/migrations/079_pricing_plan_tables.sql`
- Create: `backend/internal/repository/pricing_plan_repo_integration_test.go`

**Step 1: Write the failing test**

```go
func TestPricingPlanRepository_ListGroupsWithPlans_SortsAndLoadsContacts(t *testing.T) {
	ctx := context.Background()
	repo, db := newPricingPlanTestRepo(t)
	seedPricingPlanFixtures(t, db)

	groups, err := repo.ListGroupsWithPlans(ctx, false)
	require.NoError(t, err)
	require.NotEmpty(t, groups)
	require.True(t, len(groups[0].Plans[0].ContactMethods) > 0)
	require.LessOrEqual(t, groups[0].SortOrder, groups[len(groups)-1].SortOrder)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/repository -run 'TestPricingPlanRepository_' -count=1`

Expected: FAIL，提示 schema、扫描逻辑或 JSON 联系方式解析与旧仓库行为不一致。

**Step 3: Write minimal implementation**

```go
rows, err := r.db.QueryContext(ctx, `
	SELECT g.id, g.name, g.status, g.sort_order,
	       p.id, p.name, p.status, p.sort_order, p.contact_methods
	FROM pricing_plan_groups g
	LEFT JOIN pricing_plans p ON p.group_id = g.id
	ORDER BY g.sort_order ASC, p.sort_order ASC, p.id ASC
`)
```

同时补齐 `079_pricing_plan_tables.sql` 中旧仓库定价方案所需字段、默认值、索引和 JSON 联系方式列定义。

**Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/repository -run 'TestPricingPlanRepository_' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/repository/pricing_plan_repo.go \
  backend/migrations/079_pricing_plan_tables.sql \
  backend/internal/repository/pricing_plan_repo_integration_test.go
git commit -m "feat: persist pricing plan groups and plans"
```

### Task 3: 定价 handler、DTO、路由与 wiring 收口

**Files:**
- Modify: `backend/internal/handler/admin/pricing_plan_handler.go`
- Modify: `backend/internal/handler/pricing_plan_handler.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/routes/pricing_plan.go`
- Modify: `backend/internal/server/router.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/cmd/server/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Create: `backend/internal/handler/admin/pricing_plan_handler_test.go`
- Create: `backend/internal/handler/pricing_plan_handler_test.go`

**Step 1: Write the failing test**

```go
func TestPricingPlanHandler_ListGroups_ReturnsDTO(t *testing.T) {
	router := gin.New()
	h := NewPricingPlanHandler(&pricingPlanServiceStub{
		groups: []service.PricingPlanGroup{{ID: 1, Name: "Starter", Status: "active"}},
	})
	router.GET("/admin/pricing/plan-groups", h.ListGroups)

	req := httptest.NewRequest(http.MethodGet, "/admin/pricing/plan-groups", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "\"name\":\"Starter\"")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... ./internal/server/... -run 'TestPricingPlanHandler_|TestPublicPricingPlanHandler_' -count=1`

Expected: FAIL，提示路由、DTO 映射或返回结构未对齐。

**Step 3: Write minimal implementation**

```go
func registerPricingPlanRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	pricing := admin.Group("/pricing")
	{
		pricing.GET("/plan-groups", h.Admin.PricingPlan.ListGroups)
		pricing.POST("/plan-groups", h.Admin.PricingPlan.CreateGroup)
		pricing.PUT("/plans/:id", h.Admin.PricingPlan.UpdatePlan)
	}
}
```

然后执行：

Run: `go generate ./cmd/server`

Expected: `wire_gen.go` 重新生成且不报错。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... ./internal/server/... -run 'TestPricingPlanHandler_|TestPublicPricingPlanHandler_' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handler/admin/pricing_plan_handler.go \
  backend/internal/handler/pricing_plan_handler.go \
  backend/internal/handler/dto/mappers.go \
  backend/internal/handler/dto/types.go \
  backend/internal/server/routes/admin.go \
  backend/internal/server/routes/pricing_plan.go \
  backend/internal/server/router.go \
  backend/internal/handler/wire.go \
  backend/internal/service/wire.go \
  backend/internal/repository/wire.go \
  backend/cmd/server/wire.go \
  backend/cmd/server/wire_gen.go \
  backend/internal/handler/admin/pricing_plan_handler_test.go \
  backend/internal/handler/pricing_plan_handler_test.go
git commit -m "feat: expose pricing plan admin and public routes"
```

### Task 4: 定价前端 API、类型、页面与路由闭环

**Files:**
- Modify: `frontend/src/api/admin/pricingPlans.ts`
- Modify: `frontend/src/api/pricingPlans.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/views/PricingView.vue`
- Modify: `frontend/src/views/admin/PricingPlansView.vue`
- Create: `frontend/src/views/__tests__/PricingView.spec.ts`
- Create: `frontend/src/views/admin/__tests__/PricingPlansView.spec.ts`

**Step 1: Write the failing test**

```ts
it('renders public pricing groups returned by api', async () => {
  vi.mock('@/api/pricingPlans', () => ({
    listPublicPricingPlanGroups: vi.fn().mockResolvedValue([
      { id: 1, name: 'Starter', status: 'active', plans: [{ id: 10, name: 'Monthly', price: 9.9, currency: 'CNY', contact_methods: [] }] }
    ])
  }))

  const wrapper = render(PricingView)
  await flushPromises()

  expect(wrapper.getByText('Starter')).toBeTruthy()
  expect(wrapper.getByText(/9.9/)).toBeTruthy()
})
```

**Step 2: Run test to verify it fails**

Run: `pnpm --dir frontend run vitest --run src/views/__tests__/PricingView.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts`

Expected: FAIL，提示 API 名称、类型字段或组件渲染尚未对齐。

**Step 3: Write minimal implementation**

```ts
export interface PricingPlanGroup {
  id: number
  name: string
  status: string
  sort_order: number
  plans?: PricingPlan[]
}
```

并在路由中确保：

```ts
{
  path: '/pricing',
  name: 'Pricing',
  component: () => import('@/views/PricingView.vue'),
}
```

**Step 4: Run test to verify it passes**

Run: `pnpm --dir frontend run vitest --run src/views/__tests__/PricingView.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts && pnpm --dir frontend run typecheck`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/api/admin/pricingPlans.ts \
  frontend/src/api/pricingPlans.ts \
  frontend/src/types/index.ts \
  frontend/src/router/index.ts \
  frontend/src/i18n/locales/en.ts \
  frontend/src/i18n/locales/zh.ts \
  frontend/src/views/PricingView.vue \
  frontend/src/views/admin/PricingPlansView.vue \
  frontend/src/views/__tests__/PricingView.spec.ts \
  frontend/src/views/admin/__tests__/PricingPlansView.spec.ts
git commit -m "feat: complete pricing pages and client types"
```

### Task 5: 号池监控配置仓储与服务层收口

**Files:**
- Modify: `backend/internal/service/pool_monitor.go`
- Modify: `backend/internal/service/pool_monitor_service.go`
- Modify: `backend/internal/repository/pool_alert_config_repo.go`
- Modify: `backend/internal/repository/pool_alert_config_cache.go`
- Modify: `backend/internal/repository/proxy_transport_failure_counter.go`
- Modify: `backend/migrations/080_pool_monitor_tables.sql`
- Create: `backend/internal/service/pool_monitor_service_test.go`

**Step 1: Write the failing test**

```go
func TestPoolMonitorService_GetConfig_DefaultOpenAI(t *testing.T) {
	svc := NewPoolMonitorService(&poolMonitorRepoStub{}, &proxyCounterStub{}, nil)

	cfg, err := svc.GetConfig(context.Background(), "openai")
	require.NoError(t, err)
	require.Equal(t, "openai", cfg.Platform)
	require.True(t, cfg.MasterEnabled)
	require.NotZero(t, cfg.CheckIntervalMinutes)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/service -run 'TestPoolMonitorService_' -count=1`

Expected: FAIL，提示默认值、缓存回填或配置校验行为未对齐。

**Step 3: Write minimal implementation**

```go
func (s *PoolMonitorService) GetConfig(ctx context.Context, platform string) (*PoolMonitorConfig, error) {
	if cached, ok := s.cache.Get(platform); ok {
		return cached, nil
	}
	cfg, err := s.repo.GetConfig(ctx, platform)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = defaultPoolMonitorConfig(platform)
	}
	return cfg, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/service -run 'TestPoolMonitorService_' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/service/pool_monitor.go \
  backend/internal/service/pool_monitor_service.go \
  backend/internal/repository/pool_alert_config_repo.go \
  backend/internal/repository/pool_alert_config_cache.go \
  backend/internal/repository/proxy_transport_failure_counter.go \
  backend/migrations/080_pool_monitor_tables.sql \
  backend/internal/service/pool_monitor_service_test.go
git commit -m "feat: align pool monitor config persistence"
```

### Task 6: 号池监控 handler、worker 与 wiring 收口

**Files:**
- Modify: `backend/internal/handler/admin/pool_monitor_handler.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/service/openai_pool_monitor_worker.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/cmd/server/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Create: `backend/internal/handler/admin/pool_monitor_handler_test.go`

**Step 1: Write the failing test**

```go
func TestPoolMonitorHandler_UpdateConfig_ReturnsUpdatedPayload(t *testing.T) {
	router := gin.New()
	h := NewPoolMonitorHandler(&poolMonitorServiceStub{
		updated: &service.PoolMonitorConfig{Platform: "openai", MasterEnabled: true},
	})
	router.PUT("/admin/pool-monitor/:platform", h.UpdateConfig)

	req := httptest.NewRequest(http.MethodPut, "/admin/pool-monitor/openai", strings.NewReader(`{"master_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "\"platform\":\"openai\"")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/admin ./internal/server/... -run 'TestPoolMonitorHandler_' -count=1`

Expected: FAIL，提示绑定、校验或路由注册未对齐。

**Step 3: Write minimal implementation**

```go
func registerPoolMonitorRoutes(admin *gin.RouterGroup, h *handler.Handlers) {
	poolMonitor := admin.Group("/pool-monitor")
	{
		poolMonitor.GET("/:platform", h.Admin.PoolMonitor.GetConfig)
		poolMonitor.PUT("/:platform", h.Admin.PoolMonitor.UpdateConfig)
	}
}
```

并执行：

Run: `go generate ./cmd/server`

Expected: `wire_gen.go` 成功生成。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/admin ./internal/server/... -run 'TestPoolMonitorHandler_' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handler/admin/pool_monitor_handler.go \
  backend/internal/server/routes/admin.go \
  backend/internal/service/openai_pool_monitor_worker.go \
  backend/internal/handler/wire.go \
  backend/internal/service/wire.go \
  backend/internal/repository/wire.go \
  backend/cmd/server/wire.go \
  backend/cmd/server/wire_gen.go \
  backend/internal/handler/admin/pool_monitor_handler_test.go
git commit -m "feat: expose pool monitor config endpoints"
```

### Task 7: 账号管理后端增强以支撑监控与筛选

**Files:**
- Modify: `backend/internal/handler/admin/account_handler.go`
- Modify: `backend/internal/repository/account_repo.go`
- Modify: `backend/internal/service/account_service.go`
- Create: `backend/internal/service/account_summary.go`
- Create: `backend/internal/service/account_scheduling.go`
- Create: `backend/internal/repository/account_scheduling_strategy.go`
- Create: `backend/internal/repository/account_scheduling_strategy_test.go`
- Create: `backend/internal/handler/admin/account_handler_list_filters_test.go`
- Create: `backend/internal/handler/admin/account_handler_summary_test.go`

**Step 1: Write the failing test**

```go
func TestAccountHandler_ListAccounts_SupportsSchedulingStatusFilter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/accounts?schedulable_status=manual_unschedulable", nil)
	rec := httptest.NewRecorder()

	router := gin.New()
	router.GET("/admin/accounts", handlerUnderTest.ListAccounts)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, serviceStub.lastFilter.SchedulableStatus, "manual_unschedulable")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/admin ./internal/repository ./internal/service -run 'TestAccountHandler_|TestAccountSchedulingStrategy_' -count=1`

Expected: FAIL，提示筛选参数、汇总字段或调度状态分类逻辑缺失。

**Step 3: Write minimal implementation**

```go
type AccountSummary struct {
	Platform          string `json:"platform"`
	RawStatusCount    int    `json:"raw_status_count"`
	SchedulingEnabled int    `json:"scheduling_enabled"`
}
```

并在仓储层补齐筛选条件映射和调度状态分类逻辑，保持当前仓库现有账号接口不回退。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/admin ./internal/repository ./internal/service -run 'TestAccountHandler_|TestAccountSchedulingStrategy_' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handler/admin/account_handler.go \
  backend/internal/repository/account_repo.go \
  backend/internal/service/account_service.go \
  backend/internal/service/account_summary.go \
  backend/internal/service/account_scheduling.go \
  backend/internal/repository/account_scheduling_strategy.go \
  backend/internal/repository/account_scheduling_strategy_test.go \
  backend/internal/handler/admin/account_handler_list_filters_test.go \
  backend/internal/handler/admin/account_handler_summary_test.go
git commit -m "feat: add account scheduling filters and summary"
```

### Task 8: 账号管理前端增强与接口字段对齐

**Files:**
- Modify: `frontend/src/api/admin/accounts.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/views/admin/AccountsView.vue`
- Create: `frontend/src/views/admin/__tests__/AccountsView.spec.ts`

**Step 1: Write the failing test**

```ts
it('sends scheduling status filter to the accounts api', async () => {
  const listAccounts = vi.fn().mockResolvedValue({ items: [], total: 0 })
  vi.mock('@/api/admin/accounts', () => ({ listAccounts }))

  const wrapper = render(AccountsView)
  await userEvent.selectOptions(wrapper.getByLabelText(/调度状态|Scheduling Status/), 'manual_unschedulable')
  await userEvent.click(wrapper.getByRole('button', { name: /搜索|Search/ }))

  expect(listAccounts).toHaveBeenCalledWith(expect.objectContaining({
    schedulable_status: 'manual_unschedulable'
  }))
})
```

**Step 2: Run test to verify it fails**

Run: `pnpm --dir frontend run vitest --run src/views/admin/__tests__/AccountsView.spec.ts`

Expected: FAIL，提示筛选控件、请求参数或返回类型不完整。

**Step 3: Write minimal implementation**

```ts
export interface AdminAccountQuery {
  schedulable_status?: string
  raw_status?: string
  summary_platform?: string
}
```

并在 `AccountsView.vue` 中补齐筛选项、摘要卡片和调度状态展示映射。

**Step 4: Run test to verify it passes**

Run: `pnpm --dir frontend run vitest --run src/views/admin/__tests__/AccountsView.spec.ts && pnpm --dir frontend run typecheck`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/api/admin/accounts.ts \
  frontend/src/types/index.ts \
  frontend/src/i18n/locales/en.ts \
  frontend/src/i18n/locales/zh.ts \
  frontend/src/views/admin/AccountsView.vue \
  frontend/src/views/admin/__tests__/AccountsView.spec.ts
git commit -m "feat: add account scheduling filters to admin ui"
```

### Task 9: 号池监控前端页面、图表与路由闭环

**Files:**
- Modify: `frontend/src/api/admin/poolMonitor.ts`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/views/admin/PoolMonitorView.vue`
- Modify: `frontend/src/views/admin/PoolMonitorDashboardView.vue`
- Modify: `frontend/src/components/charts/ProxyUsageSummaryChart.vue`
- Modify: `frontend/src/components/charts/RequestCountTrend.vue`
- Create: `frontend/src/views/admin/__tests__/PoolMonitorView.spec.ts`
- Create: `frontend/src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts`

**Step 1: Write the failing test**

```ts
it('loads and saves pool monitor config for selected platform', async () => {
  const getPoolMonitorConfig = vi.fn().mockResolvedValue({ platform: 'openai', master_enabled: true })
  const updatePoolMonitorConfig = vi.fn().mockResolvedValue({ platform: 'openai', master_enabled: false })
  vi.mock('@/api/admin/poolMonitor', () => ({
    getPoolMonitorConfig,
    updatePoolMonitorConfig
  }))

  const wrapper = render(PoolMonitorView)
  await flushPromises()
  await userEvent.click(wrapper.getByRole('button', { name: /保存|Save/ }))

  expect(updatePoolMonitorConfig).toHaveBeenCalled()
})
```

**Step 2: Run test to verify it fails**

Run: `pnpm --dir frontend run vitest --run src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts`

Expected: FAIL，提示 API 结构、图表 props 或路由入口未对齐。

**Step 3: Write minimal implementation**

```ts
{
  path: '/admin/pool-monitor',
  name: 'AdminPoolMonitor',
  component: () => import('@/views/admin/PoolMonitorView.vue'),
}
```

并补齐摘要图表所需字段映射和监控总览拉取逻辑。

**Step 4: Run test to verify it passes**

Run: `pnpm --dir frontend run vitest --run src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts && pnpm --dir frontend run typecheck`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/api/admin/poolMonitor.ts \
  frontend/src/router/index.ts \
  frontend/src/types/index.ts \
  frontend/src/i18n/locales/en.ts \
  frontend/src/i18n/locales/zh.ts \
  frontend/src/views/admin/PoolMonitorView.vue \
  frontend/src/views/admin/PoolMonitorDashboardView.vue \
  frontend/src/components/charts/ProxyUsageSummaryChart.vue \
  frontend/src/components/charts/RequestCountTrend.vue \
  frontend/src/views/admin/__tests__/PoolMonitorView.spec.ts \
  frontend/src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts
git commit -m "feat: complete pool monitor admin ui"
```

### Task 10: 统一验证、回归检查与迁移记录收口

**Files:**
- Modify: `docs/plans/2026-03-24-pricing-pool-monitor-migration-design.md`
- Modify: `docs/plans/2026-03-24-pricing-pool-monitor-migration.md`

**Step 1: Run focused backend verification**

Run: `go test ./internal/service ./internal/handler/... ./internal/server/... -run 'TestPricingPlan|TestPoolMonitor|TestAccountHandler|TestAccountSchedulingStrategy' -count=1`

Expected: PASS

**Step 2: Run focused frontend verification**

Run: `pnpm --dir frontend run vitest --run src/views/__tests__/PricingView.spec.ts src/views/admin/__tests__/PricingPlansView.spec.ts src/views/admin/__tests__/PoolMonitorView.spec.ts src/views/admin/__tests__/PoolMonitorDashboardView.spec.ts src/views/admin/__tests__/AccountsView.spec.ts`

Expected: PASS

**Step 3: Run typecheck and build verification**

Run: `pnpm --dir frontend run typecheck && pnpm --dir frontend run build && go test ./cmd/server/...`

Expected: PASS，前端构建成功，后端 server 包编译与测试通过。

**Step 4: Update migration record**

```md
- 已完成：定价接口、定价页面、监控接口、监控页面、账号增强
- 未做：独立工具、pricing fallback 补丁
- 已知边界：保留当前仓库 UI 组织，不追求旧仓库外观一致
```

**Step 5: Commit**

```bash
git add docs/plans/2026-03-24-pricing-pool-monitor-migration-design.md \
  docs/plans/2026-03-24-pricing-pool-monitor-migration.md
git commit -m "docs: update migration execution record"
```
