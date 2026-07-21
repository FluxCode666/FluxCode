//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type mediaBillingRepositoryFixture struct {
	ctx         context.Context
	repo        service.MediaBillingLedgerRepository
	user        *service.User
	group       *service.Group
	apiKey      *service.APIKey
	task        *service.MediaTask
	billedAt    time.Time
	fingerprint string
}

type mediaBillingRepositoryFixtureOptions struct {
	subscriptionType string
	userBalance      float64
	apiKeyStatus     string
	apiKeyQuota      float64
	apiKeyQuotaUsed  float64
	rateLimit5h      float64
	rateLimit1d      float64
	rateLimit7d      float64
	usage5h          float64
	usage1d          float64
	usage7d          float64
	window5hStart    *time.Time
	window1dStart    *time.Time
	window7dStart    *time.Time
}

func newMediaBillingRepositoryFixture(t *testing.T, options mediaBillingRepositoryFixtureOptions) *mediaBillingRepositoryFixture {
	t.Helper()

	ctx := context.Background()
	client := testEntClient(t)
	if options.subscriptionType == "" {
		options.subscriptionType = service.SubscriptionTypeStandard
	}
	if options.apiKeyStatus == "" {
		options.apiKeyStatus = service.StatusAPIKeyActive
	}

	unique := uuid.NewString()
	user := mustCreateUser(t, client, &service.User{
		Email:   "media-billing-" + unique + "@example.com",
		Balance: options.userBalance,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "media-billing-" + unique,
		Platform:         service.PlatformMedia,
		Status:           service.StatusActive,
		SubscriptionType: options.subscriptionType,
		RateMultiplier:   1,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:        user.ID,
		GroupID:       &group.ID,
		Key:           "sk-media-billing-" + unique,
		Name:          "media billing",
		Status:        options.apiKeyStatus,
		Quota:         options.apiKeyQuota,
		QuotaUsed:     options.apiKeyQuotaUsed,
		RateLimit5h:   options.rateLimit5h,
		RateLimit1d:   options.rateLimit1d,
		RateLimit7d:   options.rateLimit7d,
		Usage5h:       options.usage5h,
		Usage1d:       options.usage1d,
		Usage7d:       options.usage7d,
		Window5hStart: options.window5hStart,
		Window1dStart: options.window1dStart,
		Window7dStart: options.window7dStart,
	})

	publicID := "media-billing-" + uuid.NewString()
	fingerprint := uuid.NewString()
	task, err := NewMediaTaskRepository(client).Create(ctx, &service.MediaTask{
		PublicID:           publicID,
		UserID:             user.ID,
		APIKeyID:           apiKey.ID,
		GroupID:            group.ID,
		MediaType:          service.MediaTypeImage,
		Operation:          service.MediaOperationTextToImage,
		RequestedModel:     "billing-test-image",
		ClientAsync:        true,
		Status:             service.MediaTaskStatusQueued,
		Stage:              service.MediaTaskStageQueued,
		RequestSpec:        []byte(`{"image":{"prompt":"billing test","n":1}}`),
		CandidateSnapshot:  []byte(`[]`),
		RequestFingerprint: fingerprint,
		BillingStatus:      service.MediaBillingStatusPending,
	})
	require.NoError(t, err)

	return &mediaBillingRepositoryFixture{
		ctx:         ctx,
		repo:        NewMediaBillingRepository(integrationDB),
		user:        user,
		group:       group,
		apiKey:      apiKey,
		task:        task,
		billedAt:    time.Now().UTC(),
		fingerprint: fingerprint,
	}
}

func (f *mediaBillingRepositoryFixture) prechargeCommand(amount float64) service.MediaBillingPrechargeCommand {
	return service.MediaBillingPrechargeCommand{
		IdempotencyKey:     f.task.PublicID + ":precharge",
		RequestFingerprint: f.fingerprint,
		TaskID:             f.task.ID,
		TaskPublicID:       f.task.PublicID,
		UserID:             f.user.ID,
		APIKeyID:           f.apiKey.ID,
		GroupID:            f.group.ID,
		Amount:             amount,
		BilledAt:           f.billedAt,
	}
}

