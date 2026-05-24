//go:build pqsmoke

// Package repository 内的 PQ smoke 测试：在真实 Postgres 上验证
// SalesCommissionRepository.CreateForOrder 的 INSERT SQL 能正确处理：
//  1. NUMERIC 字面量类型（避免 'inconsistent types deduced for parameter' 报错）
//  2. partial UNIQUE INDEX 上的 ON CONFLICT 子句
//  3. *int64 / NULLable BIGINT 参数在 IS NULL 表达式中的类型推断
//
// 用法：
//
//	SALES_COMMISSION_REPO_PQ_SMOKE_DSN="postgres://user:pass@host:port/db?sslmode=disable" \
//	    go test -tags=pqsmoke ./internal/repository -run PQSmoke -count=1 -v
//
// 不依赖 testcontainers 或 docker，可以连远程开发/测试 PG。
// 测试结束会自动清理写入的 sales_commission_records / monthly_snapshots 记录。
package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const pqSmokeDSNEnv = "SALES_COMMISSION_REPO_PQ_SMOKE_DSN"

func openSmokePQ(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv(pqSmokeDSNEnv)
	if dsn == "" {
		t.Skipf("%s not set; skipping pq smoke test", pqSmokeDSNEnv)
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(context.Background()))
	return db
}

type smokeFixture struct {
	salesUserID     int64
	refereeUserID   int64
	referralID      int64
	paymentOrderID  int64
	payAmount       float64
	creditedAmount  float64
	commissionMode  string
	commissionRate  float64
	minMonthlySales float64
	tiers           []service.SalesCommissionTier
}

// findSmokeFixtureWithOrder 在数据库里挑一组现成数据：
// referrer 是销售用户、referee 已有一笔 COMPLETED balance 订单，并加载销售 mode/tiers 配置。
func findSmokeFixtureWithOrder(t *testing.T, db *sql.DB) smokeFixture {
	t.Helper()
	ctx := context.Background()

	var f smokeFixture
	err := db.QueryRowContext(ctx, `
		SELECT r.referrer_id, r.referee_id, r.id, po.id, po.pay_amount, po.amount,
		       u.sales_commission_mode, u.sales_commission_rate, u.sales_commission_min_monthly_sales
		FROM referrals r
		JOIN payment_orders po ON po.user_id = r.referee_id
		JOIN users u ON u.id = r.referrer_id
		WHERE u.is_sales = true
		  AND po.order_type = 'balance'
		  AND po.status = 'COMPLETED'
		ORDER BY po.id DESC
		LIMIT 1
	`).Scan(&f.salesUserID, &f.refereeUserID, &f.referralID, &f.paymentOrderID, &f.payAmount, &f.creditedAmount,
		&f.commissionMode, &f.commissionRate, &f.minMonthlySales)
	require.NoError(t, err, "数据库里需要至少一组：销售推广人 + 已完成 balance 订单")

	if service.NormalizeSalesCommissionMode(f.commissionMode) == service.SalesCommissionModeTiered {
		rows, qerr := db.QueryContext(ctx, `
			SELECT month_sales_from_cny, month_sales_to_cny, commission_rate, sort_order
			FROM sales_commission_tiers
			WHERE sales_user_id = $1
			ORDER BY sort_order ASC, month_sales_from_cny ASC
		`, f.salesUserID)
		require.NoError(t, qerr)
		defer rows.Close()
		for rows.Next() {
			var (
				from   float64
				toNull sql.NullFloat64
				rate   float64
				order  int
			)
			require.NoError(t, rows.Scan(&from, &toNull, &rate, &order))
			tier := service.SalesCommissionTier{
				MonthSalesFromCNY: from,
				CommissionRate:    rate,
				SortOrder:         order,
			}
			if toNull.Valid {
				v := toNull.Float64
				tier.MonthSalesToCNY = &v
			}
			f.tiers = append(f.tiers, tier)
		}
		require.NoError(t, rows.Err())
		require.NotEmpty(t, f.tiers, "tiered 模式销售用户必须至少配置一个 tier")
	}
	return f
}

