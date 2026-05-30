# 运营大盘设计

## 背景

当前管理端已有 `/admin/dashboard`、`/admin/usage` 和 `/admin/orders/dashboard` 等页面，主要覆盖系统运行、用量、模型、账号、订单和支付概览。这些页面更接近后台统计和运维监控视角。

本次要新增的是 AI 中转站的增长运营决策中心，用来回答：

- 用户从哪里来？
- 用户是否激活并留存？
- 用户是否付费和复购？
- 哪些功能被真实使用？
- 哪些指标需要后续进入价值、流失、模型贡献和预警闭环？

因此本设计新增独立管理端菜单「运营大盘」，不替换现有后台总览。

## 目标

1. 新增管理端「运营大盘」页面，路由为 `/admin/growth`。
2. V1 聚焦核心经营指标、用户增长、留存分析、充值转化、功能使用排行。
3. 每个图表或卡片使用独立读接口，便于单独加载、缓存、重试和优化。
4. 所有日期统计固定使用 `Asia/Shanghai` 口径，不在页面暴露时区选择器。
5. 全程只展示聚合统计，不展示 prompt、AI 回复、上传文件、图片或聊天内容。

## 非目标

- 不替换现有 `/admin/dashboard`。
- 不重构现有用量、支付、订单页面。
- V1 不实现用户价值中心、流失分析、模型贡献、运营预警、自动召回、用户标签系统。
- V1 不新增普通运营、高级运营、超级管理员的细粒度运营权限；仅复用现有 admin 权限。
- V1 不用弱推断展示平均会话轮数或会话时长；缺少 session 标识时显示未接入。
- V1 不补齐完整前端行为埋点系统；缺少充值页行为事件时，充值漏斗先使用订单状态口径。

## 信息架构

### 菜单与路由

- 菜单名：运营大盘
- 前端路由：`/admin/growth`
- 路由名：`AdminGrowthDashboard`
- 权限：`requiresAuth: true`、`requiresAdmin: true`
- 页面标题 i18n key：`admin.growth.title`

### 页面布局

页面使用现有 `AppLayout`，保持后台工具页风格，不做营销化 hero。

顶部区域：

- 页面标题和简短说明
- 全局日期范围选择器
- 粒度选择器：`day`、`week`、`month`
- 刷新按钮

主体区域：

1. 核心经营指标
2. 用户增长
3. 留存分析
4. 充值转化
5. 功能使用排行

V2/V3 预留但不展示空模块：

- 用户价值中心
- 流失分析中心
- 模型贡献分析
- 运营预警中心
- 自动召回策略
- 用户标签系统

## 数据口径

### 通用日期口径

- 所有接口按 `Asia/Shanghai` 解析 `start_date` 和 `end_date`。
- `start_date` 表示北京时间当天 00:00:00。
- `end_date` 表示北京时间当天 23:59:59.999999999。
- 趋势粒度：
  - `day`：按北京时间自然日分桶。
  - `week`：按北京时间周一到周日分桶。
  - `month`：按北京时间自然月分桶。
- 默认范围：最近 30 天。
- 趋势查询最大范围：180 天。
- 留存矩阵默认最多 30 个 cohort 日期。

### 支付成功口径

沿用现有支付服务的付费状态口径：

- `PAID`
- `RECHARGING`
- `COMPLETED`

收入金额使用 `payment_orders.pay_amount`。V1 展示成功支付总额，不做退款净额抵扣；退款相关分析后续单独设计。

### 核心经营指标

指标：

- 总用户数：`users` 累计注册数，排除软删除用户。
- DAU：当天有 `usage_logs` 行为的去重 `user_id`。
- MAU：当月有 `usage_logs` 行为的去重 `user_id`。
- 今日新增：北京时间当天 `users.created_at` 新增用户数。
- 今日付费用户：北京时间当天成功支付订单的去重 `user_id`。
- 本月收入：北京时间本月成功支付订单 `pay_amount` 之和。
- ARPU：选中周期收入 / 选中周期活跃用户数；分母为 0 时返回 0。
- 付费转化率：选中周期付费用户数 / 选中周期活跃用户数；分母为 0 时返回 0。
- 复购率：选中周期内成功支付次数大于等于 2 的用户数 / 选中周期付费用户数；分母为 0 时返回 0。

