package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type openAIOAuthSessionAccountRepo struct {
	account *Account
}

func (r *openAIOAuthSessionAccountRepo) Create(context.Context, *Account) error { return nil }
func (r *openAIOAuthSessionAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *openAIOAuthSessionAccountRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ExistsByID(context.Context, int64) (bool, error) {
	return r.account != nil, nil
}
func (r *openAIOAuthSessionAccountRepo) GetByCRSAccountID(context.Context, string) (*Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) FindByExtraField(context.Context, string, any) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) Update(_ context.Context, account *Account) error {
	r.account = account
	return nil
}
func (r *openAIOAuthSessionAccountRepo) Delete(context.Context, int64) error { return nil }
func (r *openAIOAuthSessionAccountRepo) List(context.Context, pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListByGroup(context.Context, int64) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListActive(context.Context) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) UpdateLastUsed(context.Context, int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) SetError(context.Context, int64, string) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) SetBanned(context.Context, int64, string) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ClearError(context.Context, int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) SetSchedulable(context.Context, int64, bool) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *openAIOAuthSessionAccountRepo) BindGroups(context.Context, int64, []int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulable(context.Context) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	return nil, nil
}
func (r *openAIOAuthSessionAccountRepo) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ClearTempUnschedulable(context.Context, int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ClearRateLimit(context.Context, int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ClearAntigravityQuotaScopes(context.Context, int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ClearModelRateLimits(context.Context, int64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	return 0, nil
}
func (r *openAIOAuthSessionAccountRepo) IncrementQuotaUsed(context.Context, int64, float64) error {
	return nil
}
func (r *openAIOAuthSessionAccountRepo) ResetQuotaUsed(context.Context, int64) error {
	return nil
}

var _ AccountRepository = (*openAIOAuthSessionAccountRepo)(nil)

type openAIOAuthSessionRefreshExecutor struct {
	err error
}

func (e *openAIOAuthSessionRefreshExecutor) CanRefresh(*Account) bool { return true }
func (e *openAIOAuthSessionRefreshExecutor) NeedsRefresh(*Account, time.Duration) bool {
	return true
}
func (e *openAIOAuthSessionRefreshExecutor) Refresh(context.Context, *Account) (map[string]any, error) {
	return nil, e.err
}
func (e *openAIOAuthSessionRefreshExecutor) CacheKey(account *Account) string {
	return "openai:test:" + account.GetCredential("refresh_token")
}

func TestOpenAITokenProvider_SessionTerminatedWithoutExistingTokenReturnsRefreshError(t *testing.T) {
	account := openAIOAuthSessionExpiredAccount()
	repo := &openAIOAuthSessionAccountRepo{account: account}
	executor := &openAIOAuthSessionRefreshExecutor{err: NewOpenAIOAuthSessionTerminatedError()}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	require.True(t, IsOpenAIOAuthSessionTerminatedError(err))
}

func TestOpenAIGatewayService_GetAccessToken_SessionTerminatedBecomesFailoverError(t *testing.T) {
	account := openAIOAuthSessionExpiredAccount()
	repo := &openAIOAuthSessionAccountRepo{account: account}
	executor := &openAIOAuthSessionRefreshExecutor{err: NewOpenAIOAuthSessionTerminatedError()}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), executor)
	svc := &OpenAIGatewayService{openAITokenProvider: provider}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	require.Empty(t, tokenType)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)
	require.True(t, IsOpenAIOAuthSessionTerminatedResponse(failoverErr.ResponseBody))
}

func TestIsNonRetryableRefreshError_OpenAIAppSessionTerminated(t *testing.T) {
	err := errors.New(`token refresh failed: status 400, body: {"error":{"code":"app_session_terminated","message":"Your session has ended. Please log in again."}}`)

	require.True(t, isNonRetryableRefreshError(err))
}

func openAIOAuthSessionExpiredAccount() *Account {
	return &Account{
		ID:       310,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "old-refresh-token",
			"expires_at":    time.Now().Add(-time.Minute).Format(time.RFC3339),
		},
	}
}
