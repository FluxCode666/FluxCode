# Trace ID 全链路追踪改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 打通 `trace_id` 在 Nginx、后端日志、错误响应和管理端使用记录页中的全链路传播与展示，并保留 `request_id` 作为上游请求 ID。

**Architecture:** 以 Nginx 的 `X-Trace-ID` 为主链路标识，后端入口中间件统一注入 `context` 与 request-scoped logger；错误响应从上下文提取并返回 `trace_id`；`usage_logs` 新增 `trace_id` 持久化字段并通过管理员使用记录页展示。现有 `request_id` 保持为上游请求 ID，不改变语义，只优化展示文案。

**Tech Stack:** Nginx、Go、Gin、Zap/logger、Ent schema + SQL migration、Vue 3、Vitest

---

## File Map

### Backend

- Modify: `deploy/frontend/nginx.conf`
  - 统一 access log 中上游请求 ID 的字段命名，避免与系统入口请求 ID 混淆。
- Modify: `backend/internal/pkg/ctxkey/ctxkey.go`
  - 新增 `TraceID` 上下文键。
- Modify: `backend/internal/server/middleware/request_logger.go`
  - 读取/生成 `trace_id`，写入 context、logger、响应头。
- Modify: `backend/internal/server/middleware/request_access_logger_test.go`
  - 为 `trace_id` 传播与 access log 字段补测试。
- Modify: `backend/internal/pkg/response/response.go`
  - 错误响应追加 `trace_id`。
- Modify: `backend/internal/pkg/errors/http.go`
  - 如需对 metadata 拷贝逻辑做兼容补充，在此调整。
- Add: `backend/migrations/114_add_usage_log_trace_id.sql`
  - 为 `usage_logs` 增加 `trace_id` 字段与索引。
- Modify: `backend/ent/schema/usage_log.go`
  - Ent schema 新增 `trace_id` 字段和索引。
- Modify: `backend/internal/service/usage_log.go`
  - `service.UsageLog` 增加 `TraceID` 字段。
- Modify: `backend/internal/repository/usage_log_repo.go`
  - 查询列、插入列、scan 顺序增加 `trace_id`。
- Modify: `backend/internal/repository/usage_log_repo_request_type_test.go`
  - 插入参数、查询列测试增加 `trace_id`。
- Modify: `backend/internal/handler/dto/types.go`
  - Admin usage DTO 增加 `trace_id`。
- Modify: `backend/internal/handler/dto/mappers.go`
  - usage DTO 映射增加 `trace_id`。
- Modify: `backend/internal/handler/dto/mappers_usage_test.go`
  - usage DTO 测试增加 `trace_id` 断言。
- Modify: `backend/internal/service/*gateway*_record_usage*.go` and/or the concrete usage-log creation call sites discovered during implementation
  - 从上下文提取 `trace_id`，写入 usage log。

### Frontend

- Modify: `frontend/src/types/index.ts`
  - `UsageLog` / `AdminUsageLog` 类型增加 `trace_id`。
- Modify: `frontend/src/views/admin/UsageView.vue`
  - 列配置增加 `trace_id` 与 `request_id`，并更新导出字段。
- Modify: `frontend/src/components/admin/usage/UsageTable.vue`
  - 增加 `trace_id` / `request_id` 单元格渲染。
