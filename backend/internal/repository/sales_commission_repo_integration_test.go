//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionRepository_CreateListsAndSettles(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewSalesCommissionRepository(integrationDB)

	sales := mustCreateUser(t, client, &service.User{Email: "sales-" + uuid.NewString() + "@example.com", Username: "sales"})
	referee := mustCreateUser(t, client, &service.User{Email: "buyer-" + uuid.NewString() + "@example.com", Username: "buyer"})

	referralID := mustCreateSalesCommissionReferral(t, ctx, sales.ID, referee.ID)
	firstOrderID := mustCreateCompletedBalanceOrder(t, ctx, referee.ID, 10, 10)
	secondOrderID := mustCreateCompletedBalanceOrder(t, ctx, referee.ID, 20, 20)

	createInput := &service.SalesCommissionCreate{
		SalesUserID:         sales.ID,
		RefereeUserID:       referee.ID,
		ReferralID:          referralID,
		PaymentOrderID:      firstOrderID,
		OrderPayAmountCNY:   10,
		OrderCreditedAmount: 10,
		CommissionRate:      10,
		CommissionTotalCNY:  1,
		Note:                "first commission",
	}
	require.NoError(t, repo.CreateForOrder(ctx, createInput))
	require.NoError(t, repo.CreateForOrder(ctx, createInput), "duplicate payment_order_id should be ignored")
	require.NoError(t, repo.CreateForOrder(ctx, &service.SalesCommissionCreate{
		SalesUserID:         sales.ID,
		RefereeUserID:       referee.ID,
		ReferralID:          referralID,
		PaymentOrderID:      secondOrderID,
		OrderPayAmountCNY:   20,
		OrderCreditedAmount: 20,
		CommissionRate:      10,
		CommissionTotalCNY:  2,
		Note:                "second commission",
	}))

	var rowCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sales_commission_records WHERE payment_order_id = $1
	`, firstOrderID).Scan(&rowCount))
	require.Equal(t, 1, rowCount)

	summary, err := repo.GetSummaryBySalesUser(ctx, sales.ID)
	require.NoError(t, err)
	require.Equal(t, sales.ID, summary.SalesUserID)
	require.InDelta(t, 3, summary.TotalCommissionCNY, 0.000001)
	require.InDelta(t, 3, summary.FrozenCNY, 0.000001)
	require.Equal(t, 2, summary.RecordsCount)

	summaries, total, err := repo.ListSummaries(ctx, service.SalesCommissionSummaryListParams{Search: sales.Email, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 1)
	require.NotEmpty(t, summaries)

	_, err = integrationDB.ExecContext(ctx, `
		UPDATE sales_commission_records
		SET unlocked_cny = CASE payment_order_id WHEN $1 THEN 0.40 WHEN $2 THEN 1.00 END,
		    credited_used_amount = CASE payment_order_id WHEN $1 THEN 4 WHEN $2 THEN 10 END,
		    status = 'partial_unlocked'
		WHERE payment_order_id IN ($1, $2)
	`, firstOrderID, secondOrderID)
	require.NoError(t, err)

	settlement, err := repo.CreateSettlement(ctx, &service.SalesCommissionSettlementCreate{
		SalesUserID: sales.ID,
		AmountCNY:   0.75,
		Note:        "manual payout",
		CreatedBy:   &sales.ID,
	})
	require.NoError(t, err)
	require.Equal(t, sales.ID, settlement.SalesUserID)
	require.InDelta(t, 0.75, settlement.AmountCNY, 0.000001)

	records, total, err := repo.ListRecords(ctx, service.SalesCommissionRecordListParams{SalesUserID: sales.ID, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, records, 2)
	require.Equal(t, firstOrderID, records[0].PaymentOrderID)
	require.InDelta(t, 0.40, records[0].SettledCNY, 0.000001)
	require.InDelta(t, 0, records[0].SettleableCNY, 0.000001)
	require.Equal(t, secondOrderID, records[1].PaymentOrderID)
	require.InDelta(t, 0.35, records[1].SettledCNY, 0.000001)
	require.InDelta(t, 0.65, records[1].SettleableCNY, 0.000001)

	settlements, total, err := repo.ListSettlements(ctx, service.SalesCommissionSettlementListParams{SalesUserID: sales.ID, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, settlements, 1)
	require.InDelta(t, 0.75, settlements[0].AmountCNY, 0.000001)
}

func TestSalesCommissionRepository_CreateSettlementRejectsInvalidAmounts(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewSalesCommissionRepository(integrationDB)

	sales := mustCreateUser(t, client, &service.User{Email: "sales-invalid-" + uuid.NewString() + "@example.com"})

	_, err := repo.CreateSettlement(ctx, &service.SalesCommissionSettlementCreate{
		SalesUserID: sales.ID,
		AmountCNY:   0,
		Note:        "zero",
	})
	require.ErrorIs(t, err, service.ErrSalesCommissionInvalidAmount)

	_, err = repo.CreateSettlement(ctx, &service.SalesCommissionSettlementCreate{
		SalesUserID: sales.ID,
		AmountCNY:   1,
		Note:        "too much",
	})
	require.ErrorIs(t, err, service.ErrSalesCommissionSettleAmountExceeded)
	require.WithinDuration(t, time.Now(), time.Now(), time.Second)
}

func mustCreateSalesCommissionReferral(t *testing.T, ctx context.Context, salesUserID, refereeUserID int64) int64 {
	t.Helper()

	var id int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO referrals (referrer_id, referee_id, referral_code, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'completed', NOW(), NOW())
		RETURNING id
	`, salesUserID, refereeUserID, "SC"+uuid.NewString()[:8]).Scan(&id))
	return id
}

func mustCreateCompletedBalanceOrder(t *testing.T, ctx context.Context, userID int64, amount, payAmount float64) int64 {
	t.Helper()

	var id int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO payment_orders (
			user_id, user_email, user_name, amount, pay_amount, fee_rate, recharge_code,
			out_trade_no, payment_type, payment_trade_no, order_type, status,
			expires_at, paid_at, completed_at, client_ip, src_host, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, 0, $6,
			$7, 'alipay', $8, $9, $10,
			NOW() + INTERVAL '1 hour', NOW(), NOW(), '127.0.0.1', 'test.local', NOW(), NOW()
		)
		RETURNING id
	`, userID, "buyer@example.com", "buyer", amount, payAmount, "RC"+uuid.NewString()[:10], "OT"+uuid.NewString()[:10], "PT"+uuid.NewString()[:10], payment.OrderTypeBalance, payment.OrderStatusCompleted).Scan(&id))
	return id
}
