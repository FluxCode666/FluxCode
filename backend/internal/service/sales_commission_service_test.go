package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestSalesCommissionService_HandleBalanceRechargeCompleted_CreatesFrozenCommission(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{}
	refRepo := &salesCommissionReferralRepoStub{
		byReferee: map[int64]*Referral{
			20: {ID: 7, ReferrerID: 10, RefereeID: 20},
		},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: true, SalesCommissionRate: 12.5},
		},
	}
	svc := NewSalesCommissionService(repo, refRepo, userRepo)

	paidAt := time.Date(2026, 5, 31, 16, 30, 0, 0, time.UTC)

	err := svc.HandleBalanceRechargeCompleted(ctx, &dbent.PaymentOrder{
		ID:          99,
		UserID:      20,
		OrderType:   payment.OrderTypeBalance,
		Status:      payment.OrderStatusCompleted,
		PayAmount:   123.456,
		Amount:      200.12345678,
		PaidAt:      &paidAt,
		CompletedAt: &paidAt,
	})

	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.NotNil(t, repo.created[0].PaymentOrderID)
	require.Equal(t, int64(10), repo.created[0].SalesUserID)
	require.Equal(t, int64(20), repo.created[0].RefereeUserID)
	require.Equal(t, int64(7), repo.created[0].ReferralID)
	require.Equal(t, 123.456, repo.created[0].OrderPayAmountCNY)
	require.Equal(t, 200.12345678, repo.created[0].OrderCreditedAmount)
	require.Equal(t, SalesCommissionModeFixed, repo.created[0].CommissionMode)
	require.Equal(t, 12.5, repo.created[0].CommissionRate)
	require.Equal(t, "Balance recharge commission", repo.created[0].Note)
	require.Equal(t, int64(99), *repo.created[0].PaymentOrderID)
	require.Equal(t, paidAt, repo.created[0].CommissionEventAt)
	require.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), repo.created[0].CommissionMonth)
}

func TestSalesCommissionService_HandleReferralManualCompletion_CreatesCommissionRecord(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{}
	refRepo := &salesCommissionReferralRepoStub{}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: true, SalesCommissionRate: 12.5},
		},
	}
	svc := NewSalesCommissionService(repo, refRepo, userRepo)

	before := time.Now()

	err := svc.HandleReferralManualCompletion(ctx, &Referral{
		ID:         7,
		ReferrerID: 10,
		RefereeID:  20,
		Status:     ReferralStatusPending,
	}, 123.456, 200.12345678, "manual completion")

	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.Equal(t, int64(10), repo.created[0].SalesUserID)
	require.Equal(t, int64(20), repo.created[0].RefereeUserID)
	require.Equal(t, int64(7), repo.created[0].ReferralID)
	require.Nil(t, repo.created[0].PaymentOrderID)
	require.Equal(t, 123.456, repo.created[0].OrderPayAmountCNY)
	require.Equal(t, 200.12345678, repo.created[0].OrderCreditedAmount)
	require.Equal(t, SalesCommissionModeFixed, repo.created[0].CommissionMode)
	require.Contains(t, repo.created[0].Note, "manual completion")
	eventAt := repo.created[0].CommissionEventAt
	after := time.Now()
	require.False(t, eventAt.Before(before))
	require.False(t, eventAt.After(after))
	require.Equal(t, salesCommissionMonthStartForTest(eventAt), repo.created[0].CommissionMonth)
}

func TestSalesCommissionService_HandleBalanceRechargeCompleted_AllowsTieredSalesUserWithZeroFixedRate(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{}
	refRepo := &salesCommissionReferralRepoStub{
		byReferee: map[int64]*Referral{
			20: {ID: 7, ReferrerID: 10, RefereeID: 20},
		},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {
				ID:                             10,
				IsSales:                        true,
				SalesCommissionMode:            SalesCommissionModeTiered,
				SalesCommissionTiers:           []SalesCommissionTier{{MonthSalesFromCNY: 0, CommissionRate: 12}},
				SalesCommissionRate:            0,
				SalesCommissionMinMonthlySales: 0,
			},
		},
	}
	svc := NewSalesCommissionService(repo, refRepo, userRepo)

	err := svc.HandleBalanceRechargeCompleted(ctx, completedBalanceOrder())

	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	require.Equal(t, SalesCommissionModeTiered, repo.created[0].CommissionMode)
	require.Len(t, repo.created[0].CommissionTiers, 1)
	require.Equal(t, 12.0, repo.created[0].CommissionTiers[0].CommissionRate)
}

