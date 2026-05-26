package service

import (
	"context"
	"fmt"
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

	// 销售推广人不发 gift 奖励（走佣金路径），但 referral 必须被标 completed，
	// 以便推广管理界面正确显示 “该被邀请人已达首充要求”。
	require.Empty(t, giftRepo.created)
	require.Empty(t, referralRepo.inviterRewarded)
	require.Equal(t,
		[]referralRepoStubStatusCall{{ID: 7, Status: ReferralStatusCompleted}},
		referralRepo.updateStatusCalls,
	)
}

func TestReferralService_HandleInviterRewardOnFirstRecharge_RegularReferrerMarksCompletedEvenWhenReferralDisabled(t *testing.T) {
	t.Parallel()

	settingRepo := &referralSettingRepoStub{
		values: map[string]string{
			SettingKeyReferralEnabled:                   "false",
			SettingKeyReferralInviteeReward:             "10",
			SettingKeyReferralInviterReward:             "20",
			SettingKeyReferralMaxInvites:                "0",
			SettingKeyReferralRewardExpiryDays:          "0",
			SettingKeyReferralOngoingRewardEnabled:      "false",
			SettingKeyReferralOngoingRewardType:         "fixed",
			SettingKeyReferralOngoingRewardValue:        "0",
			SettingKeyReferralOngoingRewardMaxCount:     "0",
			SettingKeyReferralOngoingRewardDurationDays: "0",
		},
	}
	userRepo := &referralUserRepoStub{byID: map[int64]*User{10: {ID: 10, IsSales: false}}}
	referralRepo := &referralRepoStub{referralByReferee: map[int64]*Referral{
		20: {ID: 7, ReferrerID: 10, RefereeID: 20, Status: ReferralStatusPending},
	}}
	giftRepo := &referralGiftBalanceRepoStub{}
	resolver := NewReferralConfigResolver(settingRepo, &referralUserConfigRepoStub{})
	svc := NewReferralService(userRepo, referralRepo, giftRepo, resolver, nil)

	svc.HandleInviterRewardOnFirstRecharge(context.Background(), 20, 100)

	// 推广全局未启用时，奖励不发；但被邀请人确实首充了，referral 必须 mark completed。
	require.Empty(t, giftRepo.created)
	require.Empty(t, referralRepo.inviterRewarded)
	require.Equal(t,
		[]referralRepoStubStatusCall{{ID: 7, Status: ReferralStatusCompleted}},
		referralRepo.updateStatusCalls,
	)
}

func TestReferralService_HandleInviterRewardOnFirstRecharge_RegularReferrerMarksCompletedWhenInviterRewardZero(t *testing.T) {
	t.Parallel()

	settingRepo := &referralSettingRepoStub{
		values: map[string]string{
			SettingKeyReferralEnabled:                   "true",
			SettingKeyReferralInviteeReward:             "10",
			SettingKeyReferralInviterReward:             "0",
			SettingKeyReferralMaxInvites:                "0",
			SettingKeyReferralRewardExpiryDays:          "0",
			SettingKeyReferralOngoingRewardEnabled:      "false",
			SettingKeyReferralOngoingRewardType:         "fixed",
			SettingKeyReferralOngoingRewardValue:        "0",
			SettingKeyReferralOngoingRewardMaxCount:     "0",
			SettingKeyReferralOngoingRewardDurationDays: "0",
		},
	}
	userRepo := &referralUserRepoStub{byID: map[int64]*User{10: {ID: 10, IsSales: false}}}
	referralRepo := &referralRepoStub{referralByReferee: map[int64]*Referral{
		20: {ID: 7, ReferrerID: 10, RefereeID: 20, Status: ReferralStatusPending},
	}}
	giftRepo := &referralGiftBalanceRepoStub{}
	resolver := NewReferralConfigResolver(settingRepo, &referralUserConfigRepoStub{})
	svc := NewReferralService(userRepo, referralRepo, giftRepo, resolver, nil)

	svc.HandleInviterRewardOnFirstRecharge(context.Background(), 20, 100)

	// 邀请人奖励额度=0，不发 gift，但 referral 必须 mark completed。
	require.Empty(t, giftRepo.created)
	require.Empty(t, referralRepo.inviterRewarded)
	require.Equal(t,
		[]referralRepoStubStatusCall{{ID: 7, Status: ReferralStatusCompleted}},
		referralRepo.updateStatusCalls,
	)
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

	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 100, 101, false)

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
	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 100, 101, false)

	require.Len(t, giftRepo.created, 2)
	require.Equal(t, GiftBalanceSourceReferralInviter, giftRepo.created[0].Source)
	require.Equal(t, GiftBalanceSourceReferralOngoing, giftRepo.created[1].Source)
	require.Equal(t, []float64{20}, referralRepo.inviterRewarded)
	require.Equal(t, []float64{5}, referralRepo.ongoingRewardIncrements)
}

