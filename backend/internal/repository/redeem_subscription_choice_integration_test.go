//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/subscriptiongrant"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type RedeemSubscriptionChoiceSuite struct {
	suite.Suite
	ctx context.Context

	subSvc    *service.SubscriptionService
	redeemSvc *service.RedeemService

	grantRepo service.SubscriptionGrantRepository
}

func TestRedeemSubscriptionChoiceSuite(t *testing.T) {
	suite.Run(t, new(RedeemSubscriptionChoiceSuite))
}

func (s *RedeemSubscriptionChoiceSuite) SetupTest() {
	s.ctx = context.Background()
	client := testEntClient(s.T())

	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM subscription_grants")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM user_subscriptions")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM redeem_codes")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM groups")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM users")

	groupRepo := NewGroupRepository(client, integrationDB)
	userSubRepo := NewUserSubscriptionRepository(client)
	s.grantRepo = NewSubscriptionGrantRepository(client)
	s.subSvc = service.NewSubscriptionService(groupRepo, userSubRepo, s.grantRepo, nil, client, nil)

	redeemRepo := NewRedeemCodeRepository(client)
	userRepo := NewUserRepository(client, integrationDB)
	s.redeemSvc = service.NewRedeemService(redeemRepo, userRepo, s.subSvc, nil, nil, client, nil)
}

func (s *RedeemSubscriptionChoiceSuite) mustCreateUser(email string) *service.User {
	s.T().Helper()
	client := testEntClient(s.T())
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return userEntityToService(u)
}

func (s *RedeemSubscriptionChoiceSuite) mustCreateGroupSubscription(name string) *service.Group {
	s.T().Helper()
	client := testEntClient(s.T())
	g, err := client.Group.Create().
		SetName(name).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeSubscription).
		Save(s.ctx)
	s.Require().NoError(err, "create group")
	return groupEntityToService(g)
}

func (s *RedeemSubscriptionChoiceSuite) mustCreateSubscriptionRedeemCode(code string, groupID int64, validityDays int) {
	s.T().Helper()
	client := testEntClient(s.T())
	_, err := client.RedeemCode.Create().
		SetCode(code).
		SetType(service.RedeemTypeSubscription).
		SetStatus(service.StatusUnused).
		SetGroupID(groupID).
		SetValidityDays(validityDays).
		Save(s.ctx)
	s.Require().NoError(err, "create redeem code")
}

func (s *RedeemSubscriptionChoiceSuite) getRedeemCodeSubscriptionMode(code string) (sql.NullString, error) {
	s.T().Helper()
	var mode sql.NullString
	err := integrationDB.QueryRowContext(s.ctx, "SELECT subscription_mode FROM redeem_codes WHERE code = $1", code).Scan(&mode)
	return mode, err
}

func (s *RedeemSubscriptionChoiceSuite) TestRedeem_SubscriptionChoiceRequired_ThenExtendSucceeds() {
	user := s.mustCreateUser("redeem-choice@test.com")
	group := s.mustCreateGroupSubscription("g-redeem-choice")

	sub, err := s.subSvc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 10,
		AssignedBy:   0,
		Notes:        "base",
	})
	s.Require().NoError(err)
	oldExpires := sub.ExpiresAt

	code := "SUB-CHOICE-EXT-01"
	s.mustCreateSubscriptionRedeemCode(code, group.ID, 5)

	_, err = s.redeemSvc.Redeem(s.ctx, user.ID, code, "")
	s.Require().Error(err)
	appErr := infraerrors.FromError(err)
	s.Require().Equal(int32(409), appErr.Code)
	s.Require().Equal("SUBSCRIPTION_REDEEM_CHOICE_REQUIRED", appErr.Reason)
	s.Require().Equal(group.Name, appErr.Metadata["group_name"])

	rc, err := testEntClient(s.T()).RedeemCode.Query().Where(redeemcode.CodeEQ(code)).Only(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusUnused, rc.Status)
	s.Require().Nil(rc.UsedBy)
	s.Require().Nil(rc.UsedAt)

	_, err = s.redeemSvc.Redeem(s.ctx, user.ID, code, "extend")
	s.Require().NoError(err)

	tail, err := s.grantRepo.GetTailGrantBySubscriptionID(s.ctx, sub.ID)
	s.Require().NoError(err)
	s.Require().True(tail.ExpiresAt.Equal(oldExpires.Add(5 * 24 * time.Hour)))

	mode, err := s.getRedeemCodeSubscriptionMode(code)
	s.Require().NoError(err)
	s.Require().True(mode.Valid)
	s.Require().Equal("extend", mode.String)

	grants, err := testEntClient(s.T()).SubscriptionGrant.Query().
		Where(
			subscriptiongrant.SubscriptionIDEQ(sub.ID),
			subscriptiongrant.DeletedAtIsNil(),
		).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(grants, 1)
}

func (s *RedeemSubscriptionChoiceSuite) TestRedeem_SubscriptionChoiceRequired_ThenStackKeepsTailAndMultiplier() {
	user := s.mustCreateUser("redeem-stack@test.com")
	group := s.mustCreateGroupSubscription("g-redeem-stack")

	sub, err := s.subSvc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 20,
		AssignedBy:   0,
		Notes:        "base",
	})
	s.Require().NoError(err)
	oldExpires := sub.ExpiresAt

	code := "SUB-CHOICE-STK-01"
	s.mustCreateSubscriptionRedeemCode(code, group.ID, 3)

	_, err = s.redeemSvc.Redeem(s.ctx, user.ID, code, "")
	s.Require().Error(err)
	s.Require().Equal("SUBSCRIPTION_REDEEM_CHOICE_REQUIRED", infraerrors.FromError(err).Reason)

	_, err = s.redeemSvc.Redeem(s.ctx, user.ID, code, "stack")
	s.Require().NoError(err)

	mode, err := s.getRedeemCodeSubscriptionMode(code)
	s.Require().NoError(err)
	s.Require().True(mode.Valid)
	s.Require().Equal("stack", mode.String)

	active, err := s.subSvc.GetActiveSubscription(s.ctx, user.ID, group.ID)
	s.Require().NoError(err)
	s.Require().True(active.ExpiresAt.Equal(oldExpires))
	s.Require().Equal(2, active.QuotaMultiplier)
}
