# Trace ID 全链路追踪改造设计

## 背景

当前系统已经在 Nginx 层具备 `X-Trace-ID` 的基础能力：

- `deploy/frontend/nginx.conf` 会优先透传客户端传入的 `X-Trace-ID`
- 若客户端未传入，则使用 Nginx 的 `$request_id` 兜底生成 `trace_id`
- 代理请求时已向后端透传 `X-Trace-ID`
- 响应头已通过 `add_header X-Trace-ID $trace_id always;` 返回给客户端

但现状仍有三个缺口：

1. 后端请求上下文和大多数业务日志尚未统一携带 `trace_id`
2. 管理端“使用记录页”尚未展示 `trace_id`
3. 面向用户的错误响应尚未显式返回 `trace_id`

这导致排障时仍需要在 Nginx、后端日志、数据库记录之间做人工拼接，无法通过一个标识直接串起整条链路。

## 目标

将 `trace_id` 作为系统主关联键，满足以下能力：

1. 用户请求进入系统后，`trace_id` 贯穿 `Nginx -> 后端入口 -> 后端业务日志 -> 内部处理记录 -> 错误响应`
2. 运维或研发拿到任意一个 `trace_id`，可检索：
   - Nginx access/error 日志
   - 后端结构化日志
   - 管理端使用记录页中的对应记录
3. 用户收到错误响应时，可直接从响应头或响应体中获取 `trace_id`
4. 现有 `request_id` 保留，但语义明确为“上游请求 ID”，仅用于对接上游厂商排障、报错或工单

## 字段语义

本次改造后，链路内字段语义固定如下。

### `trace_id`

系统全链路追踪 ID。

用途：

- 串联 Nginx 与后端的同一次请求
- 关联同一请求产生的业务日志
- 关联使用记录
- 返回给用户用于报障

规则：

- 优先使用客户端传入的 `X-Trace-ID`
- 若缺失，则由 Nginx 生成
- 若请求绕过 Nginx 直接到达后端且请求头为空，则后端兜底生成

### `request_id`

上游请求 ID。

用途：

- 查询上游厂商的请求明细
- 提交上游错误工单
- 与上游错误响应或审计日志对齐

规则：

- 保持现有使用方式不变
- 在管理端明确标注为“上游 Request ID”或等价中文文案，避免与系统内部链路 ID 混淆

### 非目标：`gateway_request_id`

当前阶段不把 `gateway_request_id` 作为主要展示或排障字段。

原因：

- 用户和运维的主排障入口已统一为 `trace_id`
- 额外展示网关内部请求 ID 会增加理解成本
- 如后端内部仍需保留 `request_id` 作为现有入口请求 ID，可继续内部使用，但不作为本次面向用户的核心字段

## 现状梳理

### Nginx

已存在如下配置：

- `map $http_x_trace_id $trace_id`
- `proxy_set_header X-Trace-ID $trace_id`
- `add_header X-Trace-ID $trace_id always`
- access log 中已打印 `trace=$trace_id`

当前 access log 中还包含：

- `request_id=$upstream_http_x_request_id`

该字段更接近“上游响应头中的 request id”，与本设计中的 `request_id` 语义一致，但日志字段名容易和系统入口请求 ID 混淆。

### 后端中间件

`backend/internal/server/middleware/request_logger.go` 当前会：

- 从 `X-Request-ID` 获取或生成请求 ID
- 将其写入响应头
- 将 `request_id`、`client_request_id`、`path`、`method` 注入请求级 logger

缺口：

- 没有从 `X-Trace-ID` 读取或生成 `trace_id`
- 没有将 `trace_id` 注入 context / logger / 错误响应

### 使用记录

当前 `usage_logs` 已有 `request_id` 字段，对应现有业务/上游请求 ID。

缺口：

- 没有 `trace_id` 字段
- 管理端使用记录页没有清晰展示 `trace_id` 与“上游 request_id”的区分

### 错误响应

当前统一响应封装位于：

- `backend/internal/pkg/response/response.go`
- `backend/internal/pkg/errors/http.go`

缺口：

- 错误响应结构中没有显式带出 `trace_id`

## 目标架构

### 1. Nginx 层

请求进入 Nginx 时：

1. 优先读取客户端的 `X-Trace-ID`
2. 若不存在，则使用 Nginx `$request_id`
3. 将最终值保存为 `$trace_id`
4. 将 `$trace_id`：
   - 打入 access log
   - 透传至后端请求头 `X-Trace-ID`
   - 写入响应头 `X-Trace-ID`

建议对 access log 字段名做轻微调整：

