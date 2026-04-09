package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubAccountListAdminService struct {
	service.AdminService
	listAccountsAdvancedFn func(
		ctx context.Context,
		page, pageSize int,
		platform, accountType, status, schedulableStatus string,
		groupID int64,
		search, sortBy, sortOrder string,
		proxyIDs []int64,
		createdStart, createdEndExclusive *time.Time,
	) ([]service.Account, int64, error)
}

func (s stubAccountListAdminService) ListAccountsAdvanced(
	ctx context.Context,
	page, pageSize int,
	platform, accountType, status, schedulableStatus string,
	groupID int64,
	search, sortBy, sortOrder string,
	proxyIDs []int64,
	createdStart, createdEndExclusive *time.Time,
) ([]service.Account, int64, error) {
	if s.listAccountsAdvancedFn == nil {
		return []service.Account{}, 0, nil
	}
	return s.listAccountsAdvancedFn(ctx, page, pageSize, platform, accountType, status, schedulableStatus, groupID, search, sortBy, sortOrder, proxyIDs, createdStart, createdEndExclusive)
}

func newAccountListTestRouter(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := gin.New()
	r.GET("/api/v1/admin/accounts", h.List)
	return r
}

func TestAccountHandlerList_InvalidProxyIDs(t *testing.T) {
	r := newAccountListTestRouter(stubAccountListAdminService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?proxy_ids=1,abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAccountHandlerList_InvalidCreatedStartDate(t *testing.T) {
	r := newAccountListTestRouter(stubAccountListAdminService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?created_start_date=2026/02/01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAccountHandlerList_StartDateAfterEndDate(t *testing.T) {
	r := newAccountListTestRouter(stubAccountListAdminService{})
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts?created_start_date=2026-02-03&created_end_date=2026-02-01",
		nil,
	)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAccountHandlerList_SortByStatusIsRejected(t *testing.T) {
	r := newAccountListTestRouter(stubAccountListAdminService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?sort_by=status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAccountHandlerList_SortBySchedulableIsRejected(t *testing.T) {
	r := newAccountListTestRouter(stubAccountListAdminService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?sort_by=schedulable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAccountHandlerList_SortByCreatedAtIsAllowedAndForwarded(t *testing.T) {
	var gotSortBy string
	var gotSortOrder string

	r := newAccountListTestRouter(stubAccountListAdminService{
		listAccountsAdvancedFn: func(
			ctx context.Context,
			page, pageSize int,
			platform, accountType, status, schedulableStatus string,
			groupID int64,
			search, sortBy, sortOrder string,
			proxyIDs []int64,
			createdStart, createdEndExclusive *time.Time,
		) ([]service.Account, int64, error) {
			gotSortBy = sortBy
			gotSortOrder = sortOrder
			return []service.Account{}, 0, nil
		},
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts?sort_by=created_at&sort_order=desc",
		nil,
	)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "created_at", gotSortBy)
	require.Equal(t, "desc", gotSortOrder)
}

func TestAccountHandlerList_PassNewFiltersToService(t *testing.T) {
	var gotProxyIDs []int64
	var gotCreatedStart *time.Time
	var gotCreatedEndExclusive *time.Time
	var gotSchedulableStatus string
	var gotGroupID int64

	r := newAccountListTestRouter(stubAccountListAdminService{
		listAccountsAdvancedFn: func(
			ctx context.Context,
			page, pageSize int,
			platform, accountType, status, schedulableStatus string,
			groupID int64,
			search, sortBy, sortOrder string,
			proxyIDs []int64,
			createdStart, createdEndExclusive *time.Time,
		) ([]service.Account, int64, error) {
			gotProxyIDs = append(gotProxyIDs, proxyIDs...)
			gotCreatedStart = createdStart
			gotCreatedEndExclusive = createdEndExclusive
			gotSchedulableStatus = schedulableStatus
			gotGroupID = groupID
			return []service.Account{}, 0, nil
		},
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts?group_id=42&proxy_ids=2,5,2&created_start_date=2026-02-01&created_end_date=2026-02-03&schedulable_status=available",
		nil,
	)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []int64{2, 5}, gotProxyIDs)
	require.NotNil(t, gotCreatedStart)
	require.NotNil(t, gotCreatedEndExclusive)
	require.Equal(t, int64(42), gotGroupID)
	require.Equal(t, "available", gotSchedulableStatus)
	require.Equal(t, "2026-02-01", gotCreatedStart.Format("2006-01-02"))
	require.Equal(t, "2026-02-04", gotCreatedEndExclusive.Format("2006-01-02"))
}
