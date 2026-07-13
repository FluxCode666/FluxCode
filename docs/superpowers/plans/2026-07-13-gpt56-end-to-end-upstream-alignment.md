# GPT-5.6 End-to-End Upstream Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 GPT-5.6 的模型识别、计费、Chat→Responses、Native Responses、WebSocket 和终止 usage 行为语义对齐到固定上游基线，同时保留本地渠道价格优先级与现有兼容扩展。

**Architecture:** 先建立被计费和协议入口共同消费的 GPT-5.6 canonical model、service tier 与 terminal usage 原语，再依次接入 Chat、OAuth Responses、HTTP 响应和 WebSocket。各任务按 TDD 独立提交；不移植上游拆文件重构、完整 fast policy、APIKey Chat 路由探测或完整 Codex base prompt。

**Tech Stack:** Go 1.26、Gin、`encoding/json`、`gjson`、`sjson`、`zap`/`slog`、`testify/require`、`testify/assert`、Git。

## Global Constraints

- 设计规格：`docs/superpowers/specs/2026-07-13-gpt56-end-to-end-upstream-alignment-design.md`。
- 当前审计基线为 `upstream/main@7d239d62e`；实施第一步必须刷新并固定最终上游 commit。
- 只做语义移植，不整文件覆盖，不直接 cherry-pick 大型上游提交。
- 保留 GPT-5.6 Sol、Terra、Luna 当前已对齐的价格 JSON、cache write、priority/flex 与 272K 长上下文行为。
- 不引入完整 OpenAI fast policy、APIKey Chat 能力探测、完整 Codex base prompt、installation ID 或非 WS `previous_response_id` 变更。
- canonical model 只能追加为计费候选，不能覆盖渠道映射和原始模型优先级。
- `auto/default/scale` 按标准价格计费；`fast` 必须在请求发出前变为 `priority`。
- 保留本地 Chat usage 的 `cache_creation_input_tokens` 和 `cache_write_input_tokens` 扩展字段。
- 每个任务必须先看到目标测试失败，再实现最小代码并运行任务级回归。

## File Structure

- `backend/internal/service/openai_gpt56_alias.go`：GPT-5.6 拼写规范化、canonical model 和计费候选。
- `backend/internal/service/billing_service.go`：区间上下文选择和现有 tier 计费。
- `backend/internal/service/openai_gateway_service.go`：Native Responses 请求、统一 HTTP usage 和 OpenAI 用量计费候选。
- `backend/internal/service/openai_gateway_chat_completions.go`：Chat bridge 请求规范化、prompt cache key、流式 usage。
- `backend/internal/service/openai_codex_transform.go`：OAuth unsupported 字段、instructions 选项、encrypted reasoning include 和 tool choice。
- `backend/internal/service/openai_ws_forwarder.go`：每轮 WS payload 规范化、session model 复用和 terminal usage。
- `backend/internal/service/openai_ws_v2_passthrough_adapter.go`：passthrough relay 的每轮请求模型、service tier 规范化与 usage 元数据关联。
- `backend/internal/service/openai_ws_v2/passthrough_relay.go`：passthrough terminal 事件集合、顶层/嵌套 usage 解析和 cache write 优先级。
- `backend/internal/pkg/apicompat/types.go`：Chat/Responses 请求、终止事件和 Chat usage 类型。
- `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`：Chat 请求到 Responses 请求的字段转换。
- `backend/internal/pkg/apicompat/responses_to_chatcompletions.go`：Responses 响应和流事件到 Chat 的转换。

---

### Task 1: 固定上游基线并收敛 GPT-5.6 Canonical Model

**Files:**
- Modify: `docs/superpowers/specs/2026-07-13-gpt56-end-to-end-upstream-alignment-design.md:27-47`（仅当上游 commit 变化）
- Modify: `backend/internal/service/openai_gpt56_alias.go`
- Test: `backend/internal/service/openai_model_mapping_test.go`

**Interfaces:**
- Consumes: 原始、provider-prefixed、日期或 effort 后缀模型名。
- Produces: `normalizeGPT56ModelAlias(string) (string, bool)`、`usageBillingModelCandidates(string, ...string) []string`。

- [ ] **Step 1: 刷新并固定上游基线**

Run:

```bash
git fetch upstream main --prune
git rev-parse upstream/main
git log -1 --format='%H %cI %s' upstream/main
```

Expected: 命令成功并输出完整 commit。若不是 `7d239d62e`，只重新比较本计划涉及的文件，并在继续前更新规格和计划中的基线。

- [ ] **Step 2: 写入 alias 与候选顺序失败测试**

在 `backend/internal/service/openai_model_mapping_test.go` 增加：

```go
func TestNormalizeGPT56ModelAlias_UpstreamSpellings(t *testing.T) {
	tests := map[string]string{
		"gpt-5.6": "gpt-5.6-sol", "gpt5.6": "gpt-5.6-sol",
		"openai/gpt-5.6": "gpt-5.6-sol", "gpt-5.6-high": "gpt-5.6-sol",
		"gpt-5.6-max": "gpt-5.6-sol", "gpt-5.6-2026-07-09": "gpt-5.6-sol",
		"gpt-5.6-sol-2026-07-09": "gpt-5.6-sol",
		"gpt-5.6-terra-high": "gpt-5.6-terra", "gpt-5.6-luna-preview": "gpt-5.6-luna",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, ok := normalizeGPT56ModelAlias(input)
			require.True(t, ok)
			require.Equal(t, want, got)
		})
	}
}

func TestNormalizeGPT56ModelAlias_RejectsUnknownSuffixes(t *testing.T) {
	for _, input := range []string{"gpt-5.6-extra-high", "gpt-5.6-foo", "gpt-5.6-terra-foo", "openai/gpt-5.6-unknown"} {
		t.Run(input, func(t *testing.T) {
			_, ok := normalizeGPT56ModelAlias(input)
			require.False(t, ok)
		})
	}
}

func TestUsageBillingModelCandidates_AppendsCanonicalGPT56(t *testing.T) {
	require.Equal(t,
		[]string{"billing-explicit", "channel-model", "openai/gpt5.6", "upstream-model", "gpt-5.6-sol"},
		usageBillingModelCandidates("billing-explicit", "channel-model", "openai/gpt5.6", "upstream-model"),
	)
	require.Equal(t,
		[]string{"custom", "gpt-5.6-terra-high", "gpt-5.6-terra"},
		usageBillingModelCandidates("custom", "gpt-5.6-terra-high"),
	)
}
```

- [ ] **Step 3: 运行测试确认当前缺口**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'NormalizeGPT56ModelAlias_UpstreamSpellings|NormalizeGPT56ModelAlias_RejectsUnknownSuffixes|UsageBillingModelCandidates_AppendsCanonicalGPT56' -count=1
```

Expected: FAIL；`gpt5.6`、日期后缀或候选函数至少一项不满足预期。

- [ ] **Step 4: 实现 canonical 拼写与计费候选**

在 `openai_gpt56_alias.go` 中使用：

```go
func normalizeGPT56ModelAlias(model string) (string, bool) {
	modelID := normalizeGPT56ModelID(model)
	if modelID == "gpt-5.6" {
		return "gpt-5.6-sol", true
	}
	if !strings.HasPrefix(modelID, "gpt-5.6-") {
		return "", false
	}
	suffix := strings.TrimPrefix(modelID, "gpt-5.6-")
	for _, variant := range []string{"sol", "terra", "luna"} {
		if suffix == variant {
			return "gpt-5.6-" + variant, true
		}
		if strings.HasPrefix(suffix, variant+"-") {
			variantSuffix := strings.TrimPrefix(suffix, variant+"-")
			if variantSuffix == "preview" || isGPT56ReasoningSuffix(variantSuffix) || isGPT56DateSuffix(variantSuffix) {
				return "gpt-5.6-" + variant, true
			}
			return "", false
		}
	}
	if isGPT56ReasoningSuffix(suffix) || isGPT56DateSuffix(suffix) {
		return "gpt-5.6-sol", true
	}
	return "", false
}

