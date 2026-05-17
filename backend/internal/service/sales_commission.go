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
	eligibleStart := decimal.Max(before, minimum)
	eligibleEnd := after

	total := decimal.Zero
	if eligibleEnd.GreaterThan(eligibleStart) {
		switch mode {
		case SalesCommissionModeFixed:
			total = eligibleEnd.Sub(eligibleStart).
				Mul(decimal.NewFromFloat(fixedCommissionRate)).
				Div(decimal.NewFromInt(100))
		case SalesCommissionModeTiered:
			for _, tier := range normalizedTiers {
				segmentStart := decimal.Max(eligibleStart, decimal.NewFromFloat(tier.MonthSalesFromCNY))
				segmentEnd := eligibleEnd
				if tier.MonthSalesToCNY != nil {
					segmentEnd = decimal.Min(segmentEnd, decimal.NewFromFloat(*tier.MonthSalesToCNY))
				}
				if !segmentEnd.GreaterThan(segmentStart) {
					continue
				}
				total = total.Add(
					segmentEnd.Sub(segmentStart).
						Mul(decimal.NewFromFloat(tier.CommissionRate)).
						Div(decimal.NewFromInt(100)),
				)
			}
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

type SalesCommissionRepository interface {
	CreateForOrder(ctx context.Context, input *SalesCommissionCreate) error
	ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error)
	GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error)
	ListRecords(ctx context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error)
	CreateSettlement(ctx context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error)
	ListSettlements(ctx context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error)
}
