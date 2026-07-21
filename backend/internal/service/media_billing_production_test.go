package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type productionMediaBillingLedgerStub struct {
	prechargeCommand MediaBillingPrechargeCommand
	settleCommand    MediaBillingSettlementCommand
	prechargeResult  MediaBillingLedgerResult
	settleResult     MediaBillingLedgerResult
	prechargeErr     error
	settleErr        error
	prechargeCalls   int
	settleCalls      int
}

func (s *productionMediaBillingLedgerStub) Precharge(_ context.Context, cmd MediaBillingPrechargeCommand) (MediaBillingLedgerResult, error) {
	s.prechargeCalls++
	s.prechargeCommand = cmd
	return s.prechargeResult, s.prechargeErr
}

func (s *productionMediaBillingLedgerStub) Settle(_ context.Context, cmd MediaBillingSettlementCommand) (MediaBillingLedgerResult, error) {
	s.settleCalls++
	s.settleCommand = cmd
	return s.settleResult, s.settleErr
}

type productionMediaBillingCacheStub struct {
	balanceInvalidations      []int64
	subscriptionInvalidations [][2]int64
	rateLimitInvalidations    []int64
	balanceErr                error
	subscriptionErr           error
	rateLimitErr              error
}

func (*productionMediaBillingCacheStub) GetUserBalance(context.Context, int64) (float64, error) {
	return 0, nil
}

func (*productionMediaBillingCacheStub) SetUserBalance(context.Context, int64, float64) error {
	return nil
}

func (*productionMediaBillingCacheStub) DeductUserBalance(context.Context, int64, float64) error {
	return nil
}

func (s *productionMediaBillingCacheStub) InvalidateUserBalance(_ context.Context, userID int64) error {
	s.balanceInvalidations = append(s.balanceInvalidations, userID)
	return s.balanceErr
}

func (*productionMediaBillingCacheStub) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return nil, nil
}

func (*productionMediaBillingCacheStub) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}

func (*productionMediaBillingCacheStub) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	return nil
}

func (s *productionMediaBillingCacheStub) InvalidateSubscriptionCache(_ context.Context, userID, groupID int64) error {
	s.subscriptionInvalidations = append(s.subscriptionInvalidations, [2]int64{userID, groupID})
	return s.subscriptionErr
}

func (*productionMediaBillingCacheStub) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, nil
}

func (*productionMediaBillingCacheStub) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}

func (*productionMediaBillingCacheStub) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func (s *productionMediaBillingCacheStub) InvalidateAPIKeyRateLimit(_ context.Context, keyID int64) error {
	s.rateLimitInvalidations = append(s.rateLimitInvalidations, keyID)
	return s.rateLimitErr
}

type productionMediaBillingAuthStub struct {
	users []int64
}

func (*productionMediaBillingAuthStub) InvalidateAuthCacheByKey(context.Context, string) {}

func (s *productionMediaBillingAuthStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.users = append(s.users, userID)
}