// buildCreateInput 用 fixture + 当前时间构造一份 CreateForOrder 入参（与生产 hook 等价）。
func buildCreateInput(f smokeFixture, eventAt, commissionMonth time.Time, note string) *service.SalesCommissionCreate {
	return &service.SalesCommissionCreate{
		SalesUserID:               f.salesUserID,
		RefereeUserID:             f.refereeUserID,
		ReferralID:                f.referralID,
		PaymentOrderID:            &f.paymentOrderID,
		OrderPayAmountCNY:         f.payAmount,
		OrderCreditedAmount:       f.creditedAmount,
		CommissionMode:            f.commissionMode,
		CommissionRate:            f.commissionRate,
		CommissionMinMonthlySales: f.minMonthlySales,
		CommissionTiers:           f.tiers,
		CommissionEventAt:         eventAt,
		CommissionMonth:           commissionMonth,
		Note:                      note,
	}
}

// cleanupSmokeRecords 删除测试写入的 records / monthly_snapshot，避免污染。
func cleanupSmokeRecords(t *testing.T, db *sql.DB, salesUserID, paymentOrderID int64, commissionMonth time.Time) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM sales_commission_records WHERE payment_order_id = $1", paymentOrderID)
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM sales_commission_monthly_snapshots WHERE sales_user_id = $1 AND commission_month = $2",
			salesUserID, commissionMonth)
	})
}

// TestSalesCommissionRepository_CreateForOrder_PQSmoke_BalanceOrder 验证带
// payment_order_id（*int64 非 nil）的常规 balance 充值路径。
//
// 这个测试本身就是回归保护：以下三种 SQL 改坏都会让它失败：
//   - 'ELSE 0' 不带 ::numeric → "inconsistent types deduced for parameter $6"
//   - 'ON CONFLICT ON CONSTRAINT ...' 引用不存在的 constraint → "constraint does not exist"
//   - $4 在 IS NULL 表达式里没 cast → "could not determine data type of parameter $4"
func TestSalesCommissionRepository_CreateForOrder_PQSmoke_BalanceOrder(t *testing.T) {
	db := openSmokePQ(t)
	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	f := findSmokeFixtureWithOrder(t, db)

	now := time.Now().UTC()
	commissionMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	cleanupSmokeRecords(t, db, f.salesUserID, f.paymentOrderID, commissionMonth)

	// 先把可能存在的旧 record 清掉，确保本次必然走 INSERT 分支（而非 ON CONFLICT DO NOTHING）。
	_, err := db.ExecContext(ctx, "DELETE FROM sales_commission_records WHERE payment_order_id = $1", f.paymentOrderID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "DELETE FROM sales_commission_monthly_snapshots WHERE sales_user_id = $1 AND commission_month = $2", f.salesUserID, commissionMonth)
	require.NoError(t, err)

	err = repo.CreateForOrder(ctx, buildCreateInput(f, now, commissionMonth, "pqsmoke"))
	require.NoError(t, err, "CreateForOrder must accept *int64 PaymentOrderID, ::numeric CASE branch, and partial-unique-index ON CONFLICT")

	// 验证写入了 1 行。
	var rowCount int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sales_commission_records WHERE payment_order_id = $1", f.paymentOrderID,
	).Scan(&rowCount))
	require.Equal(t, 1, rowCount, "应当写入 1 行 sales_commission_records")
}

// TestSalesCommissionRepository_CreateForOrder_PQSmoke_DuplicateOrder 验证
// ON CONFLICT (payment_order_id) WHERE payment_order_id IS NOT NULL DO NOTHING
// 在重复 payment_order_id 时不会报错，且不会重复插入。
func TestSalesCommissionRepository_CreateForOrder_PQSmoke_DuplicateOrder(t *testing.T) {
	db := openSmokePQ(t)
	repo := NewSalesCommissionRepository(db)
	ctx := context.Background()

	f := findSmokeFixtureWithOrder(t, db)

	now := time.Now().UTC()
	commissionMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	cleanupSmokeRecords(t, db, f.salesUserID, f.paymentOrderID, commissionMonth)

	_, err := db.ExecContext(ctx, "DELETE FROM sales_commission_records WHERE payment_order_id = $1", f.paymentOrderID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "DELETE FROM sales_commission_monthly_snapshots WHERE sales_user_id = $1 AND commission_month = $2", f.salesUserID, commissionMonth)
	require.NoError(t, err)

	input := buildCreateInput(f, now, commissionMonth, "pqsmoke-dup")
	require.NoError(t, repo.CreateForOrder(ctx, input))
	require.NoError(t, repo.CreateForOrder(ctx, input), "重复 payment_order_id 应当被 ON CONFLICT DO NOTHING 静默吞掉")

	var rowCount int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sales_commission_records WHERE payment_order_id = $1", f.paymentOrderID,
	).Scan(&rowCount))
	require.Equal(t, 1, rowCount, "重复调用不应导致写入多行")
}
