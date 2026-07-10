# GPT-5.6 上游后续补丁精准移植 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `upstream/main` 中 GPT-5.6 后续修复按语义精准移植到当前分支，补齐别名、定价、cache write usage、reasoning effort、Codex 版本和前端配置。

**Architecture:** 保持当前分支文件结构，仅新增一个窄 helper `backend/internal/service/openai_gpt56_alias.go` 统一 GPT-5.6 判定和别名归一化。后端 pricing、billing、usage、reasoning 和 Codex header 逻辑复用该 helper；前端只补白名单与 OpenCode 配置，不引入上游大重构。

**Tech Stack:** Go 1.x backend, `testing` + `testify/require`, `tidwall/gjson`, `tidwall/sjson`, Vue 3 frontend, Vitest, TypeScript.

## Global Constraints

- 不合并整个 `upstream/main`。
- 不引入上游无关的大文件拆分或网关重构。
- 不改变 Claude、Gemini、Grok、图片生成、支付、推广、账号管理等无关行为。
- 不修改当前分支的默认测试模型。
- 未知 `gpt-5.6-*` 不自动回退到 Sol/Terra/Luna 或 GPT-5.4，避免误计价。
- 裸 `gpt-5.6` 作为 Sol 别名处理。
- GPT-5.6 `max` reasoning effort 普通路径保留；OpenAI OAuth compact 子请求发送上游前降级为 `xhigh`。
- Codex CLI User-Agent、compact `Version`、账号使用量探测版本升级到 `0.144.1`。
- 默认值升级不得覆盖管理员已保存的 `codex_cli_user_agent` 与 `codex_cli_version` 配置。
- GPT-5.6 Sol/Terra/Luna 官方长上下文参数固定为 `long_context_input_token_threshold=272000`、`long_context_input_cost_multiplier=2`、`long_context_output_cost_multiplier=1.5`。

---

## File Structure

- Create `backend/internal/service/openai_gpt56_alias.go`: GPT-5.6 alias normalization, variant detection, effort suffix detection.
- Modify `backend/internal/service/openai_codex_transform.go`: call the GPT-5.6 helper before generic GPT-5 fallback.
- Modify `backend/internal/pkg/openai/constants.go`: add bare `gpt-5.6` default model.
- Modify `backend/resources/model-pricing/model_prices_and_context_window.json`: update GPT-5.6 Sol/Terra/Luna official pricing.
- Modify `backend/internal/service/pricing_service.go`: parse priority cache write price and return GPT-5.6 official static fallbacks.
- Modify `backend/internal/service/billing_service.go`: carry priority cache write, explicit cache write marker, GPT-5.6 policies, long-context totals.
- Modify `backend/internal/service/model_pricing_resolver.go`: preserve explicit cache write overrides from channel flat and interval pricing.
- Modify `backend/internal/pkg/apicompat/types.go`: add cache write fields to Responses and Chat usage types.
- Modify `backend/internal/service/openai_gateway_service.go`: centralize OpenAI usage parsing and reasoning effort model candidates.
- Modify `backend/internal/service/openai_gateway_messages.go`: use central usage extraction for Responses-to-Messages paths.
- Modify `backend/internal/service/openai_ws_forwarder.go`: use central usage extraction for WS completed events and final response JSON.
- Modify `backend/internal/service/openai_ws_v2/passthrough_relay.go`: parse and accumulate cache write tokens in WS v2 relay.
- Modify `backend/internal/service/account_usage_service.go`: bump Codex probe version.
- Modify `backend/internal/service/domain_constants.go`, `backend/internal/service/settings_view.go`, `backend/internal/service/openai_gateway_service.go`: bump Codex default version comments and constants.
- Modify `frontend/src/composables/useModelWhitelist.ts`: add bare GPT-5.6 and preset mapping.
- Modify `frontend/src/components/keys/UseKeyModal.vue`: add bare GPT-5.6 OpenCode model and `max` variants.
- Update adjacent tests listed in each task.

---

### Task 1: GPT-5.6 Alias Helper And Default Model

**Files:**
- Create: `backend/internal/service/openai_gpt56_alias.go`
- Modify: `backend/internal/service/openai_codex_transform.go`
- Modify: `backend/internal/pkg/openai/constants.go`
- Test: `backend/internal/pkg/openai/constants_test.go`
- Test: `backend/internal/service/openai_codex_transform_test.go`

**Interfaces:**
- Produces: `func normalizeGPT56ModelAlias(model string) (string, bool)`
- Produces: `func isGPT56KnownModel(model string) bool`
- Produces: `func isGPT56BareOrEffortAlias(model string) bool`
- Consumes: `normalizeCodexModel(model string) string` calls `normalizeGPT56ModelAlias` before generic GPT-5 matching.

- [ ] **Step 1: Write failing default model test**

Add to `backend/internal/pkg/openai/constants_test.go`:

```go
func TestDefaultModelsIncludeBareGPT56Alias(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "gpt-5.6")
	require.Contains(t, ids, "gpt-5.6-sol")
	require.Contains(t, ids, "gpt-5.6-terra")
	require.Contains(t, ids, "gpt-5.6-luna")
}
```

- [ ] **Step 2: Write failing alias normalization tests**

Add to `backend/internal/service/openai_codex_transform_test.go`:

```go
func TestNormalizeCodexModel_GPT56Aliases(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "bare", model: "gpt-5.6", want: "gpt-5.6-sol"},
		{name: "bare max", model: "gpt-5.6-max", want: "gpt-5.6-sol"},
		{name: "bare xhigh", model: "gpt-5.6-xhigh", want: "gpt-5.6-sol"},
		{name: "provider sol", model: "openai/gpt-5.6-sol", want: "gpt-5.6-sol"},
		{name: "space terra high", model: "GPT 5.6 Terra High", want: "gpt-5.6-terra"},
		{name: "underscore luna", model: "gpt_5.6_luna_max", want: "gpt-5.6-luna"},
		{name: "sol suffix", model: "gpt-5.6-sol-preview", want: "gpt-5.6-sol"},
		{name: "unknown bare suffix stays generic fallback", model: "gpt-5.6-foo", want: "gpt-5.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeCodexModel(tt.model))
		})
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
cd backend && go test -tags unit ./internal/pkg/openai ./internal/service -run 'TestDefaultModelsIncludeBareGPT56Alias|TestNormalizeCodexModel_GPT56Aliases' -count=1
```

Expected: failures showing missing `gpt-5.6` default model and wrong alias normalization for bare/max forms.

- [ ] **Step 4: Add GPT-5.6 helper**

Create `backend/internal/service/openai_gpt56_alias.go` with this implementation shape:

```go
package service

import "strings"

func normalizeGPT56ModelAlias(model string) (string, bool) {
	modelID := normalizeGPT56ModelID(model)
	if modelID == "" {
		return "", false
	}
	if modelID == "gpt-5.6" {
		return "gpt-5.6-sol", true
	}
	if !strings.HasPrefix(modelID, "gpt-5.6-") {
		return "", false
	}

	suffix := strings.TrimPrefix(modelID, "gpt-5.6-")
	for _, variant := range []string{"sol", "terra", "luna"} {
		if suffix == variant || strings.HasPrefix(suffix, variant+"-") {
			return "gpt-5.6-" + variant, true
		}
	}
	if isGPT56ReasoningSuffix(suffix) {
		return "gpt-5.6-sol", true
	}
	return "", false
}

func isGPT56KnownModel(model string) bool {
	_, ok := normalizeGPT56ModelAlias(model)
	return ok
}

func isGPT56BareOrEffortAlias(model string) bool {
	modelID := normalizeGPT56ModelID(model)
	if modelID == "gpt-5.6" {
		return true
	}
	return strings.HasPrefix(modelID, "gpt-5.6-") &&
		isGPT56ReasoningSuffix(strings.TrimPrefix(modelID, "gpt-5.6-"))
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
	return strings.Trim(modelID, "-")
}

func isGPT56ReasoningSuffix(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "extrahigh", "max":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 5: Wire helper into model normalization and default model list**

In `backend/internal/service/openai_codex_transform.go`, at the start of `normalizeCodexModel` after provider-prefix stripping and before `getNormalizedCodexModel`:

```go
	if mapped, ok := normalizeGPT56ModelAlias(modelID); ok {
		return mapped
	}
```

In `backend/internal/pkg/openai/constants.go`, add the bare alias before the three variants:

```go
	{ID: "gpt-5.6", Object: "model", Created: 1780876800, OwnedBy: "openai", Type: "model", DisplayName: "GPT-5.6 (Sol)"},
```

- [ ] **Step 6: Run tests and commit**

Run:

```bash
cd backend && go test -tags unit ./internal/pkg/openai ./internal/service -run 'TestDefaultModelsIncludeBareGPT56|TestNormalizeCodexModel_GPT56' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/service/openai_gpt56_alias.go backend/internal/service/openai_codex_transform.go backend/internal/pkg/openai/constants.go backend/internal/pkg/openai/constants_test.go backend/internal/service/openai_codex_transform_test.go
git commit -m "feat: normalize gpt-5.6 aliases"
```

---

### Task 2: PricingService GPT-5.6 Official Prices

**Files:**
- Modify: `backend/resources/model-pricing/model_prices_and_context_window.json`
- Modify: `backend/internal/service/pricing_service.go`
- Test: `backend/internal/service/pricing_service_test.go`

**Interfaces:**
- Consumes: `normalizeGPT56ModelAlias(model string) (string, bool)`
- Produces: `LiteLLMModelPricing.CacheCreationInputTokenCostPriority float64`
- Produces: `LiteLLMRawEntry.CacheCreationInputTokenCostPriority *float64`

- [ ] **Step 1: Write failing PricingService tests**

Add tests to `backend/internal/service/pricing_service_test.go`:

```go
func TestPricingService_GPT56OfficialStaticFallbacks(t *testing.T) {
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{}}

	tests := []struct {
		model      string
		input      float64
		output     float64
		cacheWrite float64
		cacheRead  float64
	}{
		{model: "gpt-5.6", input: 5e-6, output: 30e-6, cacheWrite: 6.25e-6, cacheRead: 0.5e-6},
		{model: "gpt-5.6-max", input: 5e-6, output: 30e-6, cacheWrite: 6.25e-6, cacheRead: 0.5e-6},
		{model: "gpt-5.6-terra-high", input: 2.5e-6, output: 15e-6, cacheWrite: 3.125e-6, cacheRead: 0.25e-6},
		{model: "gpt-5.6-luna-preview", input: 1e-6, output: 6e-6, cacheWrite: 1.25e-6, cacheRead: 0.1e-6},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := svc.GetModelPricing(tt.model)
			require.NotNil(t, got)
			require.InDelta(t, tt.input, got.InputCostPerToken, 1e-12)
			require.InDelta(t, tt.output, got.OutputCostPerToken, 1e-12)
			require.InDelta(t, tt.cacheWrite, got.CacheCreationInputTokenCost, 1e-12)
			require.InDelta(t, tt.cacheRead, got.CacheReadInputTokenCost, 1e-12)
			require.Equal(t, 272000, got.LongContextInputTokenThreshold)
			require.InDelta(t, 2.0, got.LongContextInputCostMultiplier, 1e-12)
			require.InDelta(t, 1.5, got.LongContextOutputCostMultiplier, 1e-12)
			require.True(t, got.SupportsServiceTier)
			require.True(t, got.SupportsPromptCaching)
		})
	}
}

func TestPricingService_GPT56UnknownDoesNotFallback(t *testing.T) {
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{}}

	require.Nil(t, svc.GetModelPricing("gpt-5.6-foo"))
}

func TestPricingService_GPT56BareUsesDynamicSolPricing(t *testing.T) {
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.6-sol": {InputCostPerToken: 4e-6, OutputCostPerToken: 24e-6},
	}}

	got := svc.GetModelPricing("gpt-5.6")
	require.NotNil(t, got)
	require.InDelta(t, 4e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 24e-6, got.OutputCostPerToken, 1e-12)
}

func TestParsePricingData_ReadsPriorityCacheWrite(t *testing.T) {
	svc := &PricingService{}
	data := []byte(`{
		"gpt-5.6-sol": {
			"input_cost_per_token": 0.000005,
			"output_cost_per_token": 0.00003,
			"cache_creation_input_token_cost": 0.00000625,
			"cache_creation_input_token_cost_priority": 0.0000125,
			"cache_read_input_token_cost": 0.0000005,
			"cache_read_input_token_cost_priority": 0.000001,
			"supports_service_tier": true,
			"supports_prompt_caching": true
		}
	}`)

	prices, err := svc.parsePricingData(data)
	require.NoError(t, err)
	require.InDelta(t, 12.5e-6, prices["gpt-5.6-sol"].CacheCreationInputTokenCostPriority, 1e-12)
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestPricingService_GPT56|TestParsePricingData_ReadsPriorityCacheWrite' -count=1
```

Expected: FAIL because GPT-5.6 uses GPT-5.4 fallback and priority cache write is not parsed.

- [ ] **Step 3: Add pricing fields and static fallback objects**

In `backend/internal/service/pricing_service.go`, add:

```go
	CacheCreationInputTokenCostPriority float64 `json:"cache_creation_input_token_cost_priority"`
```

to `LiteLLMModelPricing`, and:

```go
	CacheCreationInputTokenCostPriority *float64 `json:"cache_creation_input_token_cost_priority"`
```

to `LiteLLMRawEntry`.

In `parsePricingData`, add:

```go
		if entry.CacheCreationInputTokenCostPriority != nil {
			pricing.CacheCreationInputTokenCostPriority = *entry.CacheCreationInputTokenCostPriority
		}
```

Add three fallback objects near the existing OpenAI fallback vars:

```go
	openAIGPT56SolFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:                   5e-6,
		InputCostPerTokenPriority:           10e-6,
		OutputCostPerToken:                  30e-6,
		OutputCostPerTokenPriority:          60e-6,
		CacheCreationInputTokenCost:         6.25e-6,
		CacheCreationInputTokenCostPriority: 12.5e-6,
		CacheReadInputTokenCost:             0.5e-6,
		CacheReadInputTokenCostPriority:     1e-6,
		LongContextInputTokenThreshold:      272000,
		LongContextInputCostMultiplier:      2.0,
		LongContextOutputCostMultiplier:     1.5,
		SupportsServiceTier:                 true,
		LiteLLMProvider:                     "openai",
		Mode:                                "chat",
		SupportsPromptCaching:               true,
	}
	openAIGPT56TerraFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:                   2.5e-6,
		InputCostPerTokenPriority:           5e-6,
		OutputCostPerToken:                  15e-6,
		OutputCostPerTokenPriority:          30e-6,
		CacheCreationInputTokenCost:         3.125e-6,
		CacheCreationInputTokenCostPriority: 6.25e-6,
		CacheReadInputTokenCost:             0.25e-6,
		CacheReadInputTokenCostPriority:     0.5e-6,
		LongContextInputTokenThreshold:      272000,
		LongContextInputCostMultiplier:      2.0,
		LongContextOutputCostMultiplier:     1.5,
		SupportsServiceTier:                 true,
		LiteLLMProvider:                     "openai",
		Mode:                                "chat",
		SupportsPromptCaching:               true,
	}
	openAIGPT56LunaFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:                   1e-6,
		InputCostPerTokenPriority:           2e-6,
		OutputCostPerToken:                  6e-6,
		OutputCostPerTokenPriority:          12e-6,
		CacheCreationInputTokenCost:         1.25e-6,
		CacheCreationInputTokenCostPriority: 2.5e-6,
		CacheReadInputTokenCost:             0.1e-6,
		CacheReadInputTokenCostPriority:     0.2e-6,
		LongContextInputTokenThreshold:      272000,
		LongContextInputCostMultiplier:      2.0,
		LongContextOutputCostMultiplier:     1.5,
		SupportsServiceTier:                 true,
		LiteLLMProvider:                     "openai",
		Mode:                                "chat",
		SupportsPromptCaching:               true,
	}