func normalizeGPT56ModelID(model string) string {
	modelID := strings.TrimSpace(model)
	if strings.Contains(modelID, "/") {
		parts := strings.Split(modelID, "/")
		modelID = parts[len(parts)-1]
	}
	modelID = strings.ToLower(modelID)
	modelID = strings.NewReplacer("_", "-", " ", "-").Replace(modelID)
	for strings.Contains(modelID, "--") {
		modelID = strings.ReplaceAll(modelID, "--", "-")
	}
	modelID = strings.Trim(modelID, "-")
	if strings.HasPrefix(modelID, "gpt5") {
		modelID = "gpt-5" + strings.TrimPrefix(modelID, "gpt5")
	}
	return modelID
}

func isGPT56ReasoningSuffix(raw string) bool {
	value := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(raw)))
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func isGPT56DateSuffix(raw string) bool {
	parts := strings.Split(raw, "-")
	if len(parts) != 3 || len(parts[0]) != 4 || len(parts[1]) != 2 || len(parts[2]) != 2 {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func appendUsageBillingModelCandidate(candidates []string, seen map[string]struct{}, model string) []string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return candidates
	}
	key := strings.ToLower(trimmed)
	if _, ok := seen[key]; ok {
		return candidates
	}
	seen[key] = struct{}{}
	candidates = append(candidates, trimmed)
	return candidates
}

func usageBillingModelCandidates(primary string, alternates ...string) []string {
	seen := make(map[string]struct{}, len(alternates)+1)
	sources := append([]string{primary}, alternates...)
	var candidates []string
	for _, source := range sources {
		candidates = appendUsageBillingModelCandidate(candidates, seen, source)
	}
	for _, source := range sources {
		if canonical, ok := normalizeGPT56ModelAlias(source); ok {
			candidates = appendUsageBillingModelCandidate(candidates, seen, canonical)
		}
	}
	return candidates
}
```

- [ ] **Step 5: 运行 alias 与现有 GPT-5.6 模型测试**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'GPT56|Gpt56|gpt56|NormalizeCodexModel' -count=1
```

Expected: PASS；新增拒绝测试明确保证 `extra-high` 不再被当作模型别名。

- [ ] **Step 6: 提交 Task 1**

```bash
git add backend/internal/service/openai_gpt56_alias.go backend/internal/service/openai_model_mapping_test.go docs/superpowers/specs/2026-07-13-gpt56-end-to-end-upstream-alignment-design.md docs/superpowers/plans/2026-07-13-gpt56-end-to-end-upstream-alignment.md
git commit -m "fix(openai): align gpt-5.6 canonical model aliases"
```

---

### Task 2: 对齐计费候选、区间选档与缺价日志

**Files:**
- Modify: `backend/internal/service/billing_service.go:535-549`
- Modify: `backend/internal/service/openai_gateway_service.go:5160-5204`
- Test: `backend/internal/service/billing_service_unified_test.go`
- Test: `backend/internal/service/openai_gateway_record_usage_test.go`

**Interfaces:**
- Consumes: Task 1 的 `usageBillingModelCandidates`。
- Produces: cache write 参与区间选档；`RecordUsage` 按候选依次尝试价格。

- [ ] **Step 1: 写入失败测试**

在 `billing_service_unified_test.go` 增加完整测试：

```go
func TestCalculateCostUnified_IntervalSelectionIncludesCacheWrite(t *testing.T) {
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 1, model: "gpt-5.6-sol"}: {
				BillingMode: BillingModeToken,
				Intervals: []PricingInterval{
					{MinTokens: 0, MaxTokens: testPtrInt(100), InputPrice: testPtrFloat64(1e-6), OutputPrice: testPtrFloat64(1e-6)},
					{MinTokens: 101, MaxTokens: testPtrInt(1000), InputPrice: testPtrFloat64(2e-6), OutputPrice: testPtrFloat64(1e-6)},
				},
			},
		},
		channelByGroupID: map[int64]*Channel{1: {ID: 1, Status: StatusActive}},
		groupPlatform: map[int64]string{1: ""},
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel: map[channelModelKey]string{},
		wildcardMappingByGP: map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID: map[int64]*Channel{},
	})
	bs := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(cs, bs)
	groupID := int64(1)
	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-5.6-sol", GroupID: &groupID,
		Tokens: UsageTokens{InputTokens: 90, CacheCreationTokens: 20},
		RateMultiplier: 1, Resolver: resolver,
	})
	require.NoError(t, err)
	require.InDelta(t, 90*2e-6, cost.InputCost, 1e-12)
}
```

在 `openai_gateway_record_usage_test.go` 增加：

结构化日志捕获复用 `openai_oauth_passthrough_test.go` 中的同包 `captureStructuredLog` 测试辅助函数。

```go
func TestOpenAIGatewayServiceRecordUsage_GPT56SpellingUsesCanonicalPrice(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "resp_gpt56_alias", Model: "gpt5.6", Usage: OpenAIUsage{InputTokens: 100, OutputTokens: 10}, Duration: time.Second},
		APIKey: &APIKey{ID: 10}, User: &User{ID: 20}, Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{OriginalModel: "gpt5.6"},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Greater(t, usageRepo.lastLog.ActualCost, 0.0)
}

func TestOpenAIGatewayServiceRecordUsage_MissingPricingLogsCandidates(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	sink, cleanup := captureStructuredLog(t)
	defer cleanup()
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result:  &OpenAIForwardResult{RequestID: "resp_missing_price", Model: "gpt-5.6-unknown", Usage: OpenAIUsage{InputTokens: 1}},
		APIKey: &APIKey{ID: 10}, User: &User{ID: 20}, Account: &Account{ID: 30},
		ChannelUsageFields: ChannelUsageFields{OriginalModel: "gpt-5.6-unknown", ChannelMappedModel: "gpt-5.6-unknown"},
	})
	require.NoError(t, err)
	require.NotNil(t, usageRepo.lastLog)
	require.Zero(t, usageRepo.lastLog.ActualCost)
	require.True(t, sink.ContainsMessage("openai_usage.pricing_missing_record_zero_cost"))
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd backend
go test -tags unit ./internal/service -run 'IntervalSelectionIncludesCacheWrite|GPT56SpellingUsesCanonicalPrice|MissingPricingLogsCandidates' -count=1
```

Expected: FAIL；区间选中第一档或 alias 产生零费用。

- [ ] **Step 3: 修正区间上下文合计**

```go
func (s *BillingService) calculateTokenCost(resolved *ResolvedPricing, input CostInput) (*CostBreakdown, error) {
	totalContext := input.Tokens.InputTokens + input.Tokens.CacheCreationTokens + input.Tokens.CacheReadTokens
	pricing := input.Resolver.GetIntervalPricing(resolved, totalContext)
	if pricing == nil {
		return nil, fmt.Errorf("no pricing available for model: %s", input.Model)
	}
	pricing = s.applyModelSpecificPricingPolicy(input.Model, pricing)
	return s.computeTokenBreakdown(pricing, input.Tokens, input.RateMultiplier, input.ServiceTier, len(resolved.Intervals) == 0), nil
}
```

- [ ] **Step 4: 按候选依次计算 token 费用**

在 `RecordUsage` 中构造：

```go
clientModel := strings.TrimSpace(input.OriginalModel)
if clientModel == "" {
	clientModel = strings.TrimSpace(result.Model)
}
billingModels := usageBillingModelCandidates(
	result.BillingModel,
	input.ChannelMappedModel,
	clientModel,
	result.UpstreamModel,
)
```