### 用户增长

图表：

- 新增用户趋势：新增注册、新增激活、新增付费。
- 渠道来源分析：渠道和注册用户数。
- 渠道付费率：渠道、注册人数、付费人数、转化率。

口径：

- 新增注册：按 `users.created_at` 统计。
- 新增激活：注册后首次产生 `usage_logs` 的用户。
- 新增付费：注册后首次成功支付的用户。
- 渠道来源：V1 当前表结构没有稳定注册来源字段，接口和前端先支持 `source` 字段；缺失时统一归为 `Unknown`。

### 留存分析

图表：

- 留存矩阵：D1、D3、D7、D15、D30。
- 留存趋势：D1、D7、D30 长期变化。

口径：

- cohort 日期：用户注册日期。
- 留存行为：再次产生 `usage_logs`。
- D1/D3/D7/D15/D30 使用精确第 N 天口径。
- 例如 D7 表示注册后第 7 个北京时间自然日有行为，不表示 7 天内任意一天回访。

### 充值转化

图表：

- 充值漏斗：V1 为创建订单到支付成功。
- 套餐分析：套餐销量和收入。
- 首次付费分析：注册后 1 天内、7 天内、30 天内、30 天以上。

口径：

- 创建订单：`payment_orders.created_at` 在选中周期内的订单数。
- 支付成功：订单状态为 `PAID`、`RECHARGING` 或 `COMPLETED`，且 `paid_at` 在选中周期内。
- 套餐分类：
  - 订阅订单优先使用 `plan_id` 和订阅计划信息。
  - 余额充值订单归为积分包或余额充值。
  - 无法匹配的计划归为其他套餐。
- 首次付费：每个用户第一笔成功支付订单的 `paid_at` 与该用户 `created_at` 的北京时间日期差。
- 完整漏斗的进入充值页、查看套餐、点击支付需要后续新增行为埋点；V1 不伪造这些阶段。

### 功能使用排行

图表：

- 功能排行：功能分类和使用次数。
- 用户占比：使用过某功能的用户数占活跃用户数比例。
- 会话指标：平均对话轮数、平均会话时长、平均输入 token、平均输出 token。

口径：

- 功能分类从 `usage_logs` 的 `request_type`、`billing_mode`、`image_count`、模型和路径特征推导。
- 能明确识别的类别包括聊天、绘图、文件分析、翻译、联网搜索。
- 不能明确识别的请求归为 `Other`。
- 平均输入 token：选中周期 `input_tokens` 平均值。
- 平均输出 token：选中周期 `output_tokens` 平均值。
- 平均对话轮数和平均会话时长在现有表缺少 session 标识时返回 `available: false`，前端显示未接入。

## API 设计

### 选型

使用 REST 读接口。接口均为 admin 内部 API，不是 public API。所有端点复用现有 admin 鉴权和 response envelope。

采用图表级接口，而不是页面聚合接口。原因：

- 单个图表慢查询不会阻塞整个页面。
- 前端可以单图 loading、失败重试和空态展示。
- 后端后续可以按接口加缓存、索引或预聚合。
- V2/V3 扩展时不会让单一响应 DTO 变成大对象。

### 通用查询参数

适用于大多数接口：

```text
start_date=2026-05-01
end_date=2026-05-30
granularity=day|week|month
```

规则：

- `start_date` 和 `end_date` 可省略，省略时默认最近 30 天。
- `granularity` 可省略，省略时默认为 `day`。
- 后端固定 `Asia/Shanghai`，不接受 `timezone` 参数。
- 非法日期返回 400。
- `start_date` 晚于 `end_date` 返回 400。
- 查询范围超过 180 天返回 400。
- 非法 `granularity` 返回 400。

### 端点清单

#### `GET /admin/growth/overview`

返回核心经营指标。

响应字段：

```json
{
  "total_users": 12000,
  "dau": 860,
  "mau": 5200,
  "today_new_users": 120,
  "today_paid_users": 35,
  "month_revenue": 98650.75,
  "arpu": 18.97,
  "payment_conversion_rate": 0.0673,
  "repurchase_rate": 0.2381
}
```

#### `GET /admin/growth/users/trend`

返回新增注册、新增激活、新增付费趋势。

响应字段：

