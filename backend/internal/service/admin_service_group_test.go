//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func ptrString[T ~string](v T) *string {
	s := string(v)
	return &s
}

// groupRepoStubForAdmin 用于测试 AdminService 的 GroupRepository Stub
type groupRepoStubForAdmin struct {
	created *Group // 记录 Create 调用的参数
	updated *Group // 记录 Update 调用的参数
	getByID *Group // GetByID 返回值
	getErr  error  // GetByID 返回的错误

	listWithFiltersCalls            int
	listWithFiltersParams           pagination.PaginationParams
	listWithFiltersPlatform         string
	listWithFiltersStatus           string
	listWithFiltersSearch           string
	listWithFiltersIsExclusive      *bool
	listWithFiltersSubscriptionType string
	listWithFiltersGroups           []Group
	listWithFiltersResult           *pagination.PaginationResult
	listWithFiltersErr              error
}

func (s *groupRepoStubForAdmin) Create(_ context.Context, g *Group) error {
	s.created = g
	return nil
}

func (s *groupRepoStubForAdmin) Update(_ context.Context, g *Group) error {
	s.updated = g
	return nil
}

func (s *groupRepoStubForAdmin) GetByID(_ context.Context, _ int64) (*Group, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getByID, nil
}

func (s *groupRepoStubForAdmin) GetByIDLite(_ context.Context, _ int64) (*Group, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getByID, nil
}

func (s *groupRepoStubForAdmin) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStubForAdmin) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *groupRepoStubForAdmin) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForAdmin) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool, subscriptionType string) ([]Group, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersPlatform = platform
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search
	s.listWithFiltersIsExclusive = isExclusive
	s.listWithFiltersSubscriptionType = subscriptionType

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
	}

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersGroups)),
			Page:     params.Page,
			PageSize: params.PageSize,
		}
	}

	return s.listWithFiltersGroups, result, nil
}

func (s *groupRepoStubForAdmin) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForAdmin) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForAdmin) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForAdmin) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForAdmin) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForAdmin) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForAdmin) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForAdmin) UpdateSortOrders(_ context.Context, _ []GroupSortOrderUpdate) error {
	return nil
}