- Modify: `frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
  - 表格新增标识列展示测试。
- Modify: `frontend/src/views/admin/__tests__/UsageView.spec.ts`
  - 列配置与导出字段补测试。
- Modify: `frontend/src/i18n/locales/zh.ts`
  - 增加 `trace_id` / `上游 request_id` 文案。
- Modify: `frontend/src/i18n/locales/en.ts`
  - 增加英文文案。

## Task 1: 打通后端入口 `trace_id` 上下文与日志字段

**Files:**
- Modify: `backend/internal/pkg/ctxkey/ctxkey.go`
- Modify: `backend/internal/server/middleware/request_logger.go`
- Test: `backend/internal/server/middleware/request_access_logger_test.go`

- [ ] **Step 1: 写 `trace_id` 中间件行为的失败测试**

```go
func TestRequestLogger_PropagatesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/t", func(c *gin.Context) {
		traceID, _ := c.Request.Context().Value(ctxkey.TraceID).(string)
		if traceID != "trace-fixed" {
			t.Fatalf("trace_id=%q, want trace-fixed", traceID)
		}
		if got := c.Writer.Header().Get("X-Trace-ID"); got != "trace-fixed" {
			t.Fatalf("header=%q, want trace-fixed", got)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("X-Trace-ID", "trace-fixed")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/server/middleware -run 'TestRequestLogger_PropagatesTraceID|TestRequestLogger_GenerateAndPropagateRequestID'`

Expected: `TestRequestLogger_PropagatesTraceID` FAIL，提示 `ctxkey.TraceID` 未定义或上下文/响应头不包含 `trace_id`

- [ ] **Step 3: 最小实现 `TraceID` context key 与中间件注入**

```go
// backend/internal/pkg/ctxkey/ctxkey.go
const (
	RequestID Key = "ctx_request_id"
	TraceID   Key = "ctx_trace_id"
)
```

```go
// backend/internal/server/middleware/request_logger.go
const (
	requestIDHeader = "X-Request-ID"
	traceIDHeader   = "X-Trace-ID"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}

		requestID := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		traceID := strings.TrimSpace(c.GetHeader(traceIDHeader))
		if traceID == "" {
			traceID = uuid.NewString()
		}

		c.Header(requestIDHeader, requestID)
		c.Header(traceIDHeader, traceID)

		ctx := context.WithValue(c.Request.Context(), ctxkey.RequestID, requestID)
		ctx = context.WithValue(ctx, ctxkey.TraceID, traceID)
		clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string)

		requestLogger := logger.With(
			zap.String("component", "http"),
			zap.String("request_id", requestID),
			zap.String("trace_id", traceID),
			zap.String("client_request_id", strings.TrimSpace(clientRequestID)),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)

		ctx = logger.IntoContext(ctx, requestLogger)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