```json
{
  "series": [
    {
      "date": "2026-05-01",
      "new_registered": 42,
      "new_activated": 31,
      "new_paid": 8
    }
  ]
}
```

#### `GET /admin/growth/users/sources`

返回渠道来源注册数。

响应字段：

```json
{
  "items": [
    {
      "source": "Unknown",
      "users": 1200
    }
  ]
}
```

#### `GET /admin/growth/users/source-payment-rates`

返回渠道付费率。

响应字段：

```json
{
  "items": [
    {
      "source": "Unknown",
      "registered_users": 1200,
      "paid_users": 180,
      "conversion_rate": 0.15
    }
  ]
}
```

#### `GET /admin/growth/retention/matrix`

返回留存矩阵。

响应字段：

```json
{
  "columns": ["D1", "D3", "D7", "D15", "D30"],
  "cohorts": [
    {
      "date": "2026-05-01",
      "new_users": 42,
      "retention": {
        "D1": 0.42,
        "D3": 0.31,
        "D7": 0.22,
        "D15": 0.16,
        "D30": 0.11
      }
    }
  ]
}
```

#### `GET /admin/growth/retention/trend`

返回 D1、D7、D30 留存趋势。

响应字段：

```json
{
  "series": [
    {
      "date": "2026-05-01",
      "d1": 0.42,
      "d7": 0.22,
      "d30": 0.11
    }
  ]
}
```

#### `GET /admin/growth/payments/funnel`

返回 V1 充值漏斗。

响应字段：

```json
{
  "steps": [
    {
      "key": "order_created",
      "label": "创建订单",
      "users": 320,
      "count": 410,
      "conversion_rate": 1
    },
    {
      "key": "payment_success",
      "label": "支付成功",
      "users": 210,
      "count": 260,
      "conversion_rate": 0.6341
    }
  ],
  "tracking_ready": false
}
```

`tracking_ready=false` 表示还没有完整前端行为埋点。

#### `GET /admin/growth/payments/plans`

返回套餐销量和收入。

响应字段：

```json
{
  "items": [
    {
      "plan_id": 1,
      "plan_name": "月卡",
      "category": "monthly",
      "sales": 80,
      "revenue": 2392
    }
  ]
}
```

#### `GET /admin/growth/payments/first-payment`

返回首次付费分布。

响应字段：

```json
{
  "items": [
    {
      "bucket": "within_1_day",
      "label": "1天内",
      "users": 62,
      "ratio": 0.31
    }
  ]
}
```

#### `GET /admin/growth/features/ranking`

返回功能使用排行和用户占比。

响应字段：

```json
{
  "items": [
    {
      "feature": "chat",
      "label": "聊天",
      "uses": 12000,
      "users": 860,
      "user_ratio": 0.78
    }
  ]
}
```

#### `GET /admin/growth/features/session-metrics`

返回会话和 token 指标。

响应字段：

```json
{
  "average_turns": {
    "available": false,
    "value": 0
  },
  "average_session_duration_seconds": {
    "available": false,
    "value": 0
  },
  "average_input_tokens": {
    "available": true,
    "value": 812.5
  },
  "average_output_tokens": {
    "available": true,
    "value": 1260.2
  }
}
```

## 后端结构

### 路由

修改：

- `backend/internal/server/routes/admin.go`

新增：

- `registerGrowthRoutes(admin, h)`
- 路由挂载在 admin group 下：`/admin/growth/*`

### Handler

新增：

- `backend/internal/handler/admin/growth_handler.go`

职责：

- 解析日期和粒度参数。
- 固定加载 `Asia/Shanghai` location。
- 调用 Growth service。
- 映射响应 DTO。
- 统一返回现有 response envelope。

Handler 不写 SQL，不承载业务口径。

### Service

新增：

- `backend/internal/service/growth_service.go`

职责：

- 定义 Growth Dashboard 的业务口径。
- 组合用户、用量、支付数据。
- 计算比例、ARPU、复购率、留存率。
- 处理空分母和不可用指标。
- 将缺失渠道归为 `Unknown`。

### Repository

新增：

- `backend/internal/repository/growth_repo.go`

职责：

- 读取 `users`、`usage_logs`、`payment_orders`。
- 按图表接口提供专用查询。
- 保持查询函数可单测。

### Wire

