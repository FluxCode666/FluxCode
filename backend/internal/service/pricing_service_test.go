package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDefaultPricingGPT56TierMetadataMatchesUpstream(t *testing.T) {
	type tierMetadata struct {
		Input                       float64 `json:"input_cost_per_token"`
		InputBatch                  float64 `json:"input_cost_per_token_batches"`
		InputFlex                   float64 `json:"input_cost_per_token_flex"`
		InputPriority               float64 `json:"input_cost_per_token_priority"`
		Output                      float64 `json:"output_cost_per_token"`
		OutputBatch                 float64 `json:"output_cost_per_token_batches"`
		OutputFlex                  float64 `json:"output_cost_per_token_flex"`
		OutputPriority              float64 `json:"output_cost_per_token_priority"`
		CacheWrite                  float64 `json:"cache_creation_input_token_cost"`
		CacheWriteBatch             float64 `json:"cache_creation_input_token_cost_batches"`
		CacheWriteFlex              float64 `json:"cache_creation_input_token_cost_flex"`
		CacheWritePriority          float64 `json:"cache_creation_input_token_cost_priority"`
		CacheRead                   float64 `json:"cache_read_input_token_cost"`
		CacheReadFlex               float64 `json:"cache_read_input_token_cost_flex"`
		CacheReadPriority           float64 `json:"cache_read_input_token_cost_priority"`
		LongContextThreshold        int     `json:"long_context_input_token_threshold"`
		LongContextInputMultiplier  float64 `json:"long_context_input_cost_multiplier"`
		LongContextOutputMultiplier float64 `json:"long_context_output_cost_multiplier"`
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	var pricing map[string]tierMetadata
	require.NoError(t, json.Unmarshal(data, &pricing))
	var rawPricing map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &rawPricing))

	tests := []struct {
		model                                                                string
		input, output, cacheWrite, cacheRead                                 float64
		inputBatch, outputBatch, cacheWriteBatch                             float64
		inputFlex, outputFlex, cacheWriteFlex, cacheReadFlex                 float64
		inputPriority, outputPriority, cacheWritePriority, cacheReadPriority float64
	}{
		{
			model: "gpt-5.6-sol",
			input: 5e-6, output: 30e-6, cacheWrite: 6.25e-6, cacheRead: 0.5e-6,
			inputBatch: 2.5e-6, outputBatch: 15e-6, cacheWriteBatch: 3.125e-6,
			inputFlex: 2.5e-6, outputFlex: 15e-6, cacheWriteFlex: 3.125e-6, cacheReadFlex: 0.25e-6,
			inputPriority: 10e-6, outputPriority: 60e-6, cacheWritePriority: 12.5e-6, cacheReadPriority: 1e-6,
		},
		{
			model: "gpt-5.6-terra",
			input: 2.5e-6, output: 15e-6, cacheWrite: 3.125e-6, cacheRead: 0.25e-6,
			inputBatch: 1.25e-6, outputBatch: 7.5e-6, cacheWriteBatch: 1.5625e-6,
			inputFlex: 1.25e-6, outputFlex: 7.5e-6, cacheWriteFlex: 1.5625e-6, cacheReadFlex: 0.125e-6,
			inputPriority: 5e-6, outputPriority: 30e-6, cacheWritePriority: 6.25e-6, cacheReadPriority: 0.5e-6,
		},
		{
			model: "gpt-5.6-luna",
			input: 1e-6, output: 6e-6, cacheWrite: 1.25e-6, cacheRead: 0.1e-6,
			inputBatch: 0.5e-6, outputBatch: 3e-6, cacheWriteBatch: 0.625e-6,
			inputFlex: 0.5e-6, outputFlex: 3e-6, cacheWriteFlex: 0.625e-6, cacheReadFlex: 0.05e-6,
			inputPriority: 2e-6, outputPriority: 12e-6, cacheWritePriority: 2.5e-6, cacheReadPriority: 0.2e-6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := pricing[tt.model]
			require.True(t, ok, "missing pricing object %s", tt.model)
			raw, ok := rawPricing[tt.model]
			require.True(t, ok, "missing raw pricing object %s", tt.model)

			require.InDelta(t, tt.input, got.Input, 1e-12)
			require.InDelta(t, tt.output, got.Output, 1e-12)
			require.InDelta(t, tt.cacheWrite, got.CacheWrite, 1e-12)
			require.InDelta(t, tt.cacheRead, got.CacheRead, 1e-12)
			require.InDelta(t, tt.inputBatch, got.InputBatch, 1e-12)
			require.InDelta(t, tt.outputBatch, got.OutputBatch, 1e-12)
			require.InDelta(t, tt.cacheWriteBatch, got.CacheWriteBatch, 1e-12)
			require.InDelta(t, tt.inputFlex, got.InputFlex, 1e-12)
			require.InDelta(t, tt.outputFlex, got.OutputFlex, 1e-12)
			require.InDelta(t, tt.cacheWriteFlex, got.CacheWriteFlex, 1e-12)
			require.InDelta(t, tt.cacheReadFlex, got.CacheReadFlex, 1e-12)
			require.InDelta(t, tt.inputPriority, got.InputPriority, 1e-12)
			require.InDelta(t, tt.outputPriority, got.OutputPriority, 1e-12)
			require.InDelta(t, tt.cacheWritePriority, got.CacheWritePriority, 1e-12)
			require.InDelta(t, tt.cacheReadPriority, got.CacheReadPriority, 1e-12)

			require.Equal(t, 272000, got.LongContextThreshold)
			require.InDelta(t, 2.0, got.LongContextInputMultiplier, 1e-12)
			require.InDelta(t, 1.5, got.LongContextOutputMultiplier, 1e-12)
			require.NotContains(t, raw, "input_cost_per_token_above_272k_tokens")
			require.NotContains(t, raw, "output_cost_per_token_above_272k_tokens")
			require.NotContains(t, raw, "cache_read_input_token_cost_above_272k_tokens")
		})
	}
}

