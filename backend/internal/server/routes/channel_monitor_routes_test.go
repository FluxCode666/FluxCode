package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
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

func TestChannelMonitorUserRoutesArePublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	settingService := service.NewSettingService(&channelMonitorRouteSettingRepoStub{
		values: map[string]string{},
	}, &config.Config{})
	authThatRejects := middleware.JWTAuthMiddleware(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	RegisterUserRoutes(v1, &handlerpkg.Handlers{
		User:            handlerpkg.NewUserHandler(nil, nil, nil, nil),
		Totp:            handlerpkg.NewTotpHandler(nil),
		Referral:        handlerpkg.NewReferralHandler(nil),
		SalesCommission: handlerpkg.NewSalesCommissionHandler(nil),
		APIKey:          handlerpkg.NewAPIKeyHandler(nil),
		Usage:           handlerpkg.NewUsageHandler(nil, nil),
		Announcement:    handlerpkg.NewAnnouncementHandler(nil),
		ChannelMonitor:  handlerpkg.NewChannelMonitorUserHandler(nil, settingService),
		Redeem:          handlerpkg.NewRedeemHandler(nil),
		Subscription:    handlerpkg.NewSubscriptionHandler(nil),
	}, authThatRejects, settingService)

	t.Run("list_bypasses_jwt_auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/channel-monitors", nil)

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.JSONEq(t, `{"code":0,"message":"success","data":{"items":[]}}`, w.Body.String())
	})

	t.Run("status_bypasses_jwt_auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/channel-monitors/1/status", nil)

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})
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

type channelMonitorRouteSettingRepoStub struct {
	values map[string]string
}

func (s *channelMonitorRouteSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *channelMonitorRouteSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}

func (s *channelMonitorRouteSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *channelMonitorRouteSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *channelMonitorRouteSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *channelMonitorRouteSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *channelMonitorRouteSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}