该顺序是显式 billing model → 渠道映射 → 客户端原始模型（缺失时用 `result.Model`）→ 上游实际模型；`usageBillingModelCandidates` 再在末尾追加 canonical GPT-5.6 候选。图片请求保留现有单模型计费路径，本次只替换非图片 token 计费路径。

非图片请求调用：

```go
func (s *OpenAIGatewayService) calculateOpenAIUsageTokenCostCandidates(ctx context.Context, apiKey *APIKey, candidates []string, tokens UsageTokens, multiplier float64, serviceTier string) (*CostBreakdown, error) {
	var lastErr error
	for _, model := range candidates {
		var cost *CostBreakdown
		var err error
		if s.resolver != nil && apiKey.Group != nil {
			gid := apiKey.Group.ID
			cost, err = s.billingService.CalculateCostUnified(CostInput{Ctx: ctx, Model: model, GroupID: &gid, Tokens: tokens, RequestCount: 1, RateMultiplier: multiplier, ServiceTier: serviceTier, Resolver: s.resolver})
		} else {
			cost, err = s.billingService.CalculateCostWithServiceTier(model, tokens, multiplier, serviceTier)
		}
		if err == nil {
			return cost, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no non-empty OpenAI billing model candidates")
	}
	return nil, fmt.Errorf("calculate OpenAI usage cost for %s: %w", strings.Join(candidates, ","), lastErr)
}
```

全部候选失败时保留零费用兜底，并记录：

```go
canonicalModel := ""
for _, candidate := range billingModels {
	if normalized, ok := normalizeGPT56ModelAlias(candidate); ok {
		canonicalModel = normalized
		break
	}
}
logger.L().With(
	zap.Strings("billing_models", billingModels), zap.String("requested_model", input.OriginalModel),
	zap.String("mapped_model", input.ChannelMappedModel), zap.String("upstream_model", result.UpstreamModel),
	zap.String("canonical_model", canonicalModel), zap.Int64("channel_id", input.ChannelID),
	zap.Int64("api_key_id", apiKey.ID), zap.Int64("account_id", account.ID),
).Warn("openai_usage.pricing_missing_record_zero_cost", zap.Error(err))
```

- [ ] **Step 5: 运行计费回归**

```bash
cd backend
go test -tags unit ./internal/service -run 'GPT56|Interval|RecordUsage|ServiceTier|CacheWrite' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交 Task 2**

```bash
git add backend/internal/service/billing_service.go backend/internal/service/openai_gateway_service.go backend/internal/service/billing_service_unified_test.go backend/internal/service/openai_gateway_record_usage_test.go
git commit -m "fix(billing): align gpt-5.6 model candidates and interval context"
```

---

### Task 3: 对齐 Chat→Responses 请求字段

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go:155-212,487-507`
- Modify: `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
- Test: `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`

**Interfaces:**
- Produces: `ResponsesRequest.ParallelToolCalls`、`ResponsesRequest.Text`、`ChatCompletionsRequest.ResponseFormat`、`IsReasoningChatModel(string) bool` 和正确的 GPT-5.x 请求转换。
- Consumes: 无 service 包依赖；reasoning model 判断在 `apicompat` 内完成。

- [ ] **Step 1: 加入失败测试**

在 `chatcompletions_responses_test.go` 增加：

```go
func TestChatCompletionsToResponses_GPT56StripsSampling(t *testing.T) {
	v := 0.7
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt5.6", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Temperature: &v, TopP: &v,
	})
	require.NoError(t, err)
	require.Nil(t, resp.Temperature)
	require.Nil(t, resp.TopP)
}

func TestChatCompletionsToResponses_ResponseFormatAndParallelTools(t *testing.T) {
	parallel := false
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt-5.6", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		ResponseFormat: json.RawMessage(`{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object"},"strict":true}}`),
		ParallelToolCalls: &parallel,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Text)
	require.JSONEq(t, `{"type":"json_schema","name":"answer","schema":{"type":"object"},"strict":true}`, string(resp.Text.Format))
	require.NotNil(t, resp.ParallelToolCalls)
	require.False(t, *resp.ParallelToolCalls)
}

func TestChatCompletionsToResponses_EmptyContentNeverNull(t *testing.T) {
	for _, content := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`[{"type":"text","text":""}]`)} {
		resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{Model: "gpt-5.6", Messages: []ChatMessage{{Role: "user", Content: content}}})
		require.NoError(t, err)
		require.NotContains(t, string(resp.Input), `"content":null`)
	}
}
```

同时把 legacy function 测试改为期望平铺 `name`，并增加：

