# GPT-5.6 模型支持迁移 Implementation Plan（实施计划）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将上游 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 支持语义手工移植到当前分支。

**Architecture:** 后端沿用当前分支的 `normalizeCodexModel`、`BillingService`、`PricingService` 结构，不引入上游新的 `openai_model_alias.go`。前端只扩展现有模型白名单、预设映射和 `UseKeyModal` 的 OpenCode 配置生成。

**Tech Stack:** Go、Ent 生成代码不改动、Vitest、Vue 3、TypeScript、pnpm、JSON 定价资源。

## Global Constraints

- 采用语义手工移植，不直接 cherry-pick `6cea1c35b`。
- 不引入 `backend/internal/service/openai_model_alias.go`。
- 不修改 `DefaultTestModel`，继续保留当前分支的 `gpt-5.1-codex`。
- 不修改管理端默认 `openai_use_key_model_id`。
- 未知 OpenAI 模型继续走现有定价错误路径，不扩大 fallback 到任意 `gpt-*`。
- 动态定价命中 GPT-5.6 时优先使用动态定价；动态定价缺失时 GPT-5.6 回退到 GPT-5.4 静态价格和长上下文策略。
- 不重构前端模型白名单结构。

---

## 文件结构

- 修改 `backend/internal/pkg/openai/constants.go`：默认 OpenAI 模型列表新增 GPT-5.6 三个模型。
- 新建 `backend/internal/pkg/openai/constants_test.go`：验证默认模型列表包含 GPT-5.6 三个模型。
- 修改 `backend/internal/service/openai_codex_transform.go`：`codexModelMap` 和 `normalizeCodexModel` 支持 GPT-5.6 三个模型。
- 修改 `backend/internal/service/openai_codex_transform_test.go`：扩展归一化测试。
- 修改 `backend/internal/service/gpt55_support_test.go`：补 GPT-5.6 归一化、计费和 `PricingService` 静态 fallback 测试。
- 修改 `backend/internal/service/billing_service.go`：GPT-5.6 计费 fallback 指向 GPT-5.4，并纳入长上下文策略。
- 修改 `backend/internal/service/billing_service_test.go`：验证 GPT-5.6 fallback 价格和长上下文倍数。
- 修改 `backend/internal/service/pricing_service.go`：动态定价缺失时 `gpt-5.6*` 回退到 `openAIGPT54FallbackPricing`。
- 修改 `backend/internal/service/pricing_service_test.go`：验证 `PricingService` 的 GPT-5.6 静态 fallback。
- 修改 `backend/resources/model-pricing/model_prices_and_context_window.json`：新增三个 GPT-5.6 定价对象。
- 修改 `frontend/src/composables/useModelWhitelist.ts`：OpenAI 模型列表和预设映射暴露 GPT-5.6 三个模型。
- 修改 `frontend/src/composables/__tests__/useModelWhitelist.spec.ts`：验证模型列表和 whitelist 自身映射。
- 修改 `frontend/src/components/keys/UseKeyModal.vue`：OpenCode 配置模型元数据新增 GPT-5.6。
- 修改 `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`：验证 OpenCode 配置输出 GPT-5.6 元数据。

---

### Task 1: 后端模型目录与 Codex 归一化

**Files:**
- Create: `backend/internal/pkg/openai/constants_test.go`
- Modify: `backend/internal/pkg/openai/constants.go`
- Modify: `backend/internal/service/openai_codex_transform.go`
- Modify: `backend/internal/service/openai_codex_transform_test.go`
- Modify: `backend/internal/service/gpt55_support_test.go`

**Interfaces:**
- Consumes: 当前 `openai.DefaultModels`、`openai.DefaultModelIDs()`、`codexModelMap map[string]string`、`normalizeCodexModel(model string) string`。
- Produces: `normalizeCodexModel` 对 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 返回精确基础模型；后续计费任务依赖这些返回值。

- [ ] **Step 1: 编写默认模型列表失败测试**

在 `backend/internal/pkg/openai/constants_test.go` 新建：

