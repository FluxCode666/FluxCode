package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserHandlerList_IncludesGiftBalanceRemaining(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.users = []service.User{{
		ID:      7,
		Email:   "gifted@example.com",
		Balance: 10,
		Role:    service.RoleUser,
		Status:  service.StatusActive,
	}}
	giftRepo := &userListGiftBalanceRepoStub{
		remainingByUserID: map[int64]float64{7: 12.5},
	}
	handler := NewUserHandler(adminSvc, nil, giftRepo)
	router := gin.New()
	router.GET("/api/v1/admin/users", handler.List)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?page=1&page_size=20", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Data struct {
			Items []struct {
				ID                   int64   `json:"id"`
				Balance              float64 `json:"balance"`
				GiftBalanceRemaining float64 `json:"gift_balance_remaining"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, int64(7), body.Data.Items[0].ID)
	require.Equal(t, 10.0, body.Data.Items[0].Balance)
	require.Equal(t, 12.5, body.Data.Items[0].GiftBalanceRemaining)
}

type userListGiftBalanceRepoStub struct {
	remainingByUserID map[int64]float64
}

func (r *userListGiftBalanceRepoStub) Create(_ context.Context, _ *service.GiftBalanceRecord) error {
	return nil
}

func (r *userListGiftBalanceRepoStub) GetByUserID(_ context.Context, _ int64, _, _ int) ([]service.GiftBalanceRecord, int, error) {
	return nil, 0, nil
}

func (r *userListGiftBalanceRepoStub) GetAvailableByUserID(_ context.Context, _ int64) ([]service.GiftBalanceRecord, error) {
	return nil, nil
}

func (r *userListGiftBalanceRepoStub) GetSummaryByUserID(_ context.Context, _ int64) (*service.GiftBalanceSummary, error) {
	return nil, nil
}

func (r *userListGiftBalanceRepoStub) DeductFIFO(_ context.Context, _ int64, _ float64) (float64, error) {
	return 0, nil
}

func (r *userListGiftBalanceRepoStub) ExpireRecords(_ context.Context) (int, error) {
	return 0, nil
}

func (r *userListGiftBalanceRepoStub) GetTotalRemainingByUserID(_ context.Context, userID int64) (float64, error) {
	return r.remainingByUserID[userID], nil
}

func (r *userListGiftBalanceRepoStub) GetNextExpiry(_ context.Context, _ int64) (*time.Time, float64, error) {
	return nil, 0, nil
}

func (r *userListGiftBalanceRepoStub) ExistsBySourceRef(_ context.Context, _ string, _ int64) (bool, error) {
	return false, nil
}

func (r *userListGiftBalanceRepoStub) GetAdminStats(_ context.Context) (*service.AdminReferralStats, error) {
	return nil, nil
}
