package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	SalesCommissionModeFixed  = "fixed"
	SalesCommissionModeTiered = "tiered"

	SalesCommissionStatusFrozen            = "frozen"
	SalesCommissionStatusPartialUnlocked   = "partial_unlocked"
	SalesCommissionStatusUnlocked          = "unlocked"
	SalesCommissionStatusSettled           = "settled"
	SalesCommissionStatusSettlementBlocked = "settlement_blocked"
)

var (
	ErrSalesCommissionSettleAmountExceeded = infraerrors.BadRequest("SALES_COMMISSION_SETTLE_AMOUNT_EXCEEDED", "settlement amount exceeds settleable commission")
	ErrSalesCommissionInvalidAmount        = infraerrors.BadRequest("SALES_COMMISSION_INVALID_AMOUNT", "amount must be greater than 0")
)

type SalesCommissionCreate struct {
	SalesUserID               int64
	RefereeUserID             int64
	ReferralID                int64
	PaymentOrderID            *int64
	OrderPayAmountCNY         float64
	OrderCreditedAmount       float64
	CommissionMode            string
	CommissionRate            float64
	CommissionTotalCNY        float64
	CommissionMinMonthlySales float64
	CommissionTiers           []SalesCommissionTier
	CommissionEventAt         time.Time
	CommissionMonth           time.Time
	Note                      string
}

type SalesCommissionTier struct {
	MonthSalesFromCNY float64  `json:"month_sales_from_cny"`
	MonthSalesToCNY   *float64 `json:"month_sales_to_cny,omitempty"`
	CommissionRate    float64  `json:"commission_rate"`
	SortOrder         int      `json:"sort_order"`
}

type SalesCommissionCalculation struct {
	CommissionRate        float64
	CommissionTotalCNY    float64
	MonthlySalesBeforeCNY float64
	MonthlySalesAfterCNY  float64
}

type SalesCommissionRecord struct {
	ID                    int64     `json:"id"`
	SalesUserID           int64     `json:"sales_user_id"`
	SalesEmail            string    `json:"sales_email"`
	SalesUsername         string    `json:"sales_username"`
	RefereeUserID         int64     `json:"referee_user_id"`
	RefereeEmail          string    `json:"referee_email"`
	RefereeUsername       string    `json:"referee_username"`
	ReferralID            int64     `json:"referral_id"`
	PaymentOrderID        *int64    `json:"payment_order_id,omitempty"`
	PaymentOrderStatus    string    `json:"payment_order_status,omitempty"`
	OrderPayAmountCNY     float64   `json:"order_pay_amount_cny"`
	OrderCreditedAmount   float64   `json:"order_credited_amount"`
	CommissionEventAt     time.Time `json:"commission_event_at"`
	CommissionMonth       time.Time `json:"commission_month"`
	SnapshotID            *int64    `json:"snapshot_id,omitempty"`
	CommissionMode        string    `json:"commission_mode"`
	MonthlySalesBeforeCNY float64   `json:"monthly_sales_before_cny"`
	MonthlySalesAfterCNY  float64   `json:"monthly_sales_after_cny"`
	CommissionRate        float64   `json:"commission_rate"`
	CommissionTotalCNY    float64   `json:"commission_total_cny"`
	CreditedUsedAmount    float64   `json:"credited_used_amount"`
	FrozenCNY             float64   `json:"frozen_cny"`
	UnlockedCNY           float64   `json:"unlocked_cny"`
	SettledCNY            float64   `json:"settled_cny"`
	SettleableCNY         float64   `json:"settleable_cny"`
	Status                string    `json:"status"`
	Note                  string    `json:"note"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type SalesCommissionSummary struct {
	SalesUserID        int64   `json:"sales_user_id"`
	SalesEmail         string  `json:"sales_email"`
	SalesUsername      string  `json:"sales_username"`
	TotalCommissionCNY float64 `json:"total_commission_cny"`
	FrozenCNY          float64 `json:"frozen_cny"`
	UnlockedCNY        float64 `json:"unlocked_cny"`
	SettleableCNY      float64 `json:"settleable_cny"`
	SettledCNY         float64 `json:"settled_cny"`
	RecordsCount       int     `json:"records_count"`
}

type SalesCommissionSettlement struct {
	ID          int64     `json:"id"`
	SalesUserID int64     `json:"sales_user_id"`
	SalesEmail  string    `json:"sales_email"`
	AmountCNY   float64   `json:"amount_cny"`
	Note        string    `json:"note"`
	CreatedBy   *int64    `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type SalesCommissionSettlementCreate struct {
	SalesUserID int64
	AmountCNY   float64
	Note        string
	CreatedBy   *int64
}

type SalesCommissionSummaryListParams struct {
	Search   string
	Page     int
	PageSize int
}

type SalesCommissionRecordListParams struct {
	SalesUserID    int64
	RefereeUserID  int64
	PaymentOrderID int64
	Status         string
	Page           int
	PageSize       int
}

type SalesCommissionSettlementListParams struct {
	SalesUserID int64
	Page        int
	PageSize    int
}

func NormalizeSalesCommissionMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		return SalesCommissionModeFixed
	}
	return normalized
}

