package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubAccountSummaryAdminService struct {
	service.AdminService
	getAccountSummaryFn func(ctx context.Context) (*service.AccountSummaryResponse, error)
}

func (s stubAccountSummaryAdminService) GetAccountSummary(ctx context.Context) (*service.AccountSummaryResponse, error) {
	if s.getAccountSummaryFn == nil {
		return &service.AccountSummaryResponse{}, nil
	}
	return s.getAccountSummaryFn(ctx)
}

func newAccountSummaryTestRouter(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := gin.New()
	r.GET("/api/v1/admin/accounts/summary", h.GetSummary)
	return r
}

func TestAccountHandlerGetSummary(t *testing.T) {
	r := newAccountSummaryTestRouter(stubAccountSummaryAdminService{
		getAccountSummaryFn: func(_ context.Context) (*service.AccountSummaryResponse, error) {
			return &service.AccountSummaryResponse{
				Overall: service.AccountSummaryCounts{All: 12, Active: 9, Available: 5},
				Platforms: []service.AccountPlatformSummaryItem{
					{Platform: service.PlatformOpenAI, Counts: service.AccountSummaryCounts{All: 7, Active: 6, Available: 4}},
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Overall struct {
				All       int `json:"all"`
				Active    int `json:"active"`
				Available int `json:"available"`
			} `json:"overall"`
			Platforms []struct {
				Platform string `json:"platform"`
				Counts   struct {
					All int `json:"all"`
				} `json:"counts"`
			} `json:"platforms"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.Equal(t, "success", body.Message)
	require.Equal(t, 12, body.Data.Overall.All)
	require.Equal(t, 9, body.Data.Overall.Active)
	require.Equal(t, 5, body.Data.Overall.Available)
	require.Len(t, body.Data.Platforms, 1)
	require.Equal(t, service.PlatformOpenAI, body.Data.Platforms[0].Platform)
	require.Equal(t, 7, body.Data.Platforms[0].Counts.All)
}
