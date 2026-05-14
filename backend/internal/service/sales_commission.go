package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
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
	SalesUserID         int64
	RefereeUserID       int64
	ReferralID          int64
	PaymentOrderID      *int64
	OrderPayAmountCNY   float64
	OrderCreditedAmount float64
	CommissionRate      float64
	CommissionTotalCNY  float64
	Note                string
}

type SalesCommissionRecord struct {
	ID                  int64     `json:"id"`
	SalesUserID         int64     `json:"sales_user_id"`
	SalesEmail          string    `json:"sales_email"`
	SalesUsername       string    `json:"sales_username"`
	RefereeUserID       int64     `json:"referee_user_id"`
	RefereeEmail        string    `json:"referee_email"`
	RefereeUsername     string    `json:"referee_username"`
	ReferralID          int64     `json:"referral_id"`
	PaymentOrderID      *int64    `json:"payment_order_id,omitempty"`
	PaymentOrderStatus  string    `json:"payment_order_status,omitempty"`
	OrderPayAmountCNY   float64   `json:"order_pay_amount_cny"`
	OrderCreditedAmount float64   `json:"order_credited_amount"`
	CommissionRate      float64   `json:"commission_rate"`
	CommissionTotalCNY  float64   `json:"commission_total_cny"`
	CreditedUsedAmount  float64   `json:"credited_used_amount"`
	FrozenCNY           float64   `json:"frozen_cny"`
	UnlockedCNY         float64   `json:"unlocked_cny"`
	SettledCNY          float64   `json:"settled_cny"`
	SettleableCNY       float64   `json:"settleable_cny"`
	Status              string    `json:"status"`
	Note                string    `json:"note"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
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

type SalesCommissionRepository interface {
	CreateForOrder(ctx context.Context, input *SalesCommissionCreate) error
	ListSummaries(ctx context.Context, params SalesCommissionSummaryListParams) ([]SalesCommissionSummary, int, error)
	GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*SalesCommissionSummary, error)
	ListRecords(ctx context.Context, params SalesCommissionRecordListParams) ([]SalesCommissionRecord, int, error)
	CreateSettlement(ctx context.Context, input *SalesCommissionSettlementCreate) (*SalesCommissionSettlement, error)
	ListSettlements(ctx context.Context, params SalesCommissionSettlementListParams) ([]SalesCommissionSettlement, int, error)
}
