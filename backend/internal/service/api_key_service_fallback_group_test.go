//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type apiKeyFallbackRepoStub struct {
	created  *APIKey
	existing *APIKey
	updated  *APIKey
}

func (s *apiKeyFallbackRepoStub) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.created = &clone
	return nil
}

func (s *apiKeyFallbackRepoStub) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	if s.existing == nil {
		return nil, ErrAPIKeyNotFound
	}
	clone := *s.existing
	return &clone, nil
}

func (s *apiKeyFallbackRepoStub) GetKeyAndOwnerID(_ context.Context, _ int64) (string, int64, error) {
	if s.existing == nil {
		return "", 0, ErrAPIKeyNotFound
	}
	return s.existing.Key, s.existing.UserID, nil
}

func (s *apiKeyFallbackRepoStub) GetByKey(context.Context, string) (*APIKey, error) {
	return nil, ErrAPIKeyNotFound
}

func (s *apiKeyFallbackRepoStub) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	return nil, ErrAPIKeyNotFound
}

func (s *apiKeyFallbackRepoStub) Update(_ context.Context, key *APIKey) error {
	clone := *key
	s.updated = &clone
	return nil
}

func (s *apiKeyFallbackRepoStub) Delete(context.Context, int64) error { return nil }

func (s *apiKeyFallbackRepoStub) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeyFallbackRepoStub) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	return nil, nil
}

func (s *apiKeyFallbackRepoStub) CountByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeyFallbackRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

func (s *apiKeyFallbackRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeyFallbackRepoStub) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	return nil, nil
}

func (s *apiKeyFallbackRepoStub) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeyFallbackRepoStub) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeyFallbackRepoStub) CountByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeyFallbackRepoStub) ListKeysByUserID(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *apiKeyFallbackRepoStub) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *apiKeyFallbackRepoStub) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	return 0, nil
}

func (s *apiKeyFallbackRepoStub) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
}

func (s *apiKeyFallbackRepoStub) IncrementRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func (s *apiKeyFallbackRepoStub) ResetRateLimitWindows(context.Context, int64) error {
	return nil
}

func (s *apiKeyFallbackRepoStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return nil, nil
}

type apiKeyFallbackUserRepoStub struct {
	user *User
}

func (s *apiKeyFallbackUserRepoStub) Create(context.Context, *User) error { return nil }

func (s *apiKeyFallbackUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	if s.user != nil {
		clone := *s.user
		return &clone, nil
	}
	return &User{ID: id, Status: StatusActive}, nil
}

func (s *apiKeyFallbackUserRepoStub) GetByEmail(context.Context, string) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *apiKeyFallbackUserRepoStub) GetFirstAdmin(context.Context) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *apiKeyFallbackUserRepoStub) Update(context.Context, *User) error { return nil }
func (s *apiKeyFallbackUserRepoStub) Delete(context.Context, int64) error { return nil }

func (s *apiKeyFallbackUserRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeyFallbackUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeyFallbackUserRepoStub) UpdateBalance(context.Context, int64, float64) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) DeductBalance(context.Context, int64, float64) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) UpdateConcurrency(context.Context, int64, int) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}

func (s *apiKeyFallbackUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeyFallbackUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) EnableTotp(context.Context, int64) error  { return nil }
func (s *apiKeyFallbackUserRepoStub) DisableTotp(context.Context, int64) error { return nil }

func (s *apiKeyFallbackUserRepoStub) GetByReferralCode(context.Context, string) (*User, error) {
	return nil, ErrUserNotFound
}

func (s *apiKeyFallbackUserRepoStub) UpdateReferralCode(context.Context, int64, string) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) UpdateReferredBy(context.Context, int64, int64) error {
	return nil
}

func (s *apiKeyFallbackUserRepoStub) IsFirstRecharge(context.Context, int64) (bool, error) {
	return false, nil
}

func (s *apiKeyFallbackUserRepoStub) ListActiveUserIDs(context.Context) ([]int64, error) {
	return nil, nil
}

type apiKeyFallbackGroupRepoStub struct {
	group *Group
}

func (s *apiKeyFallbackGroupRepoStub) Create(context.Context, *Group) error { return nil }

func (s *apiKeyFallbackGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	if s.group == nil {
		return nil, ErrGroupNotFound
	}
	clone := *s.group
	return &clone, nil
}

func (s *apiKeyFallbackGroupRepoStub) GetByIDLite(context.Context, int64) (*Group, error) {
	if s.group == nil {
		return nil, ErrGroupNotFound
	}
	clone := *s.group
	return &clone, nil
}

func (s *apiKeyFallbackGroupRepoStub) Update(context.Context, *Group) error { return nil }
func (s *apiKeyFallbackGroupRepoStub) Delete(context.Context, int64) error  { return nil }

func (s *apiKeyFallbackGroupRepoStub) DeleteCascade(context.Context, int64) ([]int64, error) {
	return nil, nil
}

func (s *apiKeyFallbackGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeyFallbackGroupRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool, string) ([]Group, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *apiKeyFallbackGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	return nil, nil
}

func (s *apiKeyFallbackGroupRepoStub) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	return nil, nil
}

func (s *apiKeyFallbackGroupRepoStub) ExistsByName(context.Context, string) (bool, error) {
	return false, nil
}

func (s *apiKeyFallbackGroupRepoStub) GetAccountCount(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}

func (s *apiKeyFallbackGroupRepoStub) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (s *apiKeyFallbackGroupRepoStub) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	return nil, nil
}

func (s *apiKeyFallbackGroupRepoStub) BindAccountsToGroup(context.Context, int64, []int64) error {
	return nil
}

func (s *apiKeyFallbackGroupRepoStub) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	return nil
}

func newAPIKeyServiceForGroupBindingTest(user *User, group *Group, existing *APIKey) *APIKeyService {
	return NewAPIKeyService(
		&apiKeyFallbackRepoStub{existing: existing},
		&apiKeyFallbackUserRepoStub{user: user},
		&apiKeyFallbackGroupRepoStub{group: group},
		userSubRepoNoop{},
		nil,
		nil,
		&config.Config{},
	)
}

func TestAPIKeyService_CreateRejectsFallbackGroup(t *testing.T) {
	groupID := int64(10)
	user := &User{ID: 42, Status: StatusActive}
	group := &Group{
		ID:               groupID,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	svc := newAPIKeyServiceForGroupBindingTest(user, group, nil)

	_, err := svc.Create(context.Background(), user.ID, CreateAPIKeyRequest{
		Name:    "test",
		GroupID: &groupID,
	})

	require.ErrorIs(t, err, ErrGroupNotAllowed)
}

func TestAPIKeyService_UpdateRejectsFallbackGroup(t *testing.T) {
	groupID := int64(10)
	user := &User{ID: 42, Status: StatusActive}
	group := &Group{
		ID:               groupID,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	}
	svc := newAPIKeyServiceForGroupBindingTest(user, group, &APIKey{ID: 100, UserID: user.ID, Key: "sk-test"})

	_, err := svc.Update(context.Background(), 100, user.ID, UpdateAPIKeyRequest{
		GroupID: &groupID,
	})

	require.ErrorIs(t, err, ErrGroupNotAllowed)
}
