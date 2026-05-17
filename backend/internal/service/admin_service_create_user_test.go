//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAdminService_CreateUser_Success(t *testing.T) {
	repo := &userRepoStub{nextID: 10}
	svc := &adminServiceImpl{userRepo: repo}

	input := &CreateUserInput{
		Email:         "user@test.com",
		Password:      "strong-pass",
		Username:      "tester",
		Notes:         "note",
		Balance:       12.5,
		Concurrency:   7,
		AllowedGroups: []int64{3, 5},
	}

	user, err := svc.CreateUser(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, int64(10), user.ID)
	require.Equal(t, input.Email, user.Email)
	require.Equal(t, input.Username, user.Username)
	require.Equal(t, input.Notes, user.Notes)
	require.Equal(t, input.Balance, user.Balance)
	require.Equal(t, input.Concurrency, user.Concurrency)
	require.Equal(t, input.AllowedGroups, user.AllowedGroups)
	require.Equal(t, RoleUser, user.Role)
	require.Equal(t, StatusActive, user.Status)
	require.True(t, user.CheckPassword(input.Password))
	require.Len(t, repo.created, 1)
	require.Equal(t, user, repo.created[0])
}

func TestAdminService_CreateUser_EmailExists(t *testing.T) {
	repo := &userRepoStub{createErr: ErrEmailExists}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "dup@test.com",
		Password: "password",
	})
	require.ErrorIs(t, err, ErrEmailExists)
	require.Empty(t, repo.created)
}

func TestAdminService_CreateUser_CreateError(t *testing.T) {
	createErr := errors.New("db down")
	repo := &userRepoStub{createErr: createErr}
	svc := &adminServiceImpl{userRepo: repo}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "user@test.com",
		Password: "password",
	})
	require.ErrorIs(t, err, createErr)
	require.Empty(t, repo.created)
}

func TestAdminService_CreateUser_AssignsDefaultSubscriptions(t *testing.T) {
	repo := &userRepoStub{nextID: 21}
	assigner := &defaultSubscriptionAssignerStub{}
	cfg := &config.Config{
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	settingService := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyDefaultSubscriptions: `[{"group_id":5,"validity_days":30}]`,
	}}, cfg)
	svc := &adminServiceImpl{
		userRepo:           repo,
		settingService:     settingService,
		defaultSubAssigner: assigner,
	}

	_, err := svc.CreateUser(context.Background(), &CreateUserInput{
		Email:    "new-user@test.com",
		Password: "password",
	})
	require.NoError(t, err)
	require.Len(t, assigner.calls, 1)
	require.Equal(t, int64(21), assigner.calls[0].UserID)
	require.Equal(t, int64(5), assigner.calls[0].GroupID)
	require.Equal(t, 30, assigner.calls[0].ValidityDays)
}

func TestAdminService_CreateUser_WithTieredSalesCommissionConfig(t *testing.T) {
	repo := &userRepoStub{nextID: 11}
	svc := &adminServiceImpl{userRepo: repo}

	minMonthlySales := 100.0
	input := &CreateUserInput{
		Email:                         "tiered-user@test.com",
		Password:                      "strong-pass",
		IsSales:                       true,
		SalesCommissionMode:           SalesCommissionModeTiered,
		SalesCommissionMinMonthlySales: minMonthlySales,
		SalesCommissionTiers: []SalesCommissionTier{
			{MonthSalesFromCNY: 0, MonthSalesToCNY: float64Ptr(500), CommissionRate: 10},
			{MonthSalesFromCNY: 500, CommissionRate: 15},
		},
	}

	user, err := svc.CreateUser(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.True(t, user.IsSales)
	require.Equal(t, SalesCommissionModeTiered, user.SalesCommissionMode)
	require.Equal(t, minMonthlySales, user.SalesCommissionMinMonthlySales)
	require.Len(t, user.SalesCommissionTiers, 2)
	require.Equal(t, 1, user.SalesCommissionTiers[0].SortOrder)
	require.Equal(t, 2, user.SalesCommissionTiers[1].SortOrder)
	require.Len(t, repo.created, 1)
	require.Equal(t, SalesCommissionModeTiered, repo.created[0].SalesCommissionMode)
	require.Equal(t, minMonthlySales, repo.created[0].SalesCommissionMinMonthlySales)
	require.Len(t, repo.created[0].SalesCommissionTiers, 2)
	require.Equal(t, 1, repo.created[0].SalesCommissionTiers[0].SortOrder)
	require.Equal(t, 2, repo.created[0].SalesCommissionTiers[1].SortOrder)
}

func float64Ptr(v float64) *float64 {
	return &v
}