func TestAdminService_ListGroups_PassesSortParams(t *testing.T) {
	repo := &groupRepoStubForAdmin{
		listWithFiltersGroups: []Group{{ID: 1, Name: "g1"}},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, _, err := svc.ListGroups(context.Background(), 3, 25, PlatformOpenAI, StatusActive, "needle", nil, "account_count", "ASC", SubscriptionTypeStandard)
	require.NoError(t, err)
	require.Equal(t, pagination.PaginationParams{
		Page:      3,
		PageSize:  25,
		SortBy:    "account_count",
		SortOrder: "ASC",
	}, repo.listWithFiltersParams)
	require.Equal(t, SubscriptionTypeStandard, repo.listWithFiltersSubscriptionType)
}

// TestAdminService_CreateGroup_WithImagePricing 测试创建分组时 ImagePrice 字段正确传递
func TestAdminService_CreateGroup_WithImagePricing(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	price1K := 0.10
	price2K := 0.15
	price4K := 0.30

	input := &CreateGroupInput{
		Name:                 "test-group",
		Description:          "Test group",
		Platform:             PlatformAntigravity,
		RateMultiplier:       1.0,
		ImagePrice1K:         &price1K,
		ImagePrice2K:         &price2K,
		ImagePrice4K:         &price4K,
		AllowImageGeneration: true,
	}

	group, err := svc.CreateGroup(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证 repo 收到了正确的字段
	require.NotNil(t, repo.created)
	require.NotNil(t, repo.created.ImagePrice1K)
	require.NotNil(t, repo.created.ImagePrice2K)
	require.NotNil(t, repo.created.ImagePrice4K)
	require.True(t, group.AllowImageGeneration)
	require.True(t, repo.created.AllowImageGeneration)
	require.InDelta(t, 0.10, *repo.created.ImagePrice1K, 0.0001)
	require.InDelta(t, 0.15, *repo.created.ImagePrice2K, 0.0001)
	require.InDelta(t, 0.30, *repo.created.ImagePrice4K, 0.0001)
}

// TestAdminService_CreateGroup_NilImagePricing 测试 ImagePrice 为 nil 时正常创建
func TestAdminService_CreateGroup_NilImagePricing(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	input := &CreateGroupInput{
		Name:           "test-group",
		Description:    "Test group",
		Platform:       PlatformAntigravity,
		RateMultiplier: 1.0,
		// ImagePrice 字段全部为 nil
	}

	group, err := svc.CreateGroup(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证 ImagePrice 字段为 nil
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.ImagePrice1K)
	require.Nil(t, repo.created.ImagePrice2K)
	require.Nil(t, repo.created.ImagePrice4K)
}

// TestAdminService_UpdateGroup_WithImagePricing 测试更新分组时 ImagePrice 字段正确更新
func TestAdminService_UpdateGroup_WithImagePricing(t *testing.T) {
	existingGroup := &Group{
		ID:       1,
		Name:     "existing-group",
		Platform: PlatformAntigravity,
		Status:   StatusActive,
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	price1K := 0.12
	price2K := 0.18
	price4K := 0.36
	allowImageGeneration := true

	input := &UpdateGroupInput{
		ImagePrice1K:         &price1K,
		ImagePrice2K:         &price2K,
		ImagePrice4K:         &price4K,
		AllowImageGeneration: &allowImageGeneration,
	}

	group, err := svc.UpdateGroup(context.Background(), 1, input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证 repo 收到了更新后的字段
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.ImagePrice1K)
	require.NotNil(t, repo.updated.ImagePrice2K)
	require.NotNil(t, repo.updated.ImagePrice4K)
	require.True(t, repo.updated.AllowImageGeneration)
	require.InDelta(t, 0.12, *repo.updated.ImagePrice1K, 0.0001)
	require.InDelta(t, 0.18, *repo.updated.ImagePrice2K, 0.0001)
	require.InDelta(t, 0.36, *repo.updated.ImagePrice4K, 0.0001)
}

// TestAdminService_UpdateGroup_PartialImagePricing 测试仅更新部分 ImagePrice 字段
func TestAdminService_UpdateGroup_PartialImagePricing(t *testing.T) {
	oldPrice2K := 0.15
	existingGroup := &Group{
		ID:           1,
		Name:         "existing-group",
		Platform:     PlatformAntigravity,
		Status:       StatusActive,
		ImagePrice2K: &oldPrice2K, // 已有 2K 价格
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	// 只更新 1K 价格
	price1K := 0.10
	input := &UpdateGroupInput{
		ImagePrice1K: &price1K,
		// ImagePrice2K 和 ImagePrice4K 为 nil，不更新
	}

	group, err := svc.UpdateGroup(context.Background(), 1, input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证：1K 被更新，2K 保持原值，4K 仍为 nil
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.ImagePrice1K)
	require.InDelta(t, 0.10, *repo.updated.ImagePrice1K, 0.0001)
	require.NotNil(t, repo.updated.ImagePrice2K)
	require.InDelta(t, 0.15, *repo.updated.ImagePrice2K, 0.0001) // 原值保持
	require.Nil(t, repo.updated.ImagePrice4K)
}

func TestAdminService_CreateGroup_NormalizesMessagesDispatchModelConfig(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "dispatch-group",
		Description:    "dispatch config",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1.0,
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   " gpt-5.4-high ",
			SonnetMappedModel: " gpt-5.3-codex ",
			HaikuMappedModel:  " gpt-5.4-mini-medium ",
			ExactModelMappings: map[string]string{
				" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   "gpt-5.4",
		SonnetMappedModel: "gpt-5.3-codex",
		HaikuMappedModel:  "gpt-5.4-mini",
		ExactModelMappings: map[string]string{
			"claude-sonnet-4-5-20250929": "gpt-5.2",
		},
	}, repo.created.MessagesDispatchModelConfig)
}

func TestAdminService_UpdateGroup_NormalizesMessagesDispatchModelConfig(t *testing.T) {
	existingGroup := &Group{
		ID:       1,
		Name:     "existing-group",
		Platform: PlatformOpenAI,
		Status:   StatusActive,
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		MessagesDispatchModelConfig: &OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: " gpt-5.4-medium ",
			ExactModelMappings: map[string]string{
				" claude-haiku-4-5-20251001 ": " gpt-5.4-mini-high ",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{
		SonnetMappedModel: "gpt-5.4",
		ExactModelMappings: map[string]string{
			"claude-haiku-4-5-20251001": "gpt-5.4-mini",
		},
	}, repo.updated.MessagesDispatchModelConfig)
}

func TestAdminService_CreateGroup_ClearsMessagesDispatchFieldsForNonOpenAIPlatform(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                  "anthropic-group",
		Description:           "non-openai",
		Platform:              PlatformAnthropic,
		RateMultiplier:        1.0,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			OpusMappedModel: "gpt-5.4",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.False(t, repo.created.AllowMessagesDispatch)
	require.Empty(t, repo.created.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, repo.created.MessagesDispatchModelConfig)
}

func TestAdminService_UpdateGroup_ClearsMessagesDispatchFieldsWhenPlatformChangesAwayFromOpenAI(t *testing.T) {
	existingGroup := &Group{
		ID:                    1,
		Name:                  "existing-openai-group",
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: "gpt-5.3-codex",
		},
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		Platform: PlatformAnthropic,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Equal(t, PlatformAnthropic, repo.updated.Platform)
	require.False(t, repo.updated.AllowMessagesDispatch)
	require.Empty(t, repo.updated.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, repo.updated.MessagesDispatchModelConfig)
}

func TestAdminService_ListGroups_WithSearch(t *testing.T) {
	// 测试：
	// 1. search 参数正常传递到 repository 层
	// 2. search 为空字符串时的行为
	// 3. search 与其他过滤条件组合使用

	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{{ID: 1, Name: "alpha"}},
			listWithFiltersResult: &pagination.PaginationResult{Total: 1},
		}
		svc := &adminServiceImpl{groupRepo: repo}

		groups, total, err := svc.ListGroups(context.Background(), 1, 20, "", "", "alpha", nil, "", "", "")
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Equal(t, []Group{{ID: 1, Name: "alpha"}}, groups)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20}, repo.listWithFiltersParams)
		require.Equal(t, "alpha", repo.listWithFiltersSearch)
		require.Nil(t, repo.listWithFiltersIsExclusive)
	})

	t.Run("search 为空字符串时传递空字符串", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{},
			listWithFiltersResult: &pagination.PaginationResult{Total: 0},
		}
		svc := &adminServiceImpl{groupRepo: repo}

		groups, total, err := svc.ListGroups(context.Background(), 2, 10, "", "", "", nil, "", "", "")
		require.NoError(t, err)
		require.Empty(t, groups)
		require.Equal(t, int64(0), total)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 10}, repo.listWithFiltersParams)
		require.Equal(t, "", repo.listWithFiltersSearch)
		require.Nil(t, repo.listWithFiltersIsExclusive)
	})

	t.Run("search 与其他过滤条件组合使用", func(t *testing.T) {
		isExclusive := true
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{{ID: 2, Name: "beta"}},
			listWithFiltersResult: &pagination.PaginationResult{Total: 42},
		}
		svc := &adminServiceImpl{groupRepo: repo}

		groups, total, err := svc.ListGroups(context.Background(), 3, 50, PlatformAntigravity, StatusActive, "beta", &isExclusive, "", "", "")
		require.NoError(t, err)
		require.Equal(t, int64(42), total)
		require.Equal(t, []Group{{ID: 2, Name: "beta"}}, groups)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 3, PageSize: 50}, repo.listWithFiltersParams)
		require.Equal(t, PlatformAntigravity, repo.listWithFiltersPlatform)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "beta", repo.listWithFiltersSearch)
		require.NotNil(t, repo.listWithFiltersIsExclusive)
		require.True(t, *repo.listWithFiltersIsExclusive)
	})
}

