package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProviderUsageBillingPreservesAndPricesCacheTokenClasses(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	billingRepo := &openAIRecordUsageBillingRepoStub{
		result: &UsageBillingApplyResult{Applied: false},
	}
	providerGateway := &GatewayService{
		cfg: cfg, billingService: NewBillingService(cfg, nil), usageBillingRepo: billingRepo,
	}
	capability := routeCapability(1, 101, ProtocolChatCompletions, false)
	capability.LogicalModel.Name = "claude-sonnet-4"
	capability.Capability.UpstreamModel = "claude-sonnet-4-20250514"
	candidate := NewNativeRouteCandidate(capability, ProtocolChatCompletions)

	err := providerGateway.RecordProviderUsage(context.Background(), &ProviderRecordUsageInput{
		Result: &ProviderGatewayResult{
			Candidate: candidate, UpstreamRequestID: "provider-cache-usage",
			Usage: ProviderUsage{
				InputTokens: 100, OutputTokens: 7,
				CacheCreationTokens: 20, CacheReadTokens: 10,
				CacheCreation5mTokens: 12, CacheCreation1hTokens: 8,
				Complete: true,
			},
		},
		APIKey: &APIKey{ID: 9}, User: &User{ID: 3},
	})

	require.NoError(t, err)
	require.Equal(t, 1, billingRepo.calls)
	require.NotNil(t, billingRepo.lastCmd)
	require.NotNil(t, billingRepo.lastCmd.UsageLog)
	require.Equal(t, 70, billingRepo.lastCmd.UsageLog.InputTokens)
	require.Equal(t, 20, billingRepo.lastCmd.UsageLog.CacheCreationTokens)
	require.Equal(t, 10, billingRepo.lastCmd.UsageLog.CacheReadTokens)
	require.Equal(t, 12, billingRepo.lastCmd.UsageLog.CacheCreation5mTokens)
	require.Equal(t, 8, billingRepo.lastCmd.UsageLog.CacheCreation1hTokens)
	require.Greater(t, billingRepo.lastCmd.UsageLog.CacheCreationCost, float64(0))
	require.Greater(t, billingRepo.lastCmd.UsageLog.CacheReadCost, float64(0))
}

func TestProviderUsageBillingRejectsIncompleteUsageBeforeFinalizingBilling(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	billingRepo := &openAIRecordUsageBillingRepoStub{
		result: &UsageBillingApplyResult{Applied: false},
	}
	providerGateway := &GatewayService{
		cfg: cfg, billingService: NewBillingService(cfg, nil), usageBillingRepo: billingRepo,
	}
	capability := routeCapability(1, 101, ProtocolChatCompletions, false)
	capability.LogicalModel.Name = "claude-sonnet-4"
	capability.Capability.UpstreamModel = "claude-sonnet-4-20250514"

	err := providerGateway.RecordProviderUsage(context.Background(), &ProviderRecordUsageInput{
		Result: &ProviderGatewayResult{
			Candidate: NewNativeRouteCandidate(capability, ProtocolChatCompletions),
			Usage:     ProviderUsage{},
		},
		APIKey: &APIKey{ID: 9}, User: &User{ID: 3},
	})

	require.ErrorIs(t, err, ErrProviderUsageIncomplete)
	require.Zero(t, billingRepo.calls)
}