func (f *mediaBillingRepositoryFixture) settlementCommand(
	operation service.MediaBillingOperation,
	finalAmount float64,
	accountID *int64,
	accountBaseAmount float64,
) service.MediaBillingSettlementCommand {
	return service.MediaBillingSettlementCommand{
		IdempotencyKey:     f.task.PublicID + ":" + string(operation),
		RequestFingerprint: f.fingerprint,
		Operation:          operation,
		TaskID:             f.task.ID,
		TaskPublicID:       f.task.PublicID,
		UserID:             f.user.ID,
		APIKeyID:           f.apiKey.ID,
		GroupID:            f.group.ID,
		AccountID:          accountID,
		FinalAmount:        finalAmount,
		AccountBaseAmount:  accountBaseAmount,
		BilledAt:           f.billedAt,
	}
}

func TestMediaBillingRepositoryGiftAndOrdinaryPrechargePartialRefund(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{userBalance: 10})

	var giftID int64
	err := integrationDB.QueryRowContext(fixture.ctx, `
		INSERT INTO gift_balance_records (user_id, amount, remaining, source, note, created_at, updated_at)
		VALUES ($1, 3, 3, 'admin_grant', 'media billing test', NOW(), NOW())
		RETURNING id
	`, fixture.user.ID).Scan(&giftID)
	require.NoError(t, err)

	precharge, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(5))
	require.NoError(t, err)
	require.True(t, precharge.Applied)
	require.InDelta(t, 5, precharge.PrechargedAmount, 0.00000001)
	require.Equal(t, service.MediaBillingFundingBalance, precharge.Allocation.FundingSource)
	require.Len(t, precharge.Allocation.GiftBalances, 1)
	require.Equal(t, giftID, precharge.Allocation.GiftBalances[0].RecordID)
	require.InDelta(t, 3, precharge.Allocation.GiftBalances[0].Amount, 0.00000001)
	require.InDelta(t, 2, precharge.Allocation.OrdinaryBalance, 0.00000001)

	var giftRemaining, balance float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT remaining FROM gift_balance_records WHERE id = $1", giftID).Scan(&giftRemaining))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.InDelta(t, 0, giftRemaining, 0.00000001)
	require.InDelta(t, 8, balance, 0.00000001)
	result, err := integrationDB.ExecContext(fixture.ctx,
		"UPDATE users SET deleted_at = NOW() WHERE id = $1", fixture.user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationFailure,
		2.5,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 2.5, settlement.FinalAmount, 0.00000001)
	require.InDelta(t, 2.5, settlement.RefundedAmount, 0.00000001)

	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT remaining FROM gift_balance_records WHERE id = $1", giftID).Scan(&giftRemaining))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.InDelta(t, 1.5, giftRemaining, 0.00000001, "退款应按原赠送余额资金占比返还")
	require.InDelta(t, 9, balance, 0.00000001, "退款应按原普通余额资金占比返还")
}