func TestAdminService_ValidateFallbackGroup_RejectsChainedTarget(t *testing.T) {
	groupID := int64(1)
	fallbackID := int64(2)
	repo := &groupRepoStubForFallbackCycle{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformAnthropic,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
				FallbackGroupID:  &groupID,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	err := svc.validateFallbackGroup(context.Background(), groupID, PlatformAnthropic, fallbackID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot have fallback_group_id configured")
}

type groupRepoStubForFallbackCycle struct {
	groups map[int64]*Group
}

func (s *groupRepoStubForFallbackCycle) Create(_ context.Context, _ *Group) error {
	panic("unexpected Create call")
}

func (s *groupRepoStubForFallbackCycle) Update(_ context.Context, _ *Group) error {
	panic("unexpected Update call")
}

func (s *groupRepoStubForFallbackCycle) GetByID(ctx context.Context, id int64) (*Group, error) {
	return s.GetByIDLite(ctx, id)
}

func (s *groupRepoStubForFallbackCycle) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if g, ok := s.groups[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func (s *groupRepoStubForFallbackCycle) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStubForFallbackCycle) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *groupRepoStubForFallbackCycle) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForFallbackCycle) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _ string, _ *bool, _ string) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupRepoStubForFallbackCycle) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForFallbackCycle) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForFallbackCycle) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForFallbackCycle) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForFallbackCycle) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForFallbackCycle) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForFallbackCycle) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForFallbackCycle) UpdateSortOrders(_ context.Context, _ []GroupSortOrderUpdate) error {
	return nil
}

