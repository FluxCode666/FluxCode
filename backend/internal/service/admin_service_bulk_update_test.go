//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForBulkUpdate struct {
	accountRepoStub
	bulkUpdateErr    error
	bulkUpdateIDs    []int64
	bindGroupErrByID map[int64]error
	bindGroupsCalls  []int64
	getByIDsAccounts []*Account
	getByIDsErr      error
	getByIDsCalled   bool
	getByIDsIDs      []int64
	getByIDAccounts  map[int64]*Account
	getByIDErrByID   map[int64]error
	getByIDCalled    []int64
	listByGroupData  map[int64][]Account
	listByGroupErr   map[int64]error
}

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{}, ids...)
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
	}
	return int64(len(ids)), nil
}

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, _ []int64) error {
	s.bindGroupsCalls = append(s.bindGroupsCalls, accountID)
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
	}
	return nil
}

func (s *accountRepoStubForBulkUpdate) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	s.getByIDsCalled = true
	s.getByIDsIDs = append([]int64{}, ids...)
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
	}
	return s.getByIDsAccounts, nil
}

func (s *accountRepoStubForBulkUpdate) GetByID(_ context.Context, id int64) (*Account, error) {
	s.getByIDCalled = append(s.getByIDCalled, id)
	if err, ok := s.getByIDErrByID[id]; ok {
		return nil, err
	}
	if account, ok := s.getByIDAccounts[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (s *accountRepoStubForBulkUpdate) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	if err, ok := s.listByGroupErr[groupID]; ok {
		return nil, err
	}
	if rows, ok := s.listByGroupData[groupID]; ok {
		return rows, nil
	}
	return nil, nil
}

// TestAdminService_BulkUpdateAccounts_AllSuccessIDs 验证批量更新成功时返回 success_ids/failed_ids。
func TestAdminService_BulkUpdateAccounts_AllSuccessIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2, 3},
		Schedulable: &schedulable,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.ElementsMatch(t, []int64{1, 2, 3}, result.SuccessIDs)
	require.Empty(t, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

// TestAdminService_BulkUpdateAccounts_PartialFailureIDs 验证部分失败时 success_ids/failed_ids 正确。
func TestAdminService_BulkUpdateAccounts_PartialFailureIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		bindGroupErrByID: map[int64]error{
			2: errors.New("bind failed"),
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"}},
	}

	groupIDs := []int64{10}
	schedulable := false
	input := &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2, 3},
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.ElementsMatch(t, []int64{1, 3}, result.SuccessIDs)
	require.ElementsMatch(t, []int64{2}, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

func TestAdminService_BulkUpdateAccounts_NilGroupRepoReturnsError(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group repository not configured")
}

// TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict verifies
// that the global pre-check detects a conflict with existing group members and returns an
// error before any DB write is performed.
func TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAntigravity},
		},
		// Group 10 already contains an Anthropic account.
		listByGroupData: map[int64][]Account{
			10: {{ID: 99, Platform: PlatformAnthropic}},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo: &groupRepoStubForAdmin{getByID: &Group{
			ID: 10, Name: "target-group", Platform: PlatformAntigravity,
		}},
	}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed channel")
	// No BindGroups should have been called since the check runs before any write.
	require.Empty(t, repo.bindGroupsCalls)
}

func TestAdminService_BulkUpdateAccounts_RejectsMediaConfigPatch(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		Extra: map[string]any{
			mediaAccountConfigExtraKey: map[string]any{"version": 1},
		},
	})

	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "MEDIA_CONFIG_BULK_UPDATE_UNSUPPORTED", infraerrors.Reason(err))
	require.False(t, repo.getByIDsCalled)
	require.Empty(t, repo.bulkUpdateIDs)
}

func TestAdminService_BulkUpdateAccounts_ValidatesCredentialPatchWithoutMutatingLoadedAccount(t *testing.T) {
	tests := []struct {
		name       string
		account    *Account
		wantReason string
	}{
		{
			name: "media account",
			account: &Account{
				ID: 1, Platform: PlatformMedia, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "media-key", "base_url": "https://media.example.com"},
				Extra: map[string]any{mediaAccountConfigExtraKey: map[string]any{
					"version": 1, "provider": "xai", "models": map[string]any{
						"grok-image": map[string]any{
							"enabled": true, "upstream_model_id": "grok-image-upstream", "async_mode": "unsupported",
						},
					},
				}},
			},
			wantReason: "INVALID_MEDIA_ACCOUNT",
		},
		{
			name: "codex2api account",
			account: &Account{
				ID: 2, Platform: PlatformCodex2API, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "codex-key", "base_url": "https://codex.example.com"},
			},
			wantReason: "INVALID_CODEX2API_ACCOUNT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{tt.account}}
			svc := &adminServiceImpl{accountRepo: repo}

			result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
				AccountIDs:  []int64{tt.account.ID},
				Credentials: map[string]any{"api_key": nil},
			})

			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
			require.Equal(t, tt.wantReason, infraerrors.Reason(err))
			require.True(t, repo.getByIDsCalled)
			require.Empty(t, repo.bulkUpdateIDs)
			require.NotNil(t, tt.account.Credentials["api_key"])
		})
	}
}
