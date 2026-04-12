//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptiongrant"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type SubscriptionServiceGrantsSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client

	groupRepo   service.GroupRepository
	userSubRepo service.UserSubscriptionRepository
	grantRepo   service.SubscriptionGrantRepository

	svc *service.SubscriptionService
}

func TestSubscriptionServiceGrantsSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionServiceGrantsSuite))
}

func (s *SubscriptionServiceGrantsSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = testEntClient(s.T())

	s.groupRepo = NewGroupRepository(s.client, integrationDB)
	s.userSubRepo = NewUserSubscriptionRepository(s.client)
	s.grantRepo = NewSubscriptionGrantRepository(s.client)
	s.svc = service.NewSubscriptionService(s.groupRepo, s.userSubRepo, s.grantRepo, nil, s.client, nil)
}

func (s *SubscriptionServiceGrantsSuite) mustCreateUser(email string) *service.User {
	s.T().Helper()
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return userEntityToService(u)
}

func (s *SubscriptionServiceGrantsSuite) mustCreateGroupSubscription(name string) *service.Group {
	s.T().Helper()
	g, err := s.client.Group.Create().
		SetName(name).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return groupEntityToService(g)
}

func (s *SubscriptionServiceGrantsSuite) TestAssignSubscription_CreatesInitialGrant() {
	user := s.mustCreateUser("svc-grant-init@test.com")
	group := s.mustCreateGroupSubscription("g-svc-grant-init")

	sub, err := s.svc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 10,
		AssignedBy:   0,
		Notes:        "init",
	})
	s.Require().NoError(err)
	s.Require().NotNil(sub)

	grants, err := s.client.SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(sub.ID),
			subscriptiongrant.DeletedAtIsNil(),
		).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(grants, 1)
	s.Require().True(grants[0].StartsAt.Equal(sub.StartsAt))
	s.Require().True(grants[0].ExpiresAt.Equal(sub.ExpiresAt))
}

func (s *SubscriptionServiceGrantsSuite) TestAssignSubscriptionWithMode_ChoiceRequiredForActiveWithoutMode() {
	user := s.mustCreateUser("svc-assign-mode-choice@test.com")
	admin := s.mustCreateUser("svc-assign-mode-choice-admin@test.com")
	group := s.mustCreateGroupSubscription("g-svc-assign-mode-choice")

	base, err := s.svc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 10,
		AssignedBy:   admin.ID,
		Notes:        "base",
	})
	s.Require().NoError(err)
	oldExpires := base.ExpiresAt

	_, err = s.svc.AssignSubscriptionWithMode(s.ctx, &service.AssignSubscriptionWithModeInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 5,
		AssignedBy:   admin.ID,
		Notes:        "try-without-mode",
	})
	s.Require().Error(err)

	appErr := infraerrors.FromError(err)
	s.Require().Equal(int32(409), appErr.Code)
	s.Require().Equal("SUBSCRIPTION_REDEEM_CHOICE_REQUIRED", appErr.Reason)
	s.Require().Equal(group.Name, appErr.Metadata["group_name"])
	s.Require().Equal("5", appErr.Metadata["validity_days"])
	s.Require().Equal("1", appErr.Metadata["current_quota_multiplier"])

	got, err := s.userSubRepo.GetByID(s.ctx, base.ID)
	s.Require().NoError(err)
	s.Require().True(got.ExpiresAt.Equal(oldExpires))
}