type groupRepoStubForInvalidRequestFallback struct {
	groups  map[int64]*Group
	created *Group
	updated *Group
}

type apiKeyRepoStubForFallbackGroupValidation struct {
	counts map[int64]int64
}

func (s *apiKeyRepoStubForFallbackGroupValidation) CountByGroupID(_ context.Context, groupID int64) (int64, error) {
	if s.counts == nil {
		return 0, nil
	}
	return s.counts[groupID], nil
}

func (s *apiKeyRepoStubForFallbackGroupValidation) Create(context.Context, *APIKey) error {
	panic("unexpected Create call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) GetByID(context.Context, int64) (*APIKey, error) {
	panic("unexpected GetByID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	panic("unexpected GetKeyAndOwnerID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) GetByKey(context.Context, string) (*APIKey, error) {
	panic("unexpected GetByKey call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	panic("unexpected GetByKeyForAuth call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) Update(context.Context, *APIKey) error {
	panic("unexpected Update call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) CountByUserID(context.Context, int64) (int64, error) {
	panic("unexpected CountByUserID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ExistsByKey(context.Context, string) (bool, error) {
	panic("unexpected ExistsByKey call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	panic("unexpected SearchAPIKeys call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	panic("unexpected UpdateGroupIDByUserAndGroup call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ListKeysByUserID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByUserID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByGroupID call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	panic("unexpected IncrementQuotaUsed call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) UpdateLastUsed(context.Context, int64, time.Time) error {
	panic("unexpected UpdateLastUsed call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) IncrementRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementRateLimitUsage call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) ResetRateLimitWindows(context.Context, int64) error {
	panic("unexpected ResetRateLimitWindows call")
}

func (s *apiKeyRepoStubForFallbackGroupValidation) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	panic("unexpected GetRateLimitData call")
}

func (s *groupRepoStubForInvalidRequestFallback) Create(_ context.Context, g *Group) error {
	s.created = g
	return nil
}

func (s *groupRepoStubForInvalidRequestFallback) Update(_ context.Context, g *Group) error {
	s.updated = g
	return nil
}

func (s *groupRepoStubForInvalidRequestFallback) GetByID(ctx context.Context, id int64) (*Group, error) {
	return s.GetByIDLite(ctx, id)
}

func (s *groupRepoStubForInvalidRequestFallback) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if g, ok := s.groups[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func (s *groupRepoStubForInvalidRequestFallback) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStubForInvalidRequestFallback) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *groupRepoStubForInvalidRequestFallback) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForInvalidRequestFallback) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _ string, _ *bool, _ string) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupRepoStubForInvalidRequestFallback) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForInvalidRequestFallback) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForInvalidRequestFallback) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForInvalidRequestFallback) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForInvalidRequestFallback) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForInvalidRequestFallback) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForInvalidRequestFallback) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForInvalidRequestFallback) UpdateSortOrders(_ context.Context, _ []GroupSortOrderUpdate) error {
	return nil
}

