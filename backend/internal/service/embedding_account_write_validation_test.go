//go:build unit

package service

import (
	"context"
	"net"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func embeddingWriteTestConfig(privateCIDRs ...string) *config.Config {
	return &config.Config{Gateway: config.GatewayConfig{Embedding: config.EmbeddingGatewayConfig{
		AllowedPrivateCIDRs: privateCIDRs,
	}}}
}

func embeddingWriteTestAccount(baseURL string) *Account {
	return &Account{
		ID: 7, Platform: PlatformEmbedding, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":      baseURL,
			"api_key":       "secret",
			"model_mapping": map[string]any{"embed": "upstream-embed"},
		},
	}
}

type embeddingUpdateRepoStub struct {
	accountRepoStubForBulkUpdate
	updated *Account
}

func (s *embeddingUpdateRepoStub) Update(_ context.Context, account *Account) error {
	s.updated = account
	s.getByIDAccounts[account.ID] = account
	return nil
}

func TestValidateEmbeddingAccountForWriteEnforcesNetworkPolicy(t *testing.T) {
	originalLookup := lookupEmbeddingHostIP
	t.Cleanup(func() { lookupEmbeddingHostIP = originalLookup })
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}

	require.NoError(t, validateEmbeddingAccountForWrite(context.Background(), embeddingWriteTestAccount("https://embedding.example.com/v1"), embeddingWriteTestConfig()))
	require.Error(t, validateEmbeddingAccountForWrite(context.Background(), embeddingWriteTestAccount("http://embedding.example.com"), embeddingWriteTestConfig()))
	require.NoError(t, validateEmbeddingAccountForWrite(context.Background(), embeddingWriteTestAccount("https://other.example.com"), embeddingWriteTestConfig()))

	proxied := embeddingWriteTestAccount("https://embedding.example.com")
	proxyID := int64(9)
	proxied.ProxyID = &proxyID
	require.ErrorContains(t, validateEmbeddingAccountForWrite(context.Background(), proxied, embeddingWriteTestConfig()), "do not support proxies")

	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.10.1.2")}, nil
	}
	require.Error(t, validateEmbeddingAccountForWrite(context.Background(), embeddingWriteTestAccount("https://embedding.example.com"), embeddingWriteTestConfig()))
	require.NoError(t, validateEmbeddingAccountForWrite(context.Background(), embeddingWriteTestAccount("https://embedding.example.com"), embeddingWriteTestConfig("10.10.0.0/16")))
}

func TestAdminEmbeddingWriteEntrypointsRejectUnsafeConfigurationBeforePersistence(t *testing.T) {
	originalLookup := lookupEmbeddingHostIP
	lookupEmbeddingHostIP = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	t.Cleanup(func() { lookupEmbeddingHostIP = originalLookup })

	t.Run("create rejects http", func(t *testing.T) {
		repo := &accountRepoStubForBulkUpdate{}
		svc := &adminServiceImpl{accountRepo: repo, cfg: embeddingWriteTestConfig()}
		result, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
			Name: "unsafe", Platform: PlatformEmbedding, Type: AccountTypeAPIKey,
			Credentials:          embeddingWriteTestAccount("http://embedding.example.com").Credentials,
			SkipDefaultGroupBind: true,
		})
		require.Nil(t, result)
		require.ErrorContains(t, err, "network policy")
	})

	t.Run("update accepts an unlisted public host", func(t *testing.T) {
		repo := &embeddingUpdateRepoStub{accountRepoStubForBulkUpdate: accountRepoStubForBulkUpdate{
			getByIDAccounts: map[int64]*Account{7: embeddingWriteTestAccount("https://embedding.example.com")},
		}}
		svc := &adminServiceImpl{accountRepo: repo, cfg: embeddingWriteTestConfig()}
		result, err := svc.UpdateAccount(context.Background(), 7, &UpdateAccountInput{
			Credentials: embeddingWriteTestAccount("https://other.example.com").Credentials,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "https://other.example.com", repo.updated.GetEmbeddingBaseURL())
	})

	t.Run("bulk rejects proxy", func(t *testing.T) {
		repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{embeddingWriteTestAccount("https://embedding.example.com")}}
		svc := &adminServiceImpl{accountRepo: repo, cfg: embeddingWriteTestConfig()}
		proxyID := int64(3)
		result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{7}, ProxyID: &proxyID})
		require.Nil(t, result)
		require.ErrorContains(t, err, "do not support proxies")
		require.Empty(t, repo.bulkUpdateIDs)
	})
}