func TestMediaBillingRepositorySubscriptionGrantFullRefund(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		subscriptionType: service.SubscriptionTypeSubscription,
		userBalance:      10,
	})
	client := testEntClient(t)
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{
		UserID:          fixture.user.ID,
		GroupID:         fixture.group.ID,
		DailyUsageUSD:   1.25,
		WeeklyUsageUSD:  1.25,
		MonthlyUsageUSD: 1.25,
	})
	grant, err := client.SubscriptionGrant.Create().
		SetSubscriptionID(subscription.ID).
		SetCreatedAt(fixture.billedAt.Add(-2 * time.Hour)).
		SetStartsAt(fixture.billedAt.Add(-time.Hour)).
		SetExpiresAt(fixture.billedAt.Add(time.Hour)).
		SetDailyUsageUsd(1.25).
		SetWeeklyUsageUsd(1.25).
		SetMonthlyUsageUsd(1.25).
		Save(fixture.ctx)
	require.NoError(t, err)

	precharge, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(4))
	require.NoError(t, err)
	require.True(t, precharge.Applied)
	require.Equal(t, service.MediaBillingFundingSubscription, precharge.Allocation.FundingSource)
	require.NotNil(t, precharge.Allocation.SubscriptionID)
	require.Equal(t, subscription.ID, *precharge.Allocation.SubscriptionID)
	require.Equal(t, []service.MediaBillingGrantAllocation{{GrantID: grant.ID, Amount: 4}}, precharge.Allocation.SubscriptionGrant)

	var subscriptionDaily, subscriptionWeekly, subscriptionMonthly float64
	var grantDaily, grantWeekly, grantMonthly float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&subscriptionDaily, &subscriptionWeekly, &subscriptionMonthly))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM subscription_grants WHERE id = $1
	`, grant.ID).Scan(&grantDaily, &grantWeekly, &grantMonthly))
	require.InDelta(t, 5.25, subscriptionDaily, 0.00000001)
	require.InDelta(t, 5.25, subscriptionWeekly, 0.00000001)
	require.InDelta(t, 5.25, subscriptionMonthly, 0.00000001)
	require.InDelta(t, 5.25, grantDaily, 0.00000001)
	require.InDelta(t, 5.25, grantWeekly, 0.00000001)
	require.InDelta(t, 5.25, grantMonthly, 0.00000001)

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationFailure,
		0,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 4, settlement.RefundedAmount, 0.00000001)

	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM user_subscriptions WHERE id = $1
	`, subscription.ID).Scan(&subscriptionDaily, &subscriptionWeekly, &subscriptionMonthly))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM subscription_grants WHERE id = $1
	`, grant.ID).Scan(&grantDaily, &grantWeekly, &grantMonthly))
	require.InDelta(t, 1.25, subscriptionDaily, 0.00000001)
	require.InDelta(t, 1.25, subscriptionWeekly, 0.00000001)
	require.InDelta(t, 1.25, subscriptionMonthly, 0.00000001)
	require.InDelta(t, 1.25, grantDaily, 0.00000001)
	require.InDelta(t, 1.25, grantWeekly, 0.00000001)
	require.InDelta(t, 1.25, grantMonthly, 0.00000001)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.InDelta(t, 10, balance, 0.00000001, "订阅资金退款不应改动普通余额")
}

func TestMediaBillingRepositoryRepeatedPrechargeAndSettlementAreIdempotent(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{userBalance: 10})
	prechargeCommand := fixture.prechargeCommand(3)

	firstPrecharge, err := fixture.repo.Precharge(fixture.ctx, prechargeCommand)
	require.NoError(t, err)
	require.True(t, firstPrecharge.Applied)
	secondPrecharge, err := fixture.repo.Precharge(fixture.ctx, prechargeCommand)
	require.NoError(t, err)
	require.False(t, secondPrecharge.Applied)
	require.InDelta(t, firstPrecharge.PrechargedAmount, secondPrecharge.PrechargedAmount, 0.00000001)

	settlementCommand := fixture.settlementCommand(service.MediaBillingOperationSuccess, 1.25, nil, 0)
	firstSettlement, err := fixture.repo.Settle(fixture.ctx, settlementCommand)
	require.NoError(t, err)
	require.True(t, firstSettlement.Applied)
	secondSettlement, err := fixture.repo.Settle(fixture.ctx, settlementCommand)
	require.NoError(t, err)
	require.False(t, secondSettlement.Applied)
	require.InDelta(t, firstSettlement.FinalAmount, secondSettlement.FinalAmount, 0.00000001)
	require.InDelta(t, firstSettlement.RefundedAmount, secondSettlement.RefundedAmount, 0.00000001)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.InDelta(t, 8.75, balance, 0.00000001, "重复预扣和重复结算都只能修改一次余额")

	var operationCount int
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT COUNT(*) FROM media_billing_operations WHERE task_id = $1", fixture.task.ID).Scan(&operationCount))
	require.Equal(t, 2, operationCount)
}

func TestMediaBillingRepositoryRejectsConflictingTerminalOperation(t *testing.T) {
	tests := []struct {
		name   string
		first  service.MediaBillingOperation
		second service.MediaBillingOperation
	}{
		{name: "success_then_failure", first: service.MediaBillingOperationSuccess, second: service.MediaBillingOperationFailure},
		{name: "failure_then_success", first: service.MediaBillingOperationFailure, second: service.MediaBillingOperationSuccess},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{userBalance: 10})
			_, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(4))
			require.NoError(t, err)
			_, err = fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(test.first, 2, nil, 0))
			require.NoError(t, err)

			var balanceBeforeConflict float64
			require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
				"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balanceBeforeConflict))

			_, err = fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(test.second, 1, nil, 0))
			require.ErrorIs(t, err, service.ErrMediaBillingOperationConflict)

			var balanceAfterConflict float64
			require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
				"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balanceAfterConflict))
			require.InDelta(t, balanceBeforeConflict, balanceAfterConflict, 0.00000001)

			var terminalCount int
			var persistedOperation string
			require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
				SELECT COUNT(*), MIN(operation)
				FROM media_billing_operations
				WHERE task_id = $1 AND operation IN ('success', 'failure')
			`, fixture.task.ID).Scan(&terminalCount, &persistedOperation))
			require.Equal(t, 1, terminalCount)
			require.Equal(t, string(test.first), persistedOperation)
		})
	}
}