func TestParsePricingData_ParsesPriorityAndServiceTierFields(t *testing.T) {
	svc := &PricingService{}
	body := []byte(`{
		"gpt-5.4": {
			"input_cost_per_token": 0.0000025,
			"input_cost_per_token_priority": 0.000005,
			"output_cost_per_token": 0.000015,
			"output_cost_per_token_priority": 0.00003,
			"cache_creation_input_token_cost": 0.0000025,
			"cache_read_input_token_cost": 0.00000025,
			"cache_read_input_token_cost_priority": 0.0000005,
			"supports_service_tier": true,
			"supports_prompt_caching": true,
			"litellm_provider": "openai",
			"mode": "chat"
		}
	}`)

	data, err := svc.parsePricingData(body)
	require.NoError(t, err)
	pricing := data["gpt-5.4"]
	require.NotNil(t, pricing)
	require.InDelta(t, 5e-6, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 3e-5, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 5e-7, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.True(t, pricing.SupportsServiceTier)
}

func TestBillingService_GPT56CacheWritePricingUsesOfficialMultiplier(t *testing.T) {
	tests := []struct {
		model             string
		input             float64
		inputPriority     float64
		output            float64
		outputPriority    float64
		cacheRead         float64
		cacheReadPriority float64
	}{
		{model: "gpt-5.6-sol", input: 5e-6, inputPriority: 10e-6, output: 30e-6, outputPriority: 60e-6, cacheRead: 0.5e-6, cacheReadPriority: 1e-6},
		{model: "gpt-5.6-terra", input: 2.5e-6, inputPriority: 5e-6, output: 15e-6, outputPriority: 30e-6, cacheRead: 0.25e-6, cacheReadPriority: 0.5e-6},
		{model: "gpt-5.6-luna", input: 1e-6, inputPriority: 2e-6, output: 6e-6, outputPriority: 12e-6, cacheRead: 0.1e-6, cacheReadPriority: 0.2e-6},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricingSvc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
				tt.model: {
					InputCostPerToken:               tt.input,
					InputCostPerTokenPriority:       tt.inputPriority,
					OutputCostPerToken:              tt.output,
					OutputCostPerTokenPriority:      tt.outputPriority,
					CacheReadInputTokenCost:         tt.cacheRead,
					CacheReadInputTokenCostPriority: tt.cacheReadPriority,
				},
			}}
			svc := NewBillingService(&config.Config{}, pricingSvc)

			pricing, err := svc.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.InDelta(t, tt.input*1.25, pricing.CacheCreationPricePerToken, 1e-12)
			require.InDelta(t, tt.inputPriority*1.25, pricing.CacheCreationPricePerTokenPriority, 1e-12)
			require.Equal(t, 272000, pricing.LongContextInputThreshold)
			require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
			require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)

			tokens := UsageTokens{InputTokens: 700, OutputTokens: 50, CacheCreationTokens: 200, CacheReadTokens: 100}
			standard, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "")
			require.NoError(t, err)
			require.InDelta(t, 200*tt.input*1.25, standard.CacheCreationCost, 1e-12)

			priority, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "priority")
			require.NoError(t, err)
			require.InDelta(t, 200*tt.inputPriority*1.25, priority.CacheCreationCost, 1e-12)

			flex, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "flex")
			require.NoError(t, err)
			require.InDelta(t, 200*tt.input*1.25*0.5, flex.CacheCreationCost, 1e-12)
		})
	}
}