func (s *SubscriptionServiceGrantsSuite) TestAssignSubscriptionWithMode_ExtendAndStack() {
	user := s.mustCreateUser("svc-assign-mode-es@test.com")
	admin := s.mustCreateUser("svc-assign-mode-es-admin@test.com")
	group := s.mustCreateGroupSubscription("g-svc-assign-mode-es")

	base, err := s.svc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 10,
		AssignedBy:   admin.ID,
		Notes:        "base",
	})
	s.Require().NoError(err)
	oldExpires := base.ExpiresAt

	extended, err := s.svc.AssignSubscriptionWithMode(s.ctx, &service.AssignSubscriptionWithModeInput{
		UserID:           user.ID,
		GroupID:          group.ID,
		ValidityDays:     5,
		AssignedBy:       admin.ID,
		SubscriptionMode: service.SubscriptionModeExtend,
		Notes:            "extend",
	})
	s.Require().NoError(err)
	s.Require().True(extended.ExpiresAt.Equal(oldExpires.Add(5 * 24 * time.Hour)))

	grantsAfterExtend, err := s.client.SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(base.ID),
			subscriptiongrant.DeletedAtIsNil(),
		).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(grantsAfterExtend, 1, "extend should not create a new grant")

	stacked, err := s.svc.AssignSubscriptionWithMode(s.ctx, &service.AssignSubscriptionWithModeInput{
		UserID:           user.ID,
		GroupID:          group.ID,
		ValidityDays:     3,
		AssignedBy:       admin.ID,
		SubscriptionMode: service.SubscriptionModeStack,
		Notes:            "stack",
	})
	s.Require().NoError(err)
	s.Require().True(stacked.ExpiresAt.Equal(extended.ExpiresAt))

	active, err := s.svc.GetActiveSubscription(s.ctx, user.ID, group.ID)
	s.Require().NoError(err)
	s.Require().Equal(2, active.QuotaMultiplier)
}

func (s *SubscriptionServiceGrantsSuite) TestAssignSubscriptionWithMode_SuspendedWithoutModeRequiresChoice() {
	user := s.mustCreateUser("svc-assign-mode-susp@test.com")
	admin := s.mustCreateUser("svc-assign-mode-susp-admin@test.com")
	group := s.mustCreateGroupSubscription("g-svc-assign-mode-susp")

	sub, err := s.svc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 10,
		AssignedBy:   admin.ID,
		Notes:        "base",
	})
	s.Require().NoError(err)

	_, err = s.client.UserSubscription.UpdateOneID(sub.ID).
		SetStatus(service.SubscriptionStatusSuspended).
		Save(s.ctx)
	s.Require().NoError(err)

	_, err = s.svc.AssignSubscriptionWithMode(s.ctx, &service.AssignSubscriptionWithModeInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 3,
		AssignedBy:   admin.ID,
		Notes:        "without-mode",
	})
	s.Require().Error(err)
	appErr := infraerrors.FromError(err)
	s.Require().Equal("SUBSCRIPTION_REDEEM_CHOICE_REQUIRED", appErr.Reason)

	updated, err := s.svc.AssignSubscriptionWithMode(s.ctx, &service.AssignSubscriptionWithModeInput{
		UserID:           user.ID,
		GroupID:          group.ID,
		ValidityDays:     3,
		AssignedBy:       admin.ID,
		SubscriptionMode: service.SubscriptionModeExtend,
		Notes:            "with-extend",
	})
	s.Require().NoError(err)
	s.Require().Equal(service.SubscriptionStatusActive, updated.Status)
}

func (s *SubscriptionServiceGrantsSuite) TestAssignSubscriptionWithMode_ExpiredWithoutModeDoesNotRequireChoice() {
	user := s.mustCreateUser("svc-assign-mode-expired@test.com")
	admin := s.mustCreateUser("svc-assign-mode-expired-admin@test.com")
	group := s.mustCreateGroupSubscription("g-svc-assign-mode-expired")

	sub, err := s.svc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 10,
		AssignedBy:   admin.ID,
		Notes:        "base",
	})
	s.Require().NoError(err)

	past := time.Now().Add(-1 * time.Hour)
	_, err = s.client.UserSubscription.UpdateOneID(sub.ID).
		SetExpiresAt(past).
		SetStatus(service.SubscriptionStatusExpired).
		Save(s.ctx)
	s.Require().NoError(err)

	tail, err := s.grantRepo.GetTailGrantBySubscriptionID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.grantRepo.UpdateExpiresAt(s.ctx, tail.ID, past))

	updated, err := s.svc.AssignSubscriptionWithMode(s.ctx, &service.AssignSubscriptionWithModeInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 2,
		AssignedBy:   admin.ID,
		Notes:        "expired-no-mode",
	})
	s.Require().NoError(err)
	s.Require().Equal(service.SubscriptionStatusActive, updated.Status)
	s.Require().True(updated.ExpiresAt.After(time.Now()))
}
