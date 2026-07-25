//go:build unit

package service

import (
	"context"
	"sort"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const embeddingEligibilityTestGroupID int64 = 90701

func newEmbeddingEligibilityTestService(t *testing.T, accounts []Account, channel Channel, fallback map[string]*ModelPricing) *OpenAIGatewayService {
	t.Helper()
	channel.Status = StatusActive
	channel.GroupIDs = []int64{embeddingEligibilityTestGroupID}

	channelService := NewChannelService(&mockChannelRepository{
		listAllFn: func(context.Context) ([]Channel, error) {
			return []Channel{channel}, nil
		},
		getGroupPlatformsFn: func(context.Context, []int64) (map[int64]string, error) {
			return map[int64]string{embeddingEligibilityTestGroupID: PlatformEmbedding}, nil
		},
	}, nil)

	return &OpenAIGatewayService{
		accountRepo:    stubOpenAIAccountRepo{accounts: accounts},
		cfg:            &config.Config{},
		channelService: channelService,
		resolver: NewModelPricingResolver(channelService, &BillingService{
			fallbackPrices: fallback,
		}),
	}
}

func embeddingEligibilityAccount(id int64, modelMapping map[string]any) Account {
	return Account{
		ID:          id,
		Platform:    PlatformEmbedding,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"base_url":      "https://embedding.example.test",
			"api_key":       "upstream-key",
			"model_mapping": modelMapping,
		},
	}
}

func TestEmbeddingEligibilitySharesPublicModelAcrossMappedAccounts(t *testing.T) {
	channelPrice := 2e-6
	svc := newEmbeddingEligibilityTestService(t,
		[]Account{
			embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream-a"}),
			embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream-b"}),
		},
		Channel{
			BillingModelSource: BillingModelSourceChannelMapped,
			ModelMapping: map[string]map[string]string{
				PlatformEmbedding: {"embed-public": "embed-billing"},
			},
			ModelPricing: []ChannelModelPricing{{
				Platform:   PlatformEmbedding,
				Models:     []string{"embed-billing"},
				InputPrice: &channelPrice,
			}},
		},
		nil,
	)

	groupID := embeddingEligibilityTestGroupID
	models, err := svc.ListAvailableEmbeddingModels(context.Background(), &groupID)
	require.NoError(t, err)
	require.Equal(t, []string{"embed-public"}, models)

	candidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "embed-public")
	require.NoError(t, err)
	require.Len(t, candidates, 2)

	upstreamModels := []string{candidates[0].UpstreamModel, candidates[1].UpstreamModel}
	sort.Strings(upstreamModels)
	require.Equal(t, []string{"upstream-a", "upstream-b"}, upstreamModels)
	for _, candidate := range candidates {
		require.Equal(t, "embed-public", candidate.PublicModel)
		require.Equal(t, "embed-billing", candidate.ChannelMapping.MappedModel)
		require.Equal(t, "embed-billing", candidate.BillingModel)
		require.NotNil(t, candidate.Pricing)
		pricing, ok := candidate.PricingForPromptTokens(16)
		require.True(t, ok)
		require.InDelta(t, channelPrice, pricing.InputPricePerToken, 1e-12)
	}
}

