# 定价与号池监控迁移设计

## 背景

当前仓库 `/Volumes/T7/project/new/FluxCode` 与旧仓库 `/Volumes/T7/project/FluxCode` 属于同源系统，但功能演进存在分叉。当前仓库已经存在一批未提交的半成品改动，覆盖了 `pricing plan`、`pool monitor`、公开定价页、后台定价页、后台监控页等文件，说明迁移工作已经开始但尚未完成。

用户确认本次迁移范围如下：

- 需要迁移：定价相关功能
- 需要迁移：号池监控相关功能
- 需要迁移：与号池监控相关的账号管理增强
- 不需要迁移：独立保本价计算工具
- 不需要迁移：pricing fallback 路径修复补丁

用户同时确认：

- 保留并续接当前仓库已有半成品改动
- 以功能对齐优先，界面适配当前系统风格

## 目标

在不回退当前仓库已有工作、不误迁非目标内容的前提下，将旧仓库中的下列能力补齐到当前仓库：

1. 公开定价页与后台定价方案管理
2. 号池监控配置页与监控总览页
3. 支撑号池监控的账号管理筛选、汇总、调度状态增强

最终结果应满足：

- 后端接口完整可用
- 前端页面和路由闭环可用
- 数据表结构与当前仓库迁移历史兼容
- 不引入独立工具与 pricing fallback 补丁

## 方案对比

### 方案 A：续接当前半成品并按旧仓库补齐差异

优点：

- 最大限度保留当前仓库已有改动
- 与当前分支状态最兼容
- 冲突面最小，便于逐步验证

缺点：

- 需要逐项核对当前实现与旧仓库行为差异
- 收口过程比整块覆盖更细

### 方案 B：将旧仓库相关文件整块搬运后再适配

优点：

- 对齐旧仓库功能最快
- 行为最接近旧仓库现状

缺点：

- 容易覆盖当前仓库已有半成品改动
- 容易把无关依赖和样式一起带入
- 回归风险较高

### 方案 C：后端先行，前端后补

优点：

- 接口和数据层先稳定
- 便于独立验证数据库与服务层

缺点：

- 中间态更长
- 用户界面无法尽快形成闭环

## 结论

采用方案 A。

本次迁移把当前仓库视为迁移工作树，把旧仓库视为行为与结构参考源。在当前仓库已有半成品基础上补齐缺口、修正分叉，并以当前仓库的 wiring、路由、类型组织、样式体系为基准完成集成。

## 整体架构

迁移工作分为三条主线：

1. 定价主线
2. 号池监控主线
3. 账号管理增强主线

三条主线之间的关系如下：

- 定价主线相对独立，可单独形成公开页与后台管理闭环
- 号池监控主线依赖配置存储、监控服务、worker 接线和前端监控页面
- 账号管理增强主线为号池监控提供筛选、汇总与调度状态支撑

## 模块边界

### 后端

#### 定价相关

重点文件包括：

- `backend/internal/service/pricing_plan.go`
- `backend/internal/service/pricing_plan_service.go`
- `backend/internal/repository/pricing_plan_repo.go`
- `backend/internal/handler/admin/pricing_plan_handler.go`
- `backend/internal/handler/pricing_plan_handler.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/server/routes/pricing_plan.go`
- `backend/internal/handler/dto/mappers.go`
- `backend/internal/handler/dto/types.go`
- `backend/cmd/server/wire_gen.go`
- `backend/internal/service/wire.go`
- `backend/internal/repository/wire.go`

#### 号池监控相关

重点文件包括：

- `backend/internal/service/pool_monitor.go`
- `backend/internal/service/pool_monitor_service.go`
- `backend/internal/service/openai_pool_monitor_worker.go`
- `backend/internal/repository/pool_alert_config_repo.go`
- `backend/internal/repository/pool_alert_config_cache.go`
- `backend/internal/repository/proxy_transport_failure_counter.go`
- `backend/internal/handler/admin/pool_monitor_handler.go`
- `backend/internal/server/routes/admin.go`
- `backend/cmd/server/wire_gen.go`
- `backend/internal/service/wire.go`

`alert_service_old.go` 不作为正式迁移目标，优先接当前系统主链路。

#### 账号管理增强

仅迁移与监控依赖直接相关的增强：

- 账号筛选条件
- 账号摘要与汇总
- 调度状态相关字段
- 仓储查询与服务层补充逻辑

不做整个账号模块的无差别回搬。

### 前端

#### 定价相关

重点文件包括：

- `frontend/src/views/PricingView.vue`
- `frontend/src/views/admin/PricingPlansView.vue`
- `frontend/src/api/pricingPlans.ts`
- `frontend/src/api/admin/pricingPlans.ts`
- `frontend/src/router/index.ts`
- `frontend/src/types/index.ts`
- `frontend/src/i18n/locales/en.ts`
- `frontend/src/i18n/locales/zh.ts`

#### 号池监控相关

重点文件包括：