func TestMediaBillingRepositoryRefundRestoresAPIKeyQuotaAndStatus(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		userBalance:     10,
		apiKeyQuota:     5,
		apiKeyQuotaUsed: 4,
	})

	precharge, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(2))
	require.NoError(t, err)
	require.True(t, precharge.Applied)
	require.True(t, precharge.APIKeyStatusChanged)
	require.True(t, precharge.Allocation.APIKeyQuotaEnabled)

	var quotaUsed float64
	var status string
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT quota_used, status
		FROM api_keys WHERE id = $1
	`, fixture.apiKey.ID).Scan(&quotaUsed, &status))
	require.InDelta(t, 6, quotaUsed, 0.00000001)
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, status)

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationFailure,
		0,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.True(t, settlement.APIKeyStatusChanged)

	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT quota_used, status
		FROM api_keys WHERE id = $1
	`, fixture.apiKey.ID).Scan(&quotaUsed, &status))
	require.InDelta(t, 4, quotaUsed, 0.00000001)
	require.Equal(t, service.StatusAPIKeyActive, status)
}

func TestMediaBillingRepositorySuccessfulSettlementChargesAccountQuotaOnce(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{userBalance: 20})
	account, err := testEntClient(t).Account.Create().
		SetName("media-billing-account-" + uuid.NewString()).
		SetPlatform(service.PlatformMedia).
		SetType(service.AccountTypeAPIKey).
		SetCredentials(map[string]any{}).
		SetExtra(map[string]any{
			"quota_limit": 100.0,
			"quota_used":  1.0,
		}).
		SetRateMultiplier(1.5).
		Save(fixture.ctx)
	require.NoError(t, err)

	_, err = fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(2.5))
	require.NoError(t, err)
	accountID := account.ID
	settlementCommand := fixture.settlementCommand(
		service.MediaBillingOperationSuccess,
		2.5,
		&accountID,
		4,
	)
	firstSettlement, err := fixture.repo.Settle(fixture.ctx, settlementCommand)
	require.NoError(t, err)
	require.True(t, firstSettlement.Applied)
	require.True(t, firstSettlement.AccountQuotaChanged)
	require.InDelta(t, 6, firstSettlement.Allocation.AccountQuota, 0.00000001)

	secondSettlement, err := fixture.repo.Settle(fixture.ctx, settlementCommand)
	require.NoError(t, err)
	require.False(t, secondSettlement.Applied)
	require.InDelta(t, 6, secondSettlement.Allocation.AccountQuota, 0.00000001)

	var quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT COALESCE((extra->>'quota_used')::numeric, 0)
		FROM accounts WHERE id = $1
	`, account.ID).Scan(&quotaUsed))
	require.InDelta(t, 7, quotaUsed, 0.00000001, "账号基础金额应乘账号倍率且只能结算一次")
}

func TestMediaBillingRepositoryFailureRefundSettlesAfterAPIKeySoftDelete(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		userBalance:     10,
		apiKeyQuota:     5,
		apiKeyQuotaUsed: 4,
		rateLimit5h:     100,
		usage5h:         1,
	})

	precharge, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(2))
	require.NoError(t, err)
	require.True(t, precharge.APIKeyStatusChanged)

	result, err := integrationDB.ExecContext(fixture.ctx,
		"UPDATE api_keys SET deleted_at = NOW() WHERE id = $1", fixture.apiKey.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationFailure,
		0.5,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 1.5, settlement.RefundedAmount, 0.00000001)
	require.True(t, settlement.APIKeyStatusChanged)

	var balance, quotaUsed, usage5h float64
	var status string
	var deletedAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT quota_used, usage_5h, status, deleted_at
		FROM api_keys WHERE id = $1
	`, fixture.apiKey.ID).Scan(&quotaUsed, &usage5h, &status, &deletedAt))
	require.InDelta(t, 9.5, balance, 0.00000001)
	require.InDelta(t, 4.5, quotaUsed, 0.00000001)
	require.InDelta(t, 1.5, usage5h, 0.00000001)
	require.Equal(t, service.StatusAPIKeyActive, status)
	require.False(t, deletedAt.IsZero())
}

