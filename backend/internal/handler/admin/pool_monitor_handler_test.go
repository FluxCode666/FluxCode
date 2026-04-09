package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type poolMonitorAdminRepoStub struct {
	cfg *service.AccountPoolAlertConfig
}

func (s *poolMonitorAdminRepoStub) GetByPlatform(ctx context.Context, platform string) (*service.AccountPoolAlertConfig, error) {
	_ = ctx
	_ = platform
	if s.cfg == nil {
		return nil, nil
	}
	clone := *s.cfg
	clone.AlertEmails = append([]string(nil), s.cfg.AlertEmails...)
	return &clone, nil
}

func (s *poolMonitorAdminRepoStub) Upsert(ctx context.Context, cfg *service.AccountPoolAlertConfig) error {
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

type poolMonitorAdminCounterStub struct{}

func (s *poolMonitorAdminCounterStub) IncrementAndCount(ctx context.Context, platform string, proxyID int64, window time.Duration, now time.Time) (int64, error) {
	_ = ctx
	_ = platform
	_ = proxyID
	_ = window
	_ = now
	return 0, nil
}

func TestPoolMonitorHandler_GetConfig_ReturnsDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &poolMonitorAdminRepoStub{cfg: &service.AccountPoolAlertConfig{
		Platform:                  service.PlatformOpenAI,
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   false,
		DisabledProxyScheduleMode: service.DisabledProxyScheduleModeExcludeAccount,
		AvailableCountThreshold:   2,
		AvailableRatioThreshold:   30,
		CheckIntervalMinutes:      6,
		ProxyProbeIntervalMinutes: 8,
		ProxyFailureWindowMinutes: 9,
		ProxyFailureThreshold:     4,
		AlertEmails:               []string{"ops@example.com"},
		AlertCooldownMinutes:      12,
	}}
	svc := service.NewPoolMonitorService(repo, &poolMonitorAdminCounterStub{}, nil)

	router := gin.New()
	router.GET("/admin/pool-monitor/:platform", NewPoolMonitorHandler(svc).GetConfig)

	req := httptest.NewRequest(http.MethodGet, "/admin/pool-monitor/openai", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"platform":"openai"`)
	require.Contains(t, rec.Body.String(), `"disabled_proxy_schedule_mode":"exclude_account"`)
	require.Contains(t, rec.Body.String(), `"alert_emails":["ops@example.com"]`)
}

func TestPoolMonitorHandler_UpdateConfig_ReturnsUpdatedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &poolMonitorAdminRepoStub{}
	svc := service.NewPoolMonitorService(repo, &poolMonitorAdminCounterStub{}, nil)

	router := gin.New()
	router.PUT("/admin/pool-monitor/:platform", NewPoolMonitorHandler(svc).UpdateConfig)

	req := httptest.NewRequest(http.MethodPut, "/admin/pool-monitor/openai", strings.NewReader(`{
		"proxy_active_probe_enabled": false,
		"disabled_proxy_schedule_mode": "exclude_account",
		"proxy_probe_interval_minutes": 7,
		"alert_emails": ["ops@example.com", "OPS@example.com"]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"platform":"openai"`)
	require.Contains(t, rec.Body.String(), `"proxy_active_probe_enabled":false`)
	require.Contains(t, rec.Body.String(), `"disabled_proxy_schedule_mode":"exclude_account"`)
	require.Contains(t, rec.Body.String(), `"proxy_probe_interval_minutes":7`)
	require.NotNil(t, repo.cfg)
	require.False(t, repo.cfg.ProxyActiveProbeEnabled)
	require.Equal(t, service.DisabledProxyScheduleModeExcludeAccount, repo.cfg.DisabledProxyScheduleMode)
	require.Equal(t, 7, repo.cfg.ProxyProbeIntervalMinutes)
	require.Equal(t, []string{"ops@example.com"}, repo.cfg.AlertEmails)
}