```

- [ ] **Step 4: Replace GPT-5.6 fallback matching**

In `buildModelLookupCandidates`, prepend the normalized GPT-5.6 alias so dynamic data wins before static fallback:

```go
	if normalized, ok := normalizeGPT56ModelAlias(modelLower); ok {
		candidates = append([]string{normalized}, candidates...)
	}
```

Place this after the initial `candidates := []string{...}` assignment and before duplicate removal.

In `matchOpenAIModel`, replace the existing GPT-5.6 GPT-5.4 fallback branch with:

```go
	if normalized, ok := normalizeGPT56ModelAlias(model); ok {
		switch normalized {
		case "gpt-5.6-sol":
			return openAIGPT56SolFallbackPricing
		case "gpt-5.6-terra":
			return openAIGPT56TerraFallbackPricing
		case "gpt-5.6-luna":
			return openAIGPT56LunaFallbackPricing
		}
	}
```

Remove `isOpenAIGPT56StaticFallbackModel` if it becomes unused.

- [ ] **Step 5: Update bundled pricing resource**

In `backend/resources/model-pricing/model_prices_and_context_window.json`, set these exact GPT-5.6 fields:

```json
"gpt-5.6-sol": {
  "input_cost_per_token": 0.000005,
  "input_cost_per_token_priority": 0.00001,
  "output_cost_per_token": 0.00003,
  "output_cost_per_token_priority": 0.00006,
  "cache_creation_input_token_cost": 0.00000625,
  "cache_creation_input_token_cost_priority": 0.0000125,
  "cache_read_input_token_cost": 0.0000005,
  "cache_read_input_token_cost_priority": 0.000001,
  "long_context_input_token_threshold": 272000,
  "long_context_input_cost_multiplier": 2,
  "long_context_output_cost_multiplier": 1.5,
  "supports_service_tier": true,
  "supports_prompt_caching": true
}
```

Use Terra values `2.5e-6`, `5e-6`, `15e-6`, `30e-6`, `3.125e-6`, `6.25e-6`, `0.25e-6`, `0.5e-6`; use Luna values `1e-6`, `2e-6`, `6e-6`, `12e-6`, `1.25e-6`, `2.5e-6`, `0.1e-6`, `0.2e-6`.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestPricingService_GPT56|TestParsePricingData_ReadsPriorityCacheWrite' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/service/pricing_service.go backend/internal/service/pricing_service_test.go backend/resources/model-pricing/model_prices_and_context_window.json
git commit -m "feat: add gpt-5.6 official pricing"
```

---

### Task 3: BillingService Cache Write And Explicit Overrides

**Files:**
- Modify: `backend/internal/service/billing_service.go`
- Modify: `backend/internal/service/model_pricing_resolver.go`
- Test: `backend/internal/service/billing_service_test.go`
- Test: `backend/internal/service/model_pricing_resolver_test.go`

**Interfaces:**
- Consumes: `LiteLLMModelPricing.CacheCreationInputTokenCostPriority`
- Consumes: `normalizeGPT56ModelAlias(model string) (string, bool)`
- Produces: `ModelPricing.CacheCreationPricePerTokenPriority float64`
- Produces: `ModelPricing.CacheCreationPriceExplicit bool`

- [ ] **Step 1: Write failing billing tests**

Add to `backend/internal/service/billing_service_test.go`:

```go
func TestBillingService_GPT56OfficialFallbackPrices(t *testing.T) {
	svc := newTestBillingService()

	sol, err := svc.GetModelPricing("gpt-5.6")
	require.NoError(t, err)
	require.InDelta(t, 5e-6, sol.InputPricePerToken, 1e-12)
	require.InDelta(t, 30e-6, sol.OutputPricePerToken, 1e-12)
	require.InDelta(t, 6.25e-6, sol.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 12.5e-6, sol.CacheCreationPricePerTokenPriority, 1e-12)

	terra, err := svc.GetModelPricing("gpt-5.6-terra-high")
	require.NoError(t, err)
	require.InDelta(t, 2.5e-6, terra.InputPricePerToken, 1e-12)

	luna, err := svc.GetModelPricing("gpt-5.6-luna-preview")
	require.NoError(t, err)
	require.InDelta(t, 1e-6, luna.InputPricePerToken, 1e-12)
}

func TestCalculateCostWithServiceTier_GPT56PriorityUsesCacheWritePriority(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 40, CacheReadTokens: 20}

	cost, err := svc.CalculateCostWithServiceTier("gpt-5.6", tokens, 1.0, "priority")
	require.NoError(t, err)

	require.InDelta(t, float64(100)*10e-6, cost.InputCost, 1e-12)
	require.InDelta(t, float64(50)*60e-6, cost.OutputCost, 1e-12)
	require.InDelta(t, float64(40)*12.5e-6, cost.CacheCreationCost, 1e-12)
	require.InDelta(t, float64(20)*1e-6, cost.CacheReadCost, 1e-12)
}

func TestBillingService_GPT56DynamicMissingCacheWriteGetsPolicyPrice(t *testing.T) {
	svc := NewBillingService(&config.Config{}, &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.6-sol": {
				InputCostPerToken: 5e-6,
				OutputCostPerToken: 30e-6,
				CacheReadInputTokenCost: 0.5e-6,
			},
		},
	})

	pricing, err := svc.GetModelPricing("gpt-5.6")
	require.NoError(t, err)
	require.InDelta(t, 6.25e-6, pricing.CacheCreationPricePerToken, 1e-12)
}

func TestBillingService_GPT56LongContextIncludesCacheWrite(t *testing.T) {
	svc := newTestBillingService()
	tokens := UsageTokens{InputTokens: 1000, CacheCreationTokens: 272001, OutputTokens: 10}

	cost, err := svc.CalculateCost("gpt-5.6", tokens, 1.0)
	require.NoError(t, err)
	require.InDelta(t, float64(1000)*5e-6*2, cost.InputCost, 1e-12)
	require.InDelta(t, float64(10)*30e-6*1.5, cost.OutputCost, 1e-12)
}
```

