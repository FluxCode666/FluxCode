//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type listQuotaSnapshotUserSubRepoStub struct {
	userSubRepoNoop

	subs []UserSubscription
	err  error
}

func (s *listQuotaSnapshotUserSubRepoStub) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	return s.subs, s.err
}

type listQuotaSnapshotGrantRepoStub struct {
	subscriptionGrantRepoNoop

	count   int
	daily   float64
	weekly  float64
	monthly float64
}

func (s *listQuotaSnapshotGrantRepoStub) CountActiveBySubscriptionID(context.Context, int64, time.Time) (int, error) {
	return s.count, nil
}

func (s *listQuotaSnapshotGrantRepoStub) SumActiveUsageBySubscriptionID(context.Context, int64, time.Time) (float64, float64, float64, error) {
	return s.daily, s.weekly, s.monthly, nil
}

func TestSubscriptionService_ListUserSubscriptions_PopulatesQuotaSnapshot(t *testing.T) {
	userRepo := &listQuotaSnapshotUserSubRepoStub{
		subs: []UserSubscription{{
			ID:              1,
			UserID:          10,
			GroupID:         20,
			QuotaMultiplier: 0,
			DailyUsageUSD:   999,
			WeeklyUsageUSD:  888,
			MonthlyUsageUSD: 777,
		}},
	}
	grantRepo := &listQuotaSnapshotGrantRepoStub{
		count:   2,
		daily:   15,
		weekly:  16,
		monthly: 17,
	}

	svc := NewSubscriptionService(groupRepoNoop{}, userRepo, grantRepo, nil, nil, nil)

	subs, err := svc.ListUserSubscriptions(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, 2, subs[0].QuotaMultiplier)
	require.InDelta(t, 15, subs[0].DailyUsageUSD, 0.000001)
	require.InDelta(t, 16, subs[0].WeeklyUsageUSD, 0.000001)
	require.InDelta(t, 17, subs[0].MonthlyUsageUSD, 0.000001)
}