func (*productionMediaBillingAuthStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

type productionMediaBillingOutboxStub struct {
	events []SchedulerOutboxEvent
	err    error
}

func (s *productionMediaBillingOutboxStub) Publish(
	_ context.Context,
	eventType string,
	accountID *int64,
	groupID *int64,
	payload map[string]any,
) error {
	s.events = append(s.events, SchedulerOutboxEvent{
		EventType: eventType, AccountID: accountID, GroupID: groupID, Payload: payload,
	})
	return s.err
}

func (*productionMediaBillingOutboxStub) Read(context.Context, int64, time.Duration) ([]SchedulerOutboxEvent, error) {
	return nil, nil
}

func (*productionMediaBillingOutboxStub) Ack(context.Context, ...string) error { return nil }

func (*productionMediaBillingOutboxStub) Pending(context.Context) (int64, error) { return 0, nil }

func TestProductionMediaBillingPrechargeUsesPersistedSnapshotAndRefreshesCaches(t *testing.T) {
	task, snapshot := productionMediaBillingTestTask(t)
	ledger := &productionMediaBillingLedgerStub{prechargeResult: MediaBillingLedgerResult{
		Applied: true, PrechargedAmount: snapshot.EstimatedAmount,
		Allocation: MediaBillingAllocation{
			FundingSource: MediaBillingFundingBalance, APIKeyRateLimitEnabled: true,
		},
		APIKeyStatusChanged: true,
	}}
	cache := &productionMediaBillingCacheStub{}
	auth := &productionMediaBillingAuthStub{}
	outbox := &productionMediaBillingOutboxStub{}
	billing := NewProductionMediaBilling(ledger, cache, auth, outbox)

	result, err := billing.Precharge(context.Background(), task, snapshot)
	require.NoError(t, err)
	require.InDelta(t, snapshot.EstimatedAmount, result.PrechargedAmount, 1e-12)
	require.Equal(t, 1, ledger.prechargeCalls)
	require.Equal(t, task.PublicID+":precharge", ledger.prechargeCommand.IdempotencyKey)
	require.Equal(t, task.RequestFingerprint, ledger.prechargeCommand.RequestFingerprint)
	require.Equal(t, task.ID, ledger.prechargeCommand.TaskID)
	require.Equal(t, task.CreatedAt, ledger.prechargeCommand.BilledAt)
	require.Equal(t, []int64{task.UserID}, cache.balanceInvalidations)
	require.Equal(t, []int64{task.APIKeyID}, cache.rateLimitInvalidations)
	require.Equal(t, []int64{task.UserID}, auth.users)
	require.Empty(t, outbox.events)
}

func TestProductionMediaBillingSuccessUsesActualUsageAndPublishesAccountChange(t *testing.T) {
	task, _ := productionMediaBillingTestTask(t)
	task.PrechargedAmount = 0.1
	ledger := &productionMediaBillingLedgerStub{settleResult: MediaBillingLedgerResult{
		Applied: true, PrechargedAmount: 0.1, FinalAmount: 0.15, AdditionalChargedAmount: 0.05,
		Allocation:          MediaBillingAllocation{FundingSource: MediaBillingFundingBalance, AccountQuota: 0.3},
		AccountQuotaChanged: true,
	}}
	cache := &productionMediaBillingCacheStub{}
	auth := &productionMediaBillingAuthStub{}
	outbox := &productionMediaBillingOutboxStub{}
	billing := NewProductionMediaBilling(ledger, cache, auth, outbox)

	result, err := billing.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 3})
	require.NoError(t, err)
	require.InDelta(t, 0.15, result.FinalAmount, 1e-12)
	require.InDelta(t, 0.05, result.AdditionalChargedAmount, 1e-12)
	require.Equal(t, MediaBillingOperationSuccess, ledger.settleCommand.Operation)
	require.Equal(t, task.PublicID+":success", ledger.settleCommand.IdempotencyKey)
	require.InDelta(t, 0.15, ledger.settleCommand.FinalAmount, 1e-12)
	require.InDelta(t, 0.3, ledger.settleCommand.AccountBaseAmount, 1e-12)
	require.Equal(t, task.AccountID, ledger.settleCommand.AccountID)
	require.Equal(t, []int64{task.UserID}, cache.balanceInvalidations)
	require.Len(t, outbox.events, 1)
	require.Equal(t, SchedulerOutboxEventAccountChanged, outbox.events[0].EventType)
	require.Equal(t, task.AccountID, outbox.events[0].AccountID)
}

func TestProductionMediaBillingFailureAppliesPenaltyRatio(t *testing.T) {
	task, _ := productionMediaBillingTestTask(t)
	task.PrechargedAmount = 3
	ledger := &productionMediaBillingLedgerStub{settleResult: MediaBillingLedgerResult{
		Applied: true, PrechargedAmount: 3, FinalAmount: 2.4, RefundedAmount: 0.6,
		Allocation: MediaBillingAllocation{FundingSource: MediaBillingFundingSubscription},
	}}
	cache := &productionMediaBillingCacheStub{}
	billing := NewProductionMediaBilling(
		ledger,
		cache,
		&productionMediaBillingAuthStub{},
		&productionMediaBillingOutboxStub{},
	)

	result, err := billing.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSyncTimeout, RefundRatio: 0.2, PenaltyRatio: 0.8,
	})
	require.NoError(t, err)
	require.InDelta(t, 2.4, ledger.settleCommand.FinalAmount, 1e-12)
	require.Zero(t, ledger.settleCommand.AccountBaseAmount)
	require.Equal(t, MediaBillingOperationFailure, ledger.settleCommand.Operation)
	require.InDelta(t, 2.4, result.FinalAmount, 1e-12)
	require.InDelta(t, 0.6, result.RefundedAmount, 1e-12)
	require.Equal(t, [][2]int64{{task.UserID, task.GroupID}}, cache.subscriptionInvalidations)
}