func TestMediaBillingRepositorySuccessSettlesAfterFrozenBalanceEntitiesSoftDelete(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		userBalance:     20,
		apiKeyQuota:     100,
		apiKeyQuotaUsed: 1,
		rateLimit5h:     100,
		usage5h:         1,
	})
	account, err := testEntClient(t).Account.Create().
		SetName("media-billing-deleted-account-" + uuid.NewString()).
		SetPlatform(service.PlatformMedia).
		SetType(service.AccountTypeAPIKey).
		SetCredentials(map[string]any{}).
		SetExtra(map[string]any{
			"quota_limit": 100.0,
			"quota_used":  1.0,
		}).
		SetRateMultiplier(1.5).
		Save(fixture.ctx)
	require.NoError(t, err)

	_, err = fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(2))
	require.NoError(t, err)
	result, err := integrationDB.ExecContext(fixture.ctx,
		"UPDATE api_keys SET group_id = NULL, deleted_at = NOW() WHERE id = $1", fixture.apiKey.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))
	result, err = integrationDB.ExecContext(fixture.ctx,
		"UPDATE accounts SET deleted_at = NOW() WHERE id = $1", account.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))
	result, err = integrationDB.ExecContext(fixture.ctx,
		"UPDATE users SET deleted_at = NOW() WHERE id = $1", fixture.user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))
	result, err = integrationDB.ExecContext(fixture.ctx,
		"UPDATE groups SET deleted_at = NOW() WHERE id = $1", fixture.group.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))

	accountID := account.ID
	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationSuccess,
		3,
		&accountID,
		4,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 1, settlement.AdditionalChargedAmount, 0.00000001)
	require.True(t, settlement.AccountQuotaChanged)
	require.InDelta(t, 6, settlement.Allocation.AccountQuota, 0.00000001)

	var balance, apiKeyQuotaUsed, usage5h, accountQuotaUsed float64
	var apiKeyDeletedAt, accountDeletedAt time.Time
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT quota_used, usage_5h, deleted_at
		FROM api_keys WHERE id = $1
	`, fixture.apiKey.ID).Scan(&apiKeyQuotaUsed, &usage5h, &apiKeyDeletedAt))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx, `
		SELECT COALESCE((extra->>'quota_used')::numeric, 0), deleted_at
		FROM accounts WHERE id = $1
	`, account.ID).Scan(&accountQuotaUsed, &accountDeletedAt))
	require.InDelta(t, 17, balance, 0.00000001)
	require.InDelta(t, 4, apiKeyQuotaUsed, 0.00000001)
	require.InDelta(t, 4, usage5h, 0.00000001)
	require.InDelta(t, 7, accountQuotaUsed, 0.00000001)
	require.False(t, apiKeyDeletedAt.IsZero())
	require.False(t, accountDeletedAt.IsZero())
}

func TestMediaBillingRepositoryZeroPrechargeRetainsBalanceFundingForAdditionalCharge(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{userBalance: 10})

	precharge, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(0))
	require.NoError(t, err)
	require.True(t, precharge.Applied)
	require.Equal(t, service.MediaBillingFundingBalance, precharge.Allocation.FundingSource)

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationSuccess,
		2,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 2, settlement.AdditionalChargedAmount, 0.00000001)
	require.Equal(t, service.MediaBillingFundingBalance, settlement.Allocation.FundingSource)
	require.InDelta(t, 2, settlement.Allocation.OrdinaryBalance, 0.00000001)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.InDelta(t, 8, balance, 0.00000001)
}

func TestMediaBillingRepositoryZeroPrechargeRetainsSubscriptionFundingForAdditionalCharge(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		subscriptionType: service.SubscriptionTypeSubscription,
		userBalance:      10,
	})
	subscription := mustCreateSubscription(t, testEntClient(t), &service.UserSubscription{
		UserID:  fixture.user.ID,
		GroupID: fixture.group.ID,
	})
	historicalGrant, err := testEntClient(t).SubscriptionGrant.Create().
		SetSubscriptionID(subscription.ID).
		SetCreatedAt(fixture.billedAt.Add(-3 * time.Hour)).
		SetStartsAt(fixture.billedAt.Add(-2 * time.Hour)).
		SetExpiresAt(fixture.billedAt.Add(time.Hour)).
		Save(fixture.ctx)
	require.NoError(t, err)
	result, err := integrationDB.ExecContext(fixture.ctx,
		"UPDATE subscription_grants SET deleted_at = $1 WHERE id = $2",
		fixture.billedAt.Add(-30*time.Minute), historicalGrant.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))
	grant, err := testEntClient(t).SubscriptionGrant.Create().
		SetSubscriptionID(subscription.ID).
		SetCreatedAt(fixture.billedAt.Add(-2 * time.Hour)).
		SetStartsAt(fixture.billedAt.Add(-time.Hour)).
		SetExpiresAt(fixture.billedAt.Add(time.Hour)).
		Save(fixture.ctx)
	require.NoError(t, err)

	precharge, err := fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(0))
	require.NoError(t, err)
	require.Equal(t, service.MediaBillingFundingSubscription, precharge.Allocation.FundingSource)
	require.NotNil(t, precharge.Allocation.SubscriptionID)
	require.Equal(t, subscription.ID, *precharge.Allocation.SubscriptionID)
	require.Empty(t, precharge.Allocation.SubscriptionGrant)

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationSuccess,
		2,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 2, settlement.AdditionalChargedAmount, 0.00000001)
	require.Equal(t, service.MediaBillingFundingSubscription, settlement.Allocation.FundingSource)
	require.Equal(t, []service.MediaBillingGrantAllocation{{GrantID: grant.ID, Amount: 2}}, settlement.Allocation.SubscriptionGrant)

	var subscriptionUsage, grantUsage, historicalGrantUsage, balance float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&subscriptionUsage))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM subscription_grants WHERE id = $1", grant.ID).Scan(&grantUsage))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM subscription_grants WHERE id = $1", historicalGrant.ID).Scan(&historicalGrantUsage))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance))
	require.InDelta(t, 2, subscriptionUsage, 0.00000001)
	require.InDelta(t, 2, grantUsage, 0.00000001)
	require.Zero(t, historicalGrantUsage, "任务创建前已删除的 grant 不得被结算重新采用")
	require.InDelta(t, 10, balance, 0.00000001)
}

func TestMediaBillingRepositoryPrechargeRejectsSoftDeletedGiftOnlyUser(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{userBalance: 0})

	var giftID int64
	err := integrationDB.QueryRowContext(fixture.ctx, `
		INSERT INTO gift_balance_records (user_id, amount, remaining, source, note, created_at, updated_at)
		VALUES ($1, 3, 3, 'admin_grant', 'deleted media user test', NOW(), NOW())
		RETURNING id
	`, fixture.user.ID).Scan(&giftID)
	require.NoError(t, err)
	result, err := integrationDB.ExecContext(fixture.ctx,
		"UPDATE users SET deleted_at = NOW() WHERE id = $1", fixture.user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, mustRowsAffected(t, result))

	_, err = fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(2))
	require.ErrorIs(t, err, service.ErrUserNotFound)

	var giftRemaining float64
	var operationCount int
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT remaining FROM gift_balance_records WHERE id = $1", giftID).Scan(&giftRemaining))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT COUNT(*) FROM media_billing_operations WHERE task_id = $1", fixture.task.ID).Scan(&operationCount))
	require.InDelta(t, 3, giftRemaining, 0.00000001, "用户活跃校验失败必须回滚赠送余额预扣")
	require.Zero(t, operationCount)
}

func TestMediaBillingRepositorySubscriptionRefundSettlesAfterFrozenEntitiesSoftDelete(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		subscriptionType: service.SubscriptionTypeSubscription,
		userBalance:      10,
	})
	subscription := mustCreateSubscription(t, testEntClient(t), &service.UserSubscription{
		UserID:  fixture.user.ID,
		GroupID: fixture.group.ID,
	})
	grant, err := testEntClient(t).SubscriptionGrant.Create().
		SetSubscriptionID(subscription.ID).
		SetCreatedAt(fixture.billedAt.Add(-2 * time.Hour)).
		SetStartsAt(fixture.billedAt.Add(-time.Hour)).
		SetExpiresAt(fixture.billedAt.Add(time.Hour)).
		Save(fixture.ctx)
	require.NoError(t, err)
	_, err = fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(4))
	require.NoError(t, err)

	for _, statement := range []struct {
		query string
		id    int64
	}{
		{query: "UPDATE subscription_grants SET deleted_at = NOW() WHERE id = $1", id: grant.ID},
		{query: "UPDATE user_subscriptions SET deleted_at = NOW() WHERE id = $1", id: subscription.ID},
		{query: "UPDATE groups SET deleted_at = NOW() WHERE id = $1", id: fixture.group.ID},
		{query: "UPDATE users SET deleted_at = NOW() WHERE id = $1", id: fixture.user.ID},
		{query: "UPDATE api_keys SET group_id = NULL, deleted_at = NOW() WHERE id = $1", id: fixture.apiKey.ID},
	} {
		result, updateErr := integrationDB.ExecContext(fixture.ctx, statement.query, statement.id)
		require.NoError(t, updateErr)
		require.EqualValues(t, 1, mustRowsAffected(t, result))
	}

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationFailure,
		0,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 4, settlement.RefundedAmount, 0.00000001)

	var subscriptionUsage, grantUsage float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&subscriptionUsage))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM subscription_grants WHERE id = $1", grant.ID).Scan(&grantUsage))
	require.Zero(t, subscriptionUsage)
	require.Zero(t, grantUsage)
}

func TestMediaBillingRepositorySubscriptionAdditionalChargeSettlesAfterFrozenEntitiesSoftDelete(t *testing.T) {
	fixture := newMediaBillingRepositoryFixture(t, mediaBillingRepositoryFixtureOptions{
		subscriptionType: service.SubscriptionTypeSubscription,
		userBalance:      10,
	})
	subscription := mustCreateSubscription(t, testEntClient(t), &service.UserSubscription{
		UserID:  fixture.user.ID,
		GroupID: fixture.group.ID,
	})
	grant, err := testEntClient(t).SubscriptionGrant.Create().
		SetSubscriptionID(subscription.ID).
		SetCreatedAt(fixture.billedAt.Add(-2 * time.Hour)).
		SetStartsAt(fixture.billedAt.Add(-time.Hour)).
		SetExpiresAt(fixture.billedAt.Add(time.Hour)).
		Save(fixture.ctx)
	require.NoError(t, err)
	_, err = fixture.repo.Precharge(fixture.ctx, fixture.prechargeCommand(1))
	require.NoError(t, err)

	for _, statement := range []struct {
		query string
		id    int64
	}{
		{query: "UPDATE subscription_grants SET deleted_at = NOW() WHERE id = $1", id: grant.ID},
		{query: "UPDATE user_subscriptions SET deleted_at = NOW() WHERE id = $1", id: subscription.ID},
		{query: "UPDATE groups SET deleted_at = NOW() WHERE id = $1", id: fixture.group.ID},
		{query: "UPDATE users SET deleted_at = NOW() WHERE id = $1", id: fixture.user.ID},
		{query: "UPDATE api_keys SET group_id = NULL WHERE id = $1", id: fixture.apiKey.ID},
	} {
		result, updateErr := integrationDB.ExecContext(fixture.ctx, statement.query, statement.id)
		require.NoError(t, updateErr)
		require.EqualValues(t, 1, mustRowsAffected(t, result))
	}

	settlement, err := fixture.repo.Settle(fixture.ctx, fixture.settlementCommand(
		service.MediaBillingOperationSuccess,
		2,
		nil,
		0,
	))
	require.NoError(t, err)
	require.True(t, settlement.Applied)
	require.InDelta(t, 1, settlement.AdditionalChargedAmount, 0.00000001)
	require.Equal(t, service.MediaBillingFundingSubscription, settlement.Allocation.FundingSource)

	var subscriptionUsage, grantUsage float64
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&subscriptionUsage))
	require.NoError(t, integrationDB.QueryRowContext(fixture.ctx,
		"SELECT daily_usage_usd FROM subscription_grants WHERE id = $1", grant.ID).Scan(&grantUsage))
	require.InDelta(t, 2, subscriptionUsage, 0.00000001)
	require.InDelta(t, 2, grantUsage, 0.00000001)
}

func mustRowsAffected(t *testing.T, result interface{ RowsAffected() (int64, error) }) int64 {
	t.Helper()
	affected, err := result.RowsAffected()
	require.NoError(t, err)
	return affected
}
