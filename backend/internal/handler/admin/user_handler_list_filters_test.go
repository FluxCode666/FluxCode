package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserHandlerList_ForwardsIsSalesFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	h := NewUserHandler(adminSvc, nil)
	r := gin.New()
	r.GET("/api/v1/admin/users", h.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?is_sales=true", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, adminSvc.lastListUsers.calls)
	require.NotNil(t, adminSvc.lastListUsers.filters.IsSales)
	require.True(t, *adminSvc.lastListUsers.filters.IsSales)
}