- [ ] **Step 2: Write failing resolver explicit override tests**

Add to `backend/internal/service/model_pricing_resolver_test.go`:

```go
func TestModelPricingResolver_GPT56FlatCacheWriteZeroIsExplicit(t *testing.T) {
	zero := 0.0
	resolved := &ResolvedPricing{BasePricing: &ModelPricing{InputPricePerToken: 5e-6}}
	r := &ModelPricingResolver{}

	r.applyTokenOverrides(&ChannelModelPricing{CacheWritePrice: &zero}, resolved)

	require.True(t, resolved.BasePricing.CacheCreationPriceExplicit)
	require.Equal(t, 0.0, resolved.BasePricing.CacheCreationPricePerToken)
}

func TestIntervalToModelPricing_CacheWriteZeroIsExplicit(t *testing.T) {
	zero := 0.0
	pricing := intervalToModelPricing(&PricingInterval{CacheWritePrice: &zero}, false)

	require.True(t, pricing.CacheCreationPriceExplicit)
	require.Equal(t, 0.0, pricing.CacheCreationPricePerToken)
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestBillingService_GPT56|TestCalculateCostWithServiceTier_GPT56|TestModelPricingResolver_GPT56|TestIntervalToModelPricing_CacheWriteZero' -count=1
```

Expected: FAIL because GPT-5.6 uses GPT-5.4 prices, priority cache write is not used, and explicit zero is not marked.

- [ ] **Step 4: Extend ModelPricing and dynamic mapping**

In `backend/internal/service/billing_service.go`, extend `ModelPricing`:

```go
	CacheCreationPricePerTokenPriority float64 // priority service tier 下缓存创建每token价格 (USD)
	CacheCreationPriceExplicit         bool    // 渠道或区间定价显式设置过缓存写入价
```

In `GetModelPricing`, copy priority cache write from LiteLLM:

```go
				CacheCreationPricePerTokenPriority: litellmPricing.CacheCreationInputTokenCostPriority,
```

In `usePriorityServiceTierPricing`, include:

```go
		pricing.CacheCreationPricePerTokenPriority > 0 ||
```

- [ ] **Step 5: Replace GPT-5.6 fallback prices and policy**

In `initFallbackPricing`, replace the three GPT-5.6 assignments with official `ModelPricing` objects matching Task 2 prices. Each object must include priority cache write and long-context parameters:

```go
	s.fallbackPrices["gpt-5.6-sol"] = &ModelPricing{
		InputPricePerToken:                  5e-6,
		InputPricePerTokenPriority:          10e-6,
		OutputPricePerToken:                 30e-6,
		OutputPricePerTokenPriority:         60e-6,
		CacheCreationPricePerToken:          6.25e-6,
		CacheCreationPricePerTokenPriority:  12.5e-6,
		CacheReadPricePerToken:              0.5e-6,
		CacheReadPricePerTokenPriority:      1e-6,
		SupportsCacheBreakdown:              false,
		LongContextInputThreshold:           openAIGPT54LongContextInputThreshold,
		LongContextInputMultiplier:          openAIGPT54LongContextInputMultiplier,
		LongContextOutputMultiplier:         openAIGPT54LongContextOutputMultiplier,
	}
```

Use Terra and Luna values from Task 2.

In `applyModelSpecificPricingPolicy`, add GPT-5.6 policy before GPT-5.4 long-context policy:

```go
	if isGPT56KnownModel(model) {
		cloned := *pricing
		if cloned.CacheCreationPricePerToken <= 0 && !cloned.CacheCreationPriceExplicit {
			cloned.CacheCreationPricePerToken = cloned.InputPricePerToken * 1.25
			if cloned.CacheCreation5mPrice <= 0 {
				cloned.CacheCreation5mPrice = cloned.CacheCreationPricePerToken
			}
		}
		if cloned.CacheCreationPricePerTokenPriority <= 0 && cloned.InputPricePerTokenPriority > 0 && !cloned.CacheCreationPriceExplicit {
			cloned.CacheCreationPricePerTokenPriority = cloned.InputPricePerTokenPriority * 1.25
		}
		if cloned.LongContextInputThreshold <= 0 {
			cloned.LongContextInputThreshold = openAIGPT54LongContextInputThreshold
		}
		if cloned.LongContextInputMultiplier <= 0 {
			cloned.LongContextInputMultiplier = openAIGPT54LongContextInputMultiplier
		}
		if cloned.LongContextOutputMultiplier <= 0 {
			cloned.LongContextOutputMultiplier = openAIGPT54LongContextOutputMultiplier
		}
		return &cloned
	}
```

- [ ] **Step 6: Use priority cache write during cost computation**

In `computeTokenBreakdown`, create `cachePricing := pricing` before priority handling. In the priority branch, clone pricing when `CacheCreationPricePerTokenPriority > 0`:

```go
	cachePricing := pricing
	if usePriorityServiceTierPricing(serviceTier, pricing) {
		if pricing.CacheCreationPricePerTokenPriority > 0 {
			cloned := *pricing
			cloned.CacheCreationPricePerToken = pricing.CacheCreationPricePerTokenPriority
			cloned.CacheCreation5mPrice = pricing.CacheCreationPricePerTokenPriority
			cloned.CacheCreation1hPrice = pricing.CacheCreationPricePerTokenPriority
			cachePricing = &cloned
		}
	}
```

Then replace:

```go
	bd.CacheCreationCost = s.computeCacheCreationCost(pricing, tokens)
```

with:

```go
	bd.CacheCreationCost = s.computeCacheCreationCost(cachePricing, tokens)
```

In `shouldApplySessionLongContextPricing`, change total input tokens to:

```go
	totalInputTokens := tokens.InputTokens + tokens.CacheCreationTokens + tokens.CacheReadTokens
```

- [ ] **Step 7: Preserve explicit channel and interval cache write**

In `GetModelPricingWithChannel`, when `channelPricing.CacheWritePrice != nil`, add:

```go
		pricing.CacheCreationPriceExplicit = true
```

In `model_pricing_resolver.go`, add the same marker in `applyTokenOverrides` and `intervalToModelPricing` when `CacheWritePrice != nil`.

- [ ] **Step 8: Run tests and commit**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestBillingService_GPT56|TestCalculateCostWithServiceTier_GPT56|TestModelPricingResolver_GPT56|TestIntervalToModelPricing_CacheWriteZero|TestGetModelPricingWithChannel_CacheWritePrice' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/service/billing_service.go backend/internal/service/model_pricing_resolver.go backend/internal/service/billing_service_test.go backend/internal/service/model_pricing_resolver_test.go
git commit -m "feat: bill gpt-5.6 cache write tokens"
```

---

### Task 4: OpenAI Usage Cache Write Parsing

**Files:**
- Modify: `backend/internal/pkg/apicompat/types.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_messages.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Modify: `backend/internal/service/openai_ws_v2/passthrough_relay.go`
- Test: `backend/internal/service/openai_gateway_service_hotpath_test.go`
- Test: `backend/internal/service/openai_ws_v2/passthrough_relay_test.go`
- Test: `backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go`

