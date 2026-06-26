package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIRefreshLogicRepo struct {
	openAIOAuthSessionAccountRepo
	setErrorCalls          int
	tempCalls              int
	updateCredentialsCalls int
	lastErrorMsg           string
	lastCredentials        map[string]any
}

func (r *openAIRefreshLogicRepo) SetError(ctx context.Context, id int64, errorMsg string) error {
	r.setErrorCalls++
	r.lastErrorMsg = errorMsg
	return nil
}

func (r *openAIRefreshLogicRepo) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	r.tempCalls++
	return nil
}

func (r *openAIRefreshLogicRepo) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	r.updateCredentialsCalls++
	r.lastCredentials = cloneCredentials(credentials)
	return nil
}

type openAIRefreshLogicTokenCache struct {
	tokens       map[string]string
	getErr       error
	lockAcquired bool
	deletedKeys  []string
}

func (c *openAIRefreshLogicTokenCache) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	if c.getErr != nil {
		return "", c.getErr
	}
	return c.tokens[cacheKey], nil
}

func (c *openAIRefreshLogicTokenCache) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	if c.tokens == nil {
		c.tokens = make(map[string]string)
	}
	c.tokens[cacheKey] = token
	return nil
}

func (c *openAIRefreshLogicTokenCache) DeleteAccessToken(ctx context.Context, cacheKey string) error {
	delete(c.tokens, cacheKey)
	c.deletedKeys = append(c.deletedKeys, cacheKey)
	return nil
}

func (c *openAIRefreshLogicTokenCache) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	return c.lockAcquired, nil
}

func (c *openAIRefreshLogicTokenCache) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	return nil
}

type openAIRefreshLogicInvalidator struct {
	accounts []*Account
}

func (i *openAIRefreshLogicInvalidator) InvalidateToken(ctx context.Context, account *Account) error {
	i.accounts = append(i.accounts, account)
	return nil
}

func TestOpenAITokenRefresherSkipsMissingRefreshToken(t *testing.T) {
	refresher := NewOpenAITokenRefresher(nil, nil)
	expiresAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)

	withoutRefreshToken := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   expiresAt,
		},
	}
	require.False(t, refresher.NeedsRefresh(withoutRefreshToken, 5*time.Minute))

	withRefreshToken := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
		},
	}
	require.True(t, refresher.NeedsRefresh(withRefreshToken, 5*time.Minute))
}

func TestOpenAITokenProviderExpiredWithoutRefreshTokenDisablesAccount(t *testing.T) {
	expiresAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       2881,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "expired-access-token",
			"expires_at":   expiresAt,
		},
	}
	cacheKey := OpenAITokenCacheKey(account)
	cache := &openAIRefreshLogicTokenCache{
		tokens:       map[string]string{cacheKey: "stale-cached-token"},
		getErr:       errors.New("cache miss"),
		lockAcquired: true,
	}
	repo := &openAIRefreshLogicRepo{}
	provider := NewOpenAITokenProvider(repo, cache, nil)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	require.Contains(t, err.Error(), "refresh_token is missing")
	require.Equal(t, 1, repo.setErrorCalls)
	require.Contains(t, repo.lastErrorMsg, "refresh_token is missing")
	require.NotContains(t, cache.tokens, cacheKey)
	require.Equal(t, []string{cacheKey}, cache.deletedKeys)
}

func TestRateLimitServiceOpenAIOAuth401NoRefreshTokenSetsError(t *testing.T) {
	t.Run("missing_refresh_token", func(t *testing.T) {
		repo := &openAIRefreshLogicRepo{}
		invalidator := &openAIRefreshLogicInvalidator{}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		service.SetTokenCacheInvalidator(invalidator)
		account := &Account{
			ID:       2882,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token": "expired-access-token",
			},
		}

		shouldDisable := service.HandleUpstreamError(context.Background(), account, http.StatusUnauthorized, http.Header{}, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrorCalls)
		require.Equal(t, 0, repo.tempCalls)
		require.Equal(t, 0, repo.updateCredentialsCalls)
		require.Contains(t, repo.lastErrorMsg, "refresh_token missing")
		require.Len(t, invalidator.accounts, 1)
	})

	t.Run("blank_refresh_token", func(t *testing.T) {
		repo := &openAIRefreshLogicRepo{}
		service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := &Account{
			ID:       2883,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token":  "expired-access-token",
				"refresh_token": "   ",
			},
		}

		shouldDisable := service.HandleUpstreamError(context.Background(), account, http.StatusUnauthorized, http.Header{}, []byte("unauthorized"))

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setErrorCalls)
		require.Equal(t, 0, repo.tempCalls)
	})
}
