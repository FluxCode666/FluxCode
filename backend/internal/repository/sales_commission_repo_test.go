package repository

import (
	"context"
	"database/sql"
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

func dec(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}
	return d
}

// TestRepriceMonthlyCommissionRecordsTx_RepricesAllRecordsInOrderAfterCrossingThreshold
// 验证仓储层 repriceMonthlyCommissionRecordsTx：
//   - SELECT FOR UPDATE 按 (commission_event_at ASC, id ASC) 顺序读取当月记录；
//   - 调用 service.RecomputeMonthlyCommissionRecords 计算后，每条记录都通过同一条 UPDATE 模板回写；
//   - 跨门槛场景下整月所有记录都被补算（spec §6.5 / §6.6）。
//
// 这条测试用 sqlmock 保护 SQL 形态不被随意改坏；金额参数用 sqlmock.AnyArg 不直接比较，
// 因为 decimal.Decimal 通过 driver.Value 编码出的具体值依赖于精度归一化，
// 改动比较容易脆。
func TestRepriceMonthlyCommissionRecordsTx_RepricesAllRecordsInOrderAfterCrossingThreshold(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	salesUserID := int64(10)
	commissionMonth := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	snapshot := &salesCommissionMonthlySnapshot{
		ID:                  999,
		CommissionMode:      service.SalesCommissionModeTiered,
		FixedCommissionRate: 0,
		MinMonthlySalesCNY:  100,
		Tiers: []service.SalesCommissionTier{
			{MonthSalesFromCNY: 0, CommissionRate: 10},
		},
	}

	// SELECT FOR UPDATE 按时间序返回 2 笔（60 + 90 = 150 跨过门槛 100）
	mock.ExpectQuery(`FROM\s+sales_commission_records.*ORDER\s+BY\s+commission_event_at\s+ASC,\s+id\s+ASC.*FOR\s+UPDATE`).
		WithArgs(salesUserID, commissionMonth).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_pay_amount_cny", "order_credited_amount", "credited_used_amount", "payment_order_id",
		}).
			AddRow(int64(1), 60.0, 60.0, 30.0, int64(101)).
			AddRow(int64(2), 90.0, 90.0, 0.0, int64(102)))

	// 两条 UPDATE 都应该按新 status CASE 模板执行。
	mock.ExpectExec(`UPDATE\s+sales_commission_records\s+SET\s+commission_total_cny\s*=`).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			service.SalesCommissionStatusSettlementBlocked,
			service.SalesCommissionStatusFrozen,
			service.SalesCommissionStatusSettled,
			service.SalesCommissionStatusUnlocked,
			service.SalesCommissionStatusPartialUnlocked,
			int64(1),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE\s+sales_commission_records\s+SET\s+commission_total_cny\s*=`).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			service.SalesCommissionStatusSettlementBlocked,
			service.SalesCommissionStatusFrozen,
			service.SalesCommissionStatusSettled,
			service.SalesCommissionStatusUnlocked,
			service.SalesCommissionStatusPartialUnlocked,
			int64(2),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	require.NoError(t, repriceMonthlyCommissionRecordsTx(ctx, tx, snapshot, salesUserID, commissionMonth))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_GetMonthlyProgress_WithFrozenSnapshot 当月已存在 snapshot
// 时，仓储层应该把 snapshot 字段 + 当月销售额/佣金 SUM 一起组装进 SalesCommissionMonthlyProgressData。
func TestSalesCommissionRepository_GetMonthlyProgress_WithFrozenSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()
	salesUserID := int64(10)
	commissionMonth := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	tiersJSON := []byte(`[{"month_sales_from_cny":0,"month_sales_to_cny":200,"commission_rate":5,"sort_order":1},{"month_sales_from_cny":200,"commission_rate":10,"sort_order":2}]`)
	mock.ExpectQuery(`FROM\s+sales_commission_monthly_snapshots`).
		WithArgs(salesUserID, commissionMonth).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "commission_mode", "fixed_commission_rate", "min_monthly_sales_cny", "tiers_json",
		}).AddRow(int64(999), "tiered", 0.0, 100.0, tiersJSON))
	mock.ExpectQuery(`FROM\s+sales_commission_records`).
		WithArgs(salesUserID, commissionMonth).
		WillReturnRows(sqlmock.NewRows([]string{"sum_sales", "sum_commission"}).AddRow(150.0, 7.5))

	data, err := repo.GetMonthlyProgress(ctx, salesUserID, commissionMonth)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotNil(t, data.Snapshot)
	require.Equal(t, service.SalesCommissionModeTiered, data.Snapshot.CommissionMode)
	require.InDelta(t, 100.0, data.Snapshot.MinMonthlySalesCNY, 0.0001)
	require.Len(t, data.Snapshot.Tiers, 2)
	require.InDelta(t, 5.0, data.Snapshot.Tiers[0].CommissionRate, 0.0001)
	require.InDelta(t, 150.0, data.MonthlySalesCNY, 0.0001)
	require.InDelta(t, 7.5, data.MonthlyCommissionCNY, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_GetMonthlyProgress_NoSnapshotReturnsAggregatesOnly 当月还没有
// snapshot 时，仓储层应返回 Snapshot=nil 但仍提供销售额聚合（即使是 0），让 service 层
// fallback 到 user 当前规则。
func TestSalesCommissionRepository_GetMonthlyProgress_NoSnapshotReturnsAggregatesOnly(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()
	salesUserID := int64(10)
	commissionMonth := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`FROM\s+sales_commission_monthly_snapshots`).
		WithArgs(salesUserID, commissionMonth).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`FROM\s+sales_commission_records`).
		WithArgs(salesUserID, commissionMonth).
		WillReturnRows(sqlmock.NewRows([]string{"sum_sales", "sum_commission"}).AddRow(0.0, 0.0))

	data, err := repo.GetMonthlyProgress(ctx, salesUserID, commissionMonth)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.Nil(t, data.Snapshot)
	require.InDelta(t, 0.0, data.MonthlySalesCNY, 0.0001)
	require.InDelta(t, 0.0, data.MonthlyCommissionCNY, 0.0001)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRepriceMonthlyCommissionRecordsTx_NoRecordsIsNoop 当月份无记录时不应发出 UPDATE。
func TestRepriceMonthlyCommissionRecordsTx_NoRecordsIsNoop(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	salesUserID := int64(10)
	commissionMonth := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	snapshot := &salesCommissionMonthlySnapshot{
		ID:                  100,
		CommissionMode:      service.SalesCommissionModeFixed,
		FixedCommissionRate: 10,
	}

	mock.ExpectQuery(`FROM\s+sales_commission_records.*FOR\s+UPDATE`).
		WithArgs(salesUserID, commissionMonth).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "order_pay_amount_cny", "order_credited_amount", "credited_used_amount", "payment_order_id",
		}))
	mock.ExpectRollback()

	require.NoError(t, repriceMonthlyCommissionRecordsTx(ctx, tx, snapshot, salesUserID, commissionMonth))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_GetOverview_AggregatesAllSections 验证 GetOverview
// 按顺序发出 4 条 SQL（KPI 聚合 / threshold_met_users / monthly_trend / top_sales），
// 并把行结果装配到 SalesCommissionOverviewData 上。
func TestSalesCommissionRepository_GetOverview_AggregatesAllSections(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	start := time.Date(2026, 4, 30, 16, 0, 0, 0, time.UTC) // 2026-05-01 00:00 Shanghai
	end := time.Date(2026, 5, 31, 15, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
	trendStart := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	trendEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// 1) KPI / status / mode 单查
	mock.ExpectQuery(`SUM\(scr\.order_pay_amount_cny\)`).
		WithArgs(start, end, payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked,
			service.SalesCommissionModeFixed, service.SalesCommissionModeTiered).
		WillReturnRows(sqlmock.NewRows([]string{
			"related", "commission_total", "frozen", "settleable", "settled",
			"active_users", "fixed_records", "tiered_records", "fixed_commission", "tiered_commission",
		}).AddRow(dec("1000"), dec("100"), dec("80"), dec("15"), dec("5"),
			3, 2, 4, dec("30"), dec("70")))

	// 2) threshold_met_users
	mock.ExpectQuery(`COUNT\(DISTINCT scr\.sales_user_id\)\s+FROM sales_commission_records scr\s+JOIN sales_commission_monthly_snapshots`).
		WithArgs(start, end, service.SalesCommissionModeTiered).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// 3) monthly_trend
	mock.ExpectQuery(`GROUP BY scr\.commission_month\s+ORDER BY scr\.commission_month ASC`).
		WithArgs(trendStart, trendEnd).
		WillReturnRows(sqlmock.NewRows([]string{"month", "related", "commission"}).
			AddRow(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), dec("1000"), dec("100")))

	// 4) top_sales
	mock.ExpectQuery(`ORDER BY SUM\(scr\.commission_total_cny\) DESC, scr\.sales_user_id ASC\s+LIMIT 10`).
		WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"sales_user_id", "email", "username", "related", "commission"}).
			AddRow(int64(1), "a@example.com", "a", dec("500"), dec("60")).
			AddRow(int64(2), "b@example.com", "b", dec("500"), dec("40")))

	overview, err := repo.GetOverview(ctx, service.SalesCommissionOverviewQuery{
		Start: start, End: end, MonthlyTrendStart: trendStart, MonthlyTrendEnd: trendEnd,
	})
	require.NoError(t, err)
	require.NotNil(t, overview)

	require.InDelta(t, 1000.0, overview.KPI.RelatedOrderAmountCNY, 0.0001)
	require.InDelta(t, 100.0, overview.KPI.CommissionTotalCNY, 0.0001)
	require.InDelta(t, 80.0, overview.KPI.FrozenCNY, 0.0001)
	require.InDelta(t, 15.0, overview.KPI.SettleableCNY, 0.0001)
	require.InDelta(t, 5.0, overview.KPI.SettledCNY, 0.0001)
	require.Equal(t, 3, overview.KPI.ActiveSalesUsers)
	require.Equal(t, 2, overview.KPI.ThresholdMetUsers)
	require.InDelta(t, 10.0, overview.KPI.AvgCommissionRate, 0.0001) // 100 / 1000 * 100

	require.Equal(t, 2, overview.ModeBreakdown.FixedRecords)
	require.Equal(t, 4, overview.ModeBreakdown.TieredRecords)
	require.InDelta(t, 30.0, overview.ModeBreakdown.FixedCommissionCNY, 0.0001)
	require.InDelta(t, 70.0, overview.ModeBreakdown.TieredCommissionCNY, 0.0001)

	require.InDelta(t, 80.0, overview.StatusBreakdown.FrozenCNY, 0.0001)
	require.InDelta(t, 15.0, overview.StatusBreakdown.SettleableCNY, 0.0001)
	require.InDelta(t, 5.0, overview.StatusBreakdown.SettledCNY, 0.0001)

	require.Len(t, overview.MonthlyTrend, 1) // repo 只返回命中月份；service 层负责补零到 12 月
	require.Equal(t, time.May, overview.MonthlyTrend[0].Month.Month())

	require.Len(t, overview.TopSales, 2)
	require.Equal(t, int64(1), overview.TopSales[0].SalesUserID)
	require.Equal(t, "a@example.com", overview.TopSales[0].SalesEmail)
	require.InDelta(t, 60.0, overview.TopSales[0].CommissionTotalCNY, 0.0001)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_GetOverview_NoRowsReturnsZeros 当区间内无记录时，
// 仓储层应返回各字段零值（含 AvgCommissionRate 防除零），并返回空 trend / top10。
func TestSalesCommissionRepository_GetOverview_NoRowsReturnsZeros(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SUM\(scr\.order_pay_amount_cny\)`).
		WillReturnRows(sqlmock.NewRows([]string{
			"related", "commission_total", "frozen", "settleable", "settled",
			"active_users", "fixed_records", "tiered_records", "fixed_commission", "tiered_commission",
		}).AddRow(dec("0"), dec("0"), dec("0"), dec("0"), dec("0"), 0, 0, 0, dec("0"), dec("0")))
	mock.ExpectQuery(`COUNT\(DISTINCT scr\.sales_user_id\)\s+FROM sales_commission_records scr\s+JOIN sales_commission_monthly_snapshots`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`GROUP BY scr\.commission_month`).
		WillReturnRows(sqlmock.NewRows([]string{"month", "related", "commission"}))
	mock.ExpectQuery(`LIMIT 10`).
		WillReturnRows(sqlmock.NewRows([]string{"sales_user_id", "email", "username", "related", "commission"}))

	overview, err := repo.GetOverview(ctx, service.SalesCommissionOverviewQuery{
		Start: start, End: end, MonthlyTrendStart: start, MonthlyTrendEnd: end,
	})
	require.NoError(t, err)
	require.NotNil(t, overview)
	require.InDelta(t, 0.0, overview.KPI.RelatedOrderAmountCNY, 0.0001)
	require.InDelta(t, 0.0, overview.KPI.AvgCommissionRate, 0.0001)
	require.Equal(t, 0, overview.KPI.ThresholdMetUsers)
	require.Empty(t, overview.MonthlyTrend)
	require.Empty(t, overview.TopSales)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_ListMissingCommissionPaymentOrders_FiltersAndMapsRows 验证：
