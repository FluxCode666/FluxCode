//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SubscriptionGrantRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   service.SubscriptionGrantRepository
}

func TestSubscriptionGrantRepoSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionGrantRepoSuite))
}

func (s *SubscriptionGrantRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = NewSubscriptionGrantRepository(s.client)
}

func (s *SubscriptionGrantRepoSuite) createUser(email string) *dbent.User {
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		Save(s.ctx)
	s.Require().NoError(err)
	return u
}

func (s *SubscriptionGrantRepoSuite) createSubscription(name string) *dbent.UserSubscription {
	user := s.createUser(uniqueTestValue(s.T(), name) + "@example.com")
	group, err := s.client.Group.Create().
		SetName(uniqueTestValue(s.T(), name+"-group")).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(s.ctx)
	s.Require().NoError(err)

	sub, err := s.client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetStartsAt(time.Now().Add(-1 * time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		SetNotes("grant repo test").
		Save(s.ctx)
	s.Require().NoError(err)
	return sub
}

func (s *SubscriptionGrantRepoSuite) TestCreateAndListActiveBySubscriptionID() {
	sub := s.createSubscription("grant-active")
	now := time.Now().UTC()

	grant := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-30 * time.Minute),
		ExpiresAt:      now.Add(2 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, grant))
	s.Require().NotZero(grant.ID)

	active, err := s.repo.ListActiveBySubscriptionID(s.ctx, sub.ID, now)
	s.Require().NoError(err)
	s.Require().Len(active, 1)
	s.Require().Equal(grant.ID, active[0].ID)
	s.Require().Equal(sub.ID, active[0].SubscriptionID)
}

func (s *SubscriptionGrantRepoSuite) TestGetTailGrantBySubscriptionID() {
	sub := s.createSubscription("grant-tail")
	now := time.Now().UTC()

	first := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-2 * time.Hour),
		ExpiresAt:      now.Add(2 * time.Hour),
	}
	second := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-1 * time.Hour),
		ExpiresAt:      now.Add(6 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, first))
	s.Require().NoError(s.repo.Create(s.ctx, second))

	tail, err := s.repo.GetTailGrantBySubscriptionID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().Equal(second.ID, tail.ID)
	s.Require().WithinDuration(second.ExpiresAt, tail.ExpiresAt, time.Microsecond)
}

func (s *SubscriptionGrantRepoSuite) TestAllocateUsageToActiveGrants_EarliestExpiryFirst() {
	sub := s.createSubscription("grant-allocate")
	now := time.Now().UTC()

	earlier := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-2 * time.Hour),
		ExpiresAt:      now.Add(2 * time.Hour),
	}
	later := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-1 * time.Hour),
		ExpiresAt:      now.Add(8 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, earlier))
	s.Require().NoError(s.repo.Create(s.ctx, later))

	err := s.repo.AllocateUsageToActiveGrants(s.ctx, sub.ID, nil, now, 3.5)
	s.Require().NoError(err)

	active, err := s.repo.ListActiveBySubscriptionID(s.ctx, sub.ID, now)
	s.Require().NoError(err)
	s.Require().Len(active, 2)

	s.Require().Equal(earlier.ID, active[0].ID)
	s.Require().InDelta(3.5, active[0].DailyUsageUSD, 0.000001)
	s.Require().InDelta(3.5, active[0].WeeklyUsageUSD, 0.000001)
	s.Require().InDelta(3.5, active[0].MonthlyUsageUSD, 0.000001)

	s.Require().Equal(later.ID, active[1].ID)
	s.Require().InDelta(0, active[1].DailyUsageUSD, 0.000001)
	s.Require().InDelta(0, active[1].WeeklyUsageUSD, 0.000001)
	s.Require().InDelta(0, active[1].MonthlyUsageUSD, 0.000001)
}

func (s *SubscriptionGrantRepoSuite) TestCountAndSumActiveUsageBySubscriptionID() {
	sub := s.createSubscription("grant-sum")
	now := time.Now().UTC()

	first := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-3 * time.Hour),
		ExpiresAt:      now.Add(1 * time.Hour),
	}
	second := &service.SubscriptionGrant{
		SubscriptionID: sub.ID,
		StartsAt:       now.Add(-2 * time.Hour),
		ExpiresAt:      now.Add(5 * time.Hour),
	}
	s.Require().NoError(s.repo.Create(s.ctx, first))
	s.Require().NoError(s.repo.Create(s.ctx, second))
	s.Require().NoError(s.repo.AllocateUsageToActiveGrants(s.ctx, sub.ID, nil, now, 2))

	count, err := s.repo.CountActiveBySubscriptionID(s.ctx, sub.ID, now)
	s.Require().NoError(err)
	s.Require().Equal(2, count)

	daily, weekly, monthly, err := s.repo.SumActiveUsageBySubscriptionID(s.ctx, sub.ID, now)
	s.Require().NoError(err)
	s.Require().InDelta(2, daily, 0.000001)
	s.Require().InDelta(2, weekly, 0.000001)
	s.Require().InDelta(2, monthly, 0.000001)
}

func TestRedeemCodeRepository_UseRecordsSubscriptionMode(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := NewRedeemCodeRepository(client)

	user, err := client.User.Create().
		SetEmail(uniqueTestValue(t, "redeem-mode") + "@example.com").
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName(uniqueTestValue(t, "redeem-mode-group")).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	code := &service.RedeemCode{
		Code:         "RMODE-CODE",
		Type:         service.RedeemTypeSubscription,
		Value:        0,
		Status:       service.StatusUnused,
		GroupID:      &group.ID,
		ValidityDays: 30,
	}
	require.NoError(t, repo.Create(ctx, code))

	mode := "stack"
	require.NoError(t, repo.Use(ctx, code.ID, user.ID, &mode))

	got, err := repo.GetByID(ctx, code.ID)
	require.NoError(t, err)
	require.NotNil(t, got.SubscriptionMode)
	require.Equal(t, mode, *got.SubscriptionMode)
}