```go
package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelsIncludeGPT56Variants(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "gpt-5.6-sol")
	require.Contains(t, ids, "gpt-5.6-terra")
	require.Contains(t, ids, "gpt-5.6-luna")
}
```

- [ ] **Step 2: 编写 Codex 归一化失败测试**

在 `backend/internal/service/openai_codex_transform_test.go` 的 `TestNormalizeCodexModel_Gpt53` 用例 map 顶部加入：

```go
"gpt-5.6-sol":             "gpt-5.6-sol",
"gpt-5.6-sol-high":        "gpt-5.6-sol",
"gpt 5.6 sol":             "gpt-5.6-sol",
"openai/gpt-5.6-sol":      "gpt-5.6-sol",
"gpt-5.6-terra":           "gpt-5.6-terra",
"gpt-5.6-terra-xhigh":     "gpt-5.6-terra",
"gpt 5.6 terra":           "gpt-5.6-terra",
"gpt-5.6-luna":            "gpt-5.6-luna",
"gpt-5.6-luna-medium":     "gpt-5.6-luna",
"gpt 5.6 luna":            "gpt-5.6-luna",
```

在 `backend/internal/service/gpt55_support_test.go` 末尾新增：

```go
func TestGPT56Support_NormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.6-sol":         "gpt-5.6-sol",
		"gpt-5.6-sol-high":    "gpt-5.6-sol",
		"gpt 5.6 sol":         "gpt-5.6-sol",
		"gpt-5.6-terra":       "gpt-5.6-terra",
		"gpt-5.6-terra-xhigh": "gpt-5.6-terra",
		"gpt 5.6 terra":       "gpt-5.6-terra",
		"gpt-5.6-luna":        "gpt-5.6-luna",
		"gpt-5.6-luna-medium": "gpt-5.6-luna",
		"gpt 5.6 luna":        "gpt-5.6-luna",
	}

	for input, expected := range cases {
		require.Equal(t, expected, normalizeCodexModel(input))
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run:

```bash
cd backend
go test -tags unit ./internal/pkg/openai ./internal/service -run 'TestDefaultModelsIncludeGPT56Variants|TestNormalizeCodexModel_Gpt53|TestGPT56Support_NormalizeCodexModel'
```

Expected: FAIL，至少包含 `gpt-5.6-sol` 未出现在默认模型列表，或 `normalizeCodexModel` 返回 `gpt-5.1`。

- [ ] **Step 4: 实现默认模型和归一化**

在 `backend/internal/pkg/openai/constants.go` 的 `DefaultModels` 开头、`gpt-5.5` 之前加入：

```go
	{ID: "gpt-5.6-sol", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 Sol"},
	{ID: "gpt-5.6-terra", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 Terra"},
	{ID: "gpt-5.6-luna", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 Luna"},
```

在 `backend/internal/service/openai_codex_transform.go` 的 `codexModelMap` 开头加入：

```go
	"gpt-5.6-sol":   "gpt-5.6-sol",
	"gpt-5.6-terra": "gpt-5.6-terra",
	"gpt-5.6-luna":  "gpt-5.6-luna",
```

在 `normalizeCodexModel` 中，`if strings.Contains(normalized, "gpt-5.5")` 之前加入：

```go
	if strings.Contains(normalized, "gpt-5.6-sol") || strings.Contains(normalized, "gpt 5.6 sol") {
		return "gpt-5.6-sol"
	}
	if strings.Contains(normalized, "gpt-5.6-terra") || strings.Contains(normalized, "gpt 5.6 terra") {
		return "gpt-5.6-terra"
	}
	if strings.Contains(normalized, "gpt-5.6-luna") || strings.Contains(normalized, "gpt 5.6 luna") {
		return "gpt-5.6-luna"
	}
```

- [ ] **Step 5: 格式化并运行测试确认通过**

Run:

```bash
cd backend
gofmt -w internal/pkg/openai/constants.go internal/pkg/openai/constants_test.go internal/service/openai_codex_transform.go internal/service/openai_codex_transform_test.go internal/service/gpt55_support_test.go
go test -tags unit ./internal/pkg/openai ./internal/service -run 'TestDefaultModelsIncludeGPT56Variants|TestNormalizeCodexModel_Gpt53|TestGPT56Support_NormalizeCodexModel'
```

Expected: PASS。

- [ ] **Step 6: 提交 Task 1**

```bash
git add backend/internal/pkg/openai/constants.go backend/internal/pkg/openai/constants_test.go backend/internal/service/openai_codex_transform.go backend/internal/service/openai_codex_transform_test.go backend/internal/service/gpt55_support_test.go
git commit -m "feat(openai): add gpt-5.6 model normalization"
```

---

### Task 2: 后端计费、动态定价 fallback 与定价资源

**Files:**
- Modify: `backend/internal/service/billing_service.go`
- Modify: `backend/internal/service/billing_service_test.go`
- Modify: `backend/internal/service/pricing_service.go`
- Modify: `backend/internal/service/pricing_service_test.go`
- Modify: `backend/internal/service/gpt55_support_test.go`
- Modify: `backend/resources/model-pricing/model_prices_and_context_window.json`

**Interfaces:**
- Consumes: Task 1 产出的 `normalizeCodexModel(model string) string`。
- Produces: `BillingService.GetModelPricing("gpt-5.6-*")` 返回 GPT-5.4 fallback 价格；`PricingService.GetModelPricing("gpt-5.6-*")` 在动态定价缺失时返回 `openAIGPT54FallbackPricing`。

- [ ] **Step 1: 编写 BillingService 失败测试**

在 `backend/internal/service/billing_service_test.go` 的 `TestGetModelPricing_OpenAIGPT55Fallback` 后加入：

```go
func TestGetModelPricing_OpenAIGPT56Fallbacks(t *testing.T) {
	svc := newTestBillingService()

	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		t.Run(model, func(t *testing.T) {
			pricing, err := svc.GetModelPricing(model)
			require.NoError(t, err)
			require.NotNil(t, pricing)
			require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
			require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
			require.InDelta(t, 0.25e-6, pricing.CacheReadPricePerToken, 1e-12)
			require.Equal(t, 272000, pricing.LongContextInputThreshold)
			require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
			require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
		})
	}
}
```

在 `TestCalculateCost_OpenAIGPT55LongContextAppliesWholeSessionMultipliers` 后加入：

```go
func TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{
		InputTokens:  300000,
		OutputTokens: 4000,
	}

	cost, err := svc.CalculateCost("gpt-5.6-sol", tokens, 1.0)
	require.NoError(t, err)

	expectedInput := float64(tokens.InputTokens) * 2.5e-6 * 2.0
	expectedOutput := float64(tokens.OutputTokens) * 15e-6 * 1.5
	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.ActualCost, 1e-10)
}
```

在 `backend/internal/service/gpt55_support_test.go` 末尾加入：

```go
func TestGPT56Support_BillingFallbackMatchesGPT54(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	pricing, err := svc.GetModelPricing("gpt-5.6-sol")
	require.NoError(t, err)
	require.NotNil(t, pricing)
	require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
}
```

- [ ] **Step 2: 编写 PricingService 失败测试**

在 `backend/internal/service/pricing_service_test.go` 的 `TestGetModelPricing_Gpt55UsesGpt54StaticFallbackWhenRemoteMissing` 后加入：

```go
func TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		t.Run(model, func(t *testing.T) {
			got := svc.GetModelPricing(model)
			require.NotNil(t, got)
			require.InDelta(t, 2.5e-6, got.InputCostPerToken, 1e-12)
			require.InDelta(t, 1.5e-5, got.OutputCostPerToken, 1e-12)
			require.InDelta(t, 2.5e-7, got.CacheReadInputTokenCost, 1e-12)
			require.Equal(t, 272000, got.LongContextInputTokenThreshold)
			require.InDelta(t, 2.0, got.LongContextInputCostMultiplier, 1e-12)
			require.InDelta(t, 1.5, got.LongContextOutputCostMultiplier, 1e-12)
		})
	}
}
```

在 `backend/internal/service/gpt55_support_test.go` 末尾加入：

```go
func TestGPT56Support_PricingServiceStaticFallback(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.6-sol")
	require.NotNil(t, got)
	require.InDelta(t, 2.5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, got.OutputCostPerToken, 1e-12)
	require.Equal(t, 272000, got.LongContextInputTokenThreshold)
}
```

- [ ] **Step 3: 运行计费测试确认失败**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'TestGetModelPricing_OpenAIGPT56Fallbacks|TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers|TestGPT56Support_BillingFallbackMatchesGPT54|TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing|TestGPT56Support_PricingServiceStaticFallback'
```