//   - 传入正确的过滤条件（status=completed + order_type=balance + 金额>0 + NOT EXISTS sales_commission_records）
//   - 把每行映射成填充了必要字段的 *dbent.PaymentOrder
//   - limit 透传到 SQL 占位
func TestSalesCommissionRepository_ListMissingCommissionPaymentOrders_FiltersAndMapsRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	paid1 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	completed1 := time.Date(2026, 5, 1, 12, 5, 0, 0, time.UTC)
	created1 := time.Date(2026, 5, 1, 11, 55, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "order_type", "status", "pay_amount", "amount",
		"paid_at", "completed_at", "created_at",
	}).
		AddRow(int64(101), int64(20), payment.OrderTypeBalance, payment.OrderStatusCompleted,
			100.0, 100.0, paid1, completed1, created1).
		// 第二行 paid_at NULL，验证 nullable 字段处理
		AddRow(int64(102), int64(21), payment.OrderTypeBalance, payment.OrderStatusCompleted,
			200.0, 200.0, nil, completed1, created1)

	mock.ExpectQuery(`FROM\s+payment_orders\s+po.*WHERE\s+po\.status\s*=\s*\$1.*AND\s+po\.order_type\s*=\s*\$2.*NOT\s+EXISTS.*FROM\s+sales_commission_records.*WHERE\s+r\.payment_order_id\s*=\s*po\.id.*ORDER\s+BY\s+po\.id\s+ASC.*LIMIT\s+\$3`).
		WithArgs(payment.OrderStatusCompleted, payment.OrderTypeBalance, 50).
		WillReturnRows(rows)

	orders, err := repo.ListMissingCommissionPaymentOrders(ctx, 50)
	require.NoError(t, err)
	require.Len(t, orders, 2)

	require.Equal(t, int64(101), orders[0].ID)
	require.Equal(t, int64(20), orders[0].UserID)
	require.Equal(t, payment.OrderTypeBalance, orders[0].OrderType)
	require.Equal(t, payment.OrderStatusCompleted, orders[0].Status)
	require.InDelta(t, 100.0, orders[0].PayAmount, 0.0001)
	require.InDelta(t, 100.0, orders[0].Amount, 0.0001)
	require.NotNil(t, orders[0].PaidAt)
	require.True(t, orders[0].PaidAt.Equal(paid1))
	require.NotNil(t, orders[0].CompletedAt)
	require.True(t, orders[0].CreatedAt.Equal(created1))

	// 第二行 paid_at 应当解析成 nil（保持与 ent 实体语义一致）
	require.Nil(t, orders[1].PaidAt)
	require.Equal(t, int64(102), orders[1].ID)

	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_ListRecords_DefaultsToCreatedAtDesc 默认排序应当按 created_at DESC,
