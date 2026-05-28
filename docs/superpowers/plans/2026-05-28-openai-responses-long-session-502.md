# OpenAI Responses Long Session 502 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 OpenAI Responses 长会话工具续链断开后被包装成 502 的问题。

**Architecture:** 保持现有 Handler -> Service -> Upstream 结构，只在 OpenAI Gateway 热路径上补齐三个边界：工具输出续链保留 `previous_response_id`，OpenAI 上游 400 保持 400 错误语义，系统提示注入在未配置 `SettingService` 时安全继承。

**Tech Stack:** Go, Gin, `httptest`, existing `OpenAIGatewayService` tests.

---

## 文件结构

- 修改：`backend/internal/service/openai_gateway_service.go`
  - 调整 HTTP 转发前删除 `previous_response_id` 的条件。
  - 调整 OpenAI 非 passthrough 错误响应映射。
- 修改：`backend/internal/service/system_prompt.go`
  - 增加对 typed nil `SystemPromptSettingsProvider` 的安全判断，未注入时返回 inherit。
- 修改：`backend/internal/service/openai_ws_protocol_forward_test.go`
  - 增加 HTTP 入站 `function_call_output` 保留 `previous_response_id` 的回归测试。
- 修改：`backend/internal/service/error_passthrough_runtime_test.go`
  - 增加 OpenAI 上游 400 返回 400 的错误语义测试。
- 修改：`backend/internal/service/openai_oauth_passthrough_test.go`
  - 复用现有测试验证未注入 `SettingService` 不再 panic。

## Task 1: 文档落盘

- [x] **Step 1: 写 bug 说明文档**

创建 `docs/superpowers/specs/2026-05-28-openai-responses-long-session-502-design.md`，记录生产证据、根因、非目标和验收标准。

- [x] **Step 2: 写修复计划文档**

创建 `docs/superpowers/plans/2026-05-28-openai-responses-long-session-502.md`，列出修改文件、测试策略和分步执行方式。

- [x] **Step 3: 检查文档无未定内容**

运行：

```bash
rg -n 'TB[D]|TO[D]O|待[定]|占[位]' docs/superpowers/specs/2026-05-28-openai-responses-long-session-502-design.md docs/superpowers/plans/2026-05-28-openai-responses-long-session-502.md
```

期望：无输出。

## Task 2: RED - 复现 HTTP 工具续链断链

- [x] **Step 1: 添加失败测试**

在 `backend/internal/service/openai_ws_protocol_forward_test.go` 增加测试：

```go
func TestOpenAIGatewayService_Forward_HTTPIngressPreservesPreviousResponseIDForFunctionCallOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"id":"resp_ok","usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	svc := &OpenAIGatewayService{
		cfg:          cfg,
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          202,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://example.com/v1",
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_tool","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "resp_prev_tool", gjson.GetBytes(upstream.lastBody, "previous_response_id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
}
```

- [x] **Step 2: 验证红灯**

运行：

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_Forward_HTTPIngressPreservesPreviousResponseIDForFunctionCallOutput' -count=1
```

期望：失败，断言 `previous_response_id` 不存在或为空。

## Task 3: GREEN - 保留工具续链锚点

- [x] **Step 1: 修改删除条件**

在 `backend/internal/service/openai_gateway_service.go` 中，把非 WSv2 删除 `previous_response_id` 的条件改为仅在没有 `function_call_output` 时删除。

```go
if wsDecision.Transport != OpenAIUpstreamTransportResponsesWebsocketV2 && !HasFunctionCallOutput(reqBody) {
	if _, has := reqBody["previous_response_id"]; has {
		delete(reqBody, "previous_response_id")
		bodyModified = true
		markPatchDelete("previous_response_id")
	}
}
```

- [x] **Step 2: 验证绿灯**

运行：

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_Forward_HTTPIngressPreservesPreviousResponseIDForFunctionCallOutput|TestOpenAIGatewayService_Forward_HTTPIngressStaysHTTPWhenWSEnabled' -count=1
```