```go
func TestChatCompletionsToResponses_ToolStrictDefaultsFalse(t *testing.T) {
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt-5.6",
		Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools: []ChatTool{{Type: "function", Function: &ChatFunction{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Tools, 1)
	require.NotNil(t, resp.Tools[0].Strict)
	require.False(t, *resp.Tools[0].Strict)
}

func TestChatCompletionsToResponses_AssistantReasoningContentPreserved(t *testing.T) {
	resp, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model: "gpt-5.6",
		Messages: []ChatMessage{{Role: "assistant", Content: json.RawMessage(`"answer"`), ReasoningContent: "internal plan"}},
	})
	require.NoError(t, err)
	require.Contains(t, string(resp.Input), "<thinking>internal plan</thinking>")
	require.Contains(t, string(resp.Input), "answer")
}

func TestChatCompletionsToResponses_InvalidResponseFormatReturnsError(t *testing.T) {
	_, err := ChatCompletionsToResponses(&ChatCompletionsRequest{
		Model:          "gpt-5.6",
		Messages:       []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		ResponseFormat: json.RawMessage(`{"type":"json_schema","json_schema":`),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "response_format")
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd backend
go test -tags unit ./internal/pkg/apicompat -run 'GPT56StripsSampling|ResponseFormatAndParallelTools|InvalidResponseFormat|EmptyContentNeverNull|LegacyFunctions|ToolStrict|ReasoningContent' -count=1
```

Expected: FAIL；类型字段缺失、sampling 未剥离或 content 产生 null。

- [ ] **Step 3: 扩展请求类型**

在 `types.go` 增加：

```go
type ResponsesText struct {
	Format    json.RawMessage `json:"format,omitempty"`
	Verbosity string          `json:"verbosity,omitempty"`
}

// ResponsesRequest
ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
Text              *ResponsesText `json:"text,omitempty"`

// ChatCompletionsRequest
ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
ResponseFormat    json.RawMessage `json:"response_format,omitempty"`
```

- [ ] **Step 4: 实现 sampling 和 response format 转换**

在 `chatcompletions_to_responses.go` 增加 `bytes` import，并实现：

```go
func IsReasoningChatModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}
	if strings.HasPrefix(model, "gpt5") {
		model = "gpt-5" + strings.TrimPrefix(model, "gpt5")
	}
	return strings.HasPrefix(model, "gpt-5")
}

func chatResponseFormatToResponsesTextFormat(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parse response_format: %w", err)
	}
	var typ string
	if err := json.Unmarshal(obj["type"], &typ); err != nil {
		return nil, fmt.Errorf("parse response_format.type: %w", err)
	}
	if typ != "json_schema" {
		return append(json.RawMessage(nil), raw...), nil
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(obj["json_schema"], &schema); err != nil {
		return nil, fmt.Errorf("parse response_format.json_schema: %w", err)
	}
	if schema == nil {
		return nil, fmt.Errorf("parse response_format.json_schema: expected object")
	}
	schema["type"] = json.RawMessage(`"json_schema"`)
	out, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal response_format.text.format: %w", err)
	}
	return out, nil
}
```

构造请求时使用：

```go
out := &ResponsesRequest{
	Model: req.Model, Instructions: req.Instructions, Input: inputJSON,
	Stream: true, Include: []string{"reasoning.encrypted_content"},
	ServiceTier: req.ServiceTier, ParallelToolCalls: req.ParallelToolCalls,
}
	if !IsReasoningChatModel(req.Model) {
	out.Temperature = req.Temperature
	out.TopP = req.TopP
}
	format, err := chatResponseFormatToResponsesTextFormat(req.ResponseFormat)
	if err != nil {
		return nil, err
	}
	if len(format) > 0 {
		out.Text = &ResponsesText{Format: format}
	}
```

- [ ] **Step 5: 修正 tool、空 content 和 reasoning history**

加入：

```go
func defaultStrictFalse(src *bool) *bool {
	if src != nil {
		return src
	}
	v := false
	return &v
}
```

function tools 使用 `Strict: defaultStrictFalse(...)`；legacy function choice 返回：

```go
return json.Marshal(map[string]any{"type": "function", "name": obj.Name})
```

`marshalChatInputContent` 在转换后 parts 长度为零时返回 `json.Marshal("")`。`chatAssistantToResponses` 先把 `ReasoningContent` 包成 `<thinking>...</thinking>`，再与正文合并为一个 assistant output_text item。

- [ ] **Step 6: 运行 apicompat 全包测试**

```bash
cd backend
go test -tags unit ./internal/pkg/apicompat -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交 Task 3**

```bash
git add backend/internal/pkg/apicompat/types.go backend/internal/pkg/apicompat/chatcompletions_to_responses.go backend/internal/pkg/apicompat/chatcompletions_responses_test.go
git commit -m "fix(openai): align gpt-5.6 chat request conversion"
```

---

### Task 4: 对齐 Chat Gateway、OAuth Transform 与 Service Tier

**Files:**
- Modify: `backend/internal/service/openai_gateway_chat_completions.go`
- Modify: `backend/internal/service/openai_codex_transform.go`
- Modify: `backend/internal/service/openai_gateway_service.go:2418-2505,5765-5797`
- Test: `backend/internal/service/openai_gateway_chat_completions_test.go`
- Test: `backend/internal/service/openai_codex_transform_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

**Interfaces:**
- Consumes: Task 3 的扩展 `ResponsesRequest`。
- Produces: `normalizeResponsesRequestServiceTier`、`normalizeResponsesBodyServiceTier`、可跳过默认 instructions 的 OAuth transform、reasoning encrypted include。

- [ ] **Step 1: 写入 tier 与 OAuth transform 失败测试**

```go
func TestNormalizeResponsesRequestServiceTier_OfficialValues(t *testing.T) {
	for input, want := range map[string]string{
		" fast ": "priority", "priority": "priority", "flex": "flex",
		"auto": "auto", "default": "default", "scale": "scale", "turbo": "",
	} {
		req := &apicompat.ResponsesRequest{ServiceTier: input}
		normalizeResponsesRequestServiceTier(req)
		require.Equal(t, want, req.ServiceTier)
	}
}

func TestNormalizeResponsesBodyServiceTier_OfficialValues(t *testing.T) {
	for input, want := range map[string]string{"fast": "priority", "auto": "auto", "default": "default", "scale": "scale", "turbo": ""} {
		body, tier, err := normalizeResponsesBodyServiceTier([]byte(`{"model":"gpt-5.6-sol","service_tier":"` + input + `"}`))
		require.NoError(t, err)
		require.Equal(t, want, tier)
		if want == "" {
			require.False(t, gjson.GetBytes(body, "service_tier").Exists())
		} else {
			require.Equal(t, want, gjson.GetBytes(body, "service_tier").String())
		}
	}
}

func TestCalculateCostWithServiceTier_StandardAliasesUseStandardPrice(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 10}
	standard, err := svc.CalculateCostWithServiceTier("gpt-5.6-sol", tokens, 1, "")
	require.NoError(t, err)
	for _, tier := range []string{"auto", "default", "scale"} {
		got, err := svc.CalculateCostWithServiceTier("gpt-5.6-sol", tokens, 1, tier)
		require.NoError(t, err)
		require.InDelta(t, standard.ActualCost, got.ActualCost, 1e-12)
	}
}

func TestApplyCodexOAuthTransform_ReasoningAddsEncryptedInclude(t *testing.T) {
	body := map[string]any{"model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "max"}}
	applyCodexOAuthTransform(body, false, false)
	require.Equal(t, []any{"reasoning.encrypted_content"}, body["include"])
}

func TestApplyCodexOAuthTransform_CompactDoesNotAddEncryptedInclude(t *testing.T) {
	body := map[string]any{"model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "max"}}
	applyCodexOAuthTransform(body, false, true)
	_, exists := body["include"]
	require.False(t, exists)
}

func TestApplyCodexOAuthTransform_ReasoningIncludeAppendsDeduplicatesAndPreservesInvalidType(t *testing.T) {
	appendBody := map[string]any{"reasoning": map[string]any{"effort": "max"}, "include": []any{"file_search_call.results"}}
	applyCodexOAuthTransform(appendBody, false, false)
	require.Equal(t, []any{"file_search_call.results", "reasoning.encrypted_content"}, appendBody["include"])

	dedupBody := map[string]any{"reasoning": map[string]any{"effort": "max"}, "include": []any{"reasoning.encrypted_content"}}
	applyCodexOAuthTransform(dedupBody, false, false)
	require.Equal(t, []any{"reasoning.encrypted_content"}, dedupBody["include"])

	invalidBody := map[string]any{"reasoning": map[string]any{"effort": "max"}, "include": "invalid"}
	applyCodexOAuthTransform(invalidBody, false, false)
	require.Equal(t, "invalid", invalidBody["include"])
}

func TestApplyCodexOAuthTransform_StripsInternalUnsupportedFields(t *testing.T) {
	body := map[string]any{"model": "gpt-5.6-sol", "user": "u", "metadata": map[string]any{"a": 1}, "safety_identifier": "s", "stream_options": map[string]any{}}
	applyCodexOAuthTransform(body, false, false)
	for _, key := range []string{"user", "metadata", "safety_identifier", "stream_options"} {
		_, exists := body[key]
		require.False(t, exists)
	}
}
```

在 `openai_gateway_chat_completions_test.go` 复用同包的 `httpUpstreamRecorder`，增加以下两个完整测试；同时补充 `bytes`、`context`、`config` 和 `gjson` imports：

```go
func TestForwardAsChatCompletions_APIKeyPropagatesPromptCacheKeyAndStripsSamplingAfterMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"custom-gpt56","messages":[{"role":"user","content":"hello"}],"temperature":0.7,"top_p":0.8,"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Set("api_key", &APIKey{ID: 99})
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "cache-key-123", "gpt-5.6-sol")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, "cache-key-123", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "temperature").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "top_p").Exists())
}

func TestForwardAsChatCompletions_OAuthDoesNotInjectDefaultInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.6","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth", "chatgpt_account_id": "acc"}}
	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.6-sol")
	require.Error(t, err)
	require.Nil(t, result)
	require.True(t, gjson.GetBytes(upstream.lastBody, "instructions").Exists())
	require.Equal(t, "", gjson.GetBytes(upstream.lastBody, "instructions").String())
	require.NotContains(t, string(upstream.lastBody), "You are a helpful coding assistant.")
}

func TestForwardAsChatCompletions_APIKeyResponsesShapePreservesOAuthOnlyFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"custom-model","input":"hello","prompt_cache_retention":"24h","safety_identifier":"safe","metadata":{"a":1},"stream_options":{"include_usage":true}}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"stop"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}}
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "custom-model")
	require.Error(t, err)
	for _, path := range []string{"prompt_cache_retention", "safety_identifier", "metadata.a", "stream_options.include_usage"} {
		require.True(t, gjson.GetBytes(upstream.lastBody, path).Exists(), path)
	}
}
```

在现有 `TestOpenAIGatewayService_OAuthPassthrough_StreamingSetsFirstTokenMs` 末尾增加请求体断言，使测试先于实现失败：

```go
require.Equal(t, "priority", gjson.GetBytes(upstream.lastBody, "service_tier").String())
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd backend
go test -tags unit ./internal/service -run 'NormalizeResponses|ApplyCodexOAuthTransform|StandardAliases|PromptCacheKey|Instructions|OAuthOnlyFields|OAuthPassthrough' -count=1
```

Expected: FAIL。

- [ ] **Step 3: 扩展 service tier 公共函数**

```go
func normalizeOpenAIServiceTier(raw string) *string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "fast" {
		value = "priority"
	}
	switch value {
	case "priority", "flex", "auto", "default", "scale":
		return &value
	default:
		return nil
	}
}

func normalizeResponsesRequestServiceTier(req *apicompat.ResponsesRequest) {
	if req == nil {
		return
	}
	if normalized := normalizeOpenAIServiceTier(req.ServiceTier); normalized != nil {
		req.ServiceTier = *normalized
	} else {
		req.ServiceTier = ""
	}
}

func normalizeResponsesBodyServiceTier(body []byte) ([]byte, string, error) {
	raw := gjson.GetBytes(body, "service_tier").String()
	if raw == "" {
		return body, "", nil
	}
	normalized := normalizeOpenAIServiceTier(raw)
	if normalized == nil {
		updated, err := sjson.DeleteBytes(body, "service_tier")
		return updated, "", err
	}
	updated, err := sjson.SetBytes(body, "service_tier", *normalized)
	return updated, *normalized, err
}
```

Chat normal conversion 后调用 request helper；Responses-shape 分支调用 body helper并把最终 tier 写入 stub。Native Responses 的 tier 规范化移到 `isCodexCLI` 条件之外。

Chat bridge 完成模型映射后，再用客户端原始模型和最终上游模型复核 sampling。正常 Chat 转换分支使用：

```go
if apicompat.IsReasoningChatModel(originalModel) || apicompat.IsReasoningChatModel(upstreamModel) {
	responsesReq.Temperature = nil
	responsesReq.TopP = nil
}
```

Responses-shape 分支在相同判断为 true 时用 `sjson.DeleteBytes` 删除 `temperature` 和 `top_p`。这样自定义客户端别名映射到 GPT-5.x 后也不会绕过 sampling 清理，非 reasoning model 继续保留原值。

- [ ] **Step 4: 为 OAuth transform 增加选项和 reasoning include**

```go
type codexOAuthTransformOptions struct {
	IsCodexCLI              bool
	IsCompact               bool
	SkipDefaultInstructions bool
}

func applyCodexOAuthTransform(reqBody map[string]any, isCodexCLI, isCompact bool) codexTransformResult {
	return applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{IsCodexCLI: isCodexCLI, IsCompact: isCompact})
}

func ensureCodexReasoningInclude(reqBody map[string]any) bool {
	reasoning, ok := reqBody["reasoning"].(map[string]any)
	if !ok || len(reasoning) == 0 {
		return false
	}
	const encrypted = "reasoning.encrypted_content"
	switch include := reqBody["include"].(type) {
	case nil:
		reqBody["include"] = []any{encrypted}
		return true
	case []string:
		for _, value := range include {
			if value == encrypted {
				return false
			}
		}
		reqBody["include"] = append(include, encrypted)
		return true
	case []any:
		for _, value := range include {
			if value == encrypted {
				return false
			}
		}
		reqBody["include"] = append(include, encrypted)
		return true
	default:
		return false
	}
}
```

把当前 `applyCodexOAuthTransform` 的完整函数体移动到：

```go
func applyCodexOAuthTransformWithOptions(reqBody map[string]any, opts codexOAuthTransformOptions) codexTransformResult
```

并在该函数体内执行以下机械替换：`isCompact` → `opts.IsCompact`、`isCodexCLI` → `opts.IsCodexCLI`；调用 `applyInstructions` 的条件改为 `!opts.SkipDefaultInstructions`。原三参数 wrapper 保留，确保现有调用点不需要一次性修改。

OAuth unsupported 列表必须精确包含：

```go
var openAICodexOAuthUnsupportedFields = []string{
	"max_output_tokens", "max_completion_tokens", "temperature", "top_p",
	"frequency_penalty", "presence_penalty", "user", "metadata",
	"prompt_cache_retention", "safety_identifier", "stream_options",
}
```

非 compact reasoning 调用 include helper；`SkipDefaultInstructions` 为 true 时跳过 `applyInstructions`。同时删除 `openai_gateway_service.go` 中 `!isCodexCLI` 分支对 `prompt_cache_retention`、`safety_identifier` 的无条件清理，并把 Chat responses-shape 的 `cursorResponsesUnsupportedFields` 清理限制为 `account.Type == AccountTypeOAuth`；APIKey 和自定义 OpenAI-compatible 上游保留这些字段。

- [ ] **Step 5: 修正 tool choice、Chat instructions 和 prompt cache key**

legacy `function_call` 生成平铺 name，并加入：

```go
func normalizeCodexToolChoice(reqBody map[string]any) bool {
	choice, ok := reqBody["tool_choice"].(map[string]any)
	if !ok || strings.TrimSpace(firstNonEmptyString(choice["type"])) != "function" {
		return false
	}
	name := strings.TrimSpace(firstNonEmptyString(choice["name"]))
	if name == "" {
		if nested, ok := choice["function"].(map[string]any); ok {
			name = strings.TrimSpace(firstNonEmptyString(nested["name"]))
		}
	}
	if name == "" {
		reqBody["tool_choice"] = "auto"
		return true
	}
	choice["name"] = name
	delete(choice, "function")
	return true
}
```

在 `normalizeCodexTools(reqBody)` 后调用 `normalizeCodexToolChoice(reqBody)`。

Chat OAuth 分支使用 `SkipDefaultInstructions:true`，随后调用：

```go
func ensureCodexOAuthInstructionsField(reqBody map[string]any) {
	if value, ok := reqBody["instructions"]; !ok || value == nil {
		reqBody["instructions"] = ""
	} else if _, ok := value.(string); !ok {
		reqBody["instructions"] = ""
	}
}
```

APIKey Chat 使用以下实现，只在现有值为空时写入：

```go
if account.Type == AccountTypeAPIKey && strings.TrimSpace(promptCacheKey) != "" {
	var reqBody map[string]any
	if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
		return nil, fmt.Errorf("unmarshal for prompt cache key injection: %w", err)
	}
	if existing, _ := reqBody["prompt_cache_key"].(string); strings.TrimSpace(existing) == "" {
		reqBody["prompt_cache_key"] = strings.TrimSpace(promptCacheKey)
		responsesBody, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("remarshal after prompt cache key injection: %w", err)
		}
	}
}
```

- [ ] **Step 6: 运行 service 请求变换回归**

```bash
cd backend
go test -tags unit ./internal/service -run 'ChatCompletions|CodexOAuthTransform|ServiceTier|PromptCache|GPT56|ReasoningEffort' -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交 Task 4**

```bash
git add backend/internal/service/openai_gateway_chat_completions.go backend/internal/service/openai_codex_transform.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_chat_completions_test.go backend/internal/service/openai_codex_transform_test.go backend/internal/service/openai_gateway_service_test.go
git commit -m "fix(openai): align gpt-5.6 responses request transforms"
```

---

### Task 5: 对齐 Responses→Chat 与 HTTP Terminal Usage

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go:420-465,570-603`
- Modify: `backend/internal/pkg/apicompat/responses_to_chatcompletions.go`
- Modify: `backend/internal/service/openai_gateway_chat_completions.go:315-455`
- Modify: `backend/internal/service/openai_gateway_service.go:4419-4539`
- Test: `backend/internal/pkg/apicompat/chatcompletions_responses_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

**Interfaces:**
- Produces: 顶层 terminal `Usage`、Chat `CompletionTokensDetails`、统一 `extractOpenAIUsageFromJSONBytes`。
- Consumes: 现有 `ResponsesUsage` cache write 显式零值逻辑。

- [ ] **Step 1: 写入 response.done、顶层 usage 和 reasoning tokens 测试**

```go
func TestResponsesEventToChatChunks_ResponseDoneTopLevelUsage(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5.6-sol"
	state.IncludeUsage = true
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.done",
		Response: &ResponsesResponse{Status: "completed"},
		Usage: &ResponsesUsage{
			InputTokens: 20, OutputTokens: 10,
			OutputTokensDetails: &ResponsesOutputTokensDetails{ReasoningTokens: 8},
		},
	}, state)
	require.Len(t, chunks, 2)
	require.NotNil(t, chunks[1].Usage.CompletionTokensDetails)
	require.Equal(t, 8, chunks[1].Usage.CompletionTokensDetails.ReasoningTokens)
	require.Nil(t, FinalizeResponsesChatStream(state))
}