// TestCalculateSalesCommission_TieredThresholdCrossing 验证 spec §2 / §6.5：
// 当本笔订单让月累计跨过门槛时，本笔记录拿到的是 P(after) - P(before)，
// 而不是 "只对超出门槛部分计提"。本笔之前未达门槛的记录由仓储层 Reprice 阶段补算。
func TestCalculateSalesCommission_TieredThresholdCrossing(t *testing.T) {
	calc, err := CalculateSalesCommission(90, 60, SalesCommissionModeTiered, 0, 100, []SalesCommissionTier{
		{MonthSalesFromCNY: 0, MonthSalesToCNY: float64Ptr(200), CommissionRate: 10},
		{MonthSalesFromCNY: 200, CommissionRate: 20},
	})

	require.NoError(t, err)
	// P(150)=15, P(60)=6 → 9；rate=9/90=10%
	require.InDelta(t, 9, calc.CommissionTotalCNY, 0.000001)
	require.InDelta(t, 10, calc.CommissionRate, 0.0001)
	require.InDelta(t, 60, calc.MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 150, calc.MonthlySalesAfterCNY, 0.000001)
}

// TestCalculateSalesCommission_FixedThresholdCrossing 同上，但 fixed 模式：
// fixedRate=12，跨门槛后本笔 = (150-60)*12% = 10.8；rate=12%。
func TestCalculateSalesCommission_FixedThresholdCrossing(t *testing.T) {
	calc, err := CalculateSalesCommission(90, 60, SalesCommissionModeFixed, 12, 100, nil)

	require.NoError(t, err)
	require.InDelta(t, 10.8, calc.CommissionTotalCNY, 0.000001)
	require.InDelta(t, 12, calc.CommissionRate, 0.0001)
	require.InDelta(t, 60, calc.MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 150, calc.MonthlySalesAfterCNY, 0.000001)
}

// TestCalculateSalesCommission_TieredBelowThreshold 验证 spec §6.5：
// 整月累计低于门槛时本笔总佣金为 0。
func TestCalculateSalesCommission_TieredBelowThreshold(t *testing.T) {
	calc, err := CalculateSalesCommission(60, 0, SalesCommissionModeTiered, 0, 100, []SalesCommissionTier{
		{MonthSalesFromCNY: 0, CommissionRate: 10},
	})

	require.NoError(t, err)
	require.InDelta(t, 0, calc.CommissionTotalCNY, 0.000001)
	require.InDelta(t, 0, calc.CommissionRate, 0.0001)
	require.InDelta(t, 0, calc.MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 60, calc.MonthlySalesAfterCNY, 0.000001)
}

// TestCalculateSalesCommission_TieredCrossesMultipleSegments 验证 spec §6.5.2：
// 单笔订单跨越多个梯度区间时，按各区间分段累加。
func TestCalculateSalesCommission_TieredCrossesMultipleSegments(t *testing.T) {
	// 0~10000 => 5%, 10000~20000 => 8%, 20000~ => 10%
	calc, err := CalculateSalesCommission(15000, 8000, SalesCommissionModeTiered, 0, 0, []SalesCommissionTier{
		{MonthSalesFromCNY: 0, MonthSalesToCNY: float64Ptr(10000), CommissionRate: 5},
		{MonthSalesFromCNY: 10000, MonthSalesToCNY: float64Ptr(20000), CommissionRate: 8},
		{MonthSalesFromCNY: 20000, CommissionRate: 10},
	})

	require.NoError(t, err)
	// before=8000, after=23000
	// P(23000) = 10000*5% + 10000*8% + 3000*10% = 500 + 800 + 300 = 1600
	// P(8000)  = 8000*5% = 400
	// commission = 1200, rate = 1200/15000 = 8%
	require.InDelta(t, 1200, calc.CommissionTotalCNY, 0.000001)
	require.InDelta(t, 8, calc.CommissionRate, 0.0001)
}

func TestNormalizeSalesCommissionTiers_RejectsTiersAfterOpenEndedRange(t *testing.T) {
	_, err := NormalizeSalesCommissionTiers([]SalesCommissionTier{
		{MonthSalesFromCNY: 0, CommissionRate: 10},
		{MonthSalesFromCNY: 500, CommissionRate: 20},
	})

	require.ErrorContains(t, err, "open-ended")
}