Expected: FAIL，`GetModelPricing("gpt-5.6-sol")` 返回定价缺失或落入错误 fallback。

- [ ] **Step 4: 实现 BillingService fallback**

在 `backend/internal/service/billing_service.go` 的 GPT-5.5 fallback 后加入：

```go
	// GPT-5.6（sol / terra / luna）暂无独立静态定价，回退到 GPT-5.4。
	s.fallbackPrices["gpt-5.6-sol"] = s.fallbackPrices["gpt-5.4"]
	s.fallbackPrices["gpt-5.6-terra"] = s.fallbackPrices["gpt-5.4"]
	s.fallbackPrices["gpt-5.6-luna"] = s.fallbackPrices["gpt-5.4"]
```

在 `getFallbackPricing` 的 OpenAI `switch normalized` 中，在 `case "gpt-5.5":` 前加入：

```go
			case "gpt-5.6-sol":
				return s.fallbackPrices["gpt-5.6-sol"]
			case "gpt-5.6-terra":
				return s.fallbackPrices["gpt-5.6-terra"]
			case "gpt-5.6-luna":
				return s.fallbackPrices["gpt-5.6-luna"]
```

将 `isOpenAIGPT54Model` 的返回表达式改为：

```go
	return normalized == "gpt-5.4" || normalized == "gpt-5.5" ||
		normalized == "gpt-5.6-sol" || normalized == "gpt-5.6-terra" || normalized == "gpt-5.6-luna"
```

