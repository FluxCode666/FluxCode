package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func embeddingBillingFixture(price float64, promptTokens int) (*EmbeddingBillingInput, *Group) {
	groupID := int64(21)
	group := &Group{ID: groupID, Platform: PlatformEmbedding, RateMultiplier: 2}
	apiKey := &APIKey{
		ID:      7,
		UserID:  9,
		GroupID: &groupID,
		Group:   group,
		Quota:   10,
	}
	user := &User{ID: 9, Balance: 20}
	account := Account{ID: 11, Type: AccountTypeAPIKey, Platform: PlatformEmbedding}
	return &EmbeddingBillingInput{
		Result: &EmbeddingForwardResult{
			PromptTokens: promptTokens,
			Duration:     125 * time.Millisecond,
			Eligibility: EmbeddingModelEligibility{
				Account:       account,
				PublicModel:   "embed-public",
				BillingModel:  "embed-billing",
				UpstreamModel: "embed-upstream",
				Pricing: &ResolvedPricing{
					Mode:        BillingModeToken,
					BasePricing: &ModelPricing{InputPricePerToken: price},
				},
			},
		},
		APIKey:             apiKey,
		User:               user,
		RequestPayloadHash: "payload-hash",
	}, group
}

func newEmbeddingBillingService(repo UsageBillingRepository) *OpenAIGatewayService {
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	return &OpenAIGatewayService{
		cfg:              cfg,
		usageBillingRepo: repo,
	}
}

func TestOpenAIGatewayServiceBillEmbeddingBuildsAtomicType4BalanceCommand(t *testing.T) {
	t.Parallel()

	repo := &openAIRecordUsageBillingRepoStub{}
	quota := &openAIRecordUsageAPIKeyQuotaStub{}
	input, _ := embeddingBillingFixture(1e-6, 100)
	input.APIKeyService = quota
	svc := newEmbeddingBillingService(repo)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "embedding-request")

	require.NoError(t, svc.BillEmbedding(ctx, input))
	require.Equal(t, 1, repo.calls)
	require.NotNil(t, repo.lastCmd)
	require.InDelta(t, 0.0002, repo.lastCmd.BalanceCost, 1e-12)
	require.InDelta(t, 0.0002, repo.lastCmd.APIKeyQuotaCost, 1e-12)
	require.NotNil(t, repo.lastCmd.UsageLog)
	require.Equal(t, RequestTypeEmbedding, repo.lastCmd.UsageLog.RequestType)
	require.Equal(t, "embed-public", repo.lastCmd.UsageLog.Model)
	require.Equal(t, 100, repo.lastCmd.UsageLog.InputTokens)
	require.InDelta(t, 0.0001, repo.lastCmd.UsageLog.TotalCost, 1e-12)
	require.InDelta(t, 0.0002, repo.lastCmd.UsageLog.ActualCost, 1e-12)
	require.False(t, repo.lastCmd.UsageLog.Stream)
	require.False(t, repo.lastCmd.UsageLog.OpenAIWSMode)
}

func TestOpenAIGatewayServiceBillEmbeddingBuildsSubscriptionAndRateLimitEffects(t *testing.T) {
	t.Parallel()

	repo := &openAIRecordUsageBillingRepoStub{}
	quota := &openAIRecordUsageAPIKeyQuotaStub{}
	input, group := embeddingBillingFixture(1e-6, 100)
	group.SubscriptionType = SubscriptionTypeSubscription
	input.Subscription = &UserSubscription{ID: 31}
	input.APIKey.RateLimit5h = 5
	input.APIKeyService = quota
	input.Result.Eligibility.Account.Extra = map[string]any{"quota_limit": 10.0}
	svc := newEmbeddingBillingService(repo)

	require.NoError(t, svc.BillEmbedding(context.Background(), input))
	require.NotNil(t, repo.lastCmd)
	require.NotNil(t, repo.lastCmd.SubscriptionID)
	require.Equal(t, int64(31), *repo.lastCmd.SubscriptionID)
	require.InDelta(t, 0.0001, repo.lastCmd.SubscriptionCost, 1e-12)
	require.Zero(t, repo.lastCmd.BalanceCost)
	require.InDelta(t, 0.0002, repo.lastCmd.APIKeyQuotaCost, 1e-12)
	require.InDelta(t, 0.0002, repo.lastCmd.APIKeyRateLimitCost, 1e-12)
	require.InDelta(t, 0.0001, repo.lastCmd.AccountQuotaCost, 1e-12)
	require.NotNil(t, repo.lastCmd.UsageLog)
	require.NotNil(t, repo.lastCmd.UsageLog.SubscriptionID)
	require.Equal(t, int64(31), *repo.lastCmd.UsageLog.SubscriptionID)
	require.Equal(t, BillingTypeSubscription, repo.lastCmd.UsageLog.BillingType)
}