// TestRecomputeMonthlyCommissionRecords_FixedModeRegression 验证 spec §10 项 1：
// 固定比例模式下三笔订单顺序累进，commission 总额 = 单笔 * fixedRate%，
// monthly_sales_before/after 累加正确，rate 等于 fixedRate。
func TestRecomputeMonthlyCommissionRecords_FixedModeRegression(t *testing.T) {
	results, err := RecomputeMonthlyCommissionRecords(
		[]SalesCommissionMonthlyRecordInput{
			{OrderPayAmountCNY: 100, OrderCreditedAmount: 100, CreditedUsedAmount: 0, HasPaymentOrder: true},
			{OrderPayAmountCNY: 50, OrderCreditedAmount: 50, CreditedUsedAmount: 25, HasPaymentOrder: true},
			{OrderPayAmountCNY: 200, OrderCreditedAmount: 200, CreditedUsedAmount: 200, HasPaymentOrder: true},
		},
		SalesCommissionSnapshot{
			CommissionMode:      SalesCommissionModeFixed,
			FixedCommissionRate: 10,
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 3)
	require.InDelta(t, 0, results[0].MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 100, results[0].MonthlySalesAfterCNY, 0.000001)
	require.InDelta(t, 10, results[0].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 10, results[0].CommissionRate, 0.0001)
	require.InDelta(t, 0, results[0].UnlockedCNY, 0.000001) // CreditedUsed=0

	require.InDelta(t, 100, results[1].MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 150, results[1].MonthlySalesAfterCNY, 0.000001)
	require.InDelta(t, 5, results[1].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 10, results[1].CommissionRate, 0.0001)
	require.InDelta(t, 2.5, results[1].UnlockedCNY, 0.000001) // 25/50 * 5

	require.InDelta(t, 150, results[2].MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 350, results[2].MonthlySalesAfterCNY, 0.000001)
	require.InDelta(t, 20, results[2].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 10, results[2].CommissionRate, 0.0001)
	require.InDelta(t, 20, results[2].UnlockedCNY, 0.000001) // CreditedUsed>=Credited → 全额
}

// TestRecomputeMonthlyCommissionRecords_TieredBelowThresholdAllZero 验证 spec §10 项 2：
// 梯度模式下整月累计低于门槛，所有记录佣金为 0、unlocked 为 0，但 monthly_sales_before/after 仍然写入。
func TestRecomputeMonthlyCommissionRecords_TieredBelowThresholdAllZero(t *testing.T) {
	results, err := RecomputeMonthlyCommissionRecords(
		[]SalesCommissionMonthlyRecordInput{
			{OrderPayAmountCNY: 40, OrderCreditedAmount: 40, CreditedUsedAmount: 20, HasPaymentOrder: true},
			{OrderPayAmountCNY: 30, OrderCreditedAmount: 30, CreditedUsedAmount: 30, HasPaymentOrder: true},
		},
		SalesCommissionSnapshot{
			CommissionMode:     SalesCommissionModeTiered,
			MinMonthlySalesCNY: 100,
			Tiers: []SalesCommissionTier{
				{MonthSalesFromCNY: 0, CommissionRate: 10},
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 2)
	// 月总额 70 < 100，所有 commission 都是 0
	require.InDelta(t, 0, results[0].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 0, results[0].UnlockedCNY, 0.000001)
	require.InDelta(t, 0, results[0].MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 40, results[0].MonthlySalesAfterCNY, 0.000001)

	require.InDelta(t, 0, results[1].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 0, results[1].UnlockedCNY, 0.000001)
	require.InDelta(t, 40, results[1].MonthlySalesBeforeCNY, 0.000001)
	require.InDelta(t, 70, results[1].MonthlySalesAfterCNY, 0.000001)
}

// TestRecomputeMonthlyCommissionRecords_TieredBackfillsPriorRecordsOnCrossing 验证 spec §10 项 3：
// 跨门槛后前序记录的 commission_total_cny 被补算成非零，且按 P(after)-P(before) 差分。
// 用 spec §6.5.2 示例：单档 5%/8%/10%，threshold=10000，三笔 9000 / 3000 / 13000。
func TestRecomputeMonthlyCommissionRecords_TieredBackfillsPriorRecordsOnCrossing(t *testing.T) {
	results, err := RecomputeMonthlyCommissionRecords(
		[]SalesCommissionMonthlyRecordInput{
			{OrderPayAmountCNY: 9000, OrderCreditedAmount: 9000, CreditedUsedAmount: 0, HasPaymentOrder: true},
			{OrderPayAmountCNY: 3000, OrderCreditedAmount: 3000, CreditedUsedAmount: 0, HasPaymentOrder: true},
			{OrderPayAmountCNY: 13000, OrderCreditedAmount: 13000, CreditedUsedAmount: 0, HasPaymentOrder: true},
		},
		SalesCommissionSnapshot{
			CommissionMode:     SalesCommissionModeTiered,
			MinMonthlySalesCNY: 10000,
			Tiers: []SalesCommissionTier{
				{MonthSalesFromCNY: 0, MonthSalesToCNY: float64Ptr(10000), CommissionRate: 5},
				{MonthSalesFromCNY: 10000, MonthSalesToCNY: float64Ptr(20000), CommissionRate: 8},
				{MonthSalesFromCNY: 20000, CommissionRate: 10},
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 3)
	// 第 1 笔补算：P(9000)-P(0) = 450 - 0 = 450
	require.InDelta(t, 450, results[0].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 5, results[0].CommissionRate, 0.0001)
	// 第 2 笔跨门槛：P(12000)-P(9000) = (500+160) - 450 = 210
	require.InDelta(t, 210, results[1].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 7, results[1].CommissionRate, 0.0001) // 210/3000=7%
	// 第 3 笔继续累进：P(25000)-P(12000) = (500+800+500) - 660 = 1140
	require.InDelta(t, 1140, results[2].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 8.7692, results[2].CommissionRate, 0.001) // 1140/13000=8.7692%
}

// TestRecomputeMonthlyCommissionRecords_BackfillsUnlockedForPartiallyConsumed 验证 spec §10 项 6：
// 月初订单当时未达门槛 (commission_total=0)，下线已先消耗部分普通余额 (credited_used > 0)，
// 跨门槛后整月重算应当让该订单 commission_total 变成正值，且 unlocked 按已用比例同步补算。
func TestRecomputeMonthlyCommissionRecords_BackfillsUnlockedForPartiallyConsumed(t *testing.T) {
	results, err := RecomputeMonthlyCommissionRecords(
		[]SalesCommissionMonthlyRecordInput{
			// 月初 60 元订单，下线已用掉对应到账额度的 30 元（半数）
			{OrderPayAmountCNY: 60, OrderCreditedAmount: 60, CreditedUsedAmount: 30, HasPaymentOrder: true},
			// 月后 90 元订单跨过门槛 100
			{OrderPayAmountCNY: 90, OrderCreditedAmount: 90, CreditedUsedAmount: 0, HasPaymentOrder: true},
		},
		SalesCommissionSnapshot{
			CommissionMode:     SalesCommissionModeTiered,
			MinMonthlySalesCNY: 100,
			Tiers: []SalesCommissionTier{
				{MonthSalesFromCNY: 0, CommissionRate: 10},
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 2)
	// 第 1 笔补算到 P(60)-P(0)=6，unlocked = 30/60 * 6 = 3
	require.InDelta(t, 6, results[0].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 3, results[0].UnlockedCNY, 0.000001)
	// 第 2 笔 P(150)-P(60)=9，unlocked=0（尚未消费）
	require.InDelta(t, 9, results[1].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 0, results[1].UnlockedCNY, 0.000001)
}

// TestRecomputeMonthlyCommissionRecords_ManualCompletionParticipatesInMonthlyAccumulation 验证 spec §10 项 7：
// 手动完成推广产生的销售佣金记录（无 payment_order_id, credited_used_amount=order_credited_amount）
// 也要纳入当月累计销售额，跨门槛后立即变成全额解锁。
func TestRecomputeMonthlyCommissionRecords_ManualCompletionParticipatesInMonthlyAccumulation(t *testing.T) {
	results, err := RecomputeMonthlyCommissionRecords(
		[]SalesCommissionMonthlyRecordInput{
			// 手动完成 80：CreditedUsedAmount = OrderCreditedAmount，HasPaymentOrder=false
			{OrderPayAmountCNY: 80, OrderCreditedAmount: 80, CreditedUsedAmount: 80, HasPaymentOrder: false},
			// 普通订单 50 → 月总额 130 > 100 跨门槛
			{OrderPayAmountCNY: 50, OrderCreditedAmount: 50, CreditedUsedAmount: 0, HasPaymentOrder: true},
		},
		SalesCommissionSnapshot{
			CommissionMode:     SalesCommissionModeTiered,
			MinMonthlySalesCNY: 100,
			Tiers: []SalesCommissionTier{
				{MonthSalesFromCNY: 0, CommissionRate: 10},
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 2)
	// 手动完成那条：P(80)-P(0)=8，CreditedUsed>=Credited 立即全额解锁
	require.InDelta(t, 8, results[0].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 8, results[0].UnlockedCNY, 0.000001)
	// 普通订单：P(130)-P(80)=5，未消费 unlocked=0
	require.InDelta(t, 5, results[1].CommissionTotalCNY, 0.000001)
	require.InDelta(t, 0, results[1].UnlockedCNY, 0.000001)
}

// TestRecomputeMonthlyCommissionRecords_EmptyAndInvalidInputs 验证基础健壮性。
func TestRecomputeMonthlyCommissionRecords_EmptyAndInvalidInputs(t *testing.T) {
	// 空输入 → 空结果
	results, err := RecomputeMonthlyCommissionRecords(nil, SalesCommissionSnapshot{
		CommissionMode: SalesCommissionModeFixed, FixedCommissionRate: 10,
	})
	require.NoError(t, err)
	require.Empty(t, results)

	// tiered 模式但没有 tiers → 报错
	_, err = RecomputeMonthlyCommissionRecords(
		[]SalesCommissionMonthlyRecordInput{{OrderPayAmountCNY: 10, OrderCreditedAmount: 10}},
		SalesCommissionSnapshot{CommissionMode: SalesCommissionModeTiered},
	)
	require.Error(t, err)
}

// TestCalculateMonthlyCommissionCurve_TieredMatchesSpecExamples 验证 spec §6.5.2 给出的具体数字。
func TestCalculateMonthlyCommissionCurve_TieredMatchesSpecExamples(t *testing.T) {
	snapshot := SalesCommissionSnapshot{
		CommissionMode: SalesCommissionModeTiered,
		Tiers: []SalesCommissionTier{
			{MonthSalesFromCNY: 0, MonthSalesToCNY: float64Ptr(10000), CommissionRate: 5},
			{MonthSalesFromCNY: 10000, MonthSalesToCNY: float64Ptr(20000), CommissionRate: 8},
			{MonthSalesFromCNY: 20000, CommissionRate: 10},
		},
	}

	for _, tc := range []struct {
		sales    float64
		expected float64
	}{
		{0, 0},
		{9000, 450},
		{12000, 660},
		{25000, 1800},
	} {
		got, err := CalculateMonthlyCommissionCurve(tc.sales, snapshot)
		require.NoError(t, err)
		require.InDelta(t, tc.expected, got, 0.000001, "P(%v) should equal %v but got %v", tc.sales, tc.expected, got)
	}
}

func TestSalesCommissionService_HandleBalanceRechargeCompleted_IgnoresIneligibleOrders(t *testing.T) {
	tests := []struct {
		name  string
		order *dbent.PaymentOrder
	}{
		{
			name: "nil order",
		},
		{
			name: "subscription order",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeSubscription,
				Status:    payment.OrderStatusCompleted,
				PayAmount: 100,
				Amount:    100,
			},
		},
		{
			name: "not completed",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeBalance,
				Status:    payment.OrderStatusPaid,
				PayAmount: 100,
				Amount:    100,
			},
		},
		{
			name: "zero pay amount",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeBalance,
				Status:    payment.OrderStatusCompleted,
				PayAmount: 0,
				Amount:    100,
			},
		},
		{
			name: "zero credited amount",
			order: &dbent.PaymentOrder{
				OrderType: payment.OrderTypeBalance,
				Status:    payment.OrderStatusCompleted,
				PayAmount: 100,
				Amount:    0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &salesCommissionRepoStub{}
			refRepo := &salesCommissionReferralRepoStub{}
			userRepo := &salesCommissionUserRepoStub{}
			svc := NewSalesCommissionService(repo, refRepo, userRepo)

			err := svc.HandleBalanceRechargeCompleted(context.Background(), tt.order)

			require.NoError(t, err)
			require.Empty(t, repo.created)
			require.Zero(t, refRepo.getByRefereeCalls)
		})
	}
}

func TestSalesCommissionService_HandleBalanceRechargeCompleted_IgnoresMissingOrNonSalesReferrer(t *testing.T) {
	tests := []struct {
		name      string
		ref       *Referral
		refErr    error
		referrer  *User
		userErr   error
		wantError error
	}{
		{name: "no referral"},
		{name: "referral lookup error", refErr: errors.New("referral store down"), wantError: errors.New("referral store down")},
		{name: "referrer lookup error", ref: &Referral{ID: 1, ReferrerID: 10, RefereeID: 20}, userErr: errors.New("user store down"), wantError: errors.New("user store down")},
		{name: "not sales", ref: &Referral{ID: 1, ReferrerID: 10, RefereeID: 20}, referrer: &User{ID: 10, IsSales: false, SalesCommissionRate: 10}},
		{name: "zero rate", ref: &Referral{ID: 1, ReferrerID: 10, RefereeID: 20}, referrer: &User{ID: 10, IsSales: true, SalesCommissionRate: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &salesCommissionRepoStub{}
			refRepo := &salesCommissionReferralRepoStub{
				byReferee: map[int64]*Referral{20: tt.ref},
				err:       tt.refErr,
			}
			userRepo := &salesCommissionUserRepoStub{
				byID: map[int64]*User{10: tt.referrer},
				err:  tt.userErr,
			}
			svc := NewSalesCommissionService(repo, refRepo, userRepo)

			err := svc.HandleBalanceRechargeCompleted(context.Background(), completedBalanceOrder())

			if tt.wantError != nil {
				require.ErrorContains(t, err, tt.wantError.Error())
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, repo.created)
		})
	}
}

func TestSalesCommissionService_CreateSettlement_RejectsInvalidAmount(t *testing.T) {
	svc := NewSalesCommissionService(&salesCommissionRepoStub{}, &salesCommissionReferralRepoStub{}, &salesCommissionUserRepoStub{})

	for _, input := range []*SalesCommissionSettlementCreate{
		nil,
		{SalesUserID: 10, AmountCNY: 0},
		{SalesUserID: 10, AmountCNY: -1},
	} {
		got, err := svc.CreateSettlement(context.Background(), input)

		require.Nil(t, got)
		require.ErrorIs(t, err, ErrSalesCommissionInvalidAmount)
	}
}

func TestSalesCommissionService_ForwardsReadOperationsToRepository(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{
		summaries:   []SalesCommissionSummary{{SalesUserID: 10}},
		records:     []SalesCommissionRecord{{ID: 1}},
		settlements: []SalesCommissionSettlement{{ID: 2}},
		summary:     &SalesCommissionSummary{SalesUserID: 10},
		settlement:  &SalesCommissionSettlement{ID: 3},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, &salesCommissionUserRepoStub{})

	summaries, total, err := svc.ListSummaries(ctx, SalesCommissionSummaryListParams{Page: 2, PageSize: 3})
	require.NoError(t, err)
	require.Equal(t, repo.summaries, summaries)
	require.Equal(t, 1, total)
	require.Equal(t, SalesCommissionSummaryListParams{Page: 2, PageSize: 3}, repo.lastSummaryParams)

	summary, err := svc.GetSummary(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, repo.summary, summary)

	records, total, err := svc.ListRecords(ctx, SalesCommissionRecordListParams{SalesUserID: 10, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, repo.records, records)
	require.Equal(t, 1, total)
	require.Equal(t, SalesCommissionRecordListParams{SalesUserID: 10, Page: 1, PageSize: 20}, repo.lastRecordParams)

	settlement, err := svc.CreateSettlement(ctx, &SalesCommissionSettlementCreate{SalesUserID: 10, AmountCNY: 5})
	require.NoError(t, err)
	require.Equal(t, repo.settlement, settlement)

	settlements, total, err := svc.ListSettlements(ctx, SalesCommissionSettlementListParams{SalesUserID: 10, Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, repo.settlements, settlements)
	require.Equal(t, 1, total)
	require.Equal(t, SalesCommissionSettlementListParams{SalesUserID: 10, Page: 1, PageSize: 20}, repo.lastSettlementParams)
}

func TestSalesCommissionService_GetMonthlyProgress_ReturnsNilForNonSalesUser(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: false},
		},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, userRepo)

	progress, err := svc.GetMonthlyProgress(ctx, 10)
	require.NoError(t, err)
	require.Nil(t, progress)

	// 用户不存在也应安全返回 nil（service 兜底而非 panic）。
	progress, err = svc.GetMonthlyProgress(ctx, 99)
	require.NoError(t, err)
	require.Nil(t, progress)
}

func TestSalesCommissionService_GetMonthlyProgress_FixedModeWithFrozenSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := &salesCommissionRepoStub{
		monthlyProgress: &SalesCommissionMonthlyProgressData{
			Snapshot: &SalesCommissionSnapshot{
				CommissionMode:      SalesCommissionModeFixed,
				FixedCommissionRate: 10,
			},
			MonthlySalesCNY:      150,
			MonthlyCommissionCNY: 15,
		},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {ID: 10, IsSales: true, SalesCommissionMode: SalesCommissionModeFixed, SalesCommissionRate: 8},
		},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, userRepo)

	progress, err := svc.GetMonthlyProgress(ctx, 10)
	require.NoError(t, err)
	require.NotNil(t, progress)
	require.Equal(t, int64(10), progress.SalesUserID)
	require.True(t, progress.SnapshotFrozen)
	require.Equal(t, SalesCommissionModeFixed, progress.CommissionMode)
	require.InDelta(t, 10.0, progress.FixedCommissionRate, 0.0001)
	require.InDelta(t, 150.0, progress.MonthlySalesCNY, 0.0001)
	require.InDelta(t, 15.0, progress.MonthlyCommissionCNY, 0.0001)
	// fixed 模式不暴露梯度档位。
	require.Equal(t, -1, progress.CurrentTierIndex)
	require.Equal(t, -1, progress.NextTierIndex)
	require.Empty(t, progress.Tiers)
	// 仓储被传入正确的 (userID, monthStart)。
	require.Equal(t, int64(10), repo.lastMonthlyProgressUserID)
	require.Equal(t, salesCommissionMonthStartForTest(time.Now()), repo.lastMonthlyProgressMonth)
}

func TestSalesCommissionService_GetMonthlyProgress_TieredFallsBackToUserRulesWhenSnapshotMissing(t *testing.T) {
	ctx := context.Background()
	to := 200.0
	tiers := []SalesCommissionTier{
		{MonthSalesFromCNY: 0, MonthSalesToCNY: &to, CommissionRate: 5, SortOrder: 1},
		{MonthSalesFromCNY: 200, CommissionRate: 10, SortOrder: 2},
	}
	repo := &salesCommissionRepoStub{
		monthlyProgress: &SalesCommissionMonthlyProgressData{},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{
			10: {
				ID:                             10,
				IsSales:                        true,
				SalesCommissionMode:            SalesCommissionModeTiered,
				SalesCommissionRate:            7, // 应被忽略
				SalesCommissionMinMonthlySales: 100,
				SalesCommissionTiers:           tiers,
			},
		},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, userRepo)

	progress, err := svc.GetMonthlyProgress(ctx, 10)
	require.NoError(t, err)
	require.NotNil(t, progress)
	require.False(t, progress.SnapshotFrozen)
	require.Equal(t, SalesCommissionModeTiered, progress.CommissionMode)
	require.InDelta(t, 0.0, progress.FixedCommissionRate, 0.0001) // tiered 强制 0
	require.InDelta(t, 100.0, progress.MinMonthlySalesCNY, 0.0001)
	require.Len(t, progress.Tiers, 2)
	require.False(t, progress.ThresholdMet)
	require.InDelta(t, 100.0, progress.ToThresholdCNY, 0.0001)
	// 销售额 0 + tier[0].from=0 → 已经命中第 1 档；下一档为索引 1。
	require.Equal(t, 0, progress.CurrentTierIndex)
	require.Equal(t, 1, progress.NextTierIndex)
	require.InDelta(t, 10.0, progress.NextTierRate, 0.0001)
	require.InDelta(t, 200.0, progress.ToNextTierCNY, 0.0001) // tier[1].from=200
}

func TestSalesCommissionService_GetMonthlyProgress_TieredCrossesFirstTier(t *testing.T) {
	ctx := context.Background()
	to1 := 200.0
	to2 := 500.0
	tiers := []SalesCommissionTier{
		{MonthSalesFromCNY: 0, MonthSalesToCNY: &to1, CommissionRate: 5, SortOrder: 1},
		{MonthSalesFromCNY: 200, MonthSalesToCNY: &to2, CommissionRate: 8, SortOrder: 2},
		{MonthSalesFromCNY: 500, CommissionRate: 12, SortOrder: 3},
	}
	repo := &salesCommissionRepoStub{
		monthlyProgress: &SalesCommissionMonthlyProgressData{
			Snapshot: &SalesCommissionSnapshot{
				CommissionMode:     SalesCommissionModeTiered,
				MinMonthlySalesCNY: 100,
				Tiers:              tiers,
			},
			MonthlySalesCNY:      150,
			MonthlyCommissionCNY: 7.5,
		},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{10: {ID: 10, IsSales: true}},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, userRepo)

	progress, err := svc.GetMonthlyProgress(ctx, 10)
	require.NoError(t, err)
	require.NotNil(t, progress)
	require.True(t, progress.SnapshotFrozen)
	require.True(t, progress.ThresholdMet)
	require.InDelta(t, 0.0, progress.ToThresholdCNY, 0.0001)
	// 销售额 150 命中第 1 档 [0, 200)，下一档为 [200, 500)。
	require.Equal(t, 0, progress.CurrentTierIndex)
	require.Equal(t, 1, progress.NextTierIndex)
	require.InDelta(t, 8.0, progress.NextTierRate, 0.0001)
	require.InDelta(t, 50.0, progress.ToNextTierCNY, 0.0001) // 200 - 150
}

func TestSalesCommissionService_GetMonthlyProgress_TieredAtTopOpenTierHasNoNext(t *testing.T) {
	ctx := context.Background()
	to1 := 200.0
	tiers := []SalesCommissionTier{
		{MonthSalesFromCNY: 0, MonthSalesToCNY: &to1, CommissionRate: 5, SortOrder: 1},
		{MonthSalesFromCNY: 200, CommissionRate: 10, SortOrder: 2}, // 开口最高档
	}
	repo := &salesCommissionRepoStub{
		monthlyProgress: &SalesCommissionMonthlyProgressData{
			Snapshot: &SalesCommissionSnapshot{
				CommissionMode: SalesCommissionModeTiered,
				Tiers:          tiers,
			},
			MonthlySalesCNY:      300,
			MonthlyCommissionCNY: 20,
		},
	}
	userRepo := &salesCommissionUserRepoStub{
		byID: map[int64]*User{10: {ID: 10, IsSales: true}},
	}
	svc := NewSalesCommissionService(repo, &salesCommissionReferralRepoStub{}, userRepo)

	progress, err := svc.GetMonthlyProgress(ctx, 10)
	require.NoError(t, err)
	require.NotNil(t, progress)
	require.Equal(t, 1, progress.CurrentTierIndex)
	require.Equal(t, -1, progress.NextTierIndex) // 已在最高开口档
	require.InDelta(t, 0.0, progress.NextTierRate, 0.0001)
	require.InDelta(t, 0.0, progress.ToNextTierCNY, 0.0001)
}

func completedBalanceOrder() *dbent.PaymentOrder {
	return &dbent.PaymentOrder{
		ID:          99,
		UserID:      20,
		OrderType:   payment.OrderTypeBalance,
		Status:      payment.OrderStatusCompleted,
		PayAmount:   100,
		Amount:      100,
		CompletedAt: timePtr(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)),
	}
}

func salesCommissionMonthStartForTest(eventAt time.Time) time.Time {
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := eventAt.In(shanghai)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

type salesCommissionRepoStub struct {
	created                   []*SalesCommissionCreate
	summaries                 []SalesCommissionSummary
	summary                   *SalesCommissionSummary
	records                   []SalesCommissionRecord
	settlement                *SalesCommissionSettlement
	settlements               []SalesCommissionSettlement
	monthlyProgress           *SalesCommissionMonthlyProgressData
	monthlyProgressErr        error
	lastSummaryParams         SalesCommissionSummaryListParams
	lastRecordParams          SalesCommissionRecordListParams
	lastSettlementParams      SalesCommissionSettlementListParams
	lastMonthlyProgressUserID int64
	lastMonthlyProgressMonth  time.Time
	createSettlementInput     *SalesCommissionSettlementCreate
}

func (r *salesCommissionRepoStub) CreateForOrder(_ context.Context, input *SalesCommissionCreate) error {
	r.created = append(r.created, input)
	return nil
}

func (r *salesCommissionRepoStub) ListSummaries(_ context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error) {
	r.lastSummaryParams = params
	return r.summaries, len(r.summaries), nil
}

func (r *salesCommissionRepoStub) GetSummaryBySalesUser(_ context.Context, salesUserID int64) (*SalesCommissionSummary, error) {
	if r.summary != nil {
		return r.summary, nil
	}
	return &SalesCommissionSummary{SalesUserID: salesUserID}, nil
}

func (r *salesCommissionRepoStub) ListRecords(_ context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error) {
	r.lastRecordParams = params
	return r.records, len(r.records), nil
}

func (r *salesCommissionRepoStub) CreateSettlement(_ context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error) {
	r.createSettlementInput = input
	return r.settlement, nil
}

func (r *salesCommissionRepoStub) ListSettlements(_ context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error) {
	r.lastSettlementParams = params
	return r.settlements, len(r.settlements), nil
}

func (r *salesCommissionRepoStub) GetMonthlyProgress(_ context.Context, salesUserID int64, commissionMonth time.Time) (*SalesCommissionMonthlyProgressData, error) {
	r.lastMonthlyProgressUserID = salesUserID
	r.lastMonthlyProgressMonth = commissionMonth
	if r.monthlyProgressErr != nil {
		return nil, r.monthlyProgressErr
	}
	return r.monthlyProgress, nil
}

type salesCommissionReferralRepoStub struct {
	byReferee         map[int64]*Referral
	err               error
	getByRefereeCalls int
}

func (r *salesCommissionReferralRepoStub) Create(context.Context, *Referral) error { return nil }

func (r *salesCommissionReferralRepoStub) GetByID(_ context.Context, _ int64) (*Referral, error) {
	return nil, r.err
}

func (r *salesCommissionReferralRepoStub) GetByRefereeID(_ context.Context, refereeID int64) (*Referral, error) {
	r.getByRefereeCalls++
	if r.err != nil {
		return nil, r.err
	}
	return r.byReferee[refereeID], nil
}

func (r *salesCommissionReferralRepoStub) GetByReferrerID(context.Context, int64, int, int) ([]Referral, int, error) {
	return nil, 0, nil
}

func (r *salesCommissionReferralRepoStub) CountByReferrerID(context.Context, int64) (int, error) {
	return 0, nil
}

func (r *salesCommissionReferralRepoStub) UpdateStatus(context.Context, int64, string) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) MarkCompleted(context.Context, int64, float64, string) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) SetInviteeRewarded(context.Context, int64) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) SetInviterRewarded(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) IncrementOngoingReward(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionReferralRepoStub) GetStatsByReferrerID(context.Context, int64) (*ReferralStats, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) ListAll(context.Context, string, int, int) ([]Referral, int, error) {
	return nil, 0, nil
}