- [ ] **Step 5: 实现 PricingService fallback**

在 `backend/internal/service/pricing_service.go` 的 `matchOpenAIModel` 中，`gpt-5.5` fallback 前加入：

```go
	// GPT-5.6（sol / terra / luna）回退到 GPT-5.4 定价。
	if isOpenAIGPT56StaticFallbackModel(model) {
		logger.With(zap.String("component", "service.pricing")).
			Info(fmt.Sprintf("[Pricing] OpenAI fallback matched %s -> %s", model, "gpt-5.4(static)"))
		return openAIGPT54FallbackPricing
	}
```

```go
func isOpenAIGPT56StaticFallbackModel(model string) bool {
	for _, base := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
		if model == base || strings.HasPrefix(model, base+"-") {
			return true
		}
	}

	return false
}
```

- [ ] **Step 6: 加入动态定价资源**

在 `backend/resources/model-pricing/model_prices_and_context_window.json` 中，定位 `"gpt-5.5": {`，在它之前插入以下完整对象：

```json
  "gpt-5.6-sol": {
    "cache_read_input_token_cost": 5e-07,
    "cache_read_input_token_cost_above_272k_tokens": 1e-06,
    "cache_read_input_token_cost_flex": 2.5e-07,
    "cache_read_input_token_cost_priority": 1e-06,
    "input_cost_per_token": 5e-06,
    "input_cost_per_token_above_272k_tokens": 1e-05,
    "input_cost_per_token_batches": 2.5e-06,
    "input_cost_per_token_flex": 2.5e-06,
    "input_cost_per_token_priority": 1e-05,
    "litellm_provider": "openai",
    "max_input_tokens": 1050000,
    "max_output_tokens": 128000,
    "max_tokens": 128000,
    "mode": "chat",
    "output_cost_per_token": 3e-05,
    "output_cost_per_token_above_272k_tokens": 4.5e-05,
    "output_cost_per_token_batches": 1.5e-05,
    "output_cost_per_token_flex": 1.5e-05,
    "output_cost_per_token_priority": 6e-05,
    "supported_endpoints": [
      "/v1/chat/completions",
      "/v1/batch",
      "/v1/responses"
    ],
    "supported_modalities": [
      "text",
      "image"
    ],
    "supported_output_modalities": [
      "text"
    ],
    "supports_function_calling": true,
    "supports_minimal_reasoning_effort": false,
    "supports_native_streaming": true,
    "supports_none_reasoning_effort": true,
    "supports_parallel_function_calling": true,
    "supports_pdf_input": true,
    "supports_prompt_caching": true,
    "supports_reasoning": true,
    "supports_response_schema": true,
    "supports_service_tier": true,
    "supports_system_messages": true,
    "supports_tool_choice": true,
    "supports_vision": true,
    "supports_web_search": true,
    "supports_xhigh_reasoning_effort": true
  },
  "gpt-5.6-terra": {
    "cache_read_input_token_cost": 5e-07,
    "cache_read_input_token_cost_above_272k_tokens": 1e-06,
    "cache_read_input_token_cost_flex": 2.5e-07,
    "cache_read_input_token_cost_priority": 1e-06,
    "input_cost_per_token": 5e-06,
    "input_cost_per_token_above_272k_tokens": 1e-05,
    "input_cost_per_token_batches": 2.5e-06,
    "input_cost_per_token_flex": 2.5e-06,
    "input_cost_per_token_priority": 1e-05,
    "litellm_provider": "openai",
    "max_input_tokens": 1050000,
    "max_output_tokens": 128000,
    "max_tokens": 128000,
    "mode": "chat",
    "output_cost_per_token": 3e-05,
    "output_cost_per_token_above_272k_tokens": 4.5e-05,
    "output_cost_per_token_batches": 1.5e-05,
    "output_cost_per_token_flex": 1.5e-05,
    "output_cost_per_token_priority": 6e-05,
    "supported_endpoints": [
      "/v1/chat/completions",
      "/v1/batch",
      "/v1/responses"
    ],
    "supported_modalities": [
      "text",
      "image"
    ],
    "supported_output_modalities": [
      "text"
    ],
    "supports_function_calling": true,
    "supports_minimal_reasoning_effort": false,
    "supports_native_streaming": true,
    "supports_none_reasoning_effort": true,
    "supports_parallel_function_calling": true,
    "supports_pdf_input": true,
    "supports_prompt_caching": true,
    "supports_reasoning": true,
    "supports_response_schema": true,
    "supports_service_tier": true,
    "supports_system_messages": true,
    "supports_tool_choice": true,
    "supports_vision": true,
    "supports_web_search": true,
    "supports_xhigh_reasoning_effort": true
  },
  "gpt-5.6-luna": {
    "cache_read_input_token_cost": 5e-07,
    "cache_read_input_token_cost_above_272k_tokens": 1e-06,
    "cache_read_input_token_cost_flex": 2.5e-07,
    "cache_read_input_token_cost_priority": 1e-06,
    "input_cost_per_token": 5e-06,
    "input_cost_per_token_above_272k_tokens": 1e-05,
    "input_cost_per_token_batches": 2.5e-06,
    "input_cost_per_token_flex": 2.5e-06,
    "input_cost_per_token_priority": 1e-05,
    "litellm_provider": "openai",
    "max_input_tokens": 1050000,
    "max_output_tokens": 128000,
    "max_tokens": 128000,
    "mode": "chat",
    "output_cost_per_token": 3e-05,
    "output_cost_per_token_above_272k_tokens": 4.5e-05,
    "output_cost_per_token_batches": 1.5e-05,
    "output_cost_per_token_flex": 1.5e-05,
    "output_cost_per_token_priority": 6e-05,
    "supported_endpoints": [
      "/v1/chat/completions",
      "/v1/batch",
      "/v1/responses"
    ],
    "supported_modalities": [
      "text",
      "image"
    ],
    "supported_output_modalities": [
      "text"
    ],
    "supports_function_calling": true,
    "supports_minimal_reasoning_effort": false,
    "supports_native_streaming": true,
    "supports_none_reasoning_effort": true,
    "supports_parallel_function_calling": true,
    "supports_pdf_input": true,
    "supports_prompt_caching": true,
    "supports_reasoning": true,
    "supports_response_schema": true,
    "supports_service_tier": true,
    "supports_system_messages": true,
    "supports_tool_choice": true,
    "supports_vision": true,
    "supports_web_search": true,
    "supports_xhigh_reasoning_effort": true
  },
```

