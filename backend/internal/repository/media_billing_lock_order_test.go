package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestMediaBillingPrechargeLocksFundingBeforeAPIKey(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	const (
		taskID   = int64(1)
		userID   = int64(2)
		apiKeyID = int64(3)
		groupID  = int64(4)
	)
	cmd := service.MediaBillingPrechargeCommand{
		IdempotencyKey: "task_1:precharge", RequestFingerprint: "fingerprint",
		TaskID: taskID, TaskPublicID: "task_1", UserID: userID, APIKeyID: apiKeyID, GroupID: groupID,
		Amount: 2,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1\s+FROM media_tasks`).
		WithArgs(taskID, cmd.TaskPublicID, userID, apiKeyID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"found"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO media_billing_operations`).
		WithArgs(taskID, cmd.TaskPublicID, userID, apiKeyID, groupID, nil,
			cmd.IdempotencyKey, string(service.MediaBillingOperationPrecharge), cmd.RequestFingerprint).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectQuery(`SELECT subscription_type, daily_limit_usd, weekly_limit_usd, monthly_limit_usd`).
		WithArgs(groupID).
		WillReturnRows(sqlmock.NewRows([]string{
			"subscription_type", "daily_limit_usd", "weekly_limit_usd", "monthly_limit_usd",
		}).AddRow(service.SubscriptionTypeStandard, nil, nil, nil))

	// Funding rows are locked and mutated before the shared api_keys row.
	mock.ExpectQuery(`SELECT id, remaining\s+FROM gift_balance_records`).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "remaining"}))
	mock.ExpectExec(`UPDATE users\s+SET balance = balance -`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT quota, quota_used, rate_limit_5h`).
		WithArgs(apiKeyID, userID, groupID).
		WillReturnRows(mediaBillingAPIKeyStateRows())
	mock.ExpectExec(`UPDATE media_billing_operations\s+SET precharged_amount`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewMediaBillingRepository(db).Precharge(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMediaBillingSettlementLocksCommissionBeforeAPIKey(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	const (
		taskID   = int64(11)
		userID   = int64(12)
		apiKeyID = int64(13)
		groupID  = int64(14)
	)
	allocationJSON, err := json.Marshal(service.MediaBillingAllocation{
		FundingSource:   service.MediaBillingFundingBalance,
		OrdinaryBalance: 2,
	})
	require.NoError(t, err)
	cmd := service.MediaBillingSettlementCommand{
		IdempotencyKey: "task_11:failure", RequestFingerprint: "fingerprint",
		Operation: service.MediaBillingOperationFailure,
		TaskID:    taskID, TaskPublicID: "task_11", UserID: userID, APIKeyID: apiKeyID, GroupID: groupID,
		FinalAmount: 1,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT 1\s+FROM media_tasks`).
		WithArgs(taskID, cmd.TaskPublicID, userID, apiKeyID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"found"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO media_billing_operations`).
		WithArgs(taskID, cmd.TaskPublicID, userID, apiKeyID, groupID, nil,
			cmd.IdempotencyKey, string(service.MediaBillingOperationFailure), cmd.RequestFingerprint).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(20)))
	mock.ExpectQuery(`SELECT id, task_id, operation, request_fingerprint,\s+precharged_amount`).
		WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "task_id", "operation", "request_fingerprint", "precharged_amount",
			"final_amount", "refunded_amount", "additional_charged_amount", "allocation",
		}).AddRow(int64(19), taskID, string(service.MediaBillingOperationPrecharge), cmd.RequestFingerprint,
			2.0, 0.0, 0.0, 0.0, allocationJSON))

	// Balance refund, commission unlock, then api_keys: this matches Usage Billing.
	mock.ExpectExec(`UPDATE users\s+SET balance = balance \+`).
		WithArgs(sqlmock.AnyArg(), userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`FROM sales_commission_records scr`).
		WithArgs(userID, service.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_credited_amount", "credited_used_amount", "commission_total_cny", "unlocked_cny",
		}))
	mock.ExpectQuery(`SELECT quota, quota_used, rate_limit_5h`).
		WithArgs(apiKeyID, userID).
		WillReturnRows(mediaBillingAPIKeyStateRows())
	mock.ExpectExec(`UPDATE media_billing_operations\s+SET precharged_amount`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := NewMediaBillingRepository(db).Settle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMediaBillingSubscriptionRefundLocksParentBeforeGrant(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	const (
		subscriptionID = int64(31)
		grantID        = int64(32)
		userID         = int64(33)
		groupID        = int64(34)
	)
	mock.ExpectQuery(`SELECT 1\s+FROM user_subscriptions`).
		WithArgs(subscriptionID, userID, groupID).
		WillReturnRows(sqlmock.NewRows([]string{"found"}).AddRow(1))
	mock.ExpectExec(`UPDATE subscription_grants sg`).
		WithArgs(nil, nil, nil, sqlmock.AnyArg(), grantID, subscriptionID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE user_subscriptions\s+SET daily_usage_usd`).
		WithArgs(nil, nil, nil, sqlmock.AnyArg(), subscriptionID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = refundMediaBillingSubscription(ctx, tx, userID, groupID, decimal.NewFromInt(2), decimal.NewFromInt(2), service.MediaBillingAllocation{
		FundingSource:     service.MediaBillingFundingSubscription,
		SubscriptionID:    mediaBillingInt64Pointer(subscriptionID),
		SubscriptionGrant: []service.MediaBillingGrantAllocation{{GrantID: grantID, Amount: 2}},
	})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingSubscriptionLocksParentBeforeLoadingGrants(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	billedAt := time.Now().UTC()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`FROM user_subscriptions us[\s\S]+FOR UPDATE OF us`).
		WithArgs(int64(41)).
		WillReturnRows(sqlmock.NewRows([]string{"daily_limit_usd", "weekly_limit_usd", "monthly_limit_usd"}).
			AddRow(nil, nil, nil))
	mock.ExpectQuery(`SELECT id, daily_usage_usd, weekly_usage_usd, monthly_usage_usd\s+FROM subscription_grants`).
		WithArgs(int64(41), billedAt).
		WillReturnRows(sqlmock.NewRows([]string{"id", "daily_usage_usd", "weekly_usage_usd", "monthly_usage_usd"}).
			AddRow(int64(42), 0.0, 0.0, 0.0))
	mock.ExpectExec(`UPDATE subscription_grants`).
		WithArgs(1.0, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE user_subscriptions us`).
		WithArgs(1.0, int64(41)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	require.NoError(t, incrementUsageBillingSubscription(ctx, tx, int64(41), billedAt, 1.0))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func mediaBillingAPIKeyStateRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"quota", "quota_used", "rate_limit_5h", "rate_limit_1d", "rate_limit_7d",
		"window_5h_start", "window_1d_start", "window_7d_start", "status",
	}).AddRow(0.0, 0.0, 0.0, 0.0, 0.0, nil, nil, nil, service.StatusAPIKeyActive)
}

func mediaBillingInt64Pointer(value int64) *int64 {
	return &value
}
