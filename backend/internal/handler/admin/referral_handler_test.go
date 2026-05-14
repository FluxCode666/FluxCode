package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReferralHandler_MarkCompleted_RequiresNotes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ReferralHandler{
		referralService: newReferralAdminTestService(),
	}

	router := gin.New()
	router.POST("/admin/referral/:id/mark-completed", handler.MarkCompleted)

	req := httptest.NewRequest(http.MethodPost, "/admin/referral/9/mark-completed", bytes.NewBufferString(`{"notes":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestReferralHandler_MarkCompleted_Succeeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ReferralHandler{
		referralService: newReferralAdminTestService(),
	}

	router := gin.New()
	router.POST("/admin/referral/:id/mark-completed", handler.MarkCompleted)

	req := httptest.NewRequest(http.MethodPost, "/admin/referral/9/mark-completed", bytes.NewBufferString(`{"notes":"webhook missed"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
}

func newReferralAdminTestService() *service.ReferralService {
	settingRepo := &referralAdminSettingRepoStub{
		values: map[string]string{
			service.SettingKeyReferralEnabled:                   "true",
			service.SettingKeyReferralInviteeReward:             "10",
			service.SettingKeyReferralInviterReward:             "20",
			service.SettingKeyReferralMaxInvites:                "0",
			service.SettingKeyReferralRewardExpiryDays:          "7",
			service.SettingKeyReferralOngoingRewardEnabled:      "false",
			service.SettingKeyReferralOngoingRewardType:         "fixed",
			service.SettingKeyReferralOngoingRewardValue:        "0",
			service.SettingKeyReferralOngoingRewardMaxCount:     "0",
			service.SettingKeyReferralOngoingRewardDurationDays: "0",
		},
	}
	resolver := service.NewReferralConfigResolver(settingRepo, &referralAdminUserConfigRepoStub{})
	return service.NewReferralService(
		&referralAdminUserRepoStub{
			byID: map[int64]*service.User{
				12: {ID: 12, Email: "referrer@example.com"},
				34: {ID: 34, Email: "buyer@example.com"},
			},
		},
		&referralAdminReferralRepoStub{
			referralByID: map[int64]*service.Referral{
				9: {
					ID:         9,
					ReferrerID: 12,
					RefereeID:  34,
					Status:     service.ReferralStatusPending,
				},
			},
		},
		&referralAdminGiftRepoStub{},
		resolver,
		nil,
	)
}

type referralAdminSettingRepoStub struct {
	values map[string]string
}

func (s *referralAdminSettingRepoStub) Get(_ context.Context, _ string) (*service.Setting, error) {
	return nil, nil
}
func (s *referralAdminSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}
func (s *referralAdminSettingRepoStub) Set(_ context.Context, _, _ string) error { return nil }
func (s *referralAdminSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = s.values[key]
	}
	return result, nil
}
func (s *referralAdminSettingRepoStub) SetMultiple(_ context.Context, _ map[string]string) error {
	return nil
}
func (s *referralAdminSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *referralAdminSettingRepoStub) Delete(_ context.Context, _ string) error { return nil }

type referralAdminUserConfigRepoStub struct{}

func (s *referralAdminUserConfigRepoStub) GetByUserID(_ context.Context, _ int64) (*service.UserReferralConfig, error) {
	return nil, nil
}
func (s *referralAdminUserConfigRepoStub) Upsert(_ context.Context, _ *service.UserReferralConfig) error {
	return nil
}
func (s *referralAdminUserConfigRepoStub) Delete(_ context.Context, _ int64) error { return nil }

type referralAdminUserRepoStub struct {
	byID map[int64]*service.User
}

func (r *referralAdminUserRepoStub) Create(_ context.Context, _ *service.User) error { return nil }
func (r *referralAdminUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	return r.byID[id], nil
}
func (r *referralAdminUserRepoStub) GetByEmail(_ context.Context, _ string) (*service.User, error) {
	return nil, nil
}
func (r *referralAdminUserRepoStub) GetFirstAdmin(_ context.Context) (*service.User, error) {
	return nil, nil
}
func (r *referralAdminUserRepoStub) Update(_ context.Context, _ *service.User) error { return nil }
func (r *referralAdminUserRepoStub) Delete(_ context.Context, _ int64) error         { return nil }
func (r *referralAdminUserRepoStub) List(_ context.Context, _ pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *referralAdminUserRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _ service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *referralAdminUserRepoStub) UpdateBalance(_ context.Context, _ int64, _ float64) error {
	return nil
}
func (r *referralAdminUserRepoStub) DeductBalance(_ context.Context, _ int64, _ float64) error {
	return nil
}
func (r *referralAdminUserRepoStub) UpdateConcurrency(_ context.Context, _ int64, _ int) error {
	return nil
}
func (r *referralAdminUserRepoStub) ExistsByEmail(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *referralAdminUserRepoStub) RemoveGroupFromAllowedGroups(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (r *referralAdminUserRepoStub) AddGroupToAllowedGroups(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *referralAdminUserRepoStub) RemoveGroupFromUserAllowedGroups(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *referralAdminUserRepoStub) UpdateTotpSecret(_ context.Context, _ int64, _ *string) error {
	return nil
}
func (r *referralAdminUserRepoStub) EnableTotp(_ context.Context, _ int64) error  { return nil }
func (r *referralAdminUserRepoStub) DisableTotp(_ context.Context, _ int64) error { return nil }
func (r *referralAdminUserRepoStub) GetByReferralCode(_ context.Context, _ string) (*service.User, error) {
	return nil, nil
}
func (r *referralAdminUserRepoStub) UpdateReferralCode(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *referralAdminUserRepoStub) UpdateReferredBy(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (r *referralAdminUserRepoStub) IsFirstRecharge(_ context.Context, _ int64) (bool, error) {
	return false, nil
}
func (r *referralAdminUserRepoStub) ListActiveUserIDs(_ context.Context) ([]int64, error) {
	return nil, nil
}

type referralAdminReferralRepoStub struct {
	referralByID map[int64]*service.Referral
}

func (r *referralAdminReferralRepoStub) Create(_ context.Context, _ *service.Referral) error {
	return nil
}
func (r *referralAdminReferralRepoStub) GetByID(_ context.Context, id int64) (*service.Referral, error) {
	return r.referralByID[id], nil
}
func (r *referralAdminReferralRepoStub) GetByRefereeID(_ context.Context, _ int64) (*service.Referral, error) {
	return nil, nil
}
func (r *referralAdminReferralRepoStub) GetByReferrerID(_ context.Context, _ int64, _, _ int) ([]service.Referral, int, error) {
	return nil, 0, nil
}
func (r *referralAdminReferralRepoStub) CountByReferrerID(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (r *referralAdminReferralRepoStub) UpdateStatus(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *referralAdminReferralRepoStub) MarkCompleted(_ context.Context, _ int64, _ float64, _ string) error {
	return nil
}
func (r *referralAdminReferralRepoStub) SetInviteeRewarded(_ context.Context, _ int64) error {
	return nil
}
func (r *referralAdminReferralRepoStub) SetInviterRewarded(_ context.Context, _ int64, _ float64) error {
	return nil
}
func (r *referralAdminReferralRepoStub) IncrementOngoingReward(_ context.Context, _ int64, _ float64) error {
	return nil
}
func (r *referralAdminReferralRepoStub) GetStatsByReferrerID(_ context.Context, _ int64) (*service.ReferralStats, error) {
	return nil, nil
}
func (r *referralAdminReferralRepoStub) ListAll(_ context.Context, _ string, _, _ int) ([]service.Referral, int, error) {
	return nil, 0, nil
}
func (r *referralAdminReferralRepoStub) GetLeaderboard(_ context.Context, _ string, _ int) ([]service.ReferralLeaderboardEntry, error) {
	return nil, nil
}
func (r *referralAdminReferralRepoStub) GetTrendByReferrerID(_ context.Context, _ int64, _ int) ([]service.ReferralTrendPoint, error) {
	return nil, nil
}
func (r *referralAdminReferralRepoStub) GetGlobalTrend(_ context.Context, _ int) ([]service.ReferralTrendPoint, error) {
	return nil, nil
}
func (r *referralAdminReferralRepoStub) CountFirstRecharges(_ context.Context) (int, error) {
	return 0, nil
}
func (r *referralAdminReferralRepoStub) CountAll(_ context.Context) (int, error) { return 0, nil }

type referralAdminGiftRepoStub struct{}

func (r *referralAdminGiftRepoStub) Create(_ context.Context, _ *service.GiftBalanceRecord) error {
	return nil
}
func (r *referralAdminGiftRepoStub) GetByUserID(_ context.Context, _ int64, _, _ int) ([]service.GiftBalanceRecord, int, error) {
	return nil, 0, nil
}
func (r *referralAdminGiftRepoStub) GetAvailableByUserID(_ context.Context, _ int64) ([]service.GiftBalanceRecord, error) {
	return nil, nil
}
func (r *referralAdminGiftRepoStub) GetSummaryByUserID(_ context.Context, _ int64) (*service.GiftBalanceSummary, error) {
	return nil, nil
}
func (r *referralAdminGiftRepoStub) DeductFIFO(_ context.Context, _ int64, _ float64) (float64, error) {
	return 0, nil
}
func (r *referralAdminGiftRepoStub) ExpireRecords(_ context.Context) (int, error) { return 0, nil }
func (r *referralAdminGiftRepoStub) GetTotalRemainingByUserID(_ context.Context, _ int64) (float64, error) {
	return 0, nil
}
func (r *referralAdminGiftRepoStub) ExistsBySourceRef(_ context.Context, _ string, _ int64) (bool, error) {
	return false, nil
}
func (r *referralAdminGiftRepoStub) GetAdminStats(_ context.Context) (*service.AdminReferralStats, error) {
	return nil, nil
}