- [ ] **Step 7: 格式化、验证 JSON，并运行计费测试**

Run:

```bash
cd backend
gofmt -w internal/service/billing_service.go internal/service/billing_service_test.go internal/service/pricing_service.go internal/service/pricing_service_test.go internal/service/gpt55_support_test.go
python3 -m json.tool resources/model-pricing/model_prices_and_context_window.json >/tmp/gpt56-pricing.json
go test -tags unit ./internal/service -run 'TestGetModelPricing_OpenAIGPT56Fallbacks|TestCalculateCost_OpenAIGPT56LongContextAppliesWholeSessionMultipliers|TestGPT56Support_BillingFallbackMatchesGPT54|TestGetModelPricing_Gpt56UsesGpt54StaticFallbackWhenRemoteMissing|TestGPT56Support_PricingServiceStaticFallback'
```

Expected: JSON validation exits 0 and Go tests PASS。

- [ ] **Step 8: 提交 Task 2**

```bash
git add backend/internal/service/billing_service.go backend/internal/service/billing_service_test.go backend/internal/service/pricing_service.go backend/internal/service/pricing_service_test.go backend/internal/service/gpt55_support_test.go backend/resources/model-pricing/model_prices_and_context_window.json
git commit -m "feat(openai): add gpt-5.6 pricing fallback"
```