func TestEmbeddingEligibilityRejectsMissingOrNonPositiveInputPricing(t *testing.T) {
	groupID := embeddingEligibilityTestGroupID
	basePrice := 3e-6
	zero := 0.0
	upperBound := 100

	tests := []struct {
		name     string
		channel  Channel
		fallback map[string]*ModelPricing
	}{
		{
			name:    "missing pricing",
			channel: Channel{},
		},
		{
			name: "explicit channel zero overrides fallback",
			channel: Channel{ModelPricing: []ChannelModelPricing{{
				Platform:   PlatformEmbedding,
				Models:     []string{"embed-public"},
				InputPrice: &zero,
			}}},
			fallback: map[string]*ModelPricing{"embed-public": {InputPricePerToken: basePrice}},
		},
		{
			name: "non token billing mode",
			channel: Channel{ModelPricing: []ChannelModelPricing{{
				Platform:        PlatformEmbedding,
				Models:          []string{"embed-public"},
				BillingMode:     BillingModePerRequest,
				PerRequestPrice: &basePrice,
			}}},
		},
		{
			name: "interval hole without positive base",
			channel: Channel{ModelPricing: []ChannelModelPricing{{
				Platform: PlatformEmbedding,
				Models:   []string{"embed-public"},
				Intervals: []PricingInterval{
					{MinTokens: 0, MaxTokens: &upperBound, InputPrice: &basePrice},
					{MinTokens: 200, MaxTokens: nil, InputPrice: &basePrice},
				},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newEmbeddingEligibilityTestService(t,
				[]Account{embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream"})},
				tt.channel,
				tt.fallback,
			)

			models, err := svc.ListAvailableEmbeddingModels(context.Background(), &groupID)
			require.NoError(t, err)
			require.Empty(t, models)

			candidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "embed-public")
			require.NoError(t, err)
			require.Empty(t, candidates)
		})
	}
}

func TestEmbeddingEligibilityUsesFrozenPricingSnapshotForPromptTokens(t *testing.T) {
	firstPrice := 2e-6
	secondPrice := 4e-6
	upperBound := 200
	svc := newEmbeddingEligibilityTestService(t,
		[]Account{embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream"})},
		Channel{ModelPricing: []ChannelModelPricing{{
			Platform: PlatformEmbedding,
			Models:   []string{"embed-public"},
			Intervals: []PricingInterval{
				{MinTokens: 0, MaxTokens: &upperBound, InputPrice: &firstPrice},
				{MinTokens: upperBound, MaxTokens: nil, InputPrice: &secondPrice},
			},
		}}},
		nil,
	)

	groupID := embeddingEligibilityTestGroupID
	candidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "embed-public")
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	firstInterval, ok := candidates[0].PricingForPromptTokens(200)
	require.True(t, ok)
	require.InDelta(t, 2e-6, firstInterval.InputPricePerToken, 1e-12)
	secondInterval, ok := candidates[0].PricingForPromptTokens(201)
	require.True(t, ok)
	require.InDelta(t, 4e-6, secondInterval.InputPricePerToken, 1e-12)

	// 模拟渠道价格在上游返回 usage 之前发生变更。已决策的候选必须继续使用请求级快照。
	firstPrice = 0
	frozen, ok := candidates[0].PricingForPromptTokens(200)
	require.True(t, ok)
	require.InDelta(t, 2e-6, frozen.InputPricePerToken, 1e-12)

	freshCandidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "embed-public")
	require.NoError(t, err)
	require.Empty(t, freshCandidates, "新的请求应重新评估渠道显式零价")
}

func TestEmbeddingEligibilityRequiresStrictEmbeddingCredentialAndMapping(t *testing.T) {
	groupID := embeddingEligibilityTestGroupID
	price := 3e-6
	invalidType := embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream"})
	invalidType.Type = AccountTypeOAuth
	missingURL := embeddingEligibilityAccount(2, map[string]any{"embed-public": "upstream"})
	missingURL.Credentials["base_url"] = ""
	blankTarget := embeddingEligibilityAccount(3, map[string]any{"embed-public": "  "})
	notSchedulable := embeddingEligibilityAccount(4, map[string]any{"embed-public": "upstream"})
	notSchedulable.Schedulable = false

	svc := newEmbeddingEligibilityTestService(t,
		[]Account{invalidType, missingURL, blankTarget, notSchedulable},
		Channel{ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformEmbedding,
			Models:     []string{"embed-public"},
			InputPrice: &price,
		}}},
		nil,
	)

	models, err := svc.ListAvailableEmbeddingModels(context.Background(), &groupID)
	require.NoError(t, err)
	require.Empty(t, models)

	candidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "embed-public")
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestEmbeddingEligibilityAcceptsLegacyWhitelistAsIdentityMapping(t *testing.T) {
	groupID := embeddingEligibilityTestGroupID
	price := 1e-6
	account := embeddingEligibilityAccount(1, nil)
	delete(account.Credentials, "model_mapping")
	account.Credentials["model_whitelist"] = []any{"legacy-embed"}

	svc := newEmbeddingEligibilityTestService(t,
		[]Account{account},
		Channel{ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformEmbedding,
			Models:     []string{"legacy-embed"},
			InputPrice: &price,
		}}},
		nil,
	)

	models, err := svc.ListAvailableEmbeddingModels(context.Background(), &groupID)
	require.NoError(t, err)
	require.Equal(t, []string{"legacy-embed"}, models)

	candidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "legacy-embed")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "legacy-embed", candidates[0].UpstreamModel)
}

func TestEmbeddingEligibilityHonorsChannelPricingRestriction(t *testing.T) {
	groupID := embeddingEligibilityTestGroupID
	basePrice := 3e-6
	svc := newEmbeddingEligibilityTestService(t,
		[]Account{embeddingEligibilityAccount(1, map[string]any{"embed-public": "upstream"})},
		Channel{
			RestrictModels: true,
			ModelPricing: []ChannelModelPricing{{
				Platform:   PlatformEmbedding,
				Models:     []string{"another-model"},
				InputPrice: &basePrice,
			}},
		},
		map[string]*ModelPricing{"embed-public": {InputPricePerToken: basePrice}},
	)

	models, err := svc.ListAvailableEmbeddingModels(context.Background(), &groupID)
	require.NoError(t, err)
	require.Empty(t, models)

	candidates, err := svc.ResolveEmbeddingModelEligibility(context.Background(), &groupID, "embed-public")
	require.NoError(t, err)
	require.Empty(t, candidates)
}
