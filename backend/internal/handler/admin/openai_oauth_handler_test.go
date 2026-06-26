package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIOAuthRefreshAdminService struct {
	*stubAdminService
	account     service.Account
	updated    *service.UpdateAccountInput
	updateID   int64
	updateCall int
}

func (s *openAIOAuthRefreshAdminService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	account := s.account
	account.ID = id
	return &account, nil
}

func (s *openAIOAuthRefreshAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.updateID = id
	s.updated = input
	s.updateCall++
	account := s.account
	account.ID = id
	account.Credentials = input.Credentials
	return &account, nil
}

type openAIOAuthRefreshClient struct {
	clientID string
}

func (c *openAIOAuthRefreshClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	return nil, nil
}

func (c *openAIOAuthRefreshClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return c.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

func (c *openAIOAuthRefreshClient) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	c.clientID = clientID
	return &openai.TokenResponse{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		ExpiresIn:    3600,
	}, nil
}

type openAIOAuthRefreshInvalidator struct {
	calls     int
	accountID int64
}

func (i *openAIOAuthRefreshInvalidator) InvalidateToken(ctx context.Context, account *service.Account) error {
	i.calls++
	if account != nil {
		i.accountID = account.ID
	}
	return nil
}

func TestOpenAIOAuthHandlerRefreshAccountInvalidatesTokenCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adminSvc := &openAIOAuthRefreshAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       42,
			Name:     "openai-oauth",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"access_token":  "old-access-token",
				"refresh_token": "old-refresh-token",
				"client_id":     "custom-client-id",
			},
		},
	}
	oauthClient := &openAIOAuthRefreshClient{}
	oauthSvc := service.NewOpenAIOAuthService(nil, oauthClient)
	invalidator := &openAIOAuthRefreshInvalidator{}
	handler := NewOpenAIOAuthHandler(oauthSvc, adminSvc, nil, invalidator)

	router := gin.New()
	router.POST("/api/v1/admin/openai/accounts/:id/refresh", handler.RefreshAccountToken)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/openai/accounts/42/refresh", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "custom-client-id", oauthClient.clientID)
	require.Equal(t, 1, adminSvc.updateCall)
	require.Equal(t, int64(42), adminSvc.updateID)
	require.Equal(t, "new-access-token", adminSvc.updated.Credentials["access_token"])
	require.Equal(t, "new-refresh-token", adminSvc.updated.Credentials["refresh_token"])
	require.Equal(t, 1, invalidator.calls)
	require.Equal(t, int64(42), invalidator.accountID)
}