---

### Task 3: 前端模型白名单、预设映射和 OpenCode 配置

**Files:**
- Modify: `frontend/src/composables/useModelWhitelist.ts`
- Modify: `frontend/src/composables/__tests__/useModelWhitelist.spec.ts`
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

**Interfaces:**
- Consumes: 当前 `getModelsByPlatform(platform: string)`、`buildModelMappingObject(mode, selectedModels, rows)`、`generateOpenCodeConfig(...)` 的既有行为。
- Produces: OpenAI 模型列表包含 GPT-5.6 三个模型；whitelist 模式可生成自身映射；OpenCode 配置包含 GPT-5.6 元数据。

- [ ] **Step 1: 编写 useModelWhitelist 失败测试**

在 `frontend/src/composables/__tests__/useModelWhitelist.spec.ts` 第一个测试中加入断言：

```ts
expect(models).toContain('gpt-5.6-sol')
expect(models).toContain('gpt-5.6-terra')
expect(models).toContain('gpt-5.6-luna')
```

在文件末尾新增：

```ts
it('whitelist keeps GPT-5.6 exact mappings', () => {
  const mapping = buildModelMappingObject('whitelist', ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna'], [])

  expect(mapping).toEqual({
    'gpt-5.6-sol': 'gpt-5.6-sol',
    'gpt-5.6-terra': 'gpt-5.6-terra',
    'gpt-5.6-luna': 'gpt-5.6-luna'
  })
})
```

- [ ] **Step 2: 编写 UseKeyModal 失败测试**

在 `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` 的 `renders updated GPT-5.4 mini/nano names in OpenCode config` 测试中加入：

```ts
expect(codeBlock.text()).toContain('"name": "GPT-5.6 Sol"')
expect(codeBlock.text()).toContain('"name": "GPT-5.6 Terra"')
expect(codeBlock.text()).toContain('"name": "GPT-5.6 Luna"')
expect(codeBlock.text()).toContain('"context": 1050000')
expect(codeBlock.text()).toContain('"output": 128000')
```

- [ ] **Step 3: 运行前端测试确认失败**

Run:

```bash
pnpm --dir frontend test:run -- useModelWhitelist UseKeyModal
```

Expected: FAIL，输出显示 GPT-5.6 相关断言未满足。

- [ ] **Step 4: 实现 useModelWhitelist 扩展**

在 `frontend/src/composables/useModelWhitelist.ts` 的 OpenAI 模型列表中，`// GPT-5.5 系列` 前加入：

```ts
  // GPT-5.6 系列
  'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna',
```

在 `openaiPresetMappings` 中，`GPT-5.2` 后、`GPT-5.5` 前加入：

```ts
  { label: 'GPT-5.6 Sol', from: 'gpt-5.6-sol', to: 'gpt-5.6-sol', color: 'bg-orange-100 text-orange-700 hover:bg-orange-200 dark:bg-orange-900/30 dark:text-orange-400' },
  { label: 'GPT-5.6 Terra', from: 'gpt-5.6-terra', to: 'gpt-5.6-terra', color: 'bg-lime-100 text-lime-700 hover:bg-lime-200 dark:bg-lime-900/30 dark:text-lime-400' },
  { label: 'GPT-5.6 Luna', from: 'gpt-5.6-luna', to: 'gpt-5.6-luna', color: 'bg-sky-100 text-sky-700 hover:bg-sky-200 dark:bg-sky-900/30 dark:text-sky-400' },
```

- [ ] **Step 5: 实现 UseKeyModal OpenCode 模型元数据**

在 `frontend/src/components/keys/UseKeyModal.vue` 的 `generateOpenCodeConfig` 内，`'gpt-5.5'` 条目前加入：

```ts
    'gpt-5.6-sol': {
      name: 'GPT-5.6 Sol',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.6-terra': {
      name: 'GPT-5.6 Terra',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
    'gpt-5.6-luna': {
      name: 'GPT-5.6 Luna',
      limit: {
        context: 1050000,
        output: 128000
      },
      options: {
        store: false
      },
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {}
      }
    },
```

- [ ] **Step 6: 运行前端测试确认通过**

Run:

```bash
pnpm --dir frontend test:run -- useModelWhitelist UseKeyModal
```

Expected: PASS。

- [ ] **Step 7: 提交 Task 3**

```bash
git add frontend/src/composables/useModelWhitelist.ts frontend/src/composables/__tests__/useModelWhitelist.spec.ts frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "feat(frontend): expose gpt-5.6 models"
```

---

### Task 4: 全量定向验证与收口

**Files:**
- Modify: 本任务不修改源码文件
- Test: backend and frontend targeted suites

**Interfaces:**
- Consumes: Task 1、Task 2、Task 3 的全部提交。
- Produces: 干净工作区和最终验证结果。

- [ ] **Step 1: 后端定向验证**

Run:

```bash
cd backend
go test -tags unit ./internal/pkg/openai ./internal/service
```

Expected: PASS。

- [ ] **Step 2: 前端定向验证**

Run:

```bash
pnpm --dir frontend test:run -- useModelWhitelist UseKeyModal
```

Expected: PASS。

- [ ] **Step 3: 搜索确认 GPT-5.6 覆盖面**

Run:

```bash
rg -n "gpt-5\\.6|GPT-5\\.6" backend/internal/pkg/openai backend/internal/service backend/resources/model-pricing frontend/src/composables frontend/src/components/keys
```

Expected: 输出包含后端默认模型、归一化、计费 fallback、定价资源、前端模型白名单、OpenCode 配置和对应测试。

- [ ] **Step 4: 检查工作区状态**

Run:

```bash
git status --short --branch
```

Expected: 只显示当前分支行，无未提交文件。

- [ ] **Step 5: 汇总实现结果**

最终回复需要包含：

```text
已完成 GPT-5.6 语义手工移植，覆盖 gpt-5.6-sol、gpt-5.6-terra、gpt-5.6-luna。
验证：后端 go test -tags unit ./internal/pkg/openai ./internal/service；前端 pnpm --dir frontend test:run -- useModelWhitelist UseKeyModal。
```
