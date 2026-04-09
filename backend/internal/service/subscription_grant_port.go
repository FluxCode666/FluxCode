package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// SubscriptionGrant represents one entitlement window (starts_at..expires_at)
// attached to a user_subscriptions row. Multiple grants may overlap to provide
// stacked quota limits.
type SubscriptionGrant struct {
	ID              int64
	SubscriptionID  int64
	StartsAt        time.Time
	ExpiresAt       time.Time
	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

var (
	ErrSubscriptionGrantNilInput = infraerrors.BadRequest("SUBSCRIPTION_GRANT_NIL_INPUT", "subscription grant input cannot be nil")
	ErrSubscriptionGrantNotFound = infraerrors.NotFound("SUBSCRIPTION_GRANT_NOT_FOUND", "subscription grant not found")
)

type SubscriptionGrantRepository interface {
	Create(ctx context.Context, grant *SubscriptionGrant) error

	CountActiveBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) (int, error)
	ListActiveBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) ([]*SubscriptionGrant, error)
	ListUnexpiredBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) ([]*SubscriptionGrant, error)
	SumActiveUsageBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) (dailyUSD, weeklyUSD, monthlyUSD float64, err error)
	MinActiveExpiresAtBySubscriptionID(ctx context.Context, subscriptionID int64, at time.Time) (*time.Time, error)
	ListActiveForUpdate(ctx context.Context, subscriptionID int64, at time.Time) ([]*SubscriptionGrant, error)
	GetTailGrantBySubscriptionID(ctx context.Context, subscriptionID int64) (*SubscriptionGrant, error)
	UpdateExpiresAt(ctx context.Context, grantID int64, newExpiresAt time.Time) error
	ResetDailyUsageBySubscriptionID(ctx context.Context, subscriptionID int64) error
	ResetWeeklyUsageBySubscriptionID(ctx context.Context, subscriptionID int64) error
	ResetMonthlyUsageBySubscriptionID(ctx context.Context, subscriptionID int64) error
	AllocateUsageToActiveGrants(ctx context.Context, subscriptionID int64, group *Group, at time.Time, costUSD float64) error
}