**Interfaces:**
- Produces: `func extractOpenAIUsageFromGJSON(root gjson.Result, usagePath string) (OpenAIUsage, bool)`
- Produces: `func clampOpenAIUsageToken(v int64) int`
- Consumes: `OpenAIUsage.CacheCreationInputTokens`

- [ ] **Step 1: Write failing usage parsing tests**

Add to `backend/internal/service/openai_gateway_service_hotpath_test.go`:

```go
func TestExtractOpenAIUsageFromJSONBytes_CacheWriteFields(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want int
	}{
		{name: "nested cache write", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_write_tokens":7}}}`), want: 7},
		{name: "nested cache creation", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_creation_tokens":8}}}`), want: 8},
		{name: "top level cache write input", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"cache_write_input_tokens":9}}`), want: 9},
		{name: "top level cache creation input", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":6}}`), want: 6},
		{name: "negative clamps to zero", body: []byte(`{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_write_tokens":-4}}}`), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractOpenAIUsageFromJSONBytes(tt.body)
			require.True(t, ok)
			require.Equal(t, tt.want, got.CacheCreationInputTokens)
		})
	}
}

func TestParseSSEUsageBytes_CacheWriteFields(t *testing.T) {
	svc := &OpenAIGatewayService{}
	usage := &OpenAIUsage{}

	svc.parseSSEUsageBytes([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":4}}}}`), usage)

	require.Equal(t, 10, usage.InputTokens)
	require.Equal(t, 1, usage.OutputTokens)
	require.Equal(t, 3, usage.CacheReadInputTokens)
	require.Equal(t, 4, usage.CacheCreationInputTokens)
}
```

- [ ] **Step 2: Write failing WS v2 cache write tests**

In `backend/internal/service/openai_ws_v2/passthrough_relay_test.go`, add a completed-event fixture with both cached and cache write tokens:

```go
func TestRelay_UsageIncludesCacheWriteTokens(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_cache_write","usage":{"input_tokens":7,"output_tokens":3,"input_tokens_details":{"cached_tokens":2,"cache_write_tokens":5}}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-5.6","input":[{"type":"input_text","text":"hello"}]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.Nil(t, relayExit)
	require.Equal(t, "gpt-5.6", result.RequestModel)
	require.Equal(t, "resp_cache_write", result.RequestID)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.CacheReadInputTokens)
	require.Equal(t, 5, result.Usage.CacheCreationInputTokens)

	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 1)
	require.JSONEq(t, `{"type":"response.completed","response":{"id":"resp_cache_write","usage":{"input_tokens":7,"output_tokens":3,"input_tokens_details":{"cached_tokens":2,"cache_write_tokens":5}}}}`, string(clientWrites[0].payload))
}
```

In `backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go`, extend `TestParseUsageAndEnrichCoverage` with:

```go
	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"input_tokens_details":{"cached_tokens":1,"cache_write_tokens":4}}}}`), "response.completed", nil)
	require.Equal(t, 5, state.usage.InputTokens)
	require.Equal(t, 3, state.usage.OutputTokens)
	require.Equal(t, 2, state.usage.CacheReadInputTokens)
	require.Equal(t, 4, state.usage.CacheCreationInputTokens)
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
cd backend && go test -tags unit ./internal/service ./internal/service/openai_ws_v2 -run 'TestExtractOpenAIUsageFromJSONBytes_CacheWriteFields|TestParseSSEUsageBytes_CacheWriteFields|TestPassthroughRelay_UsageIncludesCacheWriteTokens' -count=1
```

Expected: FAIL because cache write fields are ignored.

- [ ] **Step 4: Extend API compatibility usage structs**

In `backend/internal/pkg/apicompat/types.go`, add fields:

```go
type ResponsesUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	TotalTokens              int `json:"total_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheWriteInputTokens    int `json:"cache_write_input_tokens,omitempty"`
	CacheCreationTokens      int `json:"cache_creation_tokens,omitempty"`
	CacheWriteTokens         int `json:"cache_write_tokens,omitempty"`
	InputTokensDetails       *ResponsesInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails      *ResponsesOutputTokensDetails `json:"output_tokens_details,omitempty"`
}

type ResponsesInputTokensDetails struct {
	CachedTokens        int `json:"cached_tokens,omitempty"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheWriteTokens    int `json:"cache_write_tokens,omitempty"`
}

type ChatUsage struct {
	PromptTokens             int               `json:"prompt_tokens"`
	CompletionTokens         int               `json:"completion_tokens"`
	TotalTokens              int               `json:"total_tokens"`
	CacheCreationInputTokens int               `json:"cache_creation_input_tokens,omitempty"`
	CacheWriteInputTokens    int               `json:"cache_write_input_tokens,omitempty"`
	PromptTokensDetails      *ChatTokenDetails `json:"prompt_tokens_details,omitempty"`
}

type ChatTokenDetails struct {
	CachedTokens        int `json:"cached_tokens,omitempty"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheWriteTokens    int `json:"cache_write_tokens,omitempty"`
}
```

Keep existing field order readable and preserve JSON tags.

- [ ] **Step 5: Add centralized usage extraction helper**

In `backend/internal/service/openai_gateway_service.go`, replace direct `gjson.GetManyBytes` usage extraction with helper functions:

```go
func clampOpenAIUsageToken(v int64) int {
	if v < 0 {
		return 0
	}
	return int(v)
}

func firstOpenAIUsageInt(root gjson.Result, paths ...string) int {
	for _, path := range paths {
		value := root.Get(path)
		if value.Exists() && value.Type == gjson.Number {
			return clampOpenAIUsageToken(value.Int())
		}
	}
	return 0
}

func extractOpenAIUsageFromGJSON(root gjson.Result, usagePath string) (OpenAIUsage, bool) {
	if !root.Exists() {
		return OpenAIUsage{}, false
	}
	prefix := strings.Trim(usagePath, ".")
	path := func(p string) string {
		if prefix == "" {
			return p
		}
		return prefix + "." + p
	}
	if prefix != "" && !root.Get(prefix).Exists() {
		return OpenAIUsage{}, false
	}
	return OpenAIUsage{
		InputTokens: firstOpenAIUsageInt(root, path("input_tokens"), path("prompt_tokens")),
		OutputTokens: firstOpenAIUsageInt(root, path("output_tokens"), path("completion_tokens")),
		CacheCreationInputTokens: firstOpenAIUsageInt(root,
			path("input_tokens_details.cache_write_tokens"),
			path("input_tokens_details.cache_creation_tokens"),
			path("prompt_tokens_details.cache_write_tokens"),
			path("prompt_tokens_details.cache_creation_tokens"),
			path("cache_write_input_tokens"),
			path("cache_creation_input_tokens"),
			path("cache_write_tokens"),
			path("cache_creation_tokens"),
		),
		CacheReadInputTokens: firstOpenAIUsageInt(root,
			path("input_tokens_details.cached_tokens"),
			path("prompt_tokens_details.cached_tokens"),
			path("cache_read_input_tokens"),
		),
		ImageOutputTokens: firstOpenAIUsageInt(root, path("output_tokens_details.image_tokens")),
	}, true
}
```

Update `extractOpenAIUsageFromJSONBytes`:

```go
func extractOpenAIUsageFromJSONBytes(body []byte) (OpenAIUsage, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return OpenAIUsage{}, false
	}
	return extractOpenAIUsageFromGJSON(gjson.ParseBytes(body), "usage")
}
```

Update `parseSSEUsageBytes` to call:

```go
	if parsed, ok := extractOpenAIUsageFromGJSON(gjson.ParseBytes(data), "response.usage"); ok {
		*usage = parsed
	}
