package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateAccountGroupBindingAllowsCrossPlatformOnlyForMediaGroups(t *testing.T) {
	mediaGroup := &Group{
		Name:                      "media",
		Platform:                  PlatformOpenAI,
		MediaCrossPlatformEnabled: true,
	}
	require.NoError(t, validateAccountGroupBinding(mediaGroup, PlatformGemini, AccountTypeAPIKey))

	plainGroup := &Group{Name: "plain", Platform: PlatformOpenAI}
	require.Error(t, validateAccountGroupBinding(plainGroup, PlatformGemini, AccountTypeAPIKey))
}

func TestValidateAccountGroupBindingKeepsMediaPlatformFullyIsolated(t *testing.T) {
	legacyCrossPlatformTextGroup := &Group{
		Name:                      "legacy-cross-platform",
		Platform:                  PlatformOpenAI,
		MediaCrossPlatformEnabled: true,
	}
	require.Error(t, validateAccountGroupBinding(legacyCrossPlatformTextGroup, PlatformMedia, AccountTypeAPIKey))

	mediaGroup := &Group{
		Name:                      "media",
		Platform:                  PlatformMedia,
		MediaCrossPlatformEnabled: true,
	}
	require.Error(t, validateAccountGroupBinding(mediaGroup, PlatformOpenAI, AccountTypeAPIKey))
	require.NoError(t, validateAccountGroupBinding(mediaGroup, PlatformMedia, AccountTypeAPIKey))
}

func TestTextGatewaySchedulersRejectMediaGroupsBeforeAccountSelection(t *testing.T) {
	groupID := int64(7)
	groupRepo := &mediaGroupRepoStub{group: &Group{
		ID: groupID, Platform: PlatformMedia, Status: StatusActive,
	}}

	t.Run("generic selector", func(t *testing.T) {
		svc := &GatewayService{groupRepo: groupRepo}
		account, err := svc.SelectAccountForModelWithExclusions(
			context.Background(), &groupID, "", "gpt-4", nil,
		)
		require.Nil(t, account)
		require.ErrorIs(t, err, ErrMediaGroupTextGatewayUnsupported)
	})

	t.Run("load aware selector", func(t *testing.T) {
		svc := &GatewayService{groupRepo: groupRepo}
		selection, err := svc.SelectAccountWithLoadAwareness(
			context.Background(), &groupID, "", "claude-sonnet", nil, "", 0,
		)
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrMediaGroupTextGatewayUnsupported)
	})

	t.Run("gemini compatibility selector", func(t *testing.T) {
		svc := &GeminiMessagesCompatService{groupRepo: groupRepo}
		account, err := svc.SelectAccountForModelWithExclusions(
			context.Background(), &groupID, "", "gemini-2.5-pro", nil,
		)
		require.Nil(t, account)
		require.ErrorIs(t, err, ErrMediaGroupTextGatewayUnsupported)
	})
}

func TestValidateAccountGroupBindingKeepsCompatiblePlatformRules(t *testing.T) {
	compatibleGroup := &Group{
		Name:             "compatible",
		Platform:         PlatformGemini,
		RequireOAuthOnly: true,
	}

	// Antigravity 账号原本就兼容 Gemini 分组，因此仍应用 OAuth-only 规则。
	require.Error(t, validateAccountGroupBinding(compatibleGroup, PlatformAntigravity, AccountTypeAPIKey))
	require.NoError(t, validateAccountGroupBinding(compatibleGroup, PlatformAntigravity, AccountTypeOAuth))
}

func TestValidateAccountGroupBindingSkipsOAuthOnlyForApprovedCrossPlatformBinding(t *testing.T) {
	mediaGroup := &Group{
		Name:                      "media",
		Platform:                  PlatformOpenAI,
		RequireOAuthOnly:          true,
		MediaCrossPlatformEnabled: true,
	}

	require.NoError(t, validateAccountGroupBinding(mediaGroup, PlatformGemini, AccountTypeAPIKey))
}

type mediaGroupRepoStub struct {
	GroupRepository
	group             *Group
	groups            map[int64]*Group
	sourceAccountIDs  []int64
	created           *Group
	updated           *Group
	boundAccountIDs   []int64
	deleteBindingCall int
}

func (s *mediaGroupRepoStub) Create(_ context.Context, group *Group) error {
	group.ID = 99
	s.created = group
	return nil
}

func (s *mediaGroupRepoStub) GetByID(_ context.Context, id int64) (*Group, error) {
	if s.groups != nil {
		return s.groups[id], nil
	}
	return s.group, nil
}

func (s *mediaGroupRepoStub) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return s.GetByID(ctx, id)
}

func (s *mediaGroupRepoStub) Update(_ context.Context, group *Group) error {
	s.updated = group
	return nil
}

func (s *mediaGroupRepoStub) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	return append([]int64(nil), s.sourceAccountIDs...), nil
}

func (s *mediaGroupRepoStub) BindAccountsToGroup(_ context.Context, _ int64, accountIDs []int64) error {
	s.boundAccountIDs = append([]int64(nil), accountIDs...)
	return nil
}

func (s *mediaGroupRepoStub) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	s.deleteBindingCall++
	return 0, nil
}

type mediaGroupAccountRepoStub struct {
	AccountRepository
	accounts []*Account
}

func (s *mediaGroupAccountRepoStub) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	return s.accounts, nil
}