func NormalizeSalesCommissionTiers(tiers []SalesCommissionTier) ([]SalesCommissionTier, error) {
	if len(tiers) == 0 {
		return nil, nil
	}

	normalized := append([]SalesCommissionTier(nil), tiers...)
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].MonthSalesFromCNY != normalized[j].MonthSalesFromCNY {
			return normalized[i].MonthSalesFromCNY < normalized[j].MonthSalesFromCNY
		}
		leftSort := normalized[i].SortOrder
		rightSort := normalized[j].SortOrder
		if leftSort == 0 {
			leftSort = i + 1
		}
		if rightSort == 0 {
			rightSort = j + 1
		}
		return leftSort < rightSort
	})

	var previousUpper *float64
	hasOpenEndedTier := false
	for i := range normalized {
		tier := &normalized[i]
		if hasOpenEndedTier {
			return nil, fmt.Errorf("tier %d cannot follow an open-ended tier", i+1)
		}
		if tier.MonthSalesFromCNY < 0 {
			return nil, fmt.Errorf("tier %d month_sales_from_cny must be greater than or equal to 0", i+1)
		}
		if tier.CommissionRate < 0 || tier.CommissionRate > 100 {
			return nil, fmt.Errorf("tier %d commission_rate must be between 0 and 100", i+1)
		}
		if tier.MonthSalesToCNY != nil {
			to := roundSalesCommissionMoney(*tier.MonthSalesToCNY)
			if to <= tier.MonthSalesFromCNY {
				return nil, fmt.Errorf("tier %d month_sales_to_cny must be greater than month_sales_from_cny", i+1)
			}
			tier.MonthSalesToCNY = &to
		}
		tier.MonthSalesFromCNY = roundSalesCommissionMoney(tier.MonthSalesFromCNY)
		if previousUpper != nil && tier.MonthSalesFromCNY < *previousUpper {
			return nil, fmt.Errorf("tier %d overlaps with previous tier", i+1)
		}
		tier.SortOrder = i + 1
		previousUpper = tier.MonthSalesToCNY
		hasOpenEndedTier = tier.MonthSalesToCNY == nil
	}
	return normalized, nil
}

