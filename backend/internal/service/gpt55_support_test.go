//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGPT55Support_BillingFallbackMatchesGPT54(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	pricing, err := svc.GetModelPricing("gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, pricing)
	require.InDelta(t, 2.5e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 15e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 0.25e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
}

func TestGPT55Support_LongContextBillingUsesGPT54Multipliers(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{
		InputTokens:  300000,
		OutputTokens: 4000,
	}

	cost, err := svc.CalculateCost("gpt-5.5", tokens, 1.0)
	require.NoError(t, err)

	expectedInput := float64(tokens.InputTokens) * 2.5e-6 * 2.0
	expectedOutput := float64(tokens.OutputTokens) * 15e-6 * 1.5
	require.InDelta(t, expectedInput, cost.InputCost, 1e-10)
	require.InDelta(t, expectedOutput, cost.OutputCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedInput+expectedOutput, cost.ActualCost, 1e-10)
}

func TestGPT55Support_NormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.5":      "gpt-5.5",
		"gpt-5.5-high": "gpt-5.5",
		"gpt 5.5":      "gpt-5.5",
	}

	for input, expected := range cases {
		require.Equal(t, expected, normalizeCodexModel(input))
	}
}

func TestGPT55Support_CompatPromptCacheKeyAutoInjection(t *testing.T) {
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5"))
}

func TestGPT55Support_PricingServiceStaticFallback(t *testing.T) {
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

func TestGPT56Support_BillingFallbackUsesOfficialPrices(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)

	pricing, err := svc.GetModelPricing("gpt-5.6-sol")
	require.NoError(t, err)
	require.NotNil(t, pricing)
	require.InDelta(t, 5e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 30e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 6.25e-6, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 12.5e-6, pricing.CacheCreationPricePerTokenPriority, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
}

func TestGPT56Support_PricingServiceStaticFallback(t *testing.T) {
	svc := &PricingService{
		pricingData: map[string]*LiteLLMModelPricing{
			"gpt-5.1-codex": {InputCostPerToken: 1.25e-6},
		},
	}

	got := svc.GetModelPricing("gpt-5.6-sol")
	require.NotNil(t, got)
	require.InDelta(t, 5e-6, got.InputCostPerToken, 1e-12)
	require.InDelta(t, 30e-6, got.OutputCostPerToken, 1e-12)
	require.InDelta(t, 6.25e-6, got.CacheCreationInputTokenCost, 1e-12)
	require.Equal(t, 272000, got.LongContextInputTokenThreshold)
}
