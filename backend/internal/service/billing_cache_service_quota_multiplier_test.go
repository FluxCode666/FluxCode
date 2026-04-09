//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingCacheSubscriptionStub struct {
	sub *SubscriptionCacheData
	err error
}

func (b *billingCacheSubscriptionStub) GetUserBalance(context.Context, int64) (float64, error) {
	return 0, nil
}

func (b *billingCacheSubscriptionStub) SetUserBalance(context.Context, int64, float64) error {
	return nil
}

func (b *billingCacheSubscriptionStub) DeductUserBalance(context.Context, int64, float64) error {
	return nil
}

func (b *billingCacheSubscriptionStub) InvalidateUserBalance(context.Context, int64) error {
	return nil
}

func (b *billingCacheSubscriptionStub) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.sub, nil
}

func (b *billingCacheSubscriptionStub) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}

func (b *billingCacheSubscriptionStub) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	return nil
}

func (b *billingCacheSubscriptionStub) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return nil
}

func (b *billingCacheSubscriptionStub) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, nil
}

func (b *billingCacheSubscriptionStub) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}

func (b *billingCacheSubscriptionStub) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func (b *billingCacheSubscriptionStub) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
}

type billingCacheSubRepoStub struct {
	userSubRepoNoop
	sub *UserSubscription
	err error
}

func (s *billingCacheSubRepoStub) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sub, nil
}

type subscriptionGrantRepoNoop struct{}

func (subscriptionGrantRepoNoop) Create(context.Context, *SubscriptionGrant) error {
	panic("unexpected Create call")
}
func (subscriptionGrantRepoNoop) CountActiveBySubscriptionID(context.Context, int64, time.Time) (int, error) {
	panic("unexpected CountActiveBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) ListActiveBySubscriptionID(context.Context, int64, time.Time) ([]*SubscriptionGrant, error) {
	panic("unexpected ListActiveBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) ListUnexpiredBySubscriptionID(context.Context, int64, time.Time) ([]*SubscriptionGrant, error) {
	panic("unexpected ListUnexpiredBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) SumActiveUsageBySubscriptionID(context.Context, int64, time.Time) (float64, float64, float64, error) {
	panic("unexpected SumActiveUsageBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) MinActiveExpiresAtBySubscriptionID(context.Context, int64, time.Time) (*time.Time, error) {
	panic("unexpected MinActiveExpiresAtBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) ListActiveForUpdate(context.Context, int64, time.Time) ([]*SubscriptionGrant, error) {
	panic("unexpected ListActiveForUpdate call")
}
func (subscriptionGrantRepoNoop) GetTailGrantBySubscriptionID(context.Context, int64) (*SubscriptionGrant, error) {
	panic("unexpected GetTailGrantBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) UpdateExpiresAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateExpiresAt call")
}
func (subscriptionGrantRepoNoop) ResetDailyUsageBySubscriptionID(context.Context, int64) error {
	panic("unexpected ResetDailyUsageBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) ResetWeeklyUsageBySubscriptionID(context.Context, int64) error {
	panic("unexpected ResetWeeklyUsageBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) ResetMonthlyUsageBySubscriptionID(context.Context, int64) error {
	panic("unexpected ResetMonthlyUsageBySubscriptionID call")
}
func (subscriptionGrantRepoNoop) AllocateUsageToActiveGrants(context.Context, int64, *Group, time.Time, float64) error {
	panic("unexpected AllocateUsageToActiveGrants call")
}

type billingCacheGrantRepoStub struct {
	subscriptionGrantRepoNoop

	count      int
	daily      float64
	weekly     float64
	monthly    float64
	nextExpiry *time.Time
}

func (s *billingCacheGrantRepoStub) CountActiveBySubscriptionID(context.Context, int64, time.Time) (int, error) {
	return s.count, nil
}

func (s *billingCacheGrantRepoStub) SumActiveUsageBySubscriptionID(context.Context, int64, time.Time) (float64, float64, float64, error) {
	return s.daily, s.weekly, s.monthly, nil
}

func (s *billingCacheGrantRepoStub) MinActiveExpiresAtBySubscriptionID(context.Context, int64, time.Time) (*time.Time, error) {
	return s.nextExpiry, nil
}

func TestBillingCacheService_CheckBillingEligibility_SubscriptionQuotaMultiplier(t *testing.T) {
	now := time.Now()

	cache := &billingCacheSubscriptionStub{
		sub: &SubscriptionCacheData{
			Status:          SubscriptionStatusActive,
			ExpiresAt:       now.Add(24 * time.Hour),
			DailyUsage:      15,
			QuotaMultiplier: 2,
			Version:         1,
		},
	}
	svc := &BillingCacheService{
		cache: cache,
		cfg:   &config.Config{RunMode: config.RunModeStandard},
	}

	limit := 10.0
	group := &Group{
		ID:               1,
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &limit,
	}
	user := &User{ID: 1}
	subscription := &UserSubscription{
		ID:              1,
		UserID:          user.ID,
		GroupID:         group.ID,
		Status:          SubscriptionStatusActive,
		ExpiresAt:       now.Add(24 * time.Hour),
		QuotaMultiplier: 1,
	}

	err := svc.CheckBillingEligibility(context.Background(), user, nil, group, subscription)
	require.NoError(t, err)
}

func TestBillingCacheService_GetSubscriptionStatus_CacheMissLoadsGrantSnapshot(t *testing.T) {
	now := time.Now()
	nextExpiry := now.Add(2 * time.Hour).Round(time.Second)

	cache := &billingCacheSubscriptionStub{err: errors.New("cache miss")}
	subRepo := &billingCacheSubRepoStub{
		sub: &UserSubscription{
			ID:              91,
			UserID:          11,
			GroupID:         22,
			Status:          SubscriptionStatusActive,
			ExpiresAt:       now.Add(24 * time.Hour),
			DailyUsageUSD:   99,
			WeeklyUsageUSD:  88,
			MonthlyUsageUSD: 77,
			UpdatedAt:       now,
		},
	}
	grantRepo := &billingCacheGrantRepoStub{
		count:      2,
		daily:      15,
		weekly:     16,
		monthly:    17,
		nextExpiry: &nextExpiry,
	}

	svc := NewBillingCacheService(cache, nil, subRepo, grantRepo, nil, &config.Config{RunMode: config.RunModeStandard})
	t.Cleanup(svc.Stop)

	got, err := svc.GetSubscriptionStatus(context.Background(), 11, 22)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, got.Status)
	require.Equal(t, 2, got.QuotaMultiplier)
	require.InDelta(t, 15, got.DailyUsage, 0.000001)
	require.InDelta(t, 16, got.WeeklyUsage, 0.000001)
	require.InDelta(t, 17, got.MonthlyUsage, 0.000001)
	require.NotNil(t, got.NextGrantExpiresAt)
	require.True(t, got.NextGrantExpiresAt.Equal(nextExpiry))
}