func TestResponsesEventToChatChunks_TopLevelUsageWinsOverNestedUsage(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5.6-sol"
	state.IncludeUsage = true
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.done",
		Response: &ResponsesResponse{
			Status: "completed",
			Usage:  &ResponsesUsage{InputTokens: 99, OutputTokens: 88},
		},
		Usage: &ResponsesUsage{InputTokens: 20, OutputTokens: 10},
	}, state)
	require.Len(t, chunks, 2)
	require.Equal(t, 20, chunks[1].Usage.PromptTokens)
	require.Equal(t, 10, chunks[1].Usage.CompletionTokens)
}

func TestResponsesEventToChatChunks_ReasoningTextAndCustomToolDeltas(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.Model = "gpt-5.6-sol"
	reasoning := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.reasoning_text.delta", Delta: "plan",
	}, state)
	require.Len(t, reasoning, 1)
	require.Equal(t, "plan", *reasoning[0].Choices[0].Delta.ReasoningContent)

	_ = ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.output_item.added", OutputIndex: 2,
		Item: &ResponsesOutput{Type: "function_call", CallID: "call_1", Name: "lookup"},
	}, state)
	tool := ResponsesEventToChatChunks(&ResponsesStreamEvent{
		Type: "response.custom_tool_call_input.delta", OutputIndex: 2, Delta: `{"q":"x"}`,
	}, state)
	require.Len(t, tool, 1)
	require.Equal(t, `{"q":"x"}`, tool[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)
}
```

在 `openai_gateway_service_test.go` 增加：

```go
func TestExtractOpenAIUsageFromJSONBytes_TopLevelAndNested(t *testing.T) {
	top, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"type":"response.done","usage":{"input_tokens":20,"output_tokens":10,"input_tokens_details":{"cache_write_tokens":4}}}`))
	require.True(t, ok)
	nested, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":20,"output_tokens":10,"input_tokens_details":{"cache_write_tokens":4}}}}`))
	require.True(t, ok)
	require.Equal(t, top, nested)
	require.Equal(t, 4, top.CacheCreationInputTokens)
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd backend
go test -tags unit ./internal/pkg/apicompat -run 'ResponseDoneTopLevelUsage|TopLevelUsageWins|ReasoningTokens|ReasoningTextAndCustomToolDeltas' -count=1
go test -tags unit ./internal/service -run 'ParseSSEUsage|TopLevelUsage|TerminalUsage' -count=1
```

Expected: FAIL；`response.done` 未 finalize、顶层 usage 未解析或 reasoning details 缺失。

- [ ] **Step 3: 扩展事件与 Chat usage 类型**

在 `types.go` 增加：

```go
// ResponsesStreamEvent
Usage *ResponsesUsage `json:"usage,omitempty"`