func CalculateSalesCommission(orderPayAmountCNY float64, monthlySalesBeforeCNY float64, commissionMode string, fixedCommissionRate float64, minMonthlySalesCNY float64, tiers []SalesCommissionTier) (SalesCommissionCalculation, error) {
	if orderPayAmountCNY <= 0 {
		return SalesCommissionCalculation{}, fmt.Errorf("order pay amount must be greater than 0")
	}
	if monthlySalesBeforeCNY < 0 {
		return SalesCommissionCalculation{}, fmt.Errorf("monthly sales before must be greater than or equal to 0")
	}
	if minMonthlySalesCNY < 0 {
		return SalesCommissionCalculation{}, fmt.Errorf("min monthly sales must be greater than or equal to 0")
	}
	if fixedCommissionRate < 0 || fixedCommissionRate > 100 {
		return SalesCommissionCalculation{}, fmt.Errorf("fixed commission rate must be between 0 and 100")
	}

	mode := NormalizeSalesCommissionMode(commissionMode)
	normalizedTiers, err := NormalizeSalesCommissionTiers(tiers)
	if err != nil {
		return SalesCommissionCalculation{}, err
	}
	if mode != SalesCommissionModeFixed && mode != SalesCommissionModeTiered {
		return SalesCommissionCalculation{}, fmt.Errorf("unsupported sales commission mode: %s", commissionMode)
	}
	if mode == SalesCommissionModeTiered && len(normalizedTiers) == 0 {
		return SalesCommissionCalculation{}, fmt.Errorf("tiered sales commission requires at least one tier")
	}

	before := decimal.NewFromFloat(roundSalesCommissionMoney(monthlySalesBeforeCNY))
	orderAmount := decimal.NewFromFloat(roundSalesCommissionMoney(orderPayAmountCNY))
	after := before.Add(orderAmount).Round(2)
	minimum := decimal.NewFromFloat(roundSalesCommissionMoney(minMonthlySalesCNY))

	// spec §2 / §6.5：达到门槛后对整月销售额按梯度累进计提，单条记录的佣金为
	// P(after) - P(before)，其中 P(x) 是 "假设无门槛时 x 累计销售额对应的应得佣金总额"。
	// 未达门槛则总佣金为 0。注意：这里只算 "假定本笔已经是当月最后一笔事件" 时的应得，
	// 真正的整月累进补算由 RecomputeMonthlyCommissionRecords 在仓储事务中完成。
	total := decimal.Zero
	if after.GreaterThanOrEqual(minimum) && after.GreaterThan(before) {
		afterValue := decimalToFloat(after, 2)
		beforeValue := decimalToFloat(before, 2)
		curveAfter, err := monthlyCommissionCurve(afterValue, mode, fixedCommissionRate, normalizedTiers)
		if err != nil {
			return SalesCommissionCalculation{}, err
		}
		curveBefore, err := monthlyCommissionCurve(beforeValue, mode, fixedCommissionRate, normalizedTiers)
		if err != nil {
			return SalesCommissionCalculation{}, err
		}
		total = decimal.NewFromFloat(curveAfter).Sub(decimal.NewFromFloat(curveBefore))
		if total.IsNegative() {
			total = decimal.Zero
		}
	}

	total = total.Round(2)
	effectiveRate := decimal.Zero
	if orderAmount.IsPositive() && total.IsPositive() {
		effectiveRate = total.Div(orderAmount).Mul(decimal.NewFromInt(100)).Round(4)
	}

	return SalesCommissionCalculation{
		CommissionRate:        decimalToFloat(effectiveRate, 4),
		CommissionTotalCNY:    decimalToFloat(total, 2),
		MonthlySalesBeforeCNY: decimalToFloat(before, 2),
		MonthlySalesAfterCNY:  decimalToFloat(after, 2),
	}, nil
}

func SalesCommissionUserEligible(user *User) bool {
	if user == nil || !user.IsSales {
		return false
	}
	switch NormalizeSalesCommissionMode(user.SalesCommissionMode) {
	case SalesCommissionModeTiered:
		return len(user.SalesCommissionTiers) > 0
	default:
		return user.SalesCommissionRate > 0
	}
}

func CloneSalesCommissionTiers(tiers []SalesCommissionTier) []SalesCommissionTier {
	if len(tiers) == 0 {
		return nil
	}
	cloned := make([]SalesCommissionTier, 0, len(tiers))
	for _, tier := range tiers {
		item := tier
		if tier.MonthSalesToCNY != nil {
			to := *tier.MonthSalesToCNY
			item.MonthSalesToCNY = &to
		}
		cloned = append(cloned, item)
	}
	return cloned
}