```

- [ ] **Step 4: 扩展 access log 测试断言 `trace_id` 字段**

```go
func TestLogger_AccessLogIncludesCoreFields(t *testing.T) {
	// ...
	req.Header.Set("X-Trace-ID", "trace-access")
	// ...
	if event.Fields["trace_id"] != "trace-access" {
		t.Fatalf("trace_id mismatch: %+v", event.Fields)
	}
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `cd backend && go test ./internal/server/middleware`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/pkg/ctxkey/ctxkey.go \
  backend/internal/server/middleware/request_logger.go \
  backend/internal/server/middleware/request_access_logger_test.go
git commit -m "feat: propagate trace id in request middleware"
```

## Task 2: 统一错误响应返回 `trace_id`

**Files:**
- Modify: `backend/internal/pkg/response/response.go`
- Test: `backend/internal/server/middleware/request_access_logger_test.go` or add focused response tests under `backend/internal/pkg/response`

- [ ] **Step 1: 写错误响应包含 `trace_id` 的失败测试**

```go
func TestErrorWithDetails_IncludesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestLogger())
	r.GET("/err", func(c *gin.Context) {
		response.Error(c, http.StatusBadRequest, "bad request")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	req.Header.Set("X-Trace-ID", "trace-error")
	r.ServeHTTP(w, req)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["trace_id"] != "trace-error" {
		t.Fatalf("trace_id=%v, want trace-error", body["trace_id"])
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/server/middleware -run TestErrorWithDetails_IncludesTraceID`

Expected: FAIL，响应体中没有 `trace_id`

- [ ] **Step 3: 最小实现错误响应追加 `trace_id`**

```go
// backend/internal/pkg/response/response.go
type Response struct {
	Code     int               `json:"code"`
	Message  string            `json:"message"`
	Reason   string            `json:"reason,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	TraceID  string            `json:"trace_id,omitempty"`
	Data     any               `json:"data,omitempty"`
}

func traceIDFromContext(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if traceID, _ := c.Request.Context().Value(ctxkey.TraceID).(string); strings.TrimSpace(traceID) != "" {
		return strings.TrimSpace(traceID)
	}
	return strings.TrimSpace(c.Writer.Header().Get("X-Trace-ID"))
}
```

```go
func ErrorWithDetails(c *gin.Context, statusCode int, message, reason string, metadata map[string]string) {
	c.JSON(statusCode, Response{
		Code:     statusCode,
		Message:  message,
		Reason:   reason,
		Metadata: metadata,
		TraceID:  traceIDFromContext(c),
	})
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/server/middleware -run TestErrorWithDetails_IncludesTraceID`

Expected: PASS

- [ ] **Step 5: 跑一遍 response 相关测试**

Run: `cd backend && go test ./internal/pkg/response ./internal/pkg/errors`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/pkg/response/response.go \
  backend/internal/server/middleware/request_access_logger_test.go
git commit -m "feat: include trace id in error responses"
```

## Task 3: 为 `usage_logs` 落库 `trace_id`

**Files:**
- Add: `backend/migrations/114_add_usage_log_trace_id.sql`
- Modify: `backend/ent/schema/usage_log.go`
- Modify: `backend/internal/service/usage_log.go`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Test: `backend/internal/repository/usage_log_repo_request_type_test.go`

- [ ] **Step 1: 写 usage log 插入参数包含 `trace_id` 的失败测试**

```go
func TestUsageLogRepositoryCreate_PersistsTraceID(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	createdAt := time.Date(2025, 1, 6, 12, 0, 0, 0, time.UTC)
	log := &service.UsageLog{
		UserID:         1,
		APIKeyID:       2,
		AccountID:      3,
		TraceID:        "trace-usage-1",
		RequestID:      "req-upstream-1",
		Model:          "gpt-5",
		RequestedModel: "gpt-5",
		CreatedAt:      createdAt,
	}

	mock.ExpectQuery("INSERT INTO usage_logs").
		WithArgs(
			log.UserID,
			log.APIKeyID,
			log.AccountID,
			log.TraceID,
			log.RequestID,
			// ...
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(101), createdAt))

	_, err := repo.Create(context.Background(), log)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/repository -run TestUsageLogRepositoryCreate_PersistsTraceID`

Expected: FAIL，当前 `INSERT` 参数不包含 `trace_id`

- [ ] **Step 3: 新增 migration 与 schema 字段**

```sql
-- backend/migrations/114_add_usage_log_trace_id.sql
ALTER TABLE usage_logs
  ADD COLUMN IF NOT EXISTS trace_id VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_usage_logs_trace_id
  ON usage_logs (trace_id);
```

```go
// backend/ent/schema/usage_log.go
field.String("trace_id").
	MaxLen(64).
	Optional().
	Nillable(),
```

```go
func (UsageLog) Indexes() []ent.Index {
	return []ent.Index{
		// ...
		index.Fields("trace_id"),
	}
}
```

- [ ] **Step 4: 最小实现 service / repository 持久化 `trace_id`**

```go
// backend/internal/service/usage_log.go
type UsageLog struct {
	TraceID   *string
	RequestID string
}
```

```go
// backend/internal/repository/usage_log_repo.go
const usageLogSelectColumns = "id, user_id, api_key_id, account_id, trace_id, request_id, model, requested_model, ..."
```

```go
// insert columns/order
INSERT INTO usage_logs (
	user_id,
	api_key_id,
	account_id,
	trace_id,
	request_id,
	model,
	...
)
```

- [ ] **Step 5: 运行仓储测试确认通过**

Run: `cd backend && go test ./internal/repository -run 'TestUsageLogRepositoryCreate_PersistsTraceID|TestUsageLogRepositoryCreateSyncRequestTypeAndLegacyFields|TestPrepareUsageLogInsert_ArgCountMatchesTypes'`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/114_add_usage_log_trace_id.sql \
  backend/ent/schema/usage_log.go \
  backend/internal/service/usage_log.go \
  backend/internal/repository/usage_log_repo.go \
  backend/internal/repository/usage_log_repo_request_type_test.go
git commit -m "feat: persist trace id in usage logs"
```

## Task 4: 从请求上下文把 `trace_id` 写入 usage log 与 DTO

**Files:**
- Modify: concrete usage log creation call sites found via `rg "RequestID:" backend/internal/service backend/internal/handler`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Test: `backend/internal/handler/dto/mappers_usage_test.go`

- [ ] **Step 1: 写 DTO 映射 `trace_id` 的失败测试**

```go
func TestUsageLogFromService_IncludesTraceIDForAdmin(t *testing.T) {
	t.Parallel()

	log := &service.UsageLog{
		TraceID:   ptr("trace-dto-1"),
		RequestID: "req-upstream-1",
		Model:     "gpt-5",
	}

	adminDTO := UsageLogFromServiceAdmin(log)
	require.NotNil(t, adminDTO.TraceID)
	require.Equal(t, "trace-dto-1", *adminDTO.TraceID)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/handler/dto -run TestUsageLogFromService_IncludesTraceIDForAdmin`

Expected: FAIL，Admin DTO 中没有 `trace_id`

- [ ] **Step 3: 最小实现 DTO 与映射**

```go
// backend/internal/handler/dto/types.go
type UsageLog struct {
	RequestID string  `json:"request_id"`
	TraceID   *string `json:"trace_id,omitempty"`
}
```

```go
// backend/internal/handler/dto/mappers.go
TraceID: l.TraceID,
```

- [ ] **Step 4: 在 usage log 创建链路中从 context 读取 `trace_id`**

```go
func traceIDFromContext(ctx context.Context) *string {
	if ctx == nil {
		return nil
	}
	traceID, _ := ctx.Value(ctxkey.TraceID).(string)
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return nil
	}
	return &traceID
}
```

```go
usageLog := &service.UsageLog{
	TraceID:   traceIDFromContext(ctx),
	RequestID: upstreamRequestID,
	// ...
}
```

- [ ] **Step 5: 运行 DTO 与相关服务测试确认通过**

Run: `cd backend && go test ./internal/handler/dto`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/dto/types.go \
  backend/internal/handler/dto/mappers.go \
  backend/internal/handler/dto/mappers_usage_test.go \
  backend/internal/service \
  backend/internal/handler
git commit -m "feat: record trace id in usage log pipeline"
```

## Task 5: 管理端使用记录页展示 `trace_id` 与上游 `request_id`

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/admin/UsageView.vue`
- Modify: `frontend/src/components/admin/usage/UsageTable.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`
- Test: `frontend/src/views/admin/__tests__/UsageView.spec.ts`

- [ ] **Step 1: 写前端表格展示新字段的失败测试**

```ts
it('shows trace id and upstream request id for admin rows', () => {
  const row = {
    trace_id: 'trace-ui-1',
    request_id: 'req-upstream-ui-1',
    model: 'gpt-5',
    actual_cost: 0,
    total_cost: 0,
    input_cost: 0,
    output_cost: 0,
    cache_creation_cost: 0,
    cache_read_cost: 0,
    input_tokens: 0,
    output_tokens: 0,
  }
  // mount UsageTable and assert rendered text contains both ids
})
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd frontend && pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts`

Expected: FAIL，当前表格没有对应单元格渲染

- [ ] **Step 3: 最小实现前端类型与表格列**

```ts
// frontend/src/types/index.ts
export interface UsageLog {
  request_id: string
  trace_id?: string | null
}
```

```ts
// frontend/src/views/admin/UsageView.vue
const allColumns = computed(() => [
  { key: 'trace_id', label: t('admin.usage.traceId'), sortable: false },
  { key: 'request_id', label: t('admin.usage.upstreamRequestId'), sortable: false },
  // ...
])
```

```vue
<!-- frontend/src/components/admin/usage/UsageTable.vue -->
<template #cell-trace_id="{ row }">
  <span v-if="row.trace_id" class="text-sm font-mono text-gray-600 dark:text-gray-400">{{ row.trace_id }}</span>
  <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
</template>

<template #cell-request_id="{ row }">
  <span v-if="row.request_id" class="text-sm font-mono text-gray-600 dark:text-gray-400">{{ row.request_id }}</span>
  <span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span>
</template>
```

- [ ] **Step 4: 更新导出字段与文案**

```ts
// frontend/src/views/admin/UsageView.vue
const rowData = [
  log.trace_id || '',
  log.request_id || '',
  log.user_agent || '',
  log.ip_address || '',
]
```

```ts
// frontend/src/i18n/locales/zh.ts
traceId: 'Trace ID',
upstreamRequestId: '上游 Request ID',
```

- [ ] **Step 5: 运行前端测试确认通过**

Run: `cd frontend && pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/index.ts \
  frontend/src/views/admin/UsageView.vue \
  frontend/src/components/admin/usage/UsageTable.vue \
  frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts \
  frontend/src/views/admin/__tests__/UsageView.spec.ts \
  frontend/src/i18n/locales/zh.ts \
  frontend/src/i18n/locales/en.ts
git commit -m "feat: show trace id in admin usage records"
```

## Task 6: 调整 Nginx access log 字段名并做端到端回归

**Files:**
- Modify: `deploy/frontend/nginx.conf`
- Test: backend/frontend targeted suites from prior tasks

- [ ] **Step 1: 修改 access log 中上游请求 ID 字段名**

```nginx
log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                '$status $body_bytes_sent "$http_referer" '
                '"$http_user_agent" "$http_x_forwarded_for" '
                'trace=$trace_id upstream_request_id=$upstream_http_x_request_id '
                'rt=$request_time urt=$upstream_response_time';
```

- [ ] **Step 2: 运行后端聚焦测试**

Run: `cd backend && go test ./internal/server/middleware ./internal/pkg/response ./internal/handler/dto ./internal/repository`

Expected: PASS

- [ ] **Step 3: 运行前端聚焦测试**

Run: `cd frontend && pnpm vitest run src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts`

Expected: PASS

- [ ] **Step 4: 记录迁移与手工验收步骤**

```bash
cd backend && go test ./...
cd frontend && pnpm vitest run
```

Manual checks:

- 请求任意 API，确认响应头包含 `X-Trace-ID`
- 构造一个错误响应，确认 JSON body 中包含 `trace_id`
- 产生一条新的 usage 记录，确认管理员使用记录页可看到 `trace_id` 与 `request_id`

- [ ] **Step 5: Commit**

```bash
git add deploy/frontend/nginx.conf
git commit -m "chore: align nginx upstream request id log field"
```

## Self-Review

- Spec coverage:
  - `trace_id` 贯穿 Nginx、后端日志、使用记录、错误响应：由 Task 1/2/3/4/6 覆盖
  - 管理端展示 `trace_id + request_id`：由 Task 5 覆盖
  - 保留 `request_id` 为上游语义：由 Task 5 文案和 Task 6 nginx 字段命名覆盖
- Placeholder scan:
  - 已为每个任务提供明确文件、测试、命令和最小代码片段
- Type consistency:
  - 计划统一使用 `trace_id` 作为 JSON 字段名，`TraceID` 作为 Go 字段名，`request_id` 保持现有命名

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-13-trace-id-implementation.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