// ChatUsage
CompletionTokensDetails *ChatTokenDetails `json:"completion_tokens_details,omitempty"`

// ChatTokenDetails
ReasoningTokens int `json:"reasoning_tokens,omitempty"`
```

保留现有 cache write 顶层扩展和 `PromptTokensDetails`。

- [ ] **Step 4: 扩展 Responses→Chat terminal 和 delta 转换**

事件 switch 使用：

```go
case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
	return resToChatHandleFuncArgsDelta(evt, state)
case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
	return resToChatHandleReasoningDelta(evt, state)
case "response.completed", "response.done", "response.incomplete", "response.failed", "response.cancelled", "response.canceled":
	return resToChatHandleCompleted(evt, state)
```

terminal handler 按“顶层优先、嵌套回退”读取 usage：`evt.Usage` 存在时直接使用，只有不存在时才读取 `evt.Response.Usage`，不能让嵌套值覆盖顶层显式零值。`responsesUsageToChatUsage` 在 `OutputTokensDetails.ReasoningTokens > 0` 时创建 `CompletionTokensDetails`。

使用以下代码追加 completion details：

```go
if u.OutputTokensDetails != nil && u.OutputTokensDetails.ReasoningTokens > 0 {
	usage.CompletionTokensDetails = &ChatTokenDetails{
		ReasoningTokens: u.OutputTokensDetails.ReasoningTokens,
	}
}
```

- [ ] **Step 5: 统一 HTTP usage 位置和终止事件**

```go
func extractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{}, false
	}
	root := gjson.ParseBytes(body)
	if usage, ok := extractOpenAIUsageFromGJSON(root, "usage"); ok {
		return usage, true
	}
	return extractOpenAIUsageFromGJSON(root, "response.usage")
}

func isOpenAIResponsesTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}
```

`parseSSEUsageBytes`、Chat buffered 和 streaming 路径统一使用这两个 helper；terminal event 顶层和 nested usage 都要读取。

- [ ] **Step 6: 运行 usage 回归**

```bash
cd backend
go test -tags unit ./internal/pkg/apicompat -count=1
go test -tags unit ./internal/service -run 'Usage|ChatCompletions|ResponseDone|Terminal|CacheWrite' -count=1
```

Expected: PASS；nested explicit zero cache write 仍优先于顶层 alias。

- [ ] **Step 7: 提交 Task 5**

```bash
git add backend/internal/pkg/apicompat/types.go backend/internal/pkg/apicompat/responses_to_chatcompletions.go backend/internal/pkg/apicompat/chatcompletions_responses_test.go backend/internal/service/openai_gateway_chat_completions.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_test.go
git commit -m "fix(openai): align gpt-5.6 terminal usage conversion"
```

---

### Task 6: 对齐 Responses WebSocket 每轮请求与 Usage

**Files:**
- Modify: `backend/internal/service/openai_ws_forwarder.go:368-401,2515-2590,2935-3057`
- Modify: `backend/internal/service/openai_ws_v2_passthrough_adapter.go`
- Modify: `backend/internal/service/openai_ws_v2/passthrough_relay.go`
- Test support: `backend/internal/service/openai_ws_forwarder_success_test.go`
- Test: `backend/internal/service/openai_ws_forwarder_ingress_session_test.go`
- Test: `backend/internal/service/openai_ws_v2/passthrough_relay_test.go`
- Test: `backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go`

**Interfaces:**
- Consumes: Task 4 的 `normalizeResponsesBodyServiceTier`，Task 5 的 `isOpenAIResponsesTerminalEvent` 与 `extractOpenAIUsageFromJSONBytes`。
- Produces: 主 ingress 和 passthrough relay 的后续 turn model 复用、每帧最终 tier、全部 terminal usage。

- [ ] **Step 1: 写入 WS frame 与 usage 失败测试**

修改现有 `TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_KeepLeaseAcrossTurns`：账号 `Credentials` 增加 `model_mapping: map[string]any{"gpt-5.6": "gpt-5.6-sol"}`；第一轮发送：

```json
{"type":"response.create","model":"gpt-5.6","stream":false,"service_tier":"fast"}
```

第二轮省略 model：

```json
{"type":"response.create","stream":false,"previous_response_id":"resp_ingress_turn_1"}
```

在原 `captureConn.writes` 长度断言后增加：

```go
require.Equal(t, "gpt-5.6-sol", captureConn.writes[0]["model"])
require.Equal(t, "priority", captureConn.writes[0]["service_tier"])
require.Equal(t, "gpt-5.6-sol", captureConn.writes[1]["model"])
```

修改现有 `TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_PassthroughModeRelaysByCaddyAdapter` 的上游事件为：

```json
{"type":"response.done","usage":{"input_tokens":20,"output_tokens":10,"input_tokens_details":{"cache_write_tokens":4}}}
```

并把事件类型断言改为 `response.done`，result 断言改为 input `20`、output `10`、cache creation `4`、tier `priority`；最后增加：

```go
require.Equal(t, "priority", upstreamConn.writes[0]["service_tier"])
```

在 `openai_ws_forwarder_ingress_session_test.go` 增加：

```go
func TestOpenAIWSEventShouldParseUsage_AllTerminalEvents(t *testing.T) {
	for _, eventType := range []string{
		"response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled",
	} {
		require.True(t, openAIWSEventShouldParseUsage(eventType), eventType)
	}
}
```

- [ ] **Step 2: 增加 passthrough relay 的失败测试**

在 `backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go` 增加：

```go
func TestParseUsageAndAccumulate_TopLevelUsageAndCancelledTerminal(t *testing.T) {
	state := &relayState{}
	parsed := parseUsageAndAccumulate(state,
		[]byte(`{"type":"response.cancelled","usage":{"input_tokens":20,"output_tokens":10,"input_tokens_details":{"cache_write_tokens":4}}}`),
		"response.cancelled", nil)
	require.Equal(t, Usage{InputTokens: 20, OutputTokens: 10, CacheCreationInputTokens: 4}, parsed)
	require.True(t, isTerminalEvent("response.cancelled"))
	require.True(t, shouldParseUsage("response.cancelled"))
}