func TestAdminServiceCreateGroupPersistsMediaFlags(t *testing.T) {
	repo := &mediaGroupRepoStub{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                      "media",
		Platform:                  PlatformOpenAI,
		AllowImageGeneration:      true,
		AllowVideoGeneration:      true,
		MediaCrossPlatformEnabled: true,
	})

	require.NoError(t, err)
	require.Same(t, group, repo.created)
	require.True(t, repo.created.AllowImageGeneration)
	require.True(t, repo.created.AllowVideoGeneration)
	require.True(t, repo.created.MediaCrossPlatformEnabled)
}

func TestAdminServiceUpdateGroupPersistsExplicitFalseMediaFlags(t *testing.T) {
	repo := &mediaGroupRepoStub{group: &Group{
		ID:                        7,
		Name:                      "media",
		Platform:                  PlatformOpenAI,
		Status:                    StatusActive,
		AllowImageGeneration:      true,
		AllowVideoGeneration:      true,
		MediaCrossPlatformEnabled: true,
	}}
	svc := &adminServiceImpl{groupRepo: repo}
	disabled := false

	group, err := svc.UpdateGroup(context.Background(), 7, &UpdateGroupInput{
		AllowImageGeneration:      &disabled,
		AllowVideoGeneration:      &disabled,
		MediaCrossPlatformEnabled: &disabled,
	})

	require.NoError(t, err)
	require.Same(t, group, repo.updated)
	require.False(t, repo.updated.AllowImageGeneration)
	require.False(t, repo.updated.AllowVideoGeneration)
	require.False(t, repo.updated.MediaCrossPlatformEnabled)
}

func TestAdminServiceUpdateGroupRejectsMediaPlatformBoundaryChange(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		{name: "text to media", from: PlatformOpenAI, to: PlatformMedia},
		{name: "media to text", from: PlatformMedia, to: PlatformOpenAI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := &Group{ID: 7, Name: "group", Platform: tt.from, Status: StatusActive}
			repo := &mediaGroupRepoStub{group: group}
			svc := &adminServiceImpl{groupRepo: repo}

			updated, err := svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{Platform: tt.to})

			require.Nil(t, updated)
			require.Equal(t, "MEDIA_GROUP_PLATFORM_IMMUTABLE", infraerrors.Reason(err))
			require.Nil(t, repo.updated)
			require.Equal(t, tt.from, group.Platform)
		})
	}
}

func TestAdminServiceCreateGroupCopyKeepsCrossPlatformAPIKeyAndFiltersCompatibleAPIKey(t *testing.T) {
	repo := &mediaGroupRepoStub{
		groups:           map[int64]*Group{1: {ID: 1, Platform: PlatformGemini}},
		sourceAccountIDs: []int64{1, 2, 3},
	}
	accountRepo := &mediaGroupAccountRepoStub{accounts: []*Account{
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}}
	svc := &adminServiceImpl{groupRepo: repo, accountRepo: accountRepo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                      "media",
		Platform:                  PlatformOpenAI,
		RequireOAuthOnly:          true,
		MediaCrossPlatformEnabled: true,
		CopyAccountsFromGroupIDs:  []int64{1},
	})

	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, []int64{1, 3}, repo.boundAccountIDs)
}

func TestAdminServiceCreateGroupCopyFromDifferentPlatformRejectsCrossPlatformAccountForPlainGroup(t *testing.T) {
	repo := &mediaGroupRepoStub{
		groups:           map[int64]*Group{1: {ID: 1, Platform: PlatformGemini}},
		sourceAccountIDs: []int64{1},
	}
	accountRepo := &mediaGroupAccountRepoStub{accounts: []*Account{
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey},
	}}
	svc := &adminServiceImpl{groupRepo: repo, accountRepo: accountRepo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                     "plain",
		Platform:                 PlatformOpenAI,
		CopyAccountsFromGroupIDs: []int64{1},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "账号平台")
	require.Nil(t, repo.created)
	require.Empty(t, repo.boundAccountIDs)
}

func TestAdminServiceUpdateGroupCopyAppliesCrossPlatformBindingRules(t *testing.T) {
	tests := []struct {
		name              string
		mediaEnabled      bool
		wantErr           bool
		wantErrorContains string
		wantBoundIDs      []int64
		wantDeleteBinding int
	}{
		{
			name:              "different source platform keeps every legal binding and filters compatible apikey",
			mediaEnabled:      true,
			wantBoundIDs:      []int64{1, 3},
			wantDeleteBinding: 1,
		},
		{
			name:              "different source platform plain group rejects before side effects",
			mediaEnabled:      false,
			wantErr:           true,
			wantErrorContains: "账号平台",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destination := &Group{
				ID:                        9,
				Name:                      "destination",
				Platform:                  PlatformOpenAI,
				Status:                    StatusActive,
				RequireOAuthOnly:          true,
				MediaCrossPlatformEnabled: tt.mediaEnabled,
			}
			repo := &mediaGroupRepoStub{
				groups: map[int64]*Group{
					1: {ID: 1, Platform: PlatformGemini},
					9: destination,
				},
				sourceAccountIDs: []int64{1, 2, 3},
			}
			accountRepo := &mediaGroupAccountRepoStub{accounts: []*Account{
				{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey},
				{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
				{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			}}
			svc := &adminServiceImpl{groupRepo: repo, accountRepo: accountRepo}

			_, err := svc.UpdateGroup(context.Background(), 9, &UpdateGroupInput{
				CopyAccountsFromGroupIDs: []int64{1},
			})

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrorContains)
				require.Nil(t, repo.updated)
			} else {
				require.NoError(t, err)
				require.NotNil(t, repo.updated)
			}
			require.Equal(t, tt.wantBoundIDs, repo.boundAccountIDs)
			require.Equal(t, tt.wantDeleteBinding, repo.deleteBindingCall)
		})
	}
}