func roundSalesCommissionMoney(amount float64) float64 {
	return decimalToFloat(decimal.NewFromFloat(amount).Round(2), 2)
}

func decimalToFloat(value decimal.Decimal, scale int32) float64 {
	v, _ := value.Round(scale).Float64()
	return v
}

// SalesCommissionSnapshot 描述某销售在某自然月内被冻结使用的提成规则快照。
// 对应 spec §5.3 sales_commission_monthly_snapshots 表的语义投影。
type SalesCommissionSnapshot struct {
	CommissionMode      string
	FixedCommissionRate float64
	MinMonthlySalesCNY  float64
	Tiers               []SalesCommissionTier
}

// SalesCommissionMonthlyRecordInput 是整月重算时单条记录的不可变输入。
// 调用方应按 (commission_event_at ASC, id ASC) 顺序排序后传入。
type SalesCommissionMonthlyRecordInput struct {
	OrderPayAmountCNY   float64
	OrderCreditedAmount float64
	CreditedUsedAmount  float64
	HasPaymentOrder     bool
}

// SalesCommissionMonthlyRecordResult 是整月重算后单条记录的派生结果，
// 供仓储层批量回写到 sales_commission_records 行。
type SalesCommissionMonthlyRecordResult struct {
	MonthlySalesBeforeCNY float64
	MonthlySalesAfterCNY  float64
	CommissionTotalCNY    float64
	CommissionRate        float64
	UnlockedCNY           float64
}

// CalculateMonthlyCommissionCurve 是 spec §6.5.2 所述 P(x) 函数：
// 在不考虑最低门槛的情况下，给定累计销售额 x（人民币），返回对应的应得佣金总额。
// 调用方在判定整月是否达到门槛后，再决定是否使用 P(x) 的差分作为单条记录的佣金。
func CalculateMonthlyCommissionCurve(monthlySalesCNY float64, snapshot SalesCommissionSnapshot) (float64, error) {
	if monthlySalesCNY < 0 {
		return 0, fmt.Errorf("monthly sales must be greater than or equal to 0")
	}
	mode := NormalizeSalesCommissionMode(snapshot.CommissionMode)
	if mode != SalesCommissionModeFixed && mode != SalesCommissionModeTiered {
		return 0, fmt.Errorf("unsupported sales commission mode: %s", snapshot.CommissionMode)
	}
	if snapshot.FixedCommissionRate < 0 || snapshot.FixedCommissionRate > 100 {
		return 0, fmt.Errorf("fixed commission rate must be between 0 and 100")
	}
	tiers, err := NormalizeSalesCommissionTiers(snapshot.Tiers)
	if err != nil {
		return 0, err
	}
	if mode == SalesCommissionModeTiered && len(tiers) == 0 {
		return 0, fmt.Errorf("tiered sales commission requires at least one tier")
	}
	return monthlyCommissionCurve(roundSalesCommissionMoney(monthlySalesCNY), mode, snapshot.FixedCommissionRate, tiers)
}

// monthlyCommissionCurve 内部 P(x) 实现，假设输入已规范化、模式合法。
func monthlyCommissionCurve(monthlySalesCNY float64, mode string, fixedRate float64, tiers []SalesCommissionTier) (float64, error) {
	if monthlySalesCNY <= 0 {
		return 0, nil
	}
	salesDec := decimal.NewFromFloat(monthlySalesCNY)
	switch mode {
	case SalesCommissionModeFixed:
		total := salesDec.Mul(decimal.NewFromFloat(fixedRate)).Div(decimal.NewFromInt(100))
		return decimalToFloat(total.Round(2), 2), nil
	case SalesCommissionModeTiered:
		total := decimal.Zero
		for _, tier := range tiers {
			lower := decimal.NewFromFloat(tier.MonthSalesFromCNY)
			if salesDec.LessThanOrEqual(lower) {
				break
			}
			upper := salesDec
			if tier.MonthSalesToCNY != nil {
				tierCap := decimal.NewFromFloat(*tier.MonthSalesToCNY)
				if tierCap.LessThan(upper) {
					upper = tierCap
				}
			}
			if upper.GreaterThan(lower) {
				total = total.Add(upper.Sub(lower).Mul(decimal.NewFromFloat(tier.CommissionRate)).Div(decimal.NewFromInt(100)))
			}
		}
		return decimalToFloat(total.Round(2), 2), nil
	default:
		return 0, fmt.Errorf("unsupported sales commission mode: %s", mode)
	}
}