func TestParseUsageAndAccumulate_TopLevelUsageWinsOverNestedUsage(t *testing.T) {
	state := &relayState{}
	parsed := parseUsageAndAccumulate(state,
		[]byte(`{"type":"response.done","usage":{"input_tokens":20,"output_tokens":10},"response":{"usage":{"input_tokens":99,"output_tokens":88}}}`),
		"response.done", nil)
	require.Equal(t, 20, parsed.InputTokens)
	require.Equal(t, 10, parsed.OutputTokens)
}
```

- [ ] **Step 3: 运行 WS 定向测试确认当前缺口**

```bash
cd backend
go test -tags unit ./internal/service -run 'ProxyResponsesWebSocketFromClient_(KeepLeaseAcrossTurns|PassthroughModeRelaysByCaddyAdapter)|OpenAIWSEventShouldParseUsage_AllTerminalEvents' -count=1
go test -tags unit ./internal/service/openai_ws_v2 -run 'ParseUsageAndAccumulate_TopLevelUsage' -count=1
```

Expected: FAIL；第二轮缺 model 被拒绝、passthrough 的 fast 未改写、取消事件 usage 未解析或 done usage 为零。

- [ ] **Step 4: 保存主 ingress session model 并规范化每个 frame**

在 `ProxyResponsesWebSocketFromClient` 的闭包外维护：

```go
ingressSessionOriginalModel := ""
```

解析 frame 时使用：

```go
originalModel := strings.TrimSpace(values[1].String())
modelMissing := originalModel == ""
if modelMissing {
	originalModel = ingressSessionOriginalModel
	if originalModel == "" {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "model is required in response.create payload", nil)
	}
}
upstreamModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
if modelMissing || upstreamModel != originalModel {
	var err error
	normalized, err = applyPayloadMutation(normalized, "model", upstreamModel)
	if err != nil {
		return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", err)
	}
}
normalized, _, err := normalizeResponsesBodyServiceTier(normalized)
if err != nil {
	return openAIWSClientPayload{}, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid service_tier", err)
}
ingressSessionOriginalModel = originalModel
```

`payloadRaw` 必须保存规范化后的 `normalized`。

- [ ] **Step 5: 让 passthrough relay 在每轮请求前执行同一规范化函数**

在 `backend/internal/service/openai_ws_v2/passthrough_relay.go` 的 `RelayOptions` 末尾追加：

```go
NormalizeClientFrame func(msgType coderws.MessageType, payload []byte) ([]byte, error)
```

在 `Relay` 写入第一帧前和 `runClientToUpstream` 每次写入上游前调用该回调；回调返回错误时停止转发并返回 `RelayExit{Stage: "normalize_client_frame", Err: err}`。不设置回调时保持原始透传行为。调用点使用同一份消息类型和替换后的 payload：

```go
normalizeFrame := options.NormalizeClientFrame
if normalizeFrame != nil {
	firstClientMessage, err = normalizeFrame(firstMessageType, firstClientMessage)
	if err != nil {
		return result, &RelayExit{Stage: "normalize_client_frame", Err: err}
	}
	result.RequestModel = strings.TrimSpace(gjson.GetBytes(firstClientMessage, "model").String())
}
// write firstClientMessage as before
go runClientToUpstream(relayCtx, clientConn, writeUpstream, normalizeFrame, markActivity, clientToUpstreamFrames, onTrace, exitCh)
```

并把 `runClientToUpstream` 的签名增加 `normalizeFrame func(coderws.MessageType, []byte) ([]byte, error)`，在 `writeUpstream` 前执行并将返回的 payload 写入上游。

在 `openai_ws_v2_passthrough_adapter.go` 增加 `openAIWSPassthroughRequestNormalizer`，维护 FIFO 元数据队列（原始模型、规范化 tier），并在文件 import 中加入 `sync`、`gjson`、`sjson`。结构和转换实现如下：

```go
type passthroughTurnMetadata struct {
	OriginalModel string
	ServiceTier   string
}

type openAIWSPassthroughRequestNormalizer struct {
	mu                sync.Mutex
	account           *Account
	lastOriginalModel string
	pending           []passthroughTurnMetadata
}