func TestReferralService_HandleOngoingRewardOnRecharge_GrantsOncePerOrder(t *testing.T) {
	t.Parallel()

	svc, giftRepo, referralRepo := newReferralRewardTestService(&User{ID: 10, IsSales: false}, &Referral{
		ID:         7,
		ReferrerID: 10,
		RefereeID:  20,
		Status:     ReferralStatusCompleted,
		CreatedAt:  time.Now(),
	})

	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 100, 201, false)
	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 100, 201, false)
	svc.HandleOngoingRewardOnRecharge(context.Background(), 20, 80, 202, false)

	require.Len(t, giftRepo.created, 2)
	require.Equal(t, []float64{5, 4}, referralRepo.ongoingRewardIncrements)
	require.Len(t, giftRepo.existsChecks, 3)
}

func TestReferralService_AdminMarkReferralCompleted_GrantsInviterRewardOnly(t *testing.T) {
	t.Parallel()

	settingRepo := &referralSettingRepoStub{
		values: map[string]string{
			SettingKeyReferralEnabled:                   "true",
			SettingKeyReferralInviteeReward:             "10",
			SettingKeyReferralInviterReward:             "20",
			SettingKeyReferralMaxInvites:                "0",
			SettingKeyReferralRewardExpiryDays:          "7",
			SettingKeyReferralOngoingRewardEnabled:      "false",
			SettingKeyReferralOngoingRewardType:         "fixed",
			SettingKeyReferralOngoingRewardValue:        "0",
			SettingKeyReferralOngoingRewardMaxCount:     "0",
			SettingKeyReferralOngoingRewardDurationDays: "0",
		},
	}
	userRepo := &referralUserRepoStub{
		byID: map[int64]*User{
			12: {ID: 12, Email: "referrer@example.com"},
			34: {ID: 34, Email: "buyer@example.com"},
		},
	}
	referralRepo := &referralRepoStub{
		referralByID: map[int64]*Referral{
			9: {
				ID:                  9,
				ReferrerID:          12,
				RefereeID:           34,
				Status:              ReferralStatusPending,
				InviteeRewardAmount: 10,
				InviterRewardAmount: 20,
			},
		},
	}
	giftRepo := &referralGiftBalanceRepoStub{}
	resolver := NewReferralConfigResolver(settingRepo, &referralUserConfigRepoStub{})
	svc := NewReferralService(userRepo, referralRepo, giftRepo, resolver, nil)

	err := svc.AdminMarkReferralCompleted(context.Background(), 9, "webhook missed", 0, 0)

	require.NoError(t, err)
	require.Len(t, giftRepo.created, 1)
	require.Equal(t, int64(12), giftRepo.created[0].UserID)
	require.Equal(t, GiftBalanceSourceReferralInviter, giftRepo.created[0].Source)
	require.NotNil(t, giftRepo.created[0].SourceRefID)
	require.Equal(t, int64(9), *giftRepo.created[0].SourceRefID)
	require.Equal(t, int64(9), referralRepo.markCompletedID)
	require.Equal(t, 20.0, referralRepo.markCompletedRewardAmount)
	require.Equal(t, "webhook missed", referralRepo.markCompletedNote)
}

func TestReferralService_AdminMarkReferralCompleted_RejectsCompletedReferral(t *testing.T) {
	t.Parallel()

	settingRepo := &referralSettingRepoStub{
		values: map[string]string{
			SettingKeyReferralEnabled:                   "true",
			SettingKeyReferralInviteeReward:             "10",
			SettingKeyReferralInviterReward:             "20",
			SettingKeyReferralMaxInvites:                "0",
			SettingKeyReferralRewardExpiryDays:          "7",
			SettingKeyReferralOngoingRewardEnabled:      "false",
			SettingKeyReferralOngoingRewardType:         "fixed",
			SettingKeyReferralOngoingRewardValue:        "0",
			SettingKeyReferralOngoingRewardMaxCount:     "0",
			SettingKeyReferralOngoingRewardDurationDays: "0",
		},
	}
	referralRepo := &referralRepoStub{
		referralByID: map[int64]*Referral{
			9: {
				ID:         9,
				ReferrerID: 12,
				RefereeID:  34,
				Status:     ReferralStatusCompleted,
			},
		},
	}
	resolver := NewReferralConfigResolver(settingRepo, &referralUserConfigRepoStub{})
	svc := NewReferralService(&referralUserRepoStub{}, referralRepo, &referralGiftBalanceRepoStub{}, resolver, nil)

	err := svc.AdminMarkReferralCompleted(context.Background(), 9, "duplicate", 0, 0)

	require.Error(t, err)
	require.Zero(t, referralRepo.markCompletedID)
}