// 第二排序键用同方向的 id 保证分页结果稳定（避免同一毫秒下 ORDER BY 不确定导致分页跳行）。
//
// 这条测试是 spec 「佣金明细默认按创建时间倒序」的回归保护，破坏 SQL 形态会立即报错。
func TestSalesCommissionRepository_ListRecords_DefaultsToCreatedAtDesc(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM sales_commission_records scr`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// 主查询里必须出现 ORDER BY scr.created_at DESC, scr.id DESC，这是默认排序契约。
	mock.ExpectQuery(`ORDER BY scr\.created_at DESC, scr\.id DESC`).
		WithArgs(payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, 20, 0).
		WillReturnRows(emptySalesCommissionRecordRows())

	// 不传 SortOrder，应当走默认 DESC。
	_, _, err = repo.ListRecords(ctx, service.SalesCommissionRecordListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_ListRecords_HonorsAscSortOrder 当显式传 SortOrder="asc" 时切到正序。
func TestSalesCommissionRepository_ListRecords_HonorsAscSortOrder(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM sales_commission_records scr`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`ORDER BY scr\.created_at ASC, scr\.id ASC`).
		WithArgs(payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, 20, 0).
		WillReturnRows(emptySalesCommissionRecordRows())

	_, _, err = repo.ListRecords(ctx, service.SalesCommissionRecordListParams{Page: 1, PageSize: 20, SortOrder: "asc"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSalesCommissionRepository_ListRecords_NormalizesInvalidSortOrder 非法 SortOrder 应当回退到 DESC，
// 这是 SQL 注入兜底（即使 service 层已经做了 normalize，repo 层也必须独立安全）。
func TestSalesCommissionRepository_ListRecords_NormalizesInvalidSortOrder(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM sales_commission_records scr`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`ORDER BY scr\.created_at DESC, scr\.id DESC`).
		WithArgs(payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, 20, 0).
		WillReturnRows(emptySalesCommissionRecordRows())

	_, _, err = repo.ListRecords(ctx, service.SalesCommissionRecordListParams{
		Page: 1, PageSize: 20,
		SortOrder: "'; DROP TABLE sales_commission_records; --",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// emptySalesCommissionRecordRows 是上面三个测试共用的空结果集，列与 scanSalesCommissionRecord 期望的 29 列一致。
func emptySalesCommissionRecordRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "sales_user_id", "sales_email", "sales_username",
		"referee_user_id", "referee_email", "referee_username",
		"referral_id", "payment_order_id", "payment_order_status",
		"order_pay_amount_cny", "order_credited_amount", "commission_rate",
		"commission_event_at", "commission_month", "snapshot_id", "commission_mode",
		"monthly_sales_before_cny", "monthly_sales_after_cny",
		"commission_total_cny", "credited_used_amount", "frozen_cny",
		"unlocked_cny", "settled_cny", "settleable_cny",
		"status", "note", "created_at", "updated_at",
	})
}

// TestSalesCommissionRepository_ListMissingCommissionPaymentOrders_NoRowsReturnsEmpty 没候选时返回空切片 + nil error。
func TestSalesCommissionRepository_ListMissingCommissionPaymentOrders_NoRowsReturnsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	mock.ExpectQuery(`FROM\s+payment_orders`).
		WithArgs(payment.OrderStatusCompleted, payment.OrderTypeBalance, 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "order_type", "status", "pay_amount", "amount",
			"paid_at", "completed_at", "created_at",
		}))

	orders, err := repo.ListMissingCommissionPaymentOrders(ctx, 100)
	require.NoError(t, err)
	require.Empty(t, orders)
	require.NoError(t, mock.ExpectationsWereMet())
}