- `frontend/src/views/admin/PoolMonitorView.vue`
- `frontend/src/views/admin/PoolMonitorDashboardView.vue`
- `frontend/src/api/admin/poolMonitor.ts`
- `frontend/src/components/charts/ProxyUsageSummaryChart.vue`
- `frontend/src/components/charts/RequestCountTrend.vue`
- `frontend/src/router/index.ts`
- `frontend/src/types/index.ts`
- `frontend/src/i18n/locales/en.ts`
- `frontend/src/i18n/locales/zh.ts`

#### 账号管理增强

重点在当前账号管理页面与其依赖 API、类型、筛选配置上做增量补齐，不以旧仓库界面为目标进行重写。

## 数据兼容策略

### 数据库迁移

当前仓库已存在：

- `backend/migrations/079_pricing_plan_tables.sql`
- `backend/migrations/080_pool_monitor_tables.sql`

本次迁移以这两个脚本为基线核对字段、索引、默认值和约束，不回搬旧仓库早期 `028/029/030/032` 与 `038/042/043/046` 等迁移文件，避免编号冲突与重复建模。

### 注入链与路由

当前仓库已经存在部分 wiring 与路由注册，因此本次迁移将：

- 以当前仓库 wiring 为准补齐缺口
- 以当前仓库路由结构为准校准公开与后台入口
- 保持当前仓库 handler 聚合方式，不整套回退

### 前端类型与文案

前端优先保持当前仓库的：

- `router`
- `types`
- `i18n`
- 样式和布局组织

旧仓库字段与行为通过增量映射并入，不追求页面外观与旧仓库完全一致。

## 非目标

本次明确不包含以下内容：

1. `tools/breakeven-calculator` 及相关测试与说明
2. pricing fallback 文件路径修复及其测试补丁
3. 与上述两类功能绑定的文档、脚本和部署说明

## 测试与验收标准

### 定价相关

后端验收：

- 公开接口可返回定价分组与方案
- 后台接口可完成分组与方案的增删改查
- 价格、周期、排序、状态、购买联系方式等字段可正确保存与回显

前端验收：

- 公开定价页可正常展示分组与方案
- 后台定价页可正常完成 CRUD
- 路由、类型、文案可正常工作

### 号池监控相关

后端验收：

- 监控配置可读取与更新
- 默认值与参数校验正确
- 与 repo、cache、worker 的依赖链可编译并工作

前端验收：

- 监控配置页可读取与保存
- 监控总览页可展示摘要与图表
- 图表与接口字段匹配，不出现运行时错误

### 账号管理增强

验收重点：

- 筛选项可正确作用于查询
- 摘要与汇总字段返回正确
- 调度状态展示正确
- 不破坏当前仓库原有账号管理主流程

## 实施顺序

1. 收口定价后端
2. 收口定价前端
3. 收口号池监控后端
4. 补齐账号管理增强
5. 收口号池监控前端并联调
6. 统一执行测试与验收

## 风险与缓解

### 风险 1：当前仓库已有未提交改动，存在覆盖风险

缓解：

- 优先在现有文件上增量修改
- 修改前逐项比对旧仓库来源
- 不对无关文件做回退

### 风险 2：号池监控与账号管理增强耦合较深

缓解：

- 仅迁移监控直接依赖的数据维度与筛选能力
- 以最小依赖闭环为目标

### 风险 3：迁移脚本历史与旧仓库不一致

缓解：

- 以当前仓库 `079/080` 为准
- 不回搬旧仓库早期编号脚本

### 风险 4：wiring 与服务依赖存在分叉

缓解：

- 以当前仓库 wiring 为集成基线
- 每补齐一段依赖链就做编译与定向验证

### 风险 5：前端已有半成品与旧仓库 UI 不完全一致

缓解：

- 优先保证行为一致
- 保持当前仓库的页面风格与组织方式

## 步骤记录约定

按照用户要求，迁移过程中每完成一个子步骤，都要记录：

- 已完成
- 进行中
- 待做

确保后续执行和中断恢复时可以快速定位当前进度。

## 本次迁移落地结果（2026-03-25）

- 实际迁移完成范围：
  - 定价后端与前端
  - 号池监控后端与前端
  - 为监控闭环补齐的账号管理高级筛选、摘要汇总与调度状态能力
- 明确保留不迁移：
  - `tools/breakeven-calculator`
  - pricing fallback 补丁及其相关测试、脚本、文档
- 最终边界保持不变：
  - 以当前仓库 wiring、路由、handler 组织为准做增量并入
  - 保留当前仓库 UI 结构与样式组织，不追求旧仓库页面外观完全一致
- 验收结果：
  - focused backend/frontend 测试全部通过
  - 前端 `typecheck` 与 `build` 通过
  - 后端 `go test ./cmd/server/...` 通过
  - 构建阶段仅存在既有 chunk size warning，未发现阻断性问题
- 当前收口状态：
  - 功能与验证已完成
  - 提交仍需结合当前混合工作区做人工切分，避免覆盖或夹带无关改动