```

- [ ] **Step 6: Use helper across HTTP, Messages, and WS paths**

Replace manual usage extraction in:

```text
backend/internal/service/openai_gateway_messages.go
backend/internal/service/openai_ws_forwarder.go
```

For typed `apicompat.ResponsesUsage`, add a local converter:

```go
func openAIUsageFromResponsesUsage(u *apicompat.ResponsesUsage) OpenAIUsage {
	if u == nil {
		return OpenAIUsage{}
	}
	usage := OpenAIUsage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens}
	usage.CacheCreationInputTokens = max(
		clampOpenAIUsageToken(int64(u.CacheWriteInputTokens)),
		clampOpenAIUsageToken(int64(u.CacheCreationInputTokens)),
		clampOpenAIUsageToken(int64(u.CacheWriteTokens)),
		clampOpenAIUsageToken(int64(u.CacheCreationTokens)),
	)
	if u.InputTokensDetails != nil {
		if u.InputTokensDetails.CachedTokens > 0 {
			usage.CacheReadInputTokens = u.InputTokensDetails.CachedTokens
		}
		if u.InputTokensDetails.CacheWriteTokens > 0 {
			usage.CacheCreationInputTokens = u.InputTokensDetails.CacheWriteTokens
		} else if u.InputTokensDetails.CacheCreationTokens > 0 {
			usage.CacheCreationInputTokens = u.InputTokensDetails.CacheCreationTokens
		}
	}
	return usage
}
```

Use `openAIUsageFromResponsesUsage(event.Response.Usage)` in `openai_gateway_messages.go`.

- [ ] **Step 7: Update WS v2 relay parser**

In `backend/internal/service/openai_ws_v2/passthrough_relay.go`, extend `Usage` parsing:

```go
cacheWriteResult := firstExistingGJSON(
	gjson.GetBytes(message, "response.usage.input_tokens_details.cache_write_tokens"),
	gjson.GetBytes(message, "response.usage.input_tokens_details.cache_creation_tokens"),
	gjson.GetBytes(message, "response.usage.cache_write_input_tokens"),
	gjson.GetBytes(message, "response.usage.cache_creation_input_tokens"),
	gjson.GetBytes(message, "response.usage.cache_write_tokens"),
	gjson.GetBytes(message, "response.usage.cache_creation_tokens"),
)
cacheWriteTokens, cacheWriteOK := parseUsageIntField(cacheWriteResult, false)
```

Add `CacheCreationInputTokens: cacheWriteTokens` to `parsedUsage`, and accumulate it into `state.usage.CacheCreationInputTokens`.

If the file lacks `firstExistingGJSON`, add:

```go
func firstExistingGJSON(values ...gjson.Result) gjson.Result {
	for _, value := range values {
		if value.Exists() {
			return value
		}
	}
	return gjson.Result{}
}
```

- [ ] **Step 8: Run tests and commit**

Run:

```bash
cd backend && go test -tags unit ./internal/pkg/apicompat ./internal/service ./internal/service/openai_ws_v2 -run 'TestExtractOpenAIUsageFromJSONBytes_CacheWriteFields|TestParseSSEUsageBytes_CacheWriteFields|TestPassthroughRelay_UsageIncludesCacheWriteTokens|Usage' -count=1
```

Expected: PASS for targeted usage tests.

Commit:

```bash
git add backend/internal/pkg/apicompat/types.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_messages.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_ws_v2/passthrough_relay.go backend/internal/service/openai_gateway_service_hotpath_test.go backend/internal/service/openai_ws_v2/passthrough_relay_test.go backend/internal/service/openai_ws_v2/passthrough_relay_internal_test.go
git commit -m "feat: parse gpt-5.6 cache write usage"
```

---

### Task 5: GPT-5.6 Reasoning Effort Max And Candidate Models

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Modify: `backend/internal/service/openai_compat_model.go`
- Test: `backend/internal/service/openai_gateway_service_hotpath_test.go`
- Test: `backend/internal/service/openai_oauth_passthrough_test.go`
- Test: `backend/internal/service/openai_compat_model_test.go`

**Interfaces:**
- Produces: `func normalizeOpenAIReasoningEffortForModel(raw string, modelCandidates ...string) string`
- Produces: `func deriveOpenAIReasoningEffortFromModelCandidates(modelCandidates ...string) string`
- Changes: `extractOpenAIReasoningEffortFromBody(body []byte, requestedModels ...string) *string`
- Changes: `extractOpenAIReasoningEffort(reqBody map[string]any, requestedModels ...string) *string`

- [ ] **Step 1: Write failing reasoning effort tests**

Add to `backend/internal/service/openai_gateway_service_hotpath_test.go`:

```go
func TestExtractOpenAIReasoningEffortFromBody_GPT56Max(t *testing.T) {
	got := extractOpenAIReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"max"}}`), "gpt-5.6")
	require.NotNil(t, got)
	require.Equal(t, "max", *got)
}

func TestExtractOpenAIReasoningEffortFromBody_MaxRejectedForNonGPT56(t *testing.T) {
	require.Nil(t, extractOpenAIReasoningEffortFromBody([]byte(`{"reasoning":{"effort":"max"}}`), "gpt-5.4"))
}

func TestExtractOpenAIReasoningEffortFromBody_UsesModelCandidates(t *testing.T) {
	got := extractOpenAIReasoningEffortFromBody([]byte(`{"input":"hi"}`), "gpt-5.6-sol", "gpt-5.6-max")
	require.NotNil(t, got)
	require.Equal(t, "max", *got)
}
```

Add to `backend/internal/service/openai_compat_model_test.go`:

```go
func TestSplitOpenAICompatReasoningModel_GPT56Max(t *testing.T) {
	normalized, effort, ok := splitOpenAICompatReasoningModel("gpt-5.6-max")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", normalized)
	require.Equal(t, "max", effort)
}
```

- [ ] **Step 2: Write failing compact downgrade test**

Add to `backend/internal/service/openai_oauth_passthrough_test.go` using the existing passthrough upstream harness:

```go
func TestOpenAIOAuthCompactDowngradesGPT56MaxReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	c.Request.Header.Set("Content-Type", "application/json")

	originalBody := []byte(`{"model":"gpt-5.6","reasoning":{"effort":"max"},"input":[{"type":"text","text":"compact me"}]}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-gpt56-compact"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_56","usage":{"input_tokens":11,"output_tokens":22}}`)),
	}
	upstream := &httpUpstreamRecorder{resp: resp}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:             123,
		Name:           "acc",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		Concurrency:    1,
		Credentials:    map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"},
		Extra:          map[string]any{},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.Forward(context.Background(), c, account, originalBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestExtractOpenAIReasoningEffortFromBody_GPT56Max|TestExtractOpenAIReasoningEffortFromBody_MaxRejected|TestExtractOpenAIReasoningEffortFromBody_UsesModelCandidates|TestSplitOpenAICompatReasoningModel_GPT56Max|TestOpenAIOAuthCompactDowngradesGPT56MaxReasoningEffort' -count=1
```