修改现有依赖注入文件：

- `backend/internal/repository/wire.go`
- `backend/internal/service/wire.go`
- `backend/internal/handler/handler.go`
- `backend/internal/handler/wire.go`
- `backend/cmd/server/wire.go`
- `backend/cmd/server/wire_gen.go`

其中 `backend/cmd/server/wire_gen.go` 由 Wire 生成，实施阶段通过现有生成流程更新，不手写业务逻辑。

## 前端结构

新增：

- `frontend/src/api/admin/growth.ts`
- `frontend/src/views/admin/GrowthDashboardView.vue`
- `frontend/src/components/admin/growth/GrowthKpiCards.vue`
- `frontend/src/components/admin/growth/GrowthUserTrendChart.vue`
- `frontend/src/components/admin/growth/GrowthRetentionMatrix.vue`
- `frontend/src/components/admin/growth/GrowthPaymentFunnel.vue`
- `frontend/src/components/admin/growth/GrowthFeatureRanking.vue`

修改：

- `frontend/src/api/admin/index.ts`
- `frontend/src/router/index.ts`
- `frontend/src/components/layout/AppSidebar.vue`
- `frontend/src/i18n/locales/zh.ts`
- `frontend/src/i18n/locales/en.ts`

前端职责：

- 页面级日期和粒度状态。
- 每个图表独立请求、独立 loading、独立错误态。
- 只做格式化和空态展示，不在前端重算核心业务口径。
- 对 `available=false` 的指标显示未接入。

## 权限与隐私

### 权限

V1 仅 admin 可见：

- 前端路由使用 `requiresAdmin: true`。
- 后端复用 admin middleware。
- 不新增运营角色体系。

### 隐私

禁止响应：

- prompt 原文。
- AI 回复内容。
- 上传文件内容。
- 图片内容。
- 聊天记录。
- 第三方原始错误和调试细节。

允许响应：

- 功能分类。
- 模型或渠道分类。
- token 统计。
- 行为聚合。
- 留存统计。
- 收入统计。
- 用户计数。

V1 不展示 TOP 用户排行，避免提前扩大个人信息展示面。

## 性能与可观测性

- 每个图表一个接口，便于单接口缓存和慢查询优化。
- V1 查询范围限制为 180 天。
- 留存矩阵限制 cohort 数量为 30。
- 查询默认不返回精确总数之外的明细列表。
- 后续如发现慢查询，再针对具体接口增加索引或预聚合。
- 每个 handler 错误分支应保留 request id 和结构化日志，遵循现有错误处理方式。

## 兼容性

- 新增路由和接口，不改变现有接口契约。
- 新增前端菜单，不删除旧菜单。
- 新增响应字段不会影响旧客户端。
- 后续如果为渠道来源或充值漏斗新增埋点字段，应作为新数据源接入，不改变 V1 已定义字段含义。

## 验收标准

### 后端

- `/api/v1/admin/growth/*` 端点均需要 admin 权限。
- 未登录访问返回现有 401 行为。
- 非 admin 访问返回现有 403 或重定向行为。
- 日期按 `Asia/Shanghai` 解析。
- 非法日期、非法粒度、超范围日期返回 400。
- 分母为 0 的比例返回 0，不返回 NaN 或 Infinity。
- 渠道来源缺失时返回 `Unknown`。
- 充值漏斗在未接入行为埋点时返回 `tracking_ready=false`。
- 缺少 session 口径时会话指标返回 `available=false`。
- 响应不包含 prompt、回复、文件、图片、聊天内容字段。

### 前端

- 管理端侧边栏出现「运营大盘」菜单。
- `/admin/growth` 仅 admin 可访问。
- 页面展示全局日期范围和粒度选择。
- 每个图表独立 loading、空态、错误态。
- KPI 卡片、增长趋势、留存矩阵、充值漏斗、功能排行在 mock 或测试数据下可正常渲染。
- `available=false` 的会话指标展示未接入，不展示误导性 0 值。

### 测试建议

后端：

- `go test ./internal/service -run Growth -count=1`
- `go test ./internal/handler/admin -run Growth -count=1`
- `go test ./internal/server/routes -run Growth -count=1`

前端：

- `pnpm -C frontend test:run -- Growth`
- `pnpm -C frontend typecheck`