func TestAdminService_CreateGroup_InvalidRequestFallbackIgnoredWhenDeprecatedFieldSent(t *testing.T) {
	fallbackID := int64(10)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard, Status: StatusActive},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_CreateGroup_InvalidRequestFallbackIgnoresMissingTarget(t *testing.T) {
	fallbackID := int64(999)
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformOpenAI,
		SubscriptionType:                SubscriptionTypeSubscription,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_CreateGroup_PersistsFallbackGroupFlag(t *testing.T) {
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                 "fallback-openai",
		Platform:             PlatformOpenAI,
		SubscriptionType:     SubscriptionTypeStandard,
		IsFallbackGroup:      true,
		RateMultiplier:       1,
		SystemPromptMode:     "inherit",
		SupportedModelScopes: []string{},
	})

	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.IsFallbackGroup)
}

func TestAdminService_CreateGroup_FallbackGroupFlagRejectsUnsupportedPlatform(t *testing.T) {
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "gemini-fallback",
		Platform:         PlatformGemini,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group flag only supported for anthropic or openai standard groups")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackGroupFlagRejectsSubscription(t *testing.T) {
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-sub-fallback",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeSubscription,
		IsFallbackGroup:  true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group must be standard billing type")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackGroupFlagRejectsChainedFallback(t *testing.T) {
	fallbackID := int64(19)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "fallback-with-fallback",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup:  true,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot have fallback_group_id configured")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetRejectsInactive(t *testing.T) {
	fallbackID := int64(18)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusDisabled,
				IsFallbackGroup:  true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group must be active")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetRejectsSubscription(t *testing.T) {
	fallbackID := int64(17)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				SubscriptionType: SubscriptionTypeSubscription,
				Status:           StatusActive,
				IsFallbackGroup:  true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot be subscription type")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetRejectsChainedFallback(t *testing.T) {
	nextID := int64(30)
	fallbackID := int64(16)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
				FallbackGroupID:  &nextID,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot have fallback_group_id configured")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetRejectsClaudeCodeOnly(t *testing.T) {
	fallbackID := int64(15)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformAnthropic,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
				ClaudeCodeOnly:   true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "anthropic-entry",
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot have claude_code_only enabled")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetRequiresEnabledFlag(t *testing.T) {
	fallbackID := int64(20)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {ID: fallbackID, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, Status: StatusActive},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group must be enabled as fallback group")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetMustMatchPlatform(t *testing.T) {
	fallbackID := int64(21)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformAnthropic,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai group fallback target must be openai platform")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_OpenAICompatibleEntryAllowsOpenAIFallbackTarget(t *testing.T) {
	fallbackID := int64(211)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "codex2api-entry",
		Platform:         PlatformCodex2API,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.NotNil(t, repo.created.FallbackGroupID)
	require.Equal(t, fallbackID, *repo.created.FallbackGroupID)
}

