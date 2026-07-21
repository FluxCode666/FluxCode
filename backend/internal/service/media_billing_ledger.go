package service

import (
	"context"
	"errors"
	"time"
)

var (
	ErrMediaBillingOperationConflict = errors.New("media billing operation conflicts with the persisted operation")
	ErrMediaBillingPrechargeMissing  = errors.New("media billing precharge operation is missing")
	ErrMediaBillingFundingInvalid    = errors.New("media billing funding allocation is invalid")
)

type MediaBillingFundingSource string

const (
	MediaBillingFundingBalance      MediaBillingFundingSource = "balance"
	MediaBillingFundingSubscription MediaBillingFundingSource = "subscription"
	MediaBillingFundingFree         MediaBillingFundingSource = "free"
)

type MediaBillingGiftAllocation struct {
	RecordID int64   `json:"record_id"`
	Amount   float64 `json:"amount"`
}

type MediaBillingGrantAllocation struct {
	GrantID int64   `json:"grant_id"`
	Amount  float64 `json:"amount"`
}

// MediaBillingAllocation is the immutable funding breakdown written by the
// precharge transaction. Settlement uses it to refund the same source buckets.
type MediaBillingAllocation struct {
	FundingSource            MediaBillingFundingSource     `json:"funding_source"`
	SubscriptionID           *int64                        `json:"subscription_id,omitempty"`
	GiftBalances             []MediaBillingGiftAllocation  `json:"gift_balances,omitempty"`
	OrdinaryBalance          float64                       `json:"ordinary_balance,omitempty"`
	SubscriptionGrant        []MediaBillingGrantAllocation `json:"subscription_grants,omitempty"`
	SubscriptionDailyStart   *time.Time                    `json:"subscription_daily_start,omitempty"`
	SubscriptionWeeklyStart  *time.Time                    `json:"subscription_weekly_start,omitempty"`
	SubscriptionMonthlyStart *time.Time                    `json:"subscription_monthly_start,omitempty"`
	APIKeyQuotaEnabled       bool                          `json:"api_key_quota_enabled,omitempty"`
	APIKeyQuota              float64                       `json:"api_key_quota,omitempty"`
	APIKeyRateLimitEnabled   bool                          `json:"api_key_rate_limit_enabled,omitempty"`
	APIKeyRateLimit          float64                       `json:"api_key_rate_limit,omitempty"`
	APIKeyWindow5hStart      *time.Time                    `json:"api_key_window_5h_start,omitempty"`
	APIKeyWindow1dStart      *time.Time                    `json:"api_key_window_1d_start,omitempty"`
	APIKeyWindow7dStart      *time.Time                    `json:"api_key_window_7d_start,omitempty"`
	AccountQuota             float64                       `json:"account_quota,omitempty"`
}

type MediaBillingPrechargeCommand struct {
	IdempotencyKey     string
	RequestFingerprint string
	TaskID             int64
	TaskPublicID       string
	UserID             int64
	APIKeyID           int64
	GroupID            int64
	Amount             float64
	BilledAt           time.Time
}

type MediaBillingSettlementCommand struct {
	IdempotencyKey     string
	RequestFingerprint string
	Operation          MediaBillingOperation
	TaskID             int64
	TaskPublicID       string
	UserID             int64
	APIKeyID           int64
	GroupID            int64
	AccountID          *int64
	FinalAmount        float64
	AccountBaseAmount  float64
	BilledAt           time.Time
}

type MediaBillingLedgerResult struct {
	Applied                 bool
	PrechargedAmount        float64
	FinalAmount             float64
	RefundedAmount          float64
	AdditionalChargedAmount float64
	Allocation              MediaBillingAllocation
	APIKeyStatusChanged     bool
	AccountQuotaChanged     bool
}

// MediaBillingLedgerRepository owns the local ACID boundary for every balance,
// subscription, API-key and account-quota mutation made by media billing.
type MediaBillingLedgerRepository interface {
	Precharge(ctx context.Context, cmd MediaBillingPrechargeCommand) (MediaBillingLedgerResult, error)
	Settle(ctx context.Context, cmd MediaBillingSettlementCommand) (MediaBillingLedgerResult, error)
}