func (r *salesCommissionReferralRepoStub) GetLeaderboard(context.Context, string, int) ([]ReferralLeaderboardEntry, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) GetTrendByReferrerID(context.Context, int64, int) ([]ReferralTrendPoint, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) GetGlobalTrend(context.Context, int) ([]ReferralTrendPoint, error) {
	return nil, nil
}

func (r *salesCommissionReferralRepoStub) CountFirstRecharges(context.Context) (int, error) {
	return 0, nil
}

func (r *salesCommissionReferralRepoStub) CountAll(context.Context) (int, error) { return 0, nil }

type salesCommissionUserRepoStub struct {
	byID map[int64]*User
	err  error
}

func (r *salesCommissionUserRepoStub) Create(context.Context, *User) error { return nil }

func (r *salesCommissionUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byID[id], nil
}

func (r *salesCommissionUserRepoStub) GetByEmail(context.Context, string) (*User, error) {
	return nil, nil
}

func (r *salesCommissionUserRepoStub) GetFirstAdmin(context.Context) (*User, error) { return nil, nil }

func (r *salesCommissionUserRepoStub) Update(context.Context, *User) error { return nil }

func (r *salesCommissionUserRepoStub) Delete(context.Context, int64) error { return nil }

func (r *salesCommissionUserRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *salesCommissionUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *salesCommissionUserRepoStub) UpdateBalance(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) DeductBalance(context.Context, int64, float64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) UpdateConcurrency(context.Context, int64, int) error {
	return nil
}

func (r *salesCommissionUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}

func (r *salesCommissionUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}

func (r *salesCommissionUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error {
	return nil
}

func (r *salesCommissionUserRepoStub) EnableTotp(context.Context, int64) error { return nil }

func (r *salesCommissionUserRepoStub) DisableTotp(context.Context, int64) error { return nil }

func (r *salesCommissionUserRepoStub) GetByReferralCode(context.Context, string) (*User, error) {
	return nil, nil
}

func (r *salesCommissionUserRepoStub) UpdateReferralCode(context.Context, int64, string) error {
	return nil
}

func (r *salesCommissionUserRepoStub) UpdateReferredBy(context.Context, int64, int64) error {
	return nil
}

func (r *salesCommissionUserRepoStub) IsFirstRecharge(context.Context, int64) (bool, error) {
	return false, nil
}

func (r *salesCommissionUserRepoStub) ListActiveUserIDs(context.Context) ([]int64, error) {
	return nil, nil
}