- 保留 `trace=$trace_id`
- 将 `request_id=$upstream_http_x_request_id` 调整为 `upstream_request_id=$upstream_http_x_request_id`

这样日志语义更直观，不会和系统内部的请求 ID 混淆。

### 2. 后端入口层

新增 `TraceID` 上下文键，并在 HTTP 入口中间件统一处理：

1. 从请求头 `X-Trace-ID` 读取
2. 若为空则后端兜底生成
3. 将 `trace_id` 写回响应头 `X-Trace-ID`
4. 将 `trace_id` 写入：
   - `request.Context()`
   - request-scoped logger
   - 可选的 `gin.Context` 快捷存储

统一要求后续业务日志尽量通过 `logger.FromContext(ctx)` 或同类上下文 logger 输出，确保自动继承 `trace_id`。

### 3. 业务日志层

请求级 logger 默认字段扩展为：

- `trace_id`
- `request_id`（保留现有入口/内部请求 ID 逻辑）
- `client_request_id`
- `path`
- `method`

效果：

- 同一次 HTTP 请求的绝大多数结构化日志天然带 `trace_id`
- 日志查询时可以直接以 `trace_id=...` 聚合整条链路

说明：

并非所有历史日志调用都一定从上下文 logger 输出，因此本次改造的落点是“打通统一链路并覆盖主路径”，而不是承诺一次性清零所有散落日志调用。

### 4. 使用记录落库

为 `usage_logs` 新增：

- `trace_id VARCHAR(64)`

并建立普通索引：

- `INDEX usage_logs_trace_id(trace_id)`

写 usage log 时：

- 从上下文读取 `trace_id`
- 与现有 `request_id` 一起落库

改造后的含义：

- `usage_logs.trace_id`：本系统全链路追踪 ID
- `usage_logs.request_id`：上游请求 ID

### 5. 错误响应

所有对外错误响应在保持当前 envelope 兼容的前提下，追加 `trace_id`。

建议策略：

- 优先把 `trace_id` 放入响应体顶层
- 同时继续通过响应头返回 `X-Trace-ID`

建议形态：

```json
{
  "code": 500,
  "message": "internal server error",
  "reason": "INTERNAL_ERROR",
  "metadata": {
    "...": "..."
  },
  "trace_id": "..."
}
```

若为了尽量减少前端兼容风险，不方便新增顶层字段，则可以作为 `metadata.trace_id` 返回。但推荐顶层字段，因为：

- 更容易被前端和第三方客户端直接读取
- 用户截图和人工报障时更醒目

本设计推荐直接新增顶层 `trace_id` 字段，并同步保持响应头 `X-Trace-ID`。

### 6. 管理端使用记录页

管理员“使用记录页”新增两个展示字段：

- `trace_id`
- `request_id`

展示语义：

- `trace_id`：用于查询 Nginx、后端日志与系统内部处理链路
- `request_id`：用于查询上游厂商请求或提交上游工单

建议文案：

- `Trace ID`
- `Upstream Request ID` / `上游 Request ID`

行为要求：

- 支持列显隐
- 支持导出
- 前后端类型、DTO、API 返回结构同步更新

## 详细改造点

### A. Nginx 配置

文件：

- `deploy/frontend/nginx.conf`

改造点：

1. 保留现有 `map $http_x_trace_id $trace_id`
2. 保留所有 `proxy_set_header X-Trace-ID $trace_id`
3. 保留 `add_header X-Trace-ID $trace_id always`
4. 优化 access log 中上游请求 ID 的字段命名，建议：

```nginx
trace=$trace_id upstream_request_id=$upstream_http_x_request_id
```

### B. 后端 context key

文件：

- `backend/internal/pkg/ctxkey/ctxkey.go`

改造点：

1. 新增 `TraceID` context key
2. 注释明确其为“全链路追踪 ID”

### C. HTTP 请求中间件

主要文件：

- `backend/internal/server/middleware/request_logger.go`

改造点：

1. 读取 `X-Trace-ID`
2. 缺失时生成兜底值
3. 将 `trace_id` 注入 context
4. 将 `trace_id` 添加到 request-scoped logger 字段
5. 将 `X-Trace-ID` 写回响应头

可选项：

- 若存在访问日志中间件、panic/recovery 中间件，也应确认其能拿到同一 `trace_id`

### D. 统一错误响应

主要文件：

- `backend/internal/pkg/response/response.go`
- 相关错误封装与中间件

改造点：

1. `Response` 结构新增 `trace_id` 字段
2. `Error` / `ErrorWithDetails` / `ErrorFrom` 自动从请求上下文提取 `trace_id`
3. 所有统一错误返回自动携带 `trace_id`

