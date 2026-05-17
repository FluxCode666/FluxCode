package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestSalesCommissionRepositoryListRecordsSettleableRequiresCompletedAndUnblocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*FROM sales_commission_records scr").
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	recordsRows := sqlmock.NewRows([]string{
		"id", "sales_user_id", "sales_email", "sales_username",
		"referee_user_id", "referee_email", "referee_username",
		"referral_id", "payment_order_id", "payment_order_status",
		"order_pay_amount_cny", "order_credited_amount", "commission_rate",
		"commission_event_at", "commission_month", "snapshot_id", "commission_mode",
		"monthly_sales_before_cny", "monthly_sales_after_cny",
		"commission_total_cny", "credited_used_amount", "frozen_cny",
		"unlocked_cny", "settled_cny", "settleable_cny",
		"status", "note", "created_at", "updated_at",
	}).
		AddRow(int64(1), int64(10), "sales@example.com", "sales", int64(20), "buyer@example.com", "buyer", int64(30), int64(40), payment.OrderStatusCompleted, dec("10"), dec("10"), dec("10"), time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), int64(51), service.SalesCommissionModeTiered, dec("90"), dec("100"), dec("1"), dec("4"), dec("0.60"), dec("0.40"), dec("0.10"), dec("0.30"), service.SalesCommissionStatusPartialUnlocked, "", time.Now(), time.Now()).
		AddRow(int64(2), int64(10), "sales@example.com", "sales", int64(21), "buyer2@example.com", "buyer2", int64(31), int64(41), payment.OrderStatusRefunded, dec("10"), dec("10"), dec("10"), time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), int64(51), service.SalesCommissionModeTiered, dec("100"), dec("110"), dec("1"), dec("4"), dec("0.60"), dec("0.40"), dec("0.10"), dec("0"), service.SalesCommissionStatusPartialUnlocked, "", time.Now(), time.Now()).
		AddRow(int64(3), int64(10), "sales@example.com", "sales", int64(22), "buyer3@example.com", "buyer3", int64(32), int64(42), payment.OrderStatusCompleted, dec("10"), dec("10"), dec("10"), time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), int64(51), service.SalesCommissionModeTiered, dec("110"), dec("120"), dec("1"), dec("4"), dec("0.60"), dec("0.40"), dec("0.10"), dec("0"), service.SalesCommissionStatusSettlementBlocked, "", time.Now(), time.Now())

	mock.ExpectQuery(regexp.QuoteMeta("CASE WHEN (scr.payment_order_id IS NULL OR po.status = $2) AND scr.status <> $3 THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END")).
		WithArgs(int64(10), payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, 20, 0).
		WillReturnRows(recordsRows)

	records, total, err := repo.ListRecords(ctx, service.SalesCommissionRecordListParams{SalesUserID: 10, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, records, 3)
	require.InDelta(t, 0.30, records[0].SettleableCNY, 0.000001)
	require.InDelta(t, 0, records[1].SettleableCNY, 0.000001)
	require.InDelta(t, 0, records[2].SettleableCNY, 0.000001)
	require.Equal(t, service.SalesCommissionModeTiered, records[0].CommissionMode)
	require.NotNil(t, records[0].SnapshotID)
	require.Equal(t, int64(51), *records[0].SnapshotID)
	require.Equal(t, "2026-06-01", records[0].CommissionMonth.Format("2006-01-02"))
	require.Equal(t, time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC), records[0].CommissionEventAt)
	require.InDelta(t, 90, records[0].MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 100, records[0].MonthlySalesAfterCNY, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSalesCommissionRepositoryListSummariesSettleableRequiresCompletedAndUnblocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\).*FROM \\(").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	summaryRows := sqlmock.NewRows([]string{
		"sales_user_id", "sales_email", "sales_username",
		"total_commission_cny", "frozen_cny", "unlocked_cny",
		"settleable_cny", "settled_cny", "records_count",
	}).AddRow(int64(10), "sales@example.com", "sales", dec("3"), dec("1.80"), dec("1.20"), dec("0.30"), dec("0.20"), 3)

	mock.ExpectQuery(regexp.QuoteMeta("SUM(CASE WHEN (scr.payment_order_id IS NULL OR po.status = $1) AND scr.status <> $2 THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END)")).
		WithArgs(payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, 20, 0).
		WillReturnRows(summaryRows)

	summaries, total, err := repo.ListSummaries(ctx, service.SalesCommissionSummaryListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, summaries, 1)
	require.InDelta(t, 0.30, summaries[0].SettleableCNY, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSalesCommissionRepositoryGetSummaryBySalesUserSettleableRequiresCompletedAndUnblocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	summaryRows := sqlmock.NewRows([]string{
		"sales_user_id", "sales_email", "sales_username",
		"total_commission_cny", "frozen_cny", "unlocked_cny",
		"settleable_cny", "settled_cny", "records_count",
	}).AddRow(int64(10), "sales@example.com", "sales", dec("3"), dec("1.80"), dec("1.20"), dec("0.30"), dec("0.20"), 3)

	mock.ExpectQuery(regexp.QuoteMeta("SUM(CASE WHEN (scr.payment_order_id IS NULL OR po.status = $2) AND scr.status <> $3 THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END)")).
		WithArgs(int64(10), payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).
		WillReturnRows(summaryRows)

	summary, err := repo.GetSummaryBySalesUser(ctx, 10)
	require.NoError(t, err)
	require.InDelta(t, 0.30, summary.SettleableCNY, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSalesCommissionRepositoryCreateSettlementSkipsBlockedRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("AND scr.status <> $3")).
		WithArgs(int64(10), payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).
		WillReturnRows(sqlmock.NewRows([]string{"id", "unlocked_cny", "settled_cny"}))
	mock.ExpectRollback()

	_, err = repo.CreateSettlement(ctx, &service.SalesCommissionSettlementCreate{
		SalesUserID: 10,
		AmountCNY:   1,
		Note:        "blocked records should not settle",
	})
	require.ErrorIs(t, err, service.ErrSalesCommissionSettleAmountExceeded)
	require.NoError(t, mock.ExpectationsWereMet())
}

func dec(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}
	return d
}
