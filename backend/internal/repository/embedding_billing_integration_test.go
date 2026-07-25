//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingRepositoryApplyPersistsEmbeddingLedgerAtomically(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repository := NewUsageBillingRepository(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("embedding-billing-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Balance:      100,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "embedding-billing-" + uuid.NewString(),
		Platform:         service.PlatformEmbedding,
		SubscriptionType: service.SubscriptionTypeStandard,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID:  user.ID,
		GroupID: &group.ID,
		Key:     "sk-embedding-billing-" + uuid.NewString(),
		Name:    "embedding-billing",
		Quota:   10,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name:     "embedding-billing-account-" + uuid.NewString(),
		Type:     service.AccountTypeAPIKey,
		Platform: service.PlatformEmbedding,
	})

	requestID := "embedding-ledger-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM api_keys WHERE id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id = $1", group.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})
	upstreamModel := "embed-upstream"
	inboundEndpoint := "/v1/embeddings"
	upstreamEndpoint := "/v1/embeddings"
	billingMode := string(service.BillingModeToken)
	accountMultiplier := 1.0
	usage := &service.UsageLog{
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 "embed-public",
		RequestedModel:        "embed-public",
		UpstreamModel:         &upstreamModel,
		GroupID:               &group.ID,
		InputTokens:           100,
		InputCost:             0.001,
		TotalCost:             0.001,
		ActualCost:            0.002,
		RateMultiplier:        2,
		AccountRateMultiplier: &accountMultiplier,
		BillingType:           service.BillingTypeBalance,
		RequestType:           service.RequestTypeEmbedding,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		BillingMode:           &billingMode,
		CreatedAt:             time.Now().UTC(),
	}
	command := &service.UsageBillingCommand{
		RequestID:          requestID,
		RequestPayloadHash: "payload-hash",
		APIKeyID:           apiKey.ID,
		UserID:             user.ID,
		AccountID:          account.ID,
		AccountType:        service.AccountTypeAPIKey,
		Model:              usage.Model,
		BillingType:        service.BillingTypeBalance,
		InputTokens:        usage.InputTokens,
		BalanceCost:        usage.ActualCost,
		APIKeyQuotaCost:    usage.ActualCost,
		UsageLog:           usage,
	}

	first, err := repository.Apply(ctx, command)
	require.NoError(t, err)
	require.True(t, first.Applied)
	second, err := repository.Apply(ctx, command)
	require.NoError(t, err)
	require.False(t, second.Applied)

	var requestType int16
	var model, requestedModel, storedUpstreamModel, storedInbound, storedUpstream string
	var inputTokens, outputTokens int
	var totalCost, actualCost float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT request_type, model, requested_model, upstream_model,
		       input_tokens, output_tokens, total_cost, actual_cost,
		       inbound_endpoint, upstream_endpoint
		FROM usage_logs
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKey.ID).Scan(
		&requestType, &model, &requestedModel, &storedUpstreamModel,
		&inputTokens, &outputTokens, &totalCost, &actualCost,
		&storedInbound, &storedUpstream,
	))
	require.Equal(t, int16(service.RequestTypeEmbedding), requestType)
	require.Equal(t, "embed-public", model)
	require.Equal(t, "embed-public", requestedModel)
	require.Equal(t, "embed-upstream", storedUpstreamModel)
	require.Equal(t, 100, inputTokens)
	require.Zero(t, outputTokens)
	require.InDelta(t, 0.001, totalCost, 1e-12)
	require.InDelta(t, 0.002, actualCost, 1e-12)
	require.Equal(t, "/v1/embeddings", storedInbound)
	require.Equal(t, "/v1/embeddings", storedUpstream)

	var balance, quotaUsed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used FROM api_keys WHERE id = $1", apiKey.ID).Scan(&quotaUsed))
	require.InDelta(t, 99.998, balance, 1e-9)
	require.InDelta(t, 0.002, quotaUsed, 1e-9)

	var usageRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&usageRows))
	require.Equal(t, 1, usageRows)
}

