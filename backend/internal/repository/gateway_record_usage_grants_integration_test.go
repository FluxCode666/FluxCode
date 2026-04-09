//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptiongrant"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

type GatewayRecordUsageGrantsSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client

	subSvc     *service.SubscriptionService
	gatewaySvc *service.GatewayService
	openaiSvc  *service.OpenAIGatewayService
}

func TestGatewayRecordUsageGrantsSuite(t *testing.T) {
	suite.Run(t, new(GatewayRecordUsageGrantsSuite))
}

func (s *GatewayRecordUsageGrantsSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = testEntClient(s.T())

	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM usage_billing_dedup_archive")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM usage_billing_dedup")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM usage_logs")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM subscription_grants")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM user_subscriptions")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM api_keys")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM accounts")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM groups")
	_, _ = integrationDB.ExecContext(s.ctx, "DELETE FROM users")

	groupRepo := NewGroupRepository(s.client, integrationDB)
	userRepo := NewUserRepository(s.client, integrationDB)
	accountRepo := NewAccountRepository(s.client, integrationDB, nil)
	usageLogRepo := NewUsageLogRepository(s.client, integrationDB)
	usageBillingRepo := NewUsageBillingRepository(s.client, integrationDB)
	userSubRepo := NewUserSubscriptionRepository(s.client)
	grantRepo := NewSubscriptionGrantRepository(s.client)

	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	billingService := service.NewBillingService(cfg, nil)
	billingCacheService := service.NewBillingCacheService(nil, userRepo, userSubRepo, grantRepo, nil, cfg)
	s.T().Cleanup(billingCacheService.Stop)

	deferred := &service.DeferredService{}
	s.subSvc = service.NewSubscriptionService(groupRepo, userSubRepo, grantRepo, billingCacheService, s.client, cfg)
	s.gatewaySvc = service.NewGatewayService(
		accountRepo,
		groupRepo,
		usageLogRepo,
		usageBillingRepo,
		userRepo,
		userSubRepo,
		nil,
		nil,
		cfg,
		nil,
		nil,
		billingService,
		nil,
		billingCacheService,
		nil,
		nil,
		deferred,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	s.openaiSvc = service.NewOpenAIGatewayService(
		accountRepo,
		usageLogRepo,
		usageBillingRepo,
		userRepo,
		userSubRepo,
		nil,
		nil,
		cfg,
		nil,
		nil,
		billingService,
		nil,
		billingCacheService,
		nil,
		deferred,
		nil,
	)
}

func (s *GatewayRecordUsageGrantsSuite) mustCreateUser(email string) *service.User {
	return mustCreateUser(s.T(), s.client, &service.User{
		Email:        email,
		PasswordHash: "hash",
		Status:       service.StatusActive,
		Role:         service.RoleUser,
	})
}

func (s *GatewayRecordUsageGrantsSuite) mustCreateSubscriptionGroup(name string, platform string, dailyLimit float64) *service.Group {
	return mustCreateGroup(s.T(), s.client, &service.Group{
		Name:             name,
		Platform:         platform,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
	})
}

func (s *GatewayRecordUsageGrantsSuite) mustCreateAccount(name string, platform string) *service.Account {
	return mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        name,
		Platform:    platform,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Credentials: map[string]any{"api_key": uuid.NewString()},
	})
}

func (s *GatewayRecordUsageGrantsSuite) mustCreateAPIKey(userID, groupID int64) *service.APIKey {
	return mustCreateApiKey(s.T(), s.client, &service.APIKey{
		UserID:  userID,
		GroupID: &groupID,
		Key:     "sk-" + uuid.NewString(),
		Name:    "grant-usage",
		Status:  service.StatusActive,
	})
}

func (s *GatewayRecordUsageGrantsSuite) createStackedSubscription(user *service.User, group *service.Group) *service.UserSubscription {
	sub, err := s.subSvc.AssignSubscription(s.ctx, &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 20,
		AssignedBy:   0,
		Notes:        "base",
	})
	s.Require().NoError(err)

	_, err = s.subSvc.ApplyRedeemSubscription(s.ctx, &service.ApplyRedeemSubscriptionInput{
		UserID:           user.ID,
		GroupID:          group.ID,
		ValidityDays:     1,
		SubscriptionMode: service.SubscriptionModeStack,
		Notes:            "stack",
	})
	s.Require().NoError(err)

	return sub
}

