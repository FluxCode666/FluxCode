package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestReferralService_HandleInviterRewardOnFirstRecharge_SkipsSalesReferrer(t *testing.T) {
	t.Parallel()

	svc, giftRepo, referralRepo := newReferralRewardTestService(&User{ID: 10, IsSales: true, SalesCommissionRate: 15}, &Referral{
		ID:         7,
		ReferrerID: 10,
		RefereeID:  20,
		Status:     ReferralStatusPending,
	})

	svc.HandleInviterRewardOnFirstRecharge(context.Background(), 20, 100)

	require.Empty(t, giftRepo.created)
	require.Empty(t, referralRepo.inviterRewarded)
}

func TestReferralService_HandleOngoingRewardOnRecharge_SkipsSalesReferrer(t *testing.T) {
	t.Parallel()

	svc, giftRepo, referralRepo := newReferralRewardTestService(&User{ID: 10, IsSales: true, SalesCommissionRate: 15}, &Referral{
		ID:         7,
		ReferrerID: 10,
		RefereeID:  20,
		Status:     ReferralStatusCompleted,
		CreatedAt:  time.Now(),
	})

	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 100)

	require.Empty(t, giftRepo.created)
	require.Empty(t, referralRepo.ongoingRewardIncrements)
}

func TestReferralService_RechargeRewards_StillGrantForRegularReferrer(t *testing.T) {
	t.Parallel()

	svc, giftRepo, referralRepo := newReferralRewardTestService(&User{ID: 10, IsSales: false}, &Referral{
		ID:         7,
		ReferrerID: 10,
		RefereeID:  20,
		Status:     ReferralStatusPending,
		CreatedAt:  time.Now(),
	})

	svc.HandleInviterRewardOnFirstRecharge(context.Background(), 20, 100)
	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 100)

	require.Len(t, giftRepo.created, 2)
	require.Equal(t, GiftBalanceSourceReferralInviter, giftRepo.created[0].Source)
	require.Equal(t, GiftBalanceSourceReferralOngoing, giftRepo.created[1].Source)
	require.Equal(t, []float64{20}, referralRepo.inviterRewarded)
	require.Equal(t, []float64{5}, referralRepo.ongoingRewardIncrements)
}

func newReferralRewardTestService(referrer *User, referral *Referral) (*ReferralService, *referralGiftBalanceRepoStub, *referralRepoStub) {
	settingRepo := &referralSettingRepoStub{
		values: map[string]string{
			SettingKeyReferralEnabled:                   "true",
			SettingKeyReferralInviteeReward:             "1",
			SettingKeyReferralInviterReward:             "20",
			SettingKeyReferralMaxInvites:                "0",
			SettingKeyReferralRewardExpiryDays:          "0",
			SettingKeyReferralOngoingRewardEnabled:      "true",
			SettingKeyReferralOngoingRewardType:         "percentage",
			SettingKeyReferralOngoingRewardValue:        "5",
			SettingKeyReferralOngoingRewardMaxCount:     "0",
			SettingKeyReferralOngoingRewardDurationDays: "0",
		},
	}
	userRepo := &referralUserRepoStub{
		byID: map[int64]*User{
			referrer.ID: referrer,
		},
	}
	referralRepo := &referralRepoStub{referralByReferee: map[int64]*Referral{
		referral.RefereeID: referral,
	}}
	giftRepo := &referralGiftBalanceRepoStub{}
	resolver := NewReferralConfigResolver(settingRepo, &referralUserConfigRepoStub{})
	return NewReferralService(userRepo, referralRepo, giftRepo, resolver), giftRepo, referralRepo
}

type referralSettingRepoStub struct {
	values map[string]string
}

func (s *referralSettingRepoStub) Get(_ context.Context, _ string) (*Setting, error) { return nil, nil }
func (s *referralSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}
func (s *referralSettingRepoStub) Set(_ context.Context, _, _ string) error { return nil }
func (s *referralSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = s.values[key]
	}
	return result, nil
}
func (s *referralSettingRepoStub) SetMultiple(_ context.Context, _ map[string]string) error {
	return nil
}
func (s *referralSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *referralSettingRepoStub) Delete(_ context.Context, _ string) error { return nil }

type referralUserConfigRepoStub struct{}

func (s *referralUserConfigRepoStub) GetByUserID(_ context.Context, _ int64) (*UserReferralConfig, error) {
	return nil, nil
}
func (s *referralUserConfigRepoStub) Upsert(_ context.Context, _ *UserReferralConfig) error {
	return nil
}
func (s *referralUserConfigRepoStub) Delete(_ context.Context, _ int64) error { return nil }

type referralUserRepoStub struct {
	byID map[int64]*User
}