// RecomputeMonthlyCommissionRecords 实现 spec §6.5 / §6.6 的 "整月按事件顺序累进重算" 算法。
//
// 输入 records 必须按 (commission_event_at ASC, id ASC) 顺序排好。
// 返回结果与输入一一对应，调用方负责把结果回写到 sales_commission_records 行。
//
// 算法：
//  1. 顺序累加 OrderPayAmountCNY，得到每条记录的 monthly_sales_before/after。
//  2. 若整月总额 < snapshot.MinMonthlySalesCNY → 全部记录佣金 0。
//  3. 否则按 P(after) - P(before) 计算每条记录佣金，超过两位精度按四舍五入处理。
//  4. unlocked_cny 按 credited_used_amount / order_credited_amount 比例同步重算；
//     credited_used_amount 已 >= order_credited_amount（含手动完成、已用满订单）时直接令 unlocked = commission_total。
func RecomputeMonthlyCommissionRecords(records []SalesCommissionMonthlyRecordInput, snapshot SalesCommissionSnapshot) ([]SalesCommissionMonthlyRecordResult, error) {
	mode := NormalizeSalesCommissionMode(snapshot.CommissionMode)
	if mode != SalesCommissionModeFixed && mode != SalesCommissionModeTiered {
		return nil, fmt.Errorf("unsupported sales commission mode: %s", snapshot.CommissionMode)
	}
	if snapshot.FixedCommissionRate < 0 || snapshot.FixedCommissionRate > 100 {
		return nil, fmt.Errorf("fixed commission rate must be between 0 and 100")
	}
	tiers, err := NormalizeSalesCommissionTiers(snapshot.Tiers)
	if err != nil {
		return nil, err
	}
	if mode == SalesCommissionModeTiered && len(tiers) == 0 {
		return nil, fmt.Errorf("tiered sales commission requires at least one tier")
	}

	results := make([]SalesCommissionMonthlyRecordResult, len(records))
	if len(records) == 0 {
		return results, nil
	}

	// 第一遍：累加 monthly_sales_before/after。
	cumulative := decimal.Zero
	beforeDecimals := make([]decimal.Decimal, len(records))
	afterDecimals := make([]decimal.Decimal, len(records))
	for i, r := range records {
		if r.OrderPayAmountCNY < 0 {
			return nil, fmt.Errorf("record %d order_pay_amount must be greater than or equal to 0", i)
		}
		order := decimal.NewFromFloat(roundSalesCommissionMoney(r.OrderPayAmountCNY))
		before := cumulative
		after := before.Add(order).Round(2)
		beforeDecimals[i] = before
		afterDecimals[i] = after
		results[i].MonthlySalesBeforeCNY = decimalToFloat(before, 2)
		results[i].MonthlySalesAfterCNY = decimalToFloat(after, 2)
		cumulative = after
	}

	// 第二遍：基于整月总额是否达门槛，按差分计提佣金；同步重算 unlocked。
	monthlyTotal := cumulative
	threshold := decimal.NewFromFloat(roundSalesCommissionMoney(snapshot.MinMonthlySalesCNY))
	belowThreshold := monthlyTotal.LessThan(threshold)

	for i, r := range records {
		commissionTotal := decimal.Zero
		if !belowThreshold {
			beforeValue := decimalToFloat(beforeDecimals[i], 2)
			afterValue := decimalToFloat(afterDecimals[i], 2)
			curveAfter, err := monthlyCommissionCurve(afterValue, mode, snapshot.FixedCommissionRate, tiers)
			if err != nil {
				return nil, err
			}
			curveBefore, err := monthlyCommissionCurve(beforeValue, mode, snapshot.FixedCommissionRate, tiers)
			if err != nil {
				return nil, err
			}
			commissionTotal = decimal.NewFromFloat(curveAfter).Sub(decimal.NewFromFloat(curveBefore))
			if commissionTotal.IsNegative() {
				commissionTotal = decimal.Zero
			}
		}
		commissionTotal = commissionTotal.Round(2)

		orderAmount := decimal.NewFromFloat(roundSalesCommissionMoney(r.OrderPayAmountCNY))
		rate := decimal.Zero
		if orderAmount.IsPositive() && commissionTotal.IsPositive() {
			rate = commissionTotal.Div(orderAmount).Mul(decimal.NewFromInt(100)).Round(4)
		}

		// spec §6.6：unlocked 同步重算。
		// credited_used_amount >= order_credited_amount 时（含手动完成、订单已用尽）unlocked = commission_total；
		// 否则按 used / credited 比例。commission_total 为 0 时 unlocked 也为 0。
		unlocked := decimal.Zero
		credited := decimal.NewFromFloat(r.OrderCreditedAmount)
		used := decimal.NewFromFloat(r.CreditedUsedAmount)
		if commissionTotal.IsPositive() && credited.IsPositive() {
			if used.GreaterThanOrEqual(credited) {
				unlocked = commissionTotal
			} else if used.IsPositive() {
				unlocked = used.Div(credited).Mul(commissionTotal).Round(2)
			}
		}

		results[i].CommissionTotalCNY = decimalToFloat(commissionTotal, 2)
		results[i].CommissionRate = decimalToFloat(rate, 4)
		results[i].UnlockedCNY = decimalToFloat(unlocked, 2)
	}

	return results, nil
}

