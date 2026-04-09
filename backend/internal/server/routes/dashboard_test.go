package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newDashboardHandlersForRoutes() *handlerpkg.Handlers {
	dashboardSvc := service.NewDashboardService(nil, nil, nil, nil)
	return &handlerpkg.Handlers{
		Admin: &handlerpkg.AdminHandlers{
			Dashboard: adminhandler.NewDashboardHandler(dashboardSvc, nil),
		},
	}
}

func TestDashboardRoutes_AdminProxyUsageSummaryPathIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminGroup := router.Group("/api/v1/admin")
	registerDashboardRoutes(adminGroup, newDashboardHandlersForRoutes())

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/dashboard/proxies-usage?start_date=2026-03-24&end_date=2026-03-25&granularity=hour&timezone=Asia%2FShanghai",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"items"`)
}
