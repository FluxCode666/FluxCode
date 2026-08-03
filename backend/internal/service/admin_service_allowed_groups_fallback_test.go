//go:build unit

package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userRepoStubForAllowedGroups struct {
	userRepoStub
	updated *User
}

func (s *userRepoStubForAllowedGroups) Update(_ context.Context, user *User) error {
	clone := *user
	s.updated = &clone
	return nil
}

type groupRepoStubForAllowedGroups struct {
	group          *Group
	lastGetByIDArg int64
}

func (s *groupRepoStubForAllowedGroups) GetByID(context.Context, int64) (*Group, error) {
	panic("unexpected")
}

func (s *groupRepoStubForAllowedGroups) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	s.lastGetByIDArg = id
	if s.group == nil {
		return nil, ErrGroupNotFound
	}
	clone := *s.group
	return &clone, nil
}

func (s *groupRepoStubForAllowedGroups) Create(context.Context, *Group) error { panic("unexpected") }
func (s *groupRepoStubForAllowedGroups) Update(context.Context, *Group) error { panic("unexpected") }
func (s *groupRepoStubForAllowedGroups) Delete(context.Context, int64) error  { panic("unexpected") }
func (s *groupRepoStubForAllowedGroups) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool, string) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) ListActive(context.Context) ([]Group, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected")
}
func (s *groupRepoStubForAllowedGroups) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected")
}

func TestAdminService_CreateUser_RejectsFallbackAllowedGroup(t *testing.T) {
	repo := &userRepoStub{nextID: 10}
	groupRepo := &groupRepoStubForAllowedGroups{
		group: &Group{ID: 9, Status: StatusActive, IsFallbackGroup: true},
	}
	svc := &adminServiceImpl{userRepo: repo, groupRepo: groupRepo}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:         "user@test.com",
		Password:      "strong-pass",
		AllowedGroups: []int64{9},
	})

	require.Error(t, err)
	require.Equal(t, "FALLBACK_GROUP_NOT_ASSIGNABLE", infraerrors.Reason(err))
	require.Empty(t, repo.created)
}

func TestAdminService_UpdateUser_RejectsFallbackAllowedGroup(t *testing.T) {
	repo := &userRepoStubForAllowedGroups{
		userRepoStub: userRepoStub{
			user: &User{ID: 10, Email: "user@test.com", Status: StatusActive, AllowedGroups: []int64{1}},
		},
	}
	groupRepo := &groupRepoStubForAllowedGroups{
		group: &Group{ID: 9, Status: StatusActive, IsFallbackGroup: true},
	}
	svc := &adminServiceImpl{userRepo: repo, groupRepo: groupRepo}
	allowed := []int64{9}

	_, err := svc.UpdateUser(context.Background(), 10, &UpdateUserInput{
		AllowedGroups: &allowed,
	})

	require.Error(t, err)
	require.Equal(t, "FALLBACK_GROUP_NOT_ASSIGNABLE", infraerrors.Reason(err))
	require.Nil(t, repo.updated)
}