注意：

- 成功响应不强制在 body 中附带 `trace_id`
- 成功与失败都继续通过响应头返回 `X-Trace-ID`

### E. 使用记录模型与存储

可能涉及文件：

- `backend/ent/schema/usage_log.go`
- `backend/internal/service/usage_log.go`
- `backend/internal/repository/usage_log_repo.go`
- 相关 SQL migration

改造点：

1. `usage_logs` schema 增加 `trace_id`
2. repository 的 `SELECT` 列表、`INSERT` 列表、scan 顺序补上 `trace_id`
3. `service.UsageLog` 增加 `TraceID`
4. usage log 创建链路从上下文读 `trace_id` 并落库

### F. DTO / API / 前端类型

可能涉及文件：

- `backend/internal/handler/dto/types.go`
- `backend/internal/handler/dto/mappers.go`
- `frontend/src/types/index.ts`
- `frontend/src/api/admin/usage.ts`

改造点：

1. admin usage DTO 增加 `trace_id`
2. 前端 `UsageLog` / `AdminUsageLog` 类型增加 `trace_id`
3. 保留 `request_id`
4. `request_id` 在页面语义上标为“上游 Request ID”

### G. 管理端使用记录页

可能涉及文件：

- `frontend/src/views/admin/UsageView.vue`
- `frontend/src/components/admin/usage/UsageTable.vue`

改造点：

1. 新增 `trace_id` 列
2. 新增 `request_id` 列，文案改为上游语义
3. 列支持显隐
4. 导出逻辑补上两个字段

## 测试策略

### 后端

1. 中间件测试
- 有 `X-Trace-ID` 时，context/logger/响应头使用传入值
- 无 `X-Trace-ID` 时，后端兜底生成并写回响应头

2. 错误响应测试
- 统一错误响应 body 包含 `trace_id`
- 错误响应头包含 `X-Trace-ID`

3. usage log 仓储 / DTO 测试
- `trace_id` 能成功写入和读取
- admin DTO 正确返回 `trace_id`

4. 主链路集成测试
- 发起请求后，能在响应头拿到 `X-Trace-ID`
- usage log 中能看到同一 `trace_id`

### 前端

1. 使用记录页渲染测试
- 新增列可见
- `trace_id` / `request_id` 正确显示

2. 导出测试
- 导出结果包含新字段

## 风险与边界

### 1. 历史日志调用不一定全部走 context logger

风险：

- 个别旧日志调用可能仍不自动带 `trace_id`

策略：

- 优先打通统一入口
- 对关键业务链路按需补齐
- 不在本次设计中做与目标无关的全仓库日志重构

### 2. 绕过 Nginx 的访问路径

风险：

- 开发环境、健康检查或内部调用可能不经过 Nginx

策略：

- 后端保留 `trace_id` 兜底生成逻辑

### 3. 错误响应结构兼容性

风险：

- 少数客户端可能对错误响应结构有严格假设

策略：

- 在现有 envelope 上增量添加 `trace_id`
- 不调整已有字段的名称与层级

### 4. 历史 usage 数据没有 `trace_id`

风险：

- 老数据无法按 `trace_id` 查询

策略：

- 接受历史记录为空
- 页面展示允许 `trace_id` 为空
- 不做历史回填

## 实施顺序

推荐顺序如下：

1. 后端入口中间件打通 `trace_id`
2. 统一错误响应追加 `trace_id`
3. `usage_logs` 增加 `trace_id` 并完成写入/读取
4. 管理端使用记录页展示 `trace_id` 与 `request_id`
5. 调整 Nginx access log 字段名，统一语义
6. 补齐测试

## 验收标准

以下条件全部满足即视为完成：

1. 任意经 Nginx 进入的请求，响应头包含 `X-Trace-ID`
2. 后端同一请求主路径日志可通过 `trace_id` 聚合检索
3. 错误响应体中包含 `trace_id`
4. 新产生的 usage log 记录持久化 `trace_id`
5. 管理端“使用记录页”能展示 `trace_id` 与“上游 request_id”
6. 用户可仅凭 `trace_id` 完成本系统内的全链路排障

## 非目标

本次不包含以下内容：

1. 对所有历史 usage 记录回填 `trace_id`
2. 对全仓库所有旧日志调用做统一重构
3. 建设完整分布式 tracing 平台（如 OpenTelemetry/Jaeger/Tempo）
4. 将 `request_id` 全面替换为 `trace_id`

当前目标是先用最小可落地改造，把“单个 `trace_id` 查询 Nginx + 后端日志 + 使用记录 + 错误响应”打通。