Expected: FAIL because `max` is normalized away.

- [ ] **Step 4: Add model-aware reasoning normalization**

In `backend/internal/service/openai_gateway_service.go`, add:

```go
func normalizeOpenAIReasoningEffortForModel(raw string, modelCandidates ...string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	value = strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)

	switch value {
	case "none", "minimal":
		return ""
	case "low", "medium", "high":
		return value
	case "xhigh", "extrahigh":
		return "xhigh"
	case "max":
		for _, model := range modelCandidates {
			if isGPT56KnownModel(model) {
				return "max"
			}
		}
		return ""
	default:
		return ""
	}
}
```

Keep `normalizeOpenAIReasoningEffort(raw string)` as a wrapper:

```go
func normalizeOpenAIReasoningEffort(raw string) string {
	return normalizeOpenAIReasoningEffortForModel(raw)
}
```

- [ ] **Step 5: Change extraction to accept model candidates**

Change `extractOpenAIReasoningEffortFromBody` and `extractOpenAIReasoningEffort` signatures to variadic model candidates. Explicit body values call `normalizeOpenAIReasoningEffortForModel(reasoningEffort, requestedModels...)`; missing body values call:

```go
func deriveOpenAIReasoningEffortFromModelCandidates(modelCandidates ...string) string {
	for _, model := range modelCandidates {
		if value := deriveOpenAIReasoningEffortFromModel(model); value != "" {
			return normalizeOpenAIReasoningEffortForModel(value, modelCandidates...)
		}
	}
	return ""
}
```

Update call sites:

```go
extractOpenAIReasoningEffortFromBody(body, reqModel, originalModel)
extractOpenAIReasoningEffort(reqBody, upstreamModel, billingModel, originalModel)
```

In `openai_ws_forwarder.go`, pass `upstreamModel`, `billingModel` when both are available; otherwise pass `originalModel` plus the current normalized model variable in scope.

In `Forward`, compute request metadata before mutating compact payload:

```go
	requestReasoningEffort := extractOpenAIReasoningEffort(reqBody, upstreamModel, billingModel, originalModel)
```

Return `requestReasoningEffort` in `OpenAIForwardResult.ReasoningEffort` instead of re-reading the mutated `reqBody` after the upstream response.

- [ ] **Step 6: Downgrade compact GPT-5.6 max only on outbound payload**

In `applyCodexOAuthTransform(reqBody, isCodexCLI, isCompact)`, after `normalizedModel` is known and before return:

```go
	if isCompact && isGPT56KnownModel(normalizedModel) {
		if reasoning, ok := reqBody["reasoning"].(map[string]any); ok {
			if effort, ok := reasoning["effort"].(string); ok &&
				normalizeOpenAIReasoningEffortForModel(effort, normalizedModel) == "max" {
				reasoning["effort"] = "xhigh"
				result.Modified = true
			}
		}
	}
```

This changes the upstream body only. The result metadata still comes from extraction before outbound downgrade and remains `max`.

- [ ] **Step 7: Update OpenAI compat model splitting**

In `backend/internal/service/openai_compat_model.go`, allow `max` only for GPT-5.6:

```go
	case "max":
		if isGPT56KnownModel(modelID) {
			reasoningEffort = "max"
		} else {
			return trimmed, "", false
		}
```

Keep `openAIReasoningEffortToClaudeOutputEffort("max")` returning `"max"`:

```go
	case "xhigh", "max":
		return "max"
```

- [ ] **Step 8: Run tests and commit**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestExtractOpenAIReasoningEffortFromBody|TestSplitOpenAICompatReasoningModel|TestOpenAIOAuthCompactDowngradesGPT56MaxReasoningEffort' -count=1
```

Expected: PASS.

Commit:

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_compat_model.go backend/internal/service/openai_gateway_service_hotpath_test.go backend/internal/service/openai_oauth_passthrough_test.go backend/internal/service/openai_compat_model_test.go
git commit -m "feat: support gpt-5.6 max reasoning effort"
```

---

### Task 6: Codex Client Version Defaults

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/account_usage_service.go`
- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/service/settings_view.go`
- Test: `backend/internal/service/openai_oauth_passthrough_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`
- Test: `backend/internal/service/account_usage_service_test.go`

**Interfaces:**
- Produces: `codexCLIUserAgentDefault = "codex_cli_rs/0.144.1"`
- Produces: `codexCLIVersionDefault = "0.144.1"`
- Produces: `openAICodexProbeVersion = "0.144.1"`
- Preserves: `resolveCodexCLIUserAgent()` and `resolveCodexCLIVersion()` return DB cache values when non-empty.

- [ ] **Step 1: Write failing default version tests**

Add or update tests:

```go
func TestResolveCodexDefaultsUseUpstreamVersion(t *testing.T) {
	codexCLICfgCache.Store((*cachedCodexCLIConfig)(nil))
	require.Equal(t, "codex_cli_rs/0.144.1", resolveCodexCLIUserAgent())
	require.Equal(t, "0.144.1", resolveCodexCLIVersion())
	require.Equal(t, "0.144.1", openAICodexProbeVersion)
}

func TestResolveCodexDefaultsPreserveConfiguredValues(t *testing.T) {
	codexCLICfgCache.Store(&cachedCodexCLIConfig{userAgent: "custom/9.9.9", version: "9.9.9"})
	t.Cleanup(func() { codexCLICfgCache.Store((*cachedCodexCLIConfig)(nil)) })

	require.Equal(t, "custom/9.9.9", resolveCodexCLIUserAgent())
	require.Equal(t, "9.9.9", resolveCodexCLIVersion())
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestResolveCodexDefaultsUseUpstreamVersion|TestResolveCodexDefaultsPreserveConfiguredValues|TestOpenAIOAuth' -count=1
```

Expected: FAIL where tests still expect `1.0.0` or `0.104.0`.

- [ ] **Step 3: Update constants and comments**

In `backend/internal/service/openai_gateway_service.go`:

```go
	codexCLIUserAgentDefault = "codex_cli_rs/0.144.1"
	codexCLIVersionDefault = "0.144.1"
```

In `backend/internal/service/account_usage_service.go`:

```go
	openAICodexProbeVersion = "0.144.1"
```

Update comments in `domain_constants.go`, `settings_view.go`, and adjacent comments to reference `0.144.1`.

- [ ] **Step 4: Update existing assertions**

Replace hard-coded expected `codex_cli_rs/1.0.0` in `backend/internal/service/openai_oauth_passthrough_test.go` with `codexCLIUserAgentDefault`. Keep assertions against `codexCLIVersionDefault` where already used.

- [ ] **Step 5: Run tests and commit**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestResolveCodexDefaults|TestOpenAIOAuth|TestBuildOpenAIUpstreamRequest|TestAccountUsage' -count=1
```

Expected: PASS for version-related tests.

Commit:

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/account_usage_service.go backend/internal/service/domain_constants.go backend/internal/service/settings_view.go backend/internal/service/openai_oauth_passthrough_test.go backend/internal/service/openai_gateway_service_test.go backend/internal/service/account_usage_service_test.go
git commit -m "fix: bump codex client defaults"
```

