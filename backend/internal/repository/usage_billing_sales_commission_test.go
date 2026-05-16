package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestDeductUsageBillingBalance_UnlocksSalesCommissionFromOrdinaryRemainder(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	userID := int64(20)
	mock.ExpectQuery("SELECT id, remaining FROM gift_balance_records").
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "remaining"}).AddRow(int64(100), 1.0))
	mock.ExpectExec("UPDATE gift_balance_records").
		WithArgs(0.0, int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE users").
		WithArgs(2.0, userID).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(8.0))
	mock.ExpectQuery("FROM sales_commission_records scr").
		WithArgs(userID, service.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_credited_amount", "credited_used_amount", "commission_total_cny", "unlocked_cny",
		}).AddRow(int64(11), 10.0, 0.0, 1.0, 0.0))
	mock.ExpectExec("UPDATE sales_commission_records").
		WithArgs(2.0, 0.2, service.SalesCommissionStatusPartialUnlocked, int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	balance, err := deductUsageBillingBalance(ctx, tx, userID, 3.0)
	require.NoError(t, err)
	require.InDelta(t, 8.0, balance, 0.000001)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUnlockSalesCommissionFIFO_AllocatesAcrossRecordsInOrder(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	userID := int64(20)
	mock.ExpectQuery("FROM sales_commission_records scr").
		WithArgs(userID, service.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_credited_amount", "credited_used_amount", "commission_total_cny", "unlocked_cny",
		}).
			AddRow(int64(1), 10.0, 0.0, 1.0, 0.0).
			AddRow(int64(2), 10.0, 0.0, 1.0, 0.0))
	mock.ExpectExec("UPDATE sales_commission_records").
		WithArgs(10.0, 1.0, service.SalesCommissionStatusUnlocked, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE sales_commission_records").
		WithArgs(2.0, 0.2, service.SalesCommissionStatusPartialUnlocked, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = unlockSalesCommissionFIFO(ctx, tx, userID, 12.0)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUnlockSalesCommissionFIFO_SkipsSettlementBlockedRecords(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	userID := int64(20)
	mock.ExpectQuery("FROM sales_commission_records scr").
		WithArgs(userID, service.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_credited_amount", "credited_used_amount", "commission_total_cny", "unlocked_cny",
		}))
	mock.ExpectRollback()

	err = unlockSalesCommissionFIFO(ctx, tx, userID, 2.0)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