func TestOpenAIGatewayServiceBillEmbeddingUsesFrozenIntervalPrice(t *testing.T) {
	t.Parallel()

	repo := &openAIRecordUsageBillingRepoStub{}
	input, _ := embeddingBillingFixture(1e-6, 150)
	upper := 100
	firstPrice := 2e-6
	secondPrice := 4e-6
	input.Result.Eligibility.Pricing.Intervals = []PricingInterval{
		{MinTokens: 0, MaxTokens: &upper, InputPrice: &firstPrice},
		{MinTokens: upper, MaxTokens: nil, InputPrice: &secondPrice},
	}
	svc := newEmbeddingBillingService(repo)

	require.NoError(t, svc.BillEmbedding(context.Background(), input))
	require.InDelta(t, 0.0006, repo.lastCmd.UsageLog.TotalCost, 1e-12)
	require.InDelta(t, 0.0012, repo.lastCmd.BalanceCost, 1e-12)
}

func TestOpenAIGatewayServiceBillEmbeddingRejectsInvalidOrZeroPrice(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		price float64
	}{
		{name: "zero", price: 0},
		{name: "negative", price: -1e-6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIRecordUsageBillingRepoStub{}
			input, _ := embeddingBillingFixture(tc.price, 10)
			svc := newEmbeddingBillingService(repo)

			err := svc.BillEmbedding(context.Background(), input)
			require.ErrorIs(t, err, ErrEmbeddingPricingInvalid)
			require.Zero(t, repo.calls)
		})
	}
}

func TestOpenAIGatewayServiceBillEmbeddingPropagatesRepositoryError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("commit failed")
	repo := &openAIRecordUsageBillingRepoStub{err: wantErr}
	input, _ := embeddingBillingFixture(1e-6, 10)
	svc := newEmbeddingBillingService(repo)

	err := svc.BillEmbedding(context.Background(), input)
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, 1, repo.calls)
}

type embeddingIdempotentBillingRepo struct {
	mu      sync.Mutex
	seen    map[string]string
	results []bool
}

func (r *embeddingIdempotentBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = make(map[string]string)
	}
	key := cmd.RequestID
	if fingerprint, exists := r.seen[key]; exists {
		if fingerprint != cmd.RequestFingerprint {
			return nil, ErrUsageBillingRequestConflict
		}
		r.results = append(r.results, false)
		return &UsageBillingApplyResult{Applied: false}, nil
	}
	r.seen[key] = cmd.RequestFingerprint
	r.results = append(r.results, true)
	return &UsageBillingApplyResult{Applied: true}, nil
}

func TestOpenAIGatewayServiceBillEmbeddingSameRequestIDIsIdempotent(t *testing.T) {
	t.Parallel()

	repo := &embeddingIdempotentBillingRepo{}
	input, _ := embeddingBillingFixture(1e-6, 10)
	svc := newEmbeddingBillingService(repo)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "same-request")

	require.NoError(t, svc.BillEmbedding(ctx, input))
	require.NoError(t, svc.BillEmbedding(ctx, input))
	require.Equal(t, []bool{true, false}, repo.results)
}
