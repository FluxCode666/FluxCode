package routes

import (
	"testing"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorAdminRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminGroup := router.Group("/api/v1/admin")

	registerChannelMonitorRoutes(adminGroup, &handlerpkg.Handlers{
		Admin: &handlerpkg.AdminHandlers{
			ChannelMonitor:         adminhandler.NewChannelMonitorHandler(nil),
			ChannelMonitorTemplate: adminhandler.NewChannelMonitorRequestTemplateHandler(nil),
		},
	})

	requireRegisteredRoute(t, router, "GET", "/api/v1/admin/channel-monitors")
	requireRegisteredRoute(t, router, "POST", "/api/v1/admin/channel-monitors")
	requireRegisteredRoute(t, router, "GET", "/api/v1/admin/channel-monitors/:id")
	requireRegisteredRoute(t, router, "PUT", "/api/v1/admin/channel-monitors/:id")
	requireRegisteredRoute(t, router, "DELETE", "/api/v1/admin/channel-monitors/:id")
	requireRegisteredRoute(t, router, "POST", "/api/v1/admin/channel-monitors/:id/run")
	requireRegisteredRoute(t, router, "GET", "/api/v1/admin/channel-monitors/:id/history")
	requireRegisteredRoute(t, router, "GET", "/api/v1/admin/channel-monitor-templates")
	requireRegisteredRoute(t, router, "POST", "/api/v1/admin/channel-monitor-templates")
	requireRegisteredRoute(t, router, "GET", "/api/v1/admin/channel-monitor-templates/:id")
	requireRegisteredRoute(t, router, "PUT", "/api/v1/admin/channel-monitor-templates/:id")
	requireRegisteredRoute(t, router, "DELETE", "/api/v1/admin/channel-monitor-templates/:id")
	requireRegisteredRoute(t, router, "GET", "/api/v1/admin/channel-monitor-templates/:id/monitors")
	requireRegisteredRoute(t, router, "POST", "/api/v1/admin/channel-monitor-templates/:id/apply")
}

func TestChannelMonitorUserRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterUserRoutes(v1, &handlerpkg.Handlers{
		User:            handlerpkg.NewUserHandler(nil, nil, nil, nil),
		Totp:            handlerpkg.NewTotpHandler(nil),
		Referral:        handlerpkg.NewReferralHandler(nil),
		SalesCommission: handlerpkg.NewSalesCommissionHandler(nil),
		APIKey:          handlerpkg.NewAPIKeyHandler(nil),
		Usage:           handlerpkg.NewUsageHandler(nil, nil),
		Announcement:    handlerpkg.NewAnnouncementHandler(nil),
		ChannelMonitor:  handlerpkg.NewChannelMonitorUserHandler(nil, nil),
		Redeem:          handlerpkg.NewRedeemHandler(nil),
		Subscription:    handlerpkg.NewSubscriptionHandler(nil),
	}, middleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }), nil)

	requireRegisteredRoute(t, router, "GET", "/api/v1/channel-monitors")
	requireRegisteredRoute(t, router, "GET", "/api/v1/channel-monitors/:id/status")
}

func requireRegisteredRoute(t *testing.T, router *gin.Engine, method string, path string) {
	t.Helper()
	for _, route := range router.Routes() {
		if route.Method == method && route.Path == path {
			return
		}
	}
	require.Failf(t, "route not registered", "%s %s", method, path)
}