func (s *GatewayRecordUsageGrantsSuite) TestGatewayService_RecordUsage_AllocatesAcrossActiveGrants_EarliestExpiryFirst_AndIsIdempotent() {
	user := s.mustCreateUser("gateway-grants-" + uuid.NewString() + "@example.com")
	group := s.mustCreateSubscriptionGroup("gateway-grants-"+uuid.NewString(), service.PlatformAnthropic, 10)
	sub := s.createStackedSubscription(user, group)
	account := s.mustCreateAccount("gateway-account-"+uuid.NewString(), service.PlatformAnthropic)
	apiKey := s.mustCreateAPIKey(user.ID, group.ID)
	apiKey.User = user
	apiKey.Group = group

	billedAt := time.Now().UTC()
	input := &service.RecordUsageInput{
		Result: &service.ForwardResult{
			RequestID: "req-gateway-grants-" + uuid.NewString(),
			Model:     "claude-sonnet-4",
			Usage: service.ClaudeUsage{
				InputTokens: 5_000_000,
			},
			Duration: 100 * time.Millisecond,
		},
		APIKey:       apiKey,
		User:         user,
		Account:      account,
		Subscription: sub,
		BilledAt:     billedAt,
	}

	s.Require().NoError(s.gatewaySvc.RecordUsage(s.ctx, input))

	grants, err := s.client.SubscriptionGrant.Query().
		Where(subscriptiongrant.SubscriptionIDEQ(sub.ID), subscriptiongrant.DeletedAtIsNil()).
		Order(subscriptiongrant.ByExpiresAt()).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(grants, 2)
	s.Require().InEpsilon(10.0, grants[0].DailyUsageUsd, 1e-6)
	s.Require().InEpsilon(5.0, grants[1].DailyUsageUsd, 1e-6)

	gotSub, err := s.client.UserSubscription.Query().Where(usersubscription.IDEQ(sub.ID)).Only(s.ctx)
	s.Require().NoError(err)
	s.Require().InEpsilon(15.0, gotSub.DailyUsageUsd, 1e-6)

	s.Require().NoError(s.gatewaySvc.RecordUsage(s.ctx, input))

	grantsAfterRetry, err := s.client.SubscriptionGrant.Query().
		Where(subscriptiongrant.SubscriptionIDEQ(sub.ID), subscriptiongrant.DeletedAtIsNil()).
		Order(subscriptiongrant.ByExpiresAt()).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().InEpsilon(10.0, grantsAfterRetry[0].DailyUsageUsd, 1e-6)
	s.Require().InEpsilon(5.0, grantsAfterRetry[1].DailyUsageUsd, 1e-6)

	gotSubAfterRetry, err := s.client.UserSubscription.Query().Where(usersubscription.IDEQ(sub.ID)).Only(s.ctx)
	s.Require().NoError(err)
	s.Require().InEpsilon(15.0, gotSubAfterRetry.DailyUsageUsd, 1e-6)
}

func (s *GatewayRecordUsageGrantsSuite) TestOpenAIGatewayService_RecordUsage_AllocatesAcrossActiveGrants_EarliestExpiryFirst_AndIsIdempotent() {
	user := s.mustCreateUser("openai-grants-" + uuid.NewString() + "@example.com")
	group := s.mustCreateSubscriptionGroup("openai-grants-"+uuid.NewString(), service.PlatformOpenAI, 10)
	sub := s.createStackedSubscription(user, group)
	account := s.mustCreateAccount("openai-account-"+uuid.NewString(), service.PlatformOpenAI)
	apiKey := s.mustCreateAPIKey(user.ID, group.ID)
	apiKey.User = user
	apiKey.Group = group

	billedAt := time.Now().UTC()
	input := &service.OpenAIRecordUsageInput{
		Result: &service.OpenAIForwardResult{
			RequestID: "req-openai-grants-" + uuid.NewString(),
			Model:     "gpt-5.4",
			Usage: service.OpenAIUsage{
				InputTokens: 6_000_000,
			},
			Duration: 100 * time.Millisecond,
		},
		APIKey:       apiKey,
		User:         user,
		Account:      account,
		Subscription: sub,
		BilledAt:     billedAt,
	}

	s.Require().NoError(s.openaiSvc.RecordUsage(s.ctx, input))

	grants, err := s.client.SubscriptionGrant.Query().
		Where(subscriptiongrant.SubscriptionIDEQ(sub.ID), subscriptiongrant.DeletedAtIsNil()).
		Order(subscriptiongrant.ByExpiresAt()).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(grants, 2)
	s.Require().InEpsilon(10.0, grants[0].DailyUsageUsd, 1e-6)
	s.Require().InEpsilon(5.0, grants[1].DailyUsageUsd, 1e-6)

	gotSub, err := s.client.UserSubscription.Query().Where(usersubscription.IDEQ(sub.ID)).Only(s.ctx)
	s.Require().NoError(err)
	s.Require().InEpsilon(15.0, gotSub.DailyUsageUsd, 1e-6)

	s.Require().NoError(s.openaiSvc.RecordUsage(s.ctx, input))

	grantsAfterRetry, err := s.client.SubscriptionGrant.Query().
		Where(subscriptiongrant.SubscriptionIDEQ(sub.ID), subscriptiongrant.DeletedAtIsNil()).
		Order(subscriptiongrant.ByExpiresAt()).
		All(s.ctx)
	s.Require().NoError(err)
	s.Require().InEpsilon(10.0, grantsAfterRetry[0].DailyUsageUsd, 1e-6)
	s.Require().InEpsilon(5.0, grantsAfterRetry[1].DailyUsageUsd, 1e-6)

	gotSubAfterRetry, err := s.client.UserSubscription.Query().Where(usersubscription.IDEQ(sub.ID)).Only(s.ctx)
	s.Require().NoError(err)
	s.Require().InEpsilon(15.0, gotSubAfterRetry.DailyUsageUsd, 1e-6)
}
