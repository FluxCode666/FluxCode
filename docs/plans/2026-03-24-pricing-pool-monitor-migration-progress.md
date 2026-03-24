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

### 进行中

1. Task 6 号池监控 handler、worker 与 wiring 收口
   - 下一步：核对 `pool_monitor_handler`、`openai_pool_monitor_worker`、路由与 `wire` 注入链，先补失败测试再收口实现

### 待做

1. Task 7 账号管理增强后端
2. Task 8 账号管理增强前端
3. Task 9 号池监控前端
4. Task 10 全量验证与收口提交

## 执行日志

### 2026-03-24

- 初始化执行记录文件，接续已有迁移进度，并确认后续每完成一步都同步更新。
- 完成 Task 5：补齐号池监控配置服务测试、仓储集成测试与 migration schema 断言，修复缓存兼容旧 payload 时遗漏 `proxy_active_probe_enabled` 默认值的问题。