func TestBillingService_GPT56UsesLongContextPricingAcrossModelsAndTiers(t *testing.T) {
	models := []struct {
		name               string
		input, cached      float64
		cacheWrite, output float64
	}{
		{name: "gpt-5.6-sol", input: 5e-6, cached: 0.5e-6, cacheWrite: 6.25e-6, output: 30e-6},
		{name: "gpt-5.6-terra", input: 2.5e-6, cached: 0.25e-6, cacheWrite: 3.125e-6, output: 15e-6},
		{name: "gpt-5.6-luna", input: 1e-6, cached: 0.1e-6, cacheWrite: 1.25e-6, output: 6e-6},
	}
	tiers := []struct {
		name       string
		priceScale float64
	}{
		{name: "standard", priceScale: 1},
		{name: "priority", priceScale: 2},
		{name: "flex", priceScale: 0.5},
	}
	tokens := UsageTokens{
		InputTokens:         100000,
		CacheCreationTokens: 100000,
		CacheReadTokens:     73000,
		OutputTokens:        10,
	}

	for _, model := range models {
		for _, tier := range tiers {
			t.Run(model.name+"/"+tier.name, func(t *testing.T) {
				svc := NewBillingService(&config.Config{}, nil)
				serviceTier := ""
				if tier.name != "standard" {
					serviceTier = tier.name
				}
				cost, err := svc.CalculateCostWithServiceTier(model.name, tokens, 1, serviceTier)
				require.NoError(t, err)
				require.InDelta(t, float64(tokens.InputTokens)*model.input*tier.priceScale*2, cost.InputCost, 1e-12)
				require.InDelta(t, float64(tokens.CacheCreationTokens)*model.cacheWrite*tier.priceScale*2, cost.CacheCreationCost, 1e-12)
				require.InDelta(t, float64(tokens.CacheReadTokens)*model.cached*tier.priceScale*2, cost.CacheReadCost, 1e-12)
				require.InDelta(t, float64(tokens.OutputTokens)*model.output*tier.priceScale*1.5, cost.OutputCost, 1e-12)
			})
		}
	}
}

func TestGetModelPricing_Gpt53CodexSparkUsesGpt51CodexPricing(t *testing.T) {
	sparkPricing := &LiteLLMModelPricing{InputCostPerToken: 1}
	gpt53Pricing := &LiteLLMModelPricing{InputCostPerToken: 9}

	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": sparkPricing,
			"gpt-5.3":       gpt53Pricing,
		},
	}

	got := svc.GetModelPricing("gpt-5.3-codex-spark")
	require.Same(t, sparkPricing, got)
}

func TestGetModelPricing_Gpt53CodexFallbackStillUsesGpt52Codex(t *testing.T) {
	gpt52CodexPricing := &LiteLLMModelPricing{InputCostPerToken: 2}

	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.2-codex": gpt52CodexPricing,
		},
	}

	got := svc.GetModelPricing("gpt-5.3-codex")
	require.Same(t, gpt52CodexPricing, got)
}

func TestGetModelPricing_OpenAIFallbackMatchedLoggedAsInfo(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	gpt52CodexPricing := &LiteLLMModelPricing{InputCostPerToken: 2}
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.2-codex": gpt52CodexPricing,
		},
	}

	got := svc.GetModelPricing("gpt-5.3-codex")
	require.Same(t, gpt52CodexPricing, got)

	require.True(t, logSink.ContainsMessageAtLevel("[Pricing] OpenAI fallback matched gpt-5.3-codex -> gpt-5.2-codex", "info"))
	require.False(t, logSink.ContainsMessageAtLevel("[Pricing] OpenAI fallback matched gpt-5.3-codex -> gpt-5.2-codex", "warn"))
}

func TestGetModelPricing_Gpt54UsesStaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": &LiteLLMModelPricing{InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.4")
	require.NotNil(t, got)
	require.InDelta(t, 2.5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 2.5e-7, got.CacheReadInputTokenCost, 1e-12)
	require.Equal(t, 272000, got.LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, got.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, got.LongContextOutputCostMultiplier, 1e-12)
}

