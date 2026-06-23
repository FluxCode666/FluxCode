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

func newGrowthHandlersForRoutes() *handlerpkg.Handlers {
	growthSvc := service.NewGrowthService(nil)
	return &handlerpkg.Handlers{
		Admin: &handlerpkg.AdminHandlers{
			Growth: adminhandler.NewGrowthHandler(growthSvc),
		},
	}
}

func TestGrowthRoutes_AllChartLevelPathsAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminGroup := router.Group("/api/v1/admin")
	registerGrowthRoutes(adminGroup, newGrowthHandlersForRoutes())

	cases := []struct {
		path     string
		contains string
	}{
		{path: "/api/v1/admin/growth/overview", contains: `"total_users"`},
		{path: "/api/v1/admin/growth/users/trend", contains: `"series"`},
		{path: "/api/v1/admin/growth/users/sources", contains: `"items"`},
		{path: "/api/v1/admin/growth/users/source-payment-rates", contains: `"items"`},
		{path: "/api/v1/admin/growth/retention/matrix", contains: `"columns"`},
		{path: "/api/v1/admin/growth/retention/trend", contains: `"series"`},
		{path: "/api/v1/admin/growth/payments/funnel", contains: `"tracking_ready"`},
		{path: "/api/v1/admin/growth/payments/plans", contains: `"items"`},
		{path: "/api/v1/admin/growth/payments/first-payment", contains: `"items"`},
		{path: "/api/v1/admin/growth/features/ranking", contains: `"items"`},
		{path: "/api/v1/admin/growth/features/session-metrics", contains: `"average_turns"`},
		{path: "/api/v1/admin/growth/audience/devices", contains: `"items"`},
		{path: "/api/v1/admin/growth/audience/os", contains: `"items"`},
		{path: "/api/v1/admin/growth/audience/browsers", contains: `"items"`},
		{path: "/api/v1/admin/growth/audience/clients", contains: `"items"`},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), tc.contains)
		})
	}
}