func TestUsageBillingRepositoryApplyRollsBackEveryEmbeddingEffectWhenUsageInsertFails(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repository := NewUsageBillingRepository(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{
		Email: fmt.Sprintf("embedding-rollback-%d@example.com", time.Now().UnixNano()), PasswordHash: "hash", Balance: 100,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name: "embedding-rollback-" + uuid.NewString(), Platform: service.PlatformEmbedding, SubscriptionType: service.SubscriptionTypeSubscription,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID, GroupID: &group.ID, Key: "sk-embedding-rollback-" + uuid.NewString(), Name: "embedding-rollback", Quota: 100,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "embedding-rollback-account-" + uuid.NewString(), Type: service.AccountTypeAPIKey, Platform: service.PlatformEmbedding,
		Extra: map[string]any{"quota_limit": 100.0},
	})
	subscription := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID})
	now := time.Now().UTC()
	grant, err := client.SubscriptionGrant.Create().SetSubscriptionID(subscription.ID).SetStartsAt(now.Add(-time.Hour)).SetExpiresAt(now.Add(time.Hour)).Save(ctx)
	require.NoError(t, err)

	requestID := "embedding-rollback-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM subscription_grants WHERE id = $1", grant.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM user_subscriptions WHERE id = $1", subscription.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM api_keys WHERE id = $1", apiKey.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id = $1", group.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	usage := &service.UsageLog{
		UserID: user.ID, APIKeyID: apiKey.ID, AccountID: account.ID, RequestID: requestID,
		// model is VARCHAR(100); this forces the final usage insert to fail after
		// every preceding billing effect has executed inside the transaction.
		Model: strings.Repeat("x", 101), RequestedModel: "embed-public", GroupID: &group.ID,
		InputTokens: 10, TotalCost: 1.5, ActualCost: 1.5,
		BillingType: service.BillingTypeSubscription, RequestType: service.RequestTypeEmbedding, CreatedAt: now,
	}
	command := &service.UsageBillingCommand{
		RequestID: requestID, RequestPayloadHash: "rollback-payload", APIKeyID: apiKey.ID,
		UserID: user.ID, AccountID: account.ID, SubscriptionID: &subscription.ID,
		AccountType: service.AccountTypeAPIKey, Model: "embed-public", BillingType: service.BillingTypeSubscription,
		BalanceCost: 1.5, SubscriptionCost: 1.5, APIKeyQuotaCost: 1.5,
		APIKeyRateLimitCost: 1.5, AccountQuotaCost: 1.5, UsageLog: usage,
	}

	result, err := repository.Apply(ctx, command)
	require.Nil(t, result)
	require.Error(t, err)

	var balance, keyQuota, keyRate, accountQuota, subscriptionDaily, grantDaily float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used, usage_5h FROM api_keys WHERE id = $1", apiKey.ID).Scan(&keyQuota, &keyRate))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COALESCE((extra->>'quota_used')::numeric, 0) FROM accounts WHERE id = $1", account.ID).Scan(&accountQuota))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM user_subscriptions WHERE id = $1", subscription.ID).Scan(&subscriptionDaily))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd FROM subscription_grants WHERE id = $1", grant.ID).Scan(&grantDaily))
	require.InDelta(t, 100, balance, 1e-9)
	require.Zero(t, keyQuota)
	require.Zero(t, keyRate)
	require.Zero(t, accountQuota)
	require.Zero(t, subscriptionDaily)
	require.Zero(t, grantDaily)

	var dedupRows, usageRows int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&dedupRows))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs WHERE request_id = $1 AND api_key_id = $2", requestID, apiKey.ID).Scan(&usageRows))
	require.Zero(t, dedupRows)
	require.Zero(t, usageRows)

	usage.Model = "embed-public"
	retry, err := repository.Apply(ctx, command)
	require.NoError(t, err)
	require.True(t, retry.Applied, "failed usage insert must roll back the dedup claim")
	duplicate, err := repository.Apply(ctx, command)
	require.NoError(t, err)
	require.False(t, duplicate.Applied)
}