func TestGetModelPricing_Gpt55UsesGpt54StaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.5")
	require.NotNil(t, got)
	require.InDelta(t, 2.5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.5e-5, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 2.5e-7, got.CacheReadInputTokenCost, 1e-12)
	require.Equal(t, 272000, got.LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, got.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, got.LongContextOutputCostMultiplier, 1e-12)
}

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
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.6": {InputCostPerToken: 5e-6},
	}}

	require.Nil(t, svc.GetModelPricing("gpt-5.6-foo"))
}

func TestPricingService_GPT56BareUsesDynamicSolPricing(t *testing.T) {
	svc := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-5.6-sol": {
			InputCostPerToken:               4e-6,
			OutputCostPerToken:              24e-6,
			LongContextInputTokenThreshold:  272000,
			LongContextInputCostMultiplier:  2,
			LongContextOutputCostMultiplier: 1.5,
		},
	}}

	got := svc.GetModelPricing("gpt-5.6")
	require.NotNil(t, got)
	require.InDelta(t, 4e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 24e-6, got.OutputCostPerToken, 1e-12)
	require.Equal(t, 272000, got.LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, got.LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, got.LongContextOutputCostMultiplier, 1e-12)
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
			"long_context_input_token_threshold": 272000,
			"long_context_input_cost_multiplier": 2,
			"long_context_output_cost_multiplier": 1.5,
			"supports_service_tier": true,
			"supports_prompt_caching": true
		}
	}`)

	prices, err := svc.parsePricingData(data)
	require.NoError(t, err)
	require.InDelta(t, 12.5e-6, prices["gpt-5.6-sol"].CacheCreationInputTokenCostPriority, 1e-12)
	require.Equal(t, 272000, prices["gpt-5.6-sol"].LongContextInputTokenThreshold)
	require.InDelta(t, 2.0, prices["gpt-5.6-sol"].LongContextInputCostMultiplier, 1e-12)
	require.InDelta(t, 1.5, prices["gpt-5.6-sol"].LongContextOutputCostMultiplier, 1e-12)
}

func TestGetModelPricing_Gpt54MiniUsesDedicatedStaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.4-mini")
	require.NotNil(t, got)
	require.InDelta(t, 7.5e-7, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 4.5e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 7.5e-8, got.CacheReadInputTokenCost, 1e-12)
	require.Zero(t, got.LongContextInputTokenThreshold)
}

func TestGetModelPricing_Gpt54NanoUsesDedicatedStaticFallbackWhenRemoteMissing(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.4-nano")
	require.NotNil(t, got)
	require.InDelta(t, 2e-7, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 1.25e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 2e-8, got.CacheReadInputTokenCost, 1e-12)
	require.Zero(t, got.LongContextInputTokenThreshold)
}

func TestParsePricingData_PreservesPriorityAndServiceTierFields(t *testing.T) {
	raw := map[string]any{
		"gpt-5.4": map[string]any{
			"input_cost_per_token":                 2.5e-6,
			"input_cost_per_token_priority":        5e-6,
			"output_cost_per_token":                15e-6,
			"output_cost_per_token_priority":       30e-6,
			"cache_read_input_token_cost":          0.25e-6,
			"cache_read_input_token_cost_priority": 0.5e-6,
			"supports_service_tier":                true,
			"supports_prompt_caching":              true,
			"litellm_provider":                     "openai",
			"mode":                                 "chat",
		},
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	svc := &PricingService{}
	pricingMap, err := svc.parsePricingData(body)
	require.NoError(t, err)

	pricing := pricingMap["gpt-5.4"]
	require.NotNil(t, pricing)
	require.InDelta(t, 2.5e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 5e-6, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 15e-6, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 30e-6, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.25e-6, pricing.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 0.5e-6, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.True(t, pricing.SupportsServiceTier)
}

func TestParsePricingData_PreservesServiceTierPriorityFields(t *testing.T) {
	svc := &PricingService{}
	pricingData, err := svc.parsePricingData([]byte(`{
		"gpt-5.4": {
			"input_cost_per_token": 0.0000025,
			"input_cost_per_token_priority": 0.000005,
			"output_cost_per_token": 0.000015,
			"output_cost_per_token_priority": 0.00003,
			"cache_read_input_token_cost": 0.00000025,
			"cache_read_input_token_cost_priority": 0.0000005,
			"supports_service_tier": true,
			"litellm_provider": "openai",
			"mode": "chat"
		}
	}`))
	require.NoError(t, err)

	pricing := pricingData["gpt-5.4"]
	require.NotNil(t, pricing)
	require.InDelta(t, 0.0000025, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 0.000005, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.000015, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 0.00003, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.00000025, pricing.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 0.0000005, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.True(t, pricing.SupportsServiceTier)
}