func TestAdminService_CreateGroup_MultipleGroupsMayShareFallbackTarget(t *testing.T) {
	fallbackID := int64(22)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformOpenAI,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
				IsFallbackGroup:  true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	first, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry-1",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry-2",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, second)
}

func TestAdminService_CreateGroup_InvalidRequestFallbackClearsOnZero(t *testing.T) {
	zero := int64(0)
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		FallbackGroupIDOnInvalidRequest: &zero,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackClearsExistingValueWithoutInput(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:                              1,
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		Status:                          StatusActive,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		Platform: PlatformOpenAI,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackClearsOnZero(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:                              1,
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		Status:                          StatusActive,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	clear := int64(0)
	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		Platform:                        PlatformOpenAI,
		FallbackGroupIDOnInvalidRequest: &clear,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackClearedWhenDeprecatedFieldSent(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID: {
				ID:               fallbackID,
				Platform:         PlatformAnthropic,
				SubscriptionType: SubscriptionTypeStandard,
				Status:           StatusActive,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})

	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_FallbackGroupFlagRejectsBoundAPIKeys(t *testing.T) {
	existing := &Group{
		ID:               101,
		Name:             "openai-existing",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
		},
	}
	apiKeyRepo := &apiKeyRepoStubForFallbackGroupValidation{
		counts: map[int64]int64{
			existing.ID: 2,
		},
	}
	svc := &adminServiceImpl{groupRepo: repo, apiKeyRepo: apiKeyRepo}

	enable := true
	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		IsFallbackGroup: &enable,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group has bound api keys; migrate or unbind them first")
	require.Nil(t, repo.updated)
}

func TestAdminService_UpdateGroup_FallbackGroupFlagAllowsNonEnablementEdits(t *testing.T) {
	existing := &Group{
		ID:               103,
		Name:             "openai-existing",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
		IsFallbackGroup:  true,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
		},
	}
	apiKeyRepo := &apiKeyRepoStubForFallbackGroupValidation{
		counts: map[int64]int64{
			existing.ID: 2,
		},
	}
	svc := &adminServiceImpl{groupRepo: repo, apiKeyRepo: apiKeyRepo}

	t.Run("keep nil fallback flag", func(t *testing.T) {
		group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
			Name:   "openai-existing-renamed",
			Status: StatusActive,
		})

		require.NoError(t, err)
		require.NotNil(t, group)
		require.NotNil(t, repo.updated)
		require.Equal(t, "openai-existing-renamed", repo.updated.Name)
		require.True(t, repo.updated.IsFallbackGroup)
	})

	repo.updated = nil

	t.Run("explicit true does not revalidate bindings", func(t *testing.T) {
		enable := true
		group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
			Name:            "openai-existing-retouched",
			Status:          StatusActive,
			IsFallbackGroup: &enable,
		})

		require.NoError(t, err)
		require.NotNil(t, group)
		require.NotNil(t, repo.updated)
		require.Equal(t, "openai-existing-retouched", repo.updated.Name)
		require.True(t, repo.updated.IsFallbackGroup)
	})
}

func TestAdminService_UpdateGroup_FallbackGroupFlagRejectsExistingFallbackTarget(t *testing.T) {
	fallbackID := int64(202)
	existing := &Group{
		ID:               102,
		Name:             "openai-existing",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
		FallbackGroupID:  &fallbackID,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	enable := true
	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		IsFallbackGroup: &enable,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot have fallback_group_id configured")
	require.Nil(t, repo.updated)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackRejectsFallbackGroup(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeSubscription},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackIgnoresDeprecatedFieldWithValidTarget(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackIgnoresDeprecatedFieldForAntigravity(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAntigravity,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}
