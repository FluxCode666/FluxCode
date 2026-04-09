//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type resetWindowsUserSubRepoStub struct {
	userSubRepoNoop

	resetDailyCalled bool
	resetDailyID     int64
}

func (r *resetWindowsUserSubRepoStub) ResetDailyUsage(context.Context, int64, time.Time) error {
	r.resetDailyCalled = true
	return nil
}

type resetWindowsGrantRepoStub struct {
	subscriptionGrantRepoNoop

	resetDailyCalled bool
	resetDailyID     int64
}

func (r *resetWindowsGrantRepoStub) ResetDailyUsageBySubscriptionID(_ context.Context, subscriptionID int64) error {
	r.resetDailyCalled = true
	r.resetDailyID = subscriptionID
	return nil
}

func TestSubscriptionService_CheckAndResetWindows_ResetsGrantUsage(t *testing.T) {
	now := time.Now()

	userRepo := &resetWindowsUserSubRepoStub{}
	grantRepo := &resetWindowsGrantRepoStub{}
	svc := NewSubscriptionService(groupRepoNoop{}, userRepo, grantRepo, nil, nil, nil)

	sub := &UserSubscription{
		ID:              123,
		UserID:          10,
		GroupID:         20,
		DailyWindowStart: func() *time.Time {
			t := now.Add(-25 * time.Hour)
			return &t
		}(),
		DailyUsageUSD: 99.9,
	}

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, userRepo.resetDailyCalled, "应调用 userSubRepo.ResetDailyUsage")
	require.True(t, grantRepo.resetDailyCalled, "应调用 grantRepo.ResetDailyUsageBySubscriptionID")
	require.Equal(t, sub.ID, grantRepo.resetDailyID)
	require.NotNil(t, sub.DailyWindowStart)
	require.Equal(t, startOfDay(now), *sub.DailyWindowStart)
	require.Equal(t, float64(0), sub.DailyUsageUSD)
}

