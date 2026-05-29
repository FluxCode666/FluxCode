package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type apiKeySystemPromptRepoStub struct {
	created  *APIKey
	existing *APIKey
	updated  *APIKey
}

func (s *apiKeySystemPromptRepoStub) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.created = &clone
	return nil
}

func (s *apiKeySystemPromptRepoStub) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	if s.existing == nil {
		return nil, ErrAPIKeyNotFound
	}
	clone := *s.existing
	return &clone, nil
}

func (s *apiKeySystemPromptRepoStub) GetKeyAndOwnerID(_ context.Context, _ int64) (string, int64, error) {
	if s.existing == nil {
		return "", 0, ErrAPIKeyNotFound
	}
	return s.existing.Key, s.existing.UserID, nil
}

func (s *apiKeySystemPromptRepoStub) GetByKey(context.Context, string) (*APIKey, error) {
	return nil, ErrAPIKeyNotFound
}

func (s *apiKeySystemPromptRepoStub) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	return nil, ErrAPIKeyNotFound
}

func (s *apiKeySystemPromptRepoStub) Update(_ context.Context, key *APIKey) error {
	clone := *key
	s.updated = &clone
	return nil
}

func (s *apiKeySystemPromptRepoStub) Delete(context.Context, int64) error { return nil }

func (s *apiKeySystemPromptRepoStub) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeySystemPromptRepoStub) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	return nil, nil
}

func (s *apiKeySystemPromptRepoStub) CountByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeySystemPromptRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

func (s *apiKeySystemPromptRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeySystemPromptRepoStub) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	return nil, nil
}

func (s *apiKeySystemPromptRepoStub) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeySystemPromptRepoStub) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeySystemPromptRepoStub) CountByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeySystemPromptRepoStub) ListKeysByUserID(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *apiKeySystemPromptRepoStub) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *apiKeySystemPromptRepoStub) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	return 0, nil
}

func (s *apiKeySystemPromptRepoStub) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
}

func (s *apiKeySystemPromptRepoStub) IncrementRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func (s *apiKeySystemPromptRepoStub) ResetRateLimitWindows(context.Context, int64) error {
	return nil
}

func (s *apiKeySystemPromptRepoStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return nil, nil
}

type apiKeySystemPromptUserRepoStub struct {
	user *User
}

func (s *apiKeySystemPromptUserRepoStub) Create(context.Context, *User) error { return nil }

func (s *apiKeySystemPromptUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if s.user != nil {
		clone := *s.user
		return &clone, nil
	}
	return &User{ID: id, Status: StatusActive}, nil
}

func (s *apiKeySystemPromptUserRepoStub) GetByEmail(context.Context, string) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *apiKeySystemPromptUserRepoStub) GetFirstAdmin(context.Context) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *apiKeySystemPromptUserRepoStub) Update(context.Context, *User) error { return nil }
func (s *apiKeySystemPromptUserRepoStub) Delete(context.Context, int64) error { return nil }

func (s *apiKeySystemPromptUserRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeySystemPromptUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeySystemPromptUserRepoStub) UpdateBalance(context.Context, int64, float64) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) DeductBalance(context.Context, int64, float64) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) UpdateConcurrency(context.Context, int64, int) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}

func (s *apiKeySystemPromptUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeySystemPromptUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) EnableTotp(context.Context, int64) error  { return nil }
func (s *apiKeySystemPromptUserRepoStub) DisableTotp(context.Context, int64) error { return nil }

func (s *apiKeySystemPromptUserRepoStub) GetByReferralCode(context.Context, string) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *apiKeySystemPromptUserRepoStub) UpdateReferralCode(context.Context, int64, string) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) UpdateReferredBy(context.Context, int64, int64) error {
	return nil
}

func (s *apiKeySystemPromptUserRepoStub) IsFirstRecharge(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *apiKeySystemPromptUserRepoStub) ListActiveUserIDs(context.Context) ([]int64, error) {
	return nil, nil
}

type systemPromptPermissionProviderStub struct {
	settings SystemPromptRuntimeSettings
}

func (s systemPromptPermissionProviderStub) GetSystemPromptSettings(context.Context) SystemPromptRuntimeSettings {
	return s.settings
}