func TestReferralService_AdminMarkReferralCompleted_SalesReferrerRequiresAmounts(t *testing.T) {
	t.Parallel()

	settingRepo := &referralSettingRepoStub{values: map[string]string{SettingKeyReferralEnabled: "true"}}
	userRepo := &referralUserRepoStub{
		byID: map[int64]*User{
			12: {ID: 12, Email: "sales@example.com", IsSales: true, SalesCommissionRate: 10},
		},
	}
	referralRepo := &referralRepoStub{
		referralByID: map[int64]*Referral{
			9: {ID: 9, ReferrerID: 12, RefereeID: 34, Status: ReferralStatusPending},
		},
	}
	resolver := NewReferralConfigResolver(settingRepo, &referralUserConfigRepoStub{})
	svc := NewReferralService(userRepo, referralRepo, &referralGiftBalanceRepoStub{}, resolver, &SalesCommissionService{})

	err := svc.AdminMarkReferralCompleted(context.Background(), 9, "manual fix", 0, 0)

	require.Error(t, err)
	require.Zero(t, referralRepo.markCompletedID)
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
	return NewReferralService(userRepo, referralRepo, giftRepo, resolver, nil), giftRepo, referralRepo
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
	referralByReferee         map[int64]*Referral
	referralByID              map[int64]*Referral
	inviterRewarded           []float64
	ongoingRewardIncrements   []float64
	markCompletedID           int64
	markCompletedRewardAmount float64
	markCompletedNote         string
	updateStatusCalls         []referralRepoStubStatusCall
}

type referralRepoStubStatusCall struct {
	ID     int64
	Status string
}

func (r *referralRepoStub) Create(_ context.Context, _ *Referral) error { return nil }
func (r *referralRepoStub) GetByRefereeID(_ context.Context, refereeID int64) (*Referral, error) {
	return r.referralByReferee[refereeID], nil
}
func (r *referralRepoStub) GetByID(_ context.Context, id int64) (*Referral, error) {
	return r.referralByID[id], nil
}
func (r *referralRepoStub) GetByReferrerID(_ context.Context, _ int64, _, _ int) ([]Referral, int, error) {
	return nil, 0, nil
}
func (r *referralRepoStub) CountByReferrerID(_ context.Context, _ int64) (int, error) { return 0, nil }
func (r *referralRepoStub) UpdateStatus(_ context.Context, id int64, status string) error {
	r.updateStatusCalls = append(r.updateStatusCalls, referralRepoStubStatusCall{ID: id, Status: status})
	if ref, ok := r.referralByID[id]; ok && ref != nil {
		ref.Status = status
	}
	for _, ref := range r.referralByReferee {
		if ref != nil && ref.ID == id {
			ref.Status = status
		}
	}
	return nil
}
func (r *referralRepoStub) SetInviteeRewarded(_ context.Context, _ int64) error { return nil }
func (r *referralRepoStub) SetInviterRewarded(_ context.Context, _ int64, rewardAmount float64) error {
	r.inviterRewarded = append(r.inviterRewarded, rewardAmount)
	return nil
}
func (r *referralRepoStub) IncrementOngoingReward(_ context.Context, _ int64, amount float64) error {
	r.ongoingRewardIncrements = append(r.ongoingRewardIncrements, amount)
	return nil
}
func (r *referralRepoStub) IncrementInviteeOngoingReward(_ context.Context, _ int64, _ float64) error {
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
func (r *referralRepoStub) MarkCompleted(_ context.Context, id int64, rewardAmount float64, note string) error {
	r.markCompletedID = id
	r.markCompletedRewardAmount = rewardAmount
	r.markCompletedNote = note
	return nil
}

type referralGiftBalanceRepoStub struct {
	created      []*GiftBalanceRecord
	existsChecks []string
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
func (r *referralGiftBalanceRepoStub) GetNextExpiry(_ context.Context, _ int64) (*time.Time, float64, error) {
	return nil, 0, nil
}

func giftBalanceKey(source string, sourceRefID int64) string {
	return fmt.Sprintf("%s:%d", source, sourceRefID)
}

func (r *referralGiftBalanceRepoStub) ExistsBySourceRef(_ context.Context, source string, sourceRefID int64) (bool, error) {
	key := giftBalanceKey(source, sourceRefID)
	r.existsChecks = append(r.existsChecks, key)
	for _, record := range r.created {
		if record.Source != source || record.SourceRefID == nil {
			continue
		}
		if *record.SourceRefID == sourceRefID {
			return true, nil
		}
	}
	return false, nil
}
func (r *referralGiftBalanceRepoStub) GetAdminStats(_ context.Context) (*AdminReferralStats, error) {
	return nil, nil
}
