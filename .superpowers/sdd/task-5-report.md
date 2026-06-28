## 实现内容

- 在 `backend/internal/handler/gateway_handler.go` 为 Claude `/v1/messages` 增加运行时分组兜底 helper：`trySwitchToClaudeFallbackGroup`。
- Claude HTTP 路径新增三类兜底触发：
  - 初次选账号失败且当前分组没有可用账号时，尝试切换到 `fallback_group_id`。
  - 当前分组账号 failover 耗尽时，若响应尚未开始写出，尝试切换到 `fallback_group_id`。
  - `PromptTooLongError` 时，移除运行时对 `fallback_group_id_on_invalid_request` 的依赖，改为统一走 `fallback_group_id`。
- 保留 Claude 既有 `claude_code_only` 行为；本任务未改 OpenAI 入口。
- 新增/调整测试：
  - `backend/internal/service/gateway_multiplatform_test.go`：补充 `TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback`，确认 Anthropic 分组允许解析 Anthropic fallback。
  - `backend/internal/handler/gateway_handler_error_fallback_test.go`：补充 `TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured`，覆盖 Claude fallback helper 的基础路径。

## 测试命令和结果

### RED / GREEN 证据

1. RED

```bash
cd backend
go test ./internal/handler -run TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured -count=1
```

- 结果：FAIL（编译失败）
- 关键报错：`h.trySwitchToClaudeFallbackGroup undefined`

2. Task 4 helper 复核

```bash
cd backend
go test -tags unit ./internal/service -run TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback -count=1
```

- 结果：PASS
- 说明：Task 4 已提供的 `ResolveRuntimeFallbackGroup` helper 可直接支持本次 Anthropic fallback 断言。

3. GREEN

```bash
cd backend
go test ./internal/handler -run '^TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured$|^TestGatewayErrorResponseIncludesTraceID$|^TestOpenAIHandleStreamingAwareError' -count=1
```

- 结果：PASS

### Focused tests / 编译基线

1. 直接覆盖本次改动的 service tests

```bash
cd backend
go test -tags unit ./internal/service -run '^TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback$|^TestGatewayServiceResolveRuntimeFallbackGroup$' -count=1
```

- 结果：PASS

## 第二轮修复追加（Task 5 Messages 主流程覆盖子代理）

### 修复说明

- 在 `backend/internal/handler/gateway_handler_error_fallback_test.go` 增补了一个可控的 `Messages` 主流程测试夹具：
  - bucket-aware 的 scheduler snapshot stub，确保切组后真的按 fallback group 重新选号；
  - 可脚本化的 upstream stub，按账号返回 5xx / 403 / 200；
  - 顺序余额 stub，用于驱动“初始计费通过、fallback 计费失败”场景；
  - usage log stub，用于断言切组后的归属已经落到 fallback group/account。
- 保持生产代码不变，只补测试与最小测试辅助。

### 新增主流程测试

- `TestGatewayMessagesFallback_FailoverExhaustedRetryableSwitchesGroupAndAttributesUsage`
  - 驱动 `GatewayHandler.Messages` 走 `FailoverExhausted + 5xx` 主流程；
  - 断言会切到 fallback group；
  - 断言最终成功响应来自 fallback account；
  - 断言 usage log 归属改为 fallback `group_id` / `account_id`。
- `TestGatewayMessagesFallback_FailoverExhaustedNonRetryableKeepsOriginalTerminal`
  - 驱动 `FailoverExhausted + 403`；
  - 断言不切 fallback group；
  - 断言直接返回原终态映射错误。
- `TestGatewayMessagesFallback_BillingFailureWritesOnlyOneTerminalResponse`
  - 驱动“原组无可用账号 -> 尝试 fallback -> fallback billing 失败”；
  - 断言最终只写一次 billing 终态；
  - 断言响应体不再混入 `No available accounts` 等二次写出内容。

### 第二轮 RED / GREEN

1. RED

```bash
cd backend
go test ./internal/handler -run 'TestGatewayMessagesFallback' -count=1
```

- 初次结果：FAIL
- 说明：新主流程测试先暴露出测试夹具缺少当前仓库接口方法，以及 scheduler stub 不能按 group 分桶，导致无法真实驱动 fallback 选组。

2. GREEN

```bash
cd backend
go test ./internal/handler -run 'TestGatewayMessagesFallback' -count=1
```

- 结果：PASS

### 本轮 focused tests

```bash
cd backend
go test ./internal/handler -run 'TestGateway.*Fallback|TestGatewayTrySwitchToClaudeFallbackGroup|TestGatewayMessages.*Fallback|TestGatewayErrorResponseIncludesTraceID|TestOpenAIHandleStreamingAwareError' -count=1
```