// SalesCommissionMonthlyProgressData 是仓储层返回的当月生数据，
// 由 service 层基于 snapshot 与 user 当前规则做派生计算后再返回给前端。
//
//   - Snapshot 为 nil 表示当月还没产生任何销售返佣事件，因此还没冻结规则；
//     此时 service 层应该把 user 当前规则作为 "下笔订单进来时将使用的预期规则" 展示。
type SalesCommissionMonthlyProgressData struct {
	Snapshot             *SalesCommissionSnapshot
	MonthlySalesCNY      float64
	MonthlyCommissionCNY float64
}

// SalesCommissionMonthlyProgress 是销售用户端 GET /sales-commissions/monthly-progress
// 的响应：当月梯度进度的完整画像（spec §9）。
type SalesCommissionMonthlyProgress struct {
	SalesUserID          int64                 `json:"sales_user_id"`
	CommissionMonth      time.Time             `json:"commission_month"`
	CommissionMode       string                `json:"commission_mode"`
	FixedCommissionRate  float64               `json:"fixed_commission_rate"`
	MinMonthlySalesCNY   float64               `json:"min_monthly_sales_cny"`
	Tiers                []SalesCommissionTier `json:"tiers"`
	MonthlySalesCNY      float64               `json:"monthly_sales_cny"`
	MonthlyCommissionCNY float64               `json:"monthly_commission_cny"`
	ThresholdMet         bool                  `json:"threshold_met"`
	ToThresholdCNY       float64               `json:"to_threshold_cny"`
	CurrentTierIndex     int                   `json:"current_tier_index"`
	NextTierIndex        int                   `json:"next_tier_index"`
	ToNextTierCNY        float64               `json:"to_next_tier_cny"`
	NextTierRate         float64               `json:"next_tier_rate"`
	SnapshotFrozen       bool                  `json:"snapshot_frozen"`
}

type SalesCommissionRepository interface {
	CreateForOrder(ctx context.Context, input *SalesCommissionCreate) error
	ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error)
	GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error)
	ListRecords(ctx context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error)
	CreateSettlement(ctx context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error)
	ListSettlements(ctx context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error)
	GetMonthlyProgress(ctx context.Context, salesUserID int64, commissionMonth time.Time) (*SalesCommissionMonthlyProgressData, error)
}