func (r *referralUserRepoStub) Create(_ context.Context, _ *User) error { return nil }
func (r *referralUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	return r.byID[id], nil
}
func (r *referralUserRepoStub) GetByEmail(_ context.Context, _ string) (*User, error) {
	return nil, nil
}
func (r *referralUserRepoStub) GetFirstAdmin(_ context.Context) (*User, error) { return nil, nil }
func (r *referralUserRepoStub) Update(_ context.Context, _ *User) error        { return nil }
func (r *referralUserRepoStub) Delete(_ context.Context, _ int64) error        { return nil }
func (r *referralUserRepoStub) List(_ context.Context, _ pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *referralUserRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *referralUserRepoStub) UpdateBalance(_ context.Context, _ int64, _ float64) error { return nil }
func (r *referralUserRepoStub) DeductBalance(_ context.Context, _ int64, _ float64) error { return nil }
func (r *referralUserRepoStub) UpdateConcurrency(_ context.Context, _ int64, _ int) error { return nil }
func (r *referralUserRepoStub) ExistsByEmail(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *referralUserRepoStub) RemoveGroupFromAllowedGroups(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (r *referralUserRepoStub) AddGroupToAllowedGroups(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *referralUserRepoStub) RemoveGroupFromUserAllowedGroups(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *referralUserRepoStub) UpdateTotpSecret(_ context.Context, _ int64, _ *string) error {
	return nil
}
func (r *referralUserRepoStub) EnableTotp(_ context.Context, _ int64) error  { return nil }
func (r *referralUserRepoStub) DisableTotp(_ context.Context, _ int64) error { return nil }
func (r *referralUserRepoStub) GetByReferralCode(_ context.Context, _ string) (*User, error) {
	return nil, nil
}
func (r *referralUserRepoStub) UpdateReferralCode(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *referralUserRepoStub) UpdateReferredBy(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *referralUserRepoStub) IsFirstRecharge(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
func (r *referralUserRepoStub) ListActiveUserIDs(_ context.Context) ([]int64, error) { return nil, nil }

type referralRepoStub struct {
	referralByReferee       map[int64]*Referral
	inviterRewarded         []float64
	ongoingRewardIncrements []float64
}

func (r *referralRepoStub) Create(_ context.Context, _ *Referral) error { return nil }
func (r *referralRepoStub) GetByRefereeID(_ context.Context, refereeID int64) (*Referral, error) {
	return r.referralByReferee[refereeID], nil
}
func (r *referralRepoStub) GetByReferrerID(_ context.Context, _ int64, _, _ int) ([]Referral, int, error) {
	return nil, 0, nil
}
func (r *referralRepoStub) CountByReferrerID(_ context.Context, _ int64) (int, error) { return 0, nil }
func (r *referralRepoStub) UpdateStatus(_ context.Context, _ int64, _ string) error   { return nil }
func (r *referralRepoStub) SetInviteeRewarded(_ context.Context, _ int64) error       { return nil }
func (r *referralRepoStub) SetInviterRewarded(_ context.Context, _ int64, rewardAmount float64) error {
	r.inviterRewarded = append(r.inviterRewarded, rewardAmount)
	return nil
}
func (r *referralRepoStub) IncrementOngoingReward(_ context.Context, _ int64, amount float64) error {
	r.ongoingRewardIncrements = append(r.ongoingRewardIncrements, amount)
	return nil
}
func (r *referralRepoStub) GetStatsByReferrerID(_ context.Context, _ int64) (*ReferralStats, error) {
	return nil, nil
}
func (r *referralRepoStub) ListAll(_ context.Context, _ string, _, _ int) ([]Referral, int, error) {
	return nil, 0, nil
}
func (r *referralRepoStub) GetLeaderboard(_ context.Context, _ string, _ int) ([]ReferralLeaderboardEntry, error) {
	return nil, nil
}
func (r *referralRepoStub) GetTrendByReferrerID(_ context.Context, _ int64, _ int) ([]ReferralTrendPoint, error) {
	return nil, nil
}
func (r *referralRepoStub) GetGlobalTrend(_ context.Context, _ int) ([]ReferralTrendPoint, error) {
	return nil, nil
}
func (r *referralRepoStub) CountFirstRecharges(_ context.Context) (int, error) { return 0, nil }
func (r *referralRepoStub) CountAll(_ context.Context) (int, error)            { return 0, nil }

type referralGiftBalanceRepoStub struct {
	created []*GiftBalanceRecord
}

func (r *referralGiftBalanceRepoStub) Create(_ context.Context, record *GiftBalanceRecord) error {
	copyRecord := *record
	r.created = append(r.created, &copyRecord)
	return nil
}
func (r *referralGiftBalanceRepoStub) GetByUserID(_ context.Context, _ int64, _, _ int) ([]GiftBalanceRecord, int, error) {
	return nil, 0, nil
}
func (r *referralGiftBalanceRepoStub) GetAvailableByUserID(_ context.Context, _ int64) ([]GiftBalanceRecord, error) {
	return nil, nil
}
func (r *referralGiftBalanceRepoStub) GetSummaryByUserID(_ context.Context, _ int64) (*GiftBalanceSummary, error) {
	return nil, nil
}
func (r *referralGiftBalanceRepoStub) DeductFIFO(_ context.Context, _ int64, _ float64) (float64, error) {
	return 0, nil
}
func (r *referralGiftBalanceRepoStub) ExpireRecords(_ context.Context) (int, error) { return 0, nil }
func (r *referralGiftBalanceRepoStub) GetTotalRemainingByUserID(_ context.Context, _ int64) (float64, error) {
	return 0, nil
}
func (r *referralGiftBalanceRepoStub) ExistsBySourceRef(_ context.Context, _ string, _ int64) (bool, error) {
	return false, nil
}
func (r *referralGiftBalanceRepoStub) GetAdminStats(_ context.Context) (*AdminReferralStats, error) {
	return nil, nil
}