func TestProductionMediaBillingPostCommitFailuresRemainRetryable(t *testing.T) {
	t.Run("precharge marks the committed result unknown", func(t *testing.T) {
		task, snapshot := productionMediaBillingTestTask(t)
		ledger := &productionMediaBillingLedgerStub{prechargeResult: MediaBillingLedgerResult{
			Applied: true, PrechargedAmount: snapshot.EstimatedAmount,
			Allocation: MediaBillingAllocation{FundingSource: MediaBillingFundingBalance},
		}}
		cache := &productionMediaBillingCacheStub{balanceErr: errors.New("redis unavailable")}
		billing := NewProductionMediaBilling(
			ledger,
			cache,
			&productionMediaBillingAuthStub{},
			&productionMediaBillingOutboxStub{},
		)

		result, err := billing.Precharge(context.Background(), task, snapshot)
		require.ErrorIs(t, err, ErrMediaPrechargeResultUnknown)
		require.InDelta(t, snapshot.EstimatedAmount, result.PrechargedAmount, 1e-12)
	})

	t.Run("settlement returns an error after a durable account charge", func(t *testing.T) {
		task, _ := productionMediaBillingTestTask(t)
		task.PrechargedAmount = 0.1
		ledger := &productionMediaBillingLedgerStub{settleResult: MediaBillingLedgerResult{
			Applied: true, PrechargedAmount: 0.1, FinalAmount: 0.1,
			Allocation:          MediaBillingAllocation{FundingSource: MediaBillingFundingBalance, AccountQuota: 0.2},
			AccountQuotaChanged: true,
		}}
		outbox := &productionMediaBillingOutboxStub{err: errors.New("stream unavailable")}
		billing := NewProductionMediaBilling(
			ledger,
			&productionMediaBillingCacheStub{},
			&productionMediaBillingAuthStub{},
			outbox,
		)

		result, err := billing.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 2})
		require.ErrorContains(t, err, "publish media account quota change")
		require.InDelta(t, 0.1, result.FinalAmount, 1e-12)
	})
}

func TestProductionMediaBillingReplayRepairsAuthAndSchedulerViews(t *testing.T) {
	task, _ := productionMediaBillingTestTask(t)
	task.PrechargedAmount = 0.1
	ledger := &productionMediaBillingLedgerStub{settleResult: MediaBillingLedgerResult{
		Applied: false, PrechargedAmount: 0.1, FinalAmount: 0.1,
		Allocation: MediaBillingAllocation{
			FundingSource: MediaBillingFundingBalance, AccountQuota: 0.2,
		},
	}}
	auth := &productionMediaBillingAuthStub{}
	outbox := &productionMediaBillingOutboxStub{}
	billing := NewProductionMediaBilling(ledger, &productionMediaBillingCacheStub{}, auth, outbox)

	_, err := billing.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 2})
	require.NoError(t, err)
	require.Equal(t, []int64{task.UserID}, auth.users)
	require.Len(t, outbox.events, 1)
}

func TestProductionMediaBillingRejectsSnapshotSubstitution(t *testing.T) {
	task, snapshot := productionMediaBillingTestTask(t)
	ledger := &productionMediaBillingLedgerStub{}
	billing := NewProductionMediaBilling(
		ledger,
		&productionMediaBillingCacheStub{},
		&productionMediaBillingAuthStub{},
		&productionMediaBillingOutboxStub{},
	)
	snapshot.EstimatedAmount += 1

	_, err := billing.Precharge(context.Background(), task, snapshot)
	require.ErrorIs(t, err, ErrInvalidMediaBillingSnapshot)
	require.Zero(t, ledger.prechargeCalls)
}

func productionMediaBillingTestTask(t *testing.T) (*MediaTask, MediaBillingSnapshot) {
	t.Helper()
	defaultPrice := 0.1
	frozen := MediaPricingSnapshotV1{
		Version: MediaPricingSnapshotVersion, Source: MediaPricingSourceChannel,
		Currency: MediaPricingCurrencyUSD, UserID: 11, APIKeyID: 22, GroupID: 33,
		ChannelID: 44, PricingID: 55, Platform: PlatformMedia, Vendor: "openai",
		RequestedModel: "image-alias", CanonicalModel: "gpt-image-1",
		MediaType: MediaTypeImage, Operation: MediaOperationTextToImage,
		BillingUnit: MediaBillingUnitImage, BillingMode: BillingModeImage,
		DefaultUnitPrice: &defaultPrice, EstimatedQuantity: 2, EstimatedUnitPrice: defaultPrice,
		BaseEstimatedAmount: 0.2, GroupDefaultMultiplier: 0.5,
		EffectiveMultiplier: 0.5,
		Candidates: []MediaPricingCandidateContext{{
			AccountID: 77, Provider: "openai", UpstreamModel: "gpt-image-1",
		}},
	}
	pricingJSON, err := json.Marshal(frozen)
	require.NoError(t, err)
	snapshot := MediaBillingSnapshot{
		RequestedModel: "image-alias", CandidateModels: []string{"gpt-image-1"},
		EstimatedAmount: 0.1, GroupMultiplier: 0.5, PricingSnapshot: pricingJSON,
	}
	billingJSON, err := json.Marshal(snapshot)
	require.NoError(t, err)
	accountID := int64(77)
	createdAt := time.Date(2026, time.July, 21, 8, 30, 0, 0, time.UTC)
	return &MediaTask{
		ID: 1, PublicID: "media_task_1", UserID: 11, APIKeyID: 22, GroupID: 33,
		AccountID: &accountID, RequestedModel: "image-alias",
		RequestFingerprint: "production-media-billing-fingerprint",
		BillingSnapshot:    billingJSON, CreatedAt: createdAt,
	}, snapshot
}