---

### Task 7: Frontend GPT-5.6 Whitelist And OpenCode Config

**Files:**
- Modify: `frontend/src/composables/useModelWhitelist.ts`
- Modify: `frontend/src/composables/__tests__/useModelWhitelist.spec.ts`
- Modify: `frontend/src/components/keys/UseKeyModal.vue`
- Modify: `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`

**Interfaces:**
- Produces: OpenAI whitelist contains `gpt-5.6`, `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`.
- Produces: OpenAI preset mapping contains `{ label: 'GPT-5.6', from: 'gpt-5.6', to: 'gpt-5.6' }`.
- Produces: OpenCode config contains bare `gpt-5.6` model with `max` variant.

- [ ] **Step 1: Write failing whitelist tests**

Update `frontend/src/composables/__tests__/useModelWhitelist.spec.ts`:

```ts
it('includes bare GPT-5.6 and variants', () => {
  const models = getModelsByPlatform('openai')

  expect(models).toContain('gpt-5.6')
  expect(models).toContain('gpt-5.6-sol')
  expect(models).toContain('gpt-5.6-terra')
  expect(models).toContain('gpt-5.6-luna')
})

it('whitelist keeps GPT-5.6 exact mappings', () => {
  const mapping = buildModelMappingObject('whitelist', ['gpt-5.6', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna'], [])

  expect(mapping).toEqual({
    'gpt-5.6': 'gpt-5.6',
    'gpt-5.6-sol': 'gpt-5.6-sol',
    'gpt-5.6-terra': 'gpt-5.6-terra',
    'gpt-5.6-luna': 'gpt-5.6-luna'
  })
})
```

- [ ] **Step 2: Write failing UseKeyModal tests**

Update `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` OpenCode test:

```ts
expect(codeBlock.text()).toContain('"gpt-5.6"')
expect(codeBlock.text()).toContain('"name": "GPT-5.6 (Sol)"')
expect(codeBlock.text()).toContain('"max": {}')
```

Add a narrower count assertion if the rendered JSON exposes all GPT-5.6 entries:

```ts
expect(codeBlock.text()).toContain('"gpt-5.6-sol"')
expect(codeBlock.text()).toContain('"gpt-5.6-terra"')
expect(codeBlock.text()).toContain('"gpt-5.6-luna"')
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
pnpm --dir frontend test -- useModelWhitelist UseKeyModal
```

Expected: FAIL because bare GPT-5.6 and `max` variant are missing.

- [ ] **Step 4: Update whitelist and presets**

In `frontend/src/composables/useModelWhitelist.ts`, change the GPT-5.6 list to:

```ts
  // GPT-5.6 系列
  'gpt-5.6', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna',
```

Add preset before Sol:

```ts
  { label: 'GPT-5.6', from: 'gpt-5.6', to: 'gpt-5.6', color: 'bg-orange-100 text-orange-700 hover:bg-orange-200 dark:bg-orange-900/30 dark:text-orange-400' },
```

- [ ] **Step 5: Update OpenCode config models**

In `frontend/src/components/keys/UseKeyModal.vue`, add this model before `gpt-5.6-sol`:

```ts
    'gpt-5.6': {
      name: 'GPT-5.6 (Sol)',
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
        xhigh: {},
        max: {}
      }
    },
```

Add `max: {}` to variants for `gpt-5.6-sol`, `gpt-5.6-terra`, and `gpt-5.6-luna`:

```ts
      variants: {
        low: {},
        medium: {},
        high: {},
        xhigh: {},
        max: {}
      }
```

- [ ] **Step 6: Run tests and commit**

Run:

```bash
pnpm --dir frontend test -- useModelWhitelist UseKeyModal
```

Expected: PASS.

Commit:

```bash
git add frontend/src/composables/useModelWhitelist.ts frontend/src/composables/__tests__/useModelWhitelist.spec.ts frontend/src/components/keys/UseKeyModal.vue frontend/src/components/keys/__tests__/UseKeyModal.spec.ts
git commit -m "feat: expose gpt-5.6 frontend options"
```

---

### Task 8: Targeted Full Verification And Cleanup

**Files:**
- Verify: all files changed by Tasks 1-7
- Optional docs update: `docs/superpowers/specs/2026-07-10-gpt56-upstream-followups-design.md` only if implementation discovers a spec mismatch

**Interfaces:**
- Consumes: all task outputs.
- Produces: final verified branch with no debug leftovers and focused commits.

- [ ] **Step 1: Run backend targeted packages**

Run:

```bash
cd backend && go test -tags unit ./internal/pkg/openai ./internal/pkg/apicompat ./internal/service ./internal/service/openai_ws_v2
```

Expected: PASS.

- [ ] **Step 2: Run frontend targeted tests**

Run:

```bash
pnpm --dir frontend test -- useModelWhitelist UseKeyModal
```

Expected: PASS.

- [ ] **Step 3: Scan for forbidden placeholders and debug leftovers**

Run:

```bash
pattern='TB''D|TO''DO|PLACE''HOLDER|FIX''ME|待''定|未''定|类''似|fmt\\.Println|console\\.log'
rg -n "$pattern" backend/internal backend/resources frontend/src docs/superpowers/plans/2026-07-10-gpt56-upstream-followups.md
```

Expected: no matches introduced by this work. Existing unrelated matches may remain untouched; inspect `git diff` before deciding.

- [ ] **Step 4: Inspect diff for scope control**

Run:

```bash
git diff --stat HEAD~7..HEAD
git diff HEAD~7..HEAD -- backend/internal/service/openai_gpt56_alias.go backend/internal/service/pricing_service.go backend/internal/service/billing_service.go backend/internal/service/openai_gateway_service.go frontend/src/composables/useModelWhitelist.ts frontend/src/components/keys/UseKeyModal.vue
```

Expected: diff only touches GPT-5.6 alias, pricing, usage, reasoning, Codex version, and frontend exposure.

- [ ] **Step 5: Verify unknown GPT-5.6 safety manually through tests**

Run:

```bash
cd backend && go test -tags unit ./internal/service -run 'TestNormalizeCodexModel_GPT56Aliases|TestPricingService_GPT56UnknownDoesNotFallback' -count=1
```

Expected: PASS, with `gpt-5.6-foo` not mapping to Sol/Terra/Luna or GPT-5.4.

- [ ] **Step 6: Final cleanup commit if needed**

If Step 3 or Step 4 required small cleanup, commit only those cleanup changes:

```bash
git add <changed-files>
git commit -m "test: verify gpt-5.6 upstream followups"
```

If no cleanup changes were needed, do not create an empty commit.

---

## Self-Review Checklist

- Spec coverage: Tasks 1-7 map to alias/default model, official pricing, billing/cache write, usage parsing, reasoning `max`, Codex version, and frontend exposure.
- Unknown GPT-5.6 safety: Task 1 and Task 2 include explicit `gpt-5.6-foo` tests.
- Explicit cache write preservation: Task 3 marks channel flat and interval overrides as explicit, including zero.
- Compact behavior: Task 5 downgrades only OAuth compact outbound payload and preserves result metadata as `max`.
- Default config safety: Task 6 keeps DB cached Codex UA/version values ahead of new defaults.
- Verification: Task 8 runs backend and frontend targeted suites plus placeholder/debug scans.