- 结果：PASS

```bash
cd backend
go test -tags unit ./internal/service -run '^TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback$|^TestGatewayServiceResolveRuntimeFallbackGroup$' -count=1
```

- 结果：PASS

2. handler focused tests

```bash
cd backend
go test ./internal/handler -run '^TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured$|^TestGatewayErrorResponseIncludesTraceID$|^TestOpenAIHandleStreamingAwareError' -count=1
```

- 结果：PASS

3. package compile baseline

```bash
cd backend
go test -tags unit ./internal/service -run '^$' -count=1
```

- 结果：PASS（仅编译，无测试运行）

4. brief 指定的 broader service command

```bash
cd backend
go test -tags unit ./internal/service -run 'TestGatewayService_ResolveRuntimeFallbackGroup|TestGatewayService_GroupResolution|TestGroupIsolation' -count=1
```

- 结果：FAIL
- 说明：存在既有失败，与本次 Claude fallback 接入无直接关系：
  - `TestGroupIsolation_GroupedKey_ShouldNotScheduleUngroupedAccounts` panic，位置 `backend/internal/service/gateway_service.go:1873`
  - `TestGatewayService_GroupResolution_ReusesContextGroup` 断言失败（expected 1, actual 0）
  - `TestGatewayService_GroupResolution_IgnoresInvalidContextGroup` 断言失败（expected 1, actual 0）
  - `TestGatewayService_GroupResolution_FallbackUsesLiteOnce` 断言失败（expected 1, actual 0）

## 修改文件

- `backend/internal/handler/gateway_handler.go`
- `backend/internal/handler/gateway_handler_error_fallback_test.go`
- `backend/internal/service/gateway_multiplatform_test.go`
- `.superpowers/sdd/task-5-report.md`

## 自审发现和疑虑

- `trySwitchToClaudeFallbackGroup` 依赖注入的 `gatewayService` / `billingCacheService` 为正常运行态；当前生产路径满足这一前提，测试也未覆盖缺失依赖分支。
- brief 中写的是 `gateway_handler_test.go`，但仓库中没有该文件；因此本次将 handler 测试增补到现有的 `gateway_handler_error_fallback_test.go`。
- broader service command 的失败看起来是仓库当前基线问题，已单独记录，未在本任务中顺手修复，避免越界改动。

## 评审修复追加（Task 5 修复子代理）

### 修复说明

- 将 `trySwitchToClaudeFallbackGroup` 从二态返回改为三态协议：
  - `claudeFallbackUnavailable`：fallback 不可用，调用方可继续原有终态处理；
  - `claudeFallbackSwitched`：已成功切组，调用方进入 fallback 重试；
  - `claudeFallbackHandled`：helper 已写出终态错误，调用方必须立刻 `return`，避免二次写响应。
- 新增 `handleClaudeFallbackBillingFailure`，把 fallback group 计费失败统一归口为“已处理终态错误”，消除 `No available accounts` / `failover exhausted` 的重复写出风险。
- 新增 `shouldRetryClaudeRuntimeFallback` / `shouldAttemptClaudeRuntimeFallback`，将 Claude runtime fallback 的跨组触发收紧为：
  - 允许：`429`、`408`、`5xx`、以及 `StatusCode<=0` 的 transport/连接类错误；
  - 禁止：`400`、`401`、`403` 等非 retryable request/auth/billing 类错误。
- `FailoverExhausted` 场景下改为显式检查“最后一个上游错误是否 retryable”后才允许跨组 fallback；已写流（`c.Writer.Size()` 变化）时继续禁止切组。

### 新增/增强测试

- `TestGatewayHandleClaudeFallbackBillingFailure_ReturnsHandledTerminal`
- `TestGatewayShouldAttemptClaudeRuntimeFallback_NonRetryableFailoverExhausted`
- `TestGatewayShouldAttemptClaudeRuntimeFallback_StreamAlreadyWritten`
- `TestGatewayShouldRetryClaudeRuntimeFallback_AllowsTransportLikeErrors`
- 同步更新 `TestGatewayTrySwitchToClaudeFallbackGroup_NoFallbackConfigured`，断言新的三态协议。

### 本次 focused tests

```bash
cd backend
go test ./internal/handler -run 'TestGateway.*Fallback|TestGatewayTrySwitchToClaudeFallbackGroup|TestGatewayErrorResponseIncludesTraceID|TestOpenAIHandleStreamingAwareError' -count=1
```

- 结果：PASS

```bash
cd backend
go test -tags unit ./internal/service -run '^TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback$|^TestGatewayServiceResolveRuntimeFallbackGroup$' -count=1
```

- 结果：PASS