期望：两个测试通过；普通 HTTP 入站仍删除 `previous_response_id`，工具输出续链保留它。

## Task 4: RED/GREEN - 修复 OpenAI 上游 400 错误语义

- [x] **Step 1: 添加失败测试**

在 `backend/internal/service/error_passthrough_runtime_test.go` 增加测试：

```go
func TestOpenAIHandleErrorResponse_Upstream400ReturnsInvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	svc := &OpenAIGatewayService{}
	respBody := []byte(`{"error":{"message":"No tool output found for tool search call fc_test."}}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader(respBody)),
		Header:     http.Header{},
	}
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	errField, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errField["type"])
	require.Contains(t, errField["message"], "No tool output found")
}
```

- [x] **Step 2: 验证红灯**

运行：

```bash
go test ./internal/service -run 'TestOpenAIHandleErrorResponse_Upstream400ReturnsInvalidRequest' -count=1
```

期望：失败，当前返回 502。

- [x] **Step 3: 修改 OpenAI 错误映射**

在 `backend/internal/service/openai_gateway_service.go` 的 `handleErrorResponse` 中，将 `resp.StatusCode == 400` 映射为 `http.StatusBadRequest`、`invalid_request_error`，消息使用清洗后的上游 message；保留 401/402/403/429 的既有策略。

- [x] **Step 4: 验证绿灯**

运行：

```bash
go test ./internal/service -run 'TestOpenAIHandleErrorResponse_Upstream400ReturnsInvalidRequest|TestOpenAIHandleErrorResponse_NoRuleKeepsDefault' -count=1
```

期望：新增 400 测试通过；既有 422 默认 502 行为保持不变。

## Task 5: RED/GREEN - 修复系统提示 nil 依赖 panic

- [x] **Step 1: 用现有失败测试确认红灯**

运行：

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_UpstreamErrorIncludesPassthroughFlag' -count=1
```

期望：当前 panic，堆栈落在 `SettingService.GetSystemPromptSettings`。

- [x] **Step 2: 修改 nil provider 判断**

在 `backend/internal/service/system_prompt.go` 增加 typed nil 检测，`settings` 为 nil 或 typed nil 时不读取设置，直接继承。

- [x] **Step 3: 验证绿灯**

运行：

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_OAuthPassthrough_UpstreamErrorIncludesPassthroughFlag|TestOpenAIGatewayService_Forward_HTTPIngressStaysHTTPWhenWSEnabled' -count=1
```

期望：不再 panic，测试通过。

## Task 6: 回归验证

- [x] **Step 1: 运行 OpenAI 相关定向测试**

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_Forward_HTTPIngressPreservesPreviousResponseIDForFunctionCallOutput|TestOpenAIGatewayService_Forward_HTTPIngressStaysHTTPWhenWSEnabled|TestOpenAIHandleErrorResponse_Upstream400ReturnsInvalidRequest|TestOpenAIHandleErrorResponse_NoRuleKeepsDefault|TestOpenAIGatewayService_OAuthPassthrough_UpstreamErrorIncludesPassthroughFlag|TestOpenAIGatewayService_Forward_WSv2PreviousResponseNotFoundSkipsRecoveryForFunctionCallOutput' -count=1
```

期望：通过。

- [x] **Step 2: 运行 handler 相关测试**

```bash
go test ./internal/handler -run 'TestOpenAIGatewayHandler.*previous_response_id|TestReadRequestBodyWithPrealloc' -count=1
```

期望：通过。

- [x] **Step 3: 检查 diff**

```bash
git diff -- backend/internal/service/openai_gateway_service.go backend/internal/service/system_prompt.go backend/internal/service/openai_ws_protocol_forward_test.go backend/internal/service/error_passthrough_runtime_test.go docs/superpowers/specs/2026-05-28-openai-responses-long-session-502-design.md docs/superpowers/plans/2026-05-28-openai-responses-long-session-502.md
```

期望：只包含本次 bug 文档、测试和最小修复。