func blockedSystemPromptPermissionProvider() systemPromptPermissionProviderStub {
	return systemPromptPermissionProviderStub{
		settings: SystemPromptRuntimeSettings{
			UserScope: SystemPromptUserScope{
				Enabled: true,
				Mode:    SystemPromptUserScopeWhitelist,
				UserIDs: []int64{99},
			},
		},
	}
}

func disabledSystemPromptPermissionProvider() systemPromptPermissionProviderStub {
	return systemPromptPermissionProviderStub{
		settings: SystemPromptRuntimeSettings{
			UserScope: SystemPromptUserScope{
				Enabled: false,
				Mode:    SystemPromptUserScopeAll,
				UserIDs: []int64{},
			},
		},
	}
}

func TestAPIKeyService_CreateRejectsCustomSystemPromptWhenUserBlocked(t *testing.T) {
	repo := &apiKeySystemPromptRepoStub{}
	svc := NewAPIKeyService(repo, &apiKeySystemPromptUserRepoStub{}, nil, nil, nil, nil, &config.Config{})
	svc.SetSystemPromptSettingsProvider(blockedSystemPromptPermissionProvider())

	_, err := svc.Create(context.Background(), 42, CreateAPIKeyRequest{
		Name:             "blocked prompt",
		SystemPrompt:     "only for allowed users",
		SystemPromptMode: SystemPromptModeOverride,
	})

	require.ErrorIs(t, err, ErrSystemPromptConfigNotAllowed)
	require.Nil(t, repo.created)
}

func TestAPIKeyService_CreateRejectsCustomSystemPromptWhenUserScopeDisabled(t *testing.T) {
	repo := &apiKeySystemPromptRepoStub{}
	svc := NewAPIKeyService(repo, &apiKeySystemPromptUserRepoStub{}, nil, nil, nil, nil, &config.Config{})
	svc.SetSystemPromptSettingsProvider(disabledSystemPromptPermissionProvider())

	_, err := svc.Create(context.Background(), 42, CreateAPIKeyRequest{
		Name:             "disabled prompt",
		SystemPrompt:     "should not be saved",
		SystemPromptMode: SystemPromptModeOverride,
	})

	require.ErrorIs(t, err, ErrSystemPromptConfigNotAllowed)
	require.Nil(t, repo.created)
}

func TestAPIKeyService_UpdateRejectsCustomSystemPromptWhenUserBlocked(t *testing.T) {
	prompt := "only for allowed users"
	mode := SystemPromptModeAppend
	repo := &apiKeySystemPromptRepoStub{
		existing: &APIKey{ID: 10, UserID: 42, Key: "sk-existing", Name: "existing", Status: StatusActive},
	}
	svc := NewAPIKeyService(repo, &apiKeySystemPromptUserRepoStub{}, nil, nil, nil, nil, &config.Config{})
	svc.SetSystemPromptSettingsProvider(blockedSystemPromptPermissionProvider())

	_, err := svc.Update(context.Background(), 10, 42, UpdateAPIKeyRequest{
		SystemPrompt:     &prompt,
		SystemPromptMode: &mode,
	})

	require.ErrorIs(t, err, ErrSystemPromptConfigNotAllowed)
	require.Nil(t, repo.updated)
}

func TestAPIKeyService_UpdateRejectsCustomSystemPromptWhenUserScopeDisabled(t *testing.T) {
	prompt := "should not be saved"
	mode := SystemPromptModeAppend
	repo := &apiKeySystemPromptRepoStub{
		existing: &APIKey{ID: 10, UserID: 42, Key: "sk-existing", Name: "existing", Status: StatusActive},
	}
	svc := NewAPIKeyService(repo, &apiKeySystemPromptUserRepoStub{}, nil, nil, nil, nil, &config.Config{})
	svc.SetSystemPromptSettingsProvider(disabledSystemPromptPermissionProvider())

	_, err := svc.Update(context.Background(), 10, 42, UpdateAPIKeyRequest{
		SystemPrompt:     &prompt,
		SystemPromptMode: &mode,
	})

	require.ErrorIs(t, err, ErrSystemPromptConfigNotAllowed)
	require.Nil(t, repo.updated)
}
