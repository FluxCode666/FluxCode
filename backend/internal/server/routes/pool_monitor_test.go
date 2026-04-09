package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type poolMonitorRoutesRepoStub struct {
	cfg *service.AccountPoolAlertConfig
}

func (s *poolMonitorRoutesRepoStub) GetByPlatform(ctx context.Context, platform string) (*service.AccountPoolAlertConfig, error) {
	_ = ctx
	_ = platform
	if s.cfg == nil {
		return nil, nil
	}
	clone := *s.cfg
	clone.AlertEmails = append([]string(nil), s.cfg.AlertEmails...)
	return &clone, nil
}

func (s *poolMonitorRoutesRepoStub) Upsert(ctx context.Context, cfg *service.AccountPoolAlertConfig) error {
	_ = ctx
	if cfg == nil {
		s.cfg = nil
		return nil
	}
	clone := *cfg
	clone.AlertEmails = append([]string(nil), cfg.AlertEmails...)
	s.cfg = &clone
	return nil
}

type poolMonitorRoutesCounterStub struct{}

func (s *poolMonitorRoutesCounterStub) IncrementAndCount(ctx context.Context, platform string, proxyID int64, window time.Duration, now time.Time) (int64, error) {
	_ = ctx
	_ = platform
	_ = proxyID
	_ = window
	_ = now
	return 0, nil
}

func newPoolMonitorHandlersForRoutes() *handlerpkg.Handlers {
	svc := service.NewPoolMonitorService(&poolMonitorRoutesRepoStub{}, &poolMonitorRoutesCounterStub{}, nil)
	return &handlerpkg.Handlers{
		Admin: &handlerpkg.AdminHandlers{
			PoolMonitor: adminhandler.NewPoolMonitorHandler(svc),
		},
	}
}

func TestPoolMonitorRoutes_AdminPathIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminGroup := router.Group("/api/v1/admin")
	registerPoolMonitorRoutes(adminGroup, newPoolMonitorHandlersForRoutes())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pool-monitor/openai", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"platform":"openai"`)
}