func (n *openAIWSPassthroughRequestNormalizer) Normalize(msgType coderws.MessageType, payload []byte) ([]byte, error) {
	if msgType == coderws.MessageBinary {
		return payload, nil
	}
	if msgType != coderws.MessageText {
		return payload, nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if !gjson.ValidBytes(payload) {
		return nil, fmt.Errorf("invalid websocket request payload")
	}
	eventType := gjson.GetBytes(payload, "type").String()
	if eventType != "" && eventType != "response.create" {
		return payload, nil
	}
	if eventType == "" {
		var err error
		payload, err = sjson.SetBytes(payload, "type", "response.create")
		if err != nil {
			return nil, err
		}
	}
	originalModel := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	modelMissing := originalModel == ""
	if modelMissing {
		originalModel = n.lastOriginalModel
	}
	if originalModel == "" {
		return nil, fmt.Errorf("model is required in response.create payload")
	}
	var err error
	upstreamModel := normalizeOpenAIModelForUpstream(n.account, n.account.GetMappedModel(originalModel))
	if modelMissing || upstreamModel != originalModel {
		payload, err = sjson.SetBytes(payload, "model", upstreamModel)
		if err != nil {
			return nil, err
		}
	}
	var tier string
	payload, tier, err = normalizeResponsesBodyServiceTier(payload)
	if err != nil {
		return nil, err
	}
	n.lastOriginalModel = originalModel
	n.pending = append(n.pending, passthroughTurnMetadata{OriginalModel: originalModel, ServiceTier: tier})
	return payload, nil
}

func (n *openAIWSPassthroughRequestNormalizer) TakeTurnMetadata() (passthroughTurnMetadata, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.pending) == 0 {
		return passthroughTurnMetadata{}, false
	}
	meta := n.pending[0]
	n.pending = n.pending[1:]
	return meta, true
}
```

`Normalize` 必须在互斥锁内更新 `lastOriginalModel` 和 `pending`；`OnTurnComplete` 回调按 FIFO 取出元数据，覆盖 `turn.RequestModel` 并设置结果的规范化 `ServiceTier`。第一帧也必须经过该回调，不能只规范化后续帧。

在 `proxyResponsesWebSocketV2Passthrough` 调用 `RunEntry` 前创建 normalizer：

```go
normalizer := &openAIWSPassthroughRequestNormalizer{account: account}
```

在当前 `RelayOptions` literal 中加入：

```go
NormalizeClientFrame: normalizer.Normalize,
```

把当前 `OnTurnComplete` 回调开头和 `turnResult` 构造改为：

```go
meta, ok := normalizer.TakeTurnMetadata()
if ok {
	turn.RequestModel = meta.OriginalModel
}
var serviceTier *string
if ok && meta.ServiceTier != "" {
	tier := meta.ServiceTier
	serviceTier = &tier
}
turnResult := &OpenAIForwardResult{
	RequestID: turn.RequestID,
	Usage: OpenAIUsage{
		InputTokens:              turn.Usage.InputTokens,
		OutputTokens:             turn.Usage.OutputTokens,
		CacheCreationInputTokens: turn.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     turn.Usage.CacheReadInputTokens,
	},
	Model:           turn.RequestModel,
	ServiceTier:     serviceTier,
	Stream:          true,
	OpenAIWSMode:    true,
	ResponseHeaders: cloneHeader(handshakeHeaders),
	Duration:        turn.Duration,
	FirstTokenMs:    turn.FirstTokenMs,
}
```

每轮 `turn.RequestModel` 和 `OpenAIForwardResult.ServiceTier` 均必须来自 normalizer 的 FIFO 元数据；不得继续从未规范化的 `firstClientMessage` 或首轮原始 tier 读取。`relayResult.RequestModel` 则从已规范化的第一帧重新提取，只用于零 turn 的错误/日志兜底。

- [ ] **Step 6: 统一 WS terminal usage 解析**

```go
func openAIWSEventShouldParseUsage(eventType string) bool {
	return isOpenAIResponsesTerminalEvent(eventType)
}

func parseOpenAIWSResponseUsageFromCompletedEvent(message []byte, usage *OpenAIUsage) {
	if usage == nil {
		return
	}
	if parsed, ok := extractOpenAIUsageFromJSONBytes(message); ok {
		*usage = parsed
	}
}
```

terminal result 的 `ServiceTier` 从实际发送的规范化 payload 提取。

在 `openai_ws_v2/passthrough_relay.go` 中将 `shouldParseUsage` 改为覆盖所有 `isTerminalEvent`；`parseUsageAndAccumulate` 的入口改为：

```go
func shouldParseUsage(eventType string) bool {
	return isTerminalEvent(eventType)
}

usageResult := gjson.GetBytes(message, "usage")
if !usageResult.Exists() {
	usageResult = gjson.GetBytes(message, "response.usage")
}
if !usageResult.Exists() {
	return Usage{}
}
inputResult := usageResult.Get("input_tokens")
outputResult := usageResult.Get("output_tokens")
cachedResult := usageResult.Get("input_tokens_details.cached_tokens")
```

后续对选定的 `usageResult` 复用现有 cache write 优先级和负数钳制。这样顶层显式 usage 优先于嵌套 usage，嵌套字段的显式零值不会被同一 usage 对象的顶层 alias 覆盖，且 `response.failed/incomplete/cancelled/canceled` 携带 usage 时也能计费。

- [ ] **Step 7: 运行 WS 全部相关测试**

```bash
cd backend
go test -tags unit ./internal/service ./internal/service/openai_ws_v2 -run 'OpenAIWS|WebSocket|WSV2|ProxyResponsesWebSocketFromClient|GPT56|ServiceTier|CacheWrite|Terminal|Usage' -count=1
```

Expected: PASS。

- [ ] **Step 8: 提交 Task 6**

```bash
git add backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_ws_v2_passthrough_adapter.go backend/internal/service/openai_ws_v2/passthrough_relay.go backend/internal/service/openai_ws_forwarder_ingress_session_test.go backend/internal/service/openai_ws_v2/passthrough_relay_test.go backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go
git commit -m "fix(openai): align gpt-5.6 websocket request and usage flow"
```

---

### Task 7: 端到端回归、上游语义复核与收口

**Files:**
- Modify only if a regression requires correction: files already listed in Tasks 1-6
- Test: `backend/internal/service`
- Test: `backend/internal/service/openai_ws_v2`
- Test: `backend/internal/pkg/apicompat`

**Interfaces:**
- Consumes: Tasks 1-6 的全部提交。
- Produces: 可审计、无无关改动的 GPT-5.6 端到端对齐分支。

- [ ] **Step 1: 运行 apicompat 全包测试**

```bash
cd backend
go test -tags unit ./internal/pkg/apicompat -count=1
```

Expected: PASS。

- [ ] **Step 2: 运行 GPT-5.6 端到端定向测试**

```bash
cd backend
go test -tags unit ./internal/service ./internal/service/openai_ws_v2 -run 'GPT56|Gpt56|gpt56|Billing|Pricing|ChatCompletions|Responses|ServiceTier|ReasoningEffort|CacheWrite|WebSocket|WSV2|Usage' -count=1
```

Expected: PASS。

- [ ] **Step 3: 运行相关后端完整测试**

```bash
cd backend
go test -tags unit ./internal/service ./internal/service/openai_ws_v2 ./internal/pkg/apicompat -count=1
```

Expected: PASS。若完整 `internal/service` 或 `internal/service/openai_ws_v2` 因无关环境测试失败，记录测试名和证据；Tasks 1-6 的定向测试仍必须全部通过。

- [ ] **Step 4: 与固定上游重新做目标语义检查**

```bash
git diff --no-ext-diff --unified=3 HEAD upstream/main -- \
  backend/internal/pkg/apicompat/chatcompletions_to_responses.go \
  backend/internal/pkg/apicompat/responses_to_chatcompletions.go \
  backend/internal/pkg/apicompat/types.go \
  backend/internal/service/openai_codex_transform.go \
  backend/internal/service/openai_gateway_chat_completions.go \
  backend/internal/service/openai_gateway_service.go \
  backend/internal/service/openai_ws_forwarder.go \
  backend/internal/service/openai_ws_v2_passthrough_adapter.go \
  backend/internal/service/openai_ws_v2/passthrough_relay.go \
  backend/internal/service/billing_service.go
```

Expected: 仍可存在本地结构和非目标功能差异，但规格列出的 GPT-5.6 行为不得再出现在差异清单中。

- [ ] **Step 5: 格式化并检查工作区**

```bash
gofmt -w \
  backend/internal/service/openai_gpt56_alias.go \
  backend/internal/service/billing_service.go \
  backend/internal/service/openai_gateway_service.go \
  backend/internal/service/openai_gateway_chat_completions.go \
  backend/internal/service/openai_codex_transform.go \
  backend/internal/service/openai_ws_forwarder.go \
  backend/internal/service/openai_ws_v2_passthrough_adapter.go \
  backend/internal/service/openai_ws_v2/passthrough_relay.go \
  backend/internal/pkg/apicompat/types.go \
  backend/internal/pkg/apicompat/chatcompletions_to_responses.go \
  backend/internal/pkg/apicompat/responses_to_chatcompletions.go
git diff --check
git status --short
```

Expected: `git diff --check` 无输出；状态只包含本计划允许的文件和测试。

- [ ] **Step 6: 提交验证期间的必要修正**

仅当 Steps 1-5 产生必要修正时执行：

```bash
git add backend/internal/service backend/internal/pkg/apicompat
git commit -m "test(openai): close gpt-5.6 alignment regressions"
```

若没有额外修正，不创建空提交。
