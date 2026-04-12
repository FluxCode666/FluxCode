package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type poolMonitorRepoStub struct {
	byPlatform  map[string]*AccountPoolAlertConfig
	getCalls    int
	upsertCalls int
	failGet     error
	failUpsert  bool
}

func (s *poolMonitorRepoStub) GetByPlatform(ctx context.Context, platform string) (*AccountPoolAlertConfig, error) {
	_ = ctx
	s.getCalls++
	if s.failGet != nil {
		return nil, s.failGet
	}
	if s.byPlatform == nil {
		return nil, nil
	}
	cfg, ok := s.byPlatform[normalizePlatform(platform)]
	if !ok || cfg == nil {
		return nil, nil
	}
	clone := *cfg
	clone.AlertEmails = append([]string(nil), cfg.AlertEmails...)
	return &clone, nil
}

func (s *poolMonitorRepoStub) Upsert(ctx context.Context, cfg *AccountPoolAlertConfig) error {
	_ = ctx
	s.upsertCalls++
	if s.failUpsert {
		return fmt.Errorf("upsert failed")
	}
	if cfg == nil {
		return nil
	}
	if s.byPlatform == nil {
		s.byPlatform = map[string]*AccountPoolAlertConfig{}
	}
	clone := *cfg
	clone.AlertEmails = append([]string(nil), cfg.AlertEmails...)
	s.byPlatform[normalizePlatform(cfg.Platform)] = &clone
	return nil
}

type poolMonitorCacheStub struct {
	byPlatform map[string]*AccountPoolAlertConfig
	getCalls   int
	setCalls   int
	failGet    error
	failSet    error
}

func (s *poolMonitorCacheStub) GetByPlatform(ctx context.Context, platform string) (*AccountPoolAlertConfig, error) {
	_ = ctx
	s.getCalls++
	if s.failGet != nil {
		return nil, s.failGet
	}
	if s.byPlatform == nil {
		return nil, nil
	}
	cfg, ok := s.byPlatform[normalizePlatform(platform)]
	if !ok || cfg == nil {
		return nil, nil
	}
	clone := *cfg
	clone.AlertEmails = append([]string(nil), cfg.AlertEmails...)
	return &clone, nil
}

func (s *poolMonitorCacheStub) Set(ctx context.Context, cfg *AccountPoolAlertConfig) error {
	_ = ctx
	s.setCalls++
	if s.failSet != nil {
		return s.failSet
	}
	if cfg == nil {
		return nil
	}
	if s.byPlatform == nil {
		s.byPlatform = map[string]*AccountPoolAlertConfig{}
	}
	clone := *cfg
	clone.AlertEmails = append([]string(nil), cfg.AlertEmails...)
	s.byPlatform[normalizePlatform(cfg.Platform)] = &clone
	return nil
}

func (s *poolMonitorCacheStub) Delete(ctx context.Context, platform string) error {
	_ = ctx
	if s.byPlatform != nil {
		delete(s.byPlatform, normalizePlatform(platform))
	}
	return nil
}

type proxyCounterStub struct {
	count int64
	calls int
}

func (s *proxyCounterStub) IncrementAndCount(ctx context.Context, platform string, proxyID int64, window time.Duration, now time.Time) (int64, error) {
	_ = ctx
	_ = platform
	_ = proxyID
	_ = window
	_ = now
	s.calls++
	return s.count, nil
}

func TestPoolMonitorService_GetConfig_DefaultOpenAI(t *testing.T) {
	svc := NewPoolMonitorService(&poolMonitorRepoStub{}, &proxyCounterStub{}, nil)
	cfg, err := svc.GetConfig(context.Background(), "openai")
	require.NoError(t, err)
	require.Equal(t, PlatformOpenAI, cfg.Platform)
	require.True(t, cfg.PoolThresholdEnabled)
	require.True(t, cfg.ProxyFailureEnabled)
	require.True(t, cfg.ProxyActiveProbeEnabled)
	require.Equal(t, defaultDisabledProxyScheduleMode, cfg.DisabledProxyScheduleMode)
	require.Equal(t, defaultPoolMonitorAvailableRatioThreshold, cfg.AvailableRatioThreshold)
	require.Equal(t, defaultPoolMonitorCheckIntervalMinutes, cfg.CheckIntervalMinutes)
	require.Equal(t, defaultProxyProbeIntervalMinutes, cfg.ProxyProbeIntervalMinutes)
}

func TestPoolMonitorService_GetConfig_UsesCache(t *testing.T) {
	repo := &poolMonitorRepoStub{byPlatform: map[string]*AccountPoolAlertConfig{
		PlatformOpenAI: {
			Platform:                PlatformOpenAI,
			AvailableCountThreshold: 9,
		},
	}}
	cache := &poolMonitorCacheStub{byPlatform: map[string]*AccountPoolAlertConfig{
		PlatformOpenAI: {
			Platform:                  PlatformOpenAI,
			AvailableCountThreshold:   2,
			ProxyProbeIntervalMinutes: 7,
			PoolThresholdEnabled:      false,
		},
	}}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, cache)

	cfg, err := svc.GetConfig(context.Background(), "openai")
	require.NoError(t, err)
	require.Equal(t, 2, cfg.AvailableCountThreshold)
	require.Equal(t, 7, cfg.ProxyProbeIntervalMinutes)
	require.False(t, cfg.PoolThresholdEnabled)
	require.Equal(t, 0, repo.getCalls)
}

func TestPoolMonitorService_GetConfig_WarmsCacheOnRepoRead(t *testing.T) {
	repo := &poolMonitorRepoStub{byPlatform: map[string]*AccountPoolAlertConfig{
		PlatformOpenAI: {
			Platform:                  PlatformOpenAI,
			ProxyFailureWindowMinutes: 12,
		},
	}}
	cache := &poolMonitorCacheStub{}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, cache)

	cfg, err := svc.GetConfig(context.Background(), "openai")
	require.NoError(t, err)
	require.Equal(t, 12, cfg.ProxyFailureWindowMinutes)
	require.Equal(t, 1, repo.getCalls)
	require.Equal(t, 1, cache.setCalls)
	require.NotNil(t, cache.byPlatform[PlatformOpenAI])
}

func TestPoolMonitorService_UpdateConfig_RollbackWhenCacheSetFails(t *testing.T) {
	oldCfg := &AccountPoolAlertConfig{
		Platform:                  PlatformOpenAI,
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   true,
		DisabledProxyScheduleMode: DisabledProxyScheduleModeDirectWithoutProxy,
		AvailableCountThreshold:   1,
		AvailableRatioThreshold:   20,
		CheckIntervalMinutes:      5,
		ProxyProbeIntervalMinutes: 5,
		ProxyFailureWindowMinutes: 10,
		ProxyFailureThreshold:     5,
		AlertEmails:               []string{"old@example.com"},
		AlertCooldownMinutes:      5,
	}
	repo := &poolMonitorRepoStub{byPlatform: map[string]*AccountPoolAlertConfig{PlatformOpenAI: oldCfg}}
	cache := &poolMonitorCacheStub{failSet: errors.New("cache down")}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, cache)

	_, err := svc.UpdateConfig(context.Background(), &AccountPoolAlertConfig{
		Platform:                  PlatformOpenAI,
		PoolThresholdEnabled:      false,
		ProxyFailureEnabled:       false,
		ProxyActiveProbeEnabled:   false,
		DisabledProxyScheduleMode: DisabledProxyScheduleModeExcludeAccount,
		AvailableCountThreshold:   8,
		AvailableRatioThreshold:   66,
		CheckIntervalMinutes:      9,
		ProxyProbeIntervalMinutes: 7,
		ProxyFailureWindowMinutes: 11,
		ProxyFailureThreshold:     6,
		AlertEmails:               []string{"new@example.com"},
		AlertCooldownMinutes:      7,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to update pool monitor cache")
	require.Equal(t, 2, repo.upsertCalls)
	current := repo.byPlatform[PlatformOpenAI]
	require.NotNil(t, current)
	require.True(t, current.PoolThresholdEnabled)
	require.True(t, current.ProxyActiveProbeEnabled)
	require.Equal(t, DisabledProxyScheduleModeDirectWithoutProxy, current.DisabledProxyScheduleMode)
	require.Equal(t, 1, current.AvailableCountThreshold)
	require.Equal(t, 5, current.ProxyProbeIntervalMinutes)
	require.Equal(t, []string{"old@example.com"}, current.AlertEmails)
}

func TestPoolMonitorService_UpdateConfig_PersistsAlertSettingsToPoolConfig(t *testing.T) {
	repo := &poolMonitorRepoStub{}
	cache := &poolMonitorCacheStub{}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, cache)

	cfg, err := svc.UpdateConfig(context.Background(), &AccountPoolAlertConfig{
		Platform:                  PlatformOpenAI,
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   false,
		DisabledProxyScheduleMode: DisabledProxyScheduleModeExcludeAccount,
		AvailableCountThreshold:   2,
		AvailableRatioThreshold:   30,
		CheckIntervalMinutes:      6,
		ProxyProbeIntervalMinutes: 8,
		ProxyFailureWindowMinutes: 9,
		ProxyFailureThreshold:     4,
		AlertEmails:               []string{"ops@example.com", "OPS@example.com"},
		AlertCooldownMinutes:      12,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ops@example.com"}, cfg.AlertEmails)
	require.False(t, cfg.ProxyActiveProbeEnabled)
	require.Equal(t, DisabledProxyScheduleModeExcludeAccount, cfg.DisabledProxyScheduleMode)
	require.Equal(t, 8, cfg.ProxyProbeIntervalMinutes)
	require.Equal(t, 1, repo.upsertCalls)
	require.Equal(t, []string{"ops@example.com"}, repo.byPlatform[PlatformOpenAI].AlertEmails)
}

func TestPoolMonitorService_UpdateConfig_InvalidProxyProbeInterval(t *testing.T) {
	repo := &poolMonitorRepoStub{}
	cache := &poolMonitorCacheStub{}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, cache)

	_, err := svc.UpdateConfig(context.Background(), &AccountPoolAlertConfig{
		Platform:                  PlatformOpenAI,
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   true,
		AvailableCountThreshold:   2,
		AvailableRatioThreshold:   30,
		CheckIntervalMinutes:      6,
		ProxyProbeIntervalMinutes: 0,
		ProxyFailureWindowMinutes: 9,
		ProxyFailureThreshold:     4,
		AlertCooldownMinutes:      12,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxy_probe_interval_minutes")
}

func TestPoolMonitorService_UpdateConfig_InvalidDisabledProxyScheduleMode(t *testing.T) {
	repo := &poolMonitorRepoStub{}
	cache := &poolMonitorCacheStub{}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, cache)

	_, err := svc.UpdateConfig(context.Background(), &AccountPoolAlertConfig{
		Platform:                  PlatformOpenAI,
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   true,
		DisabledProxyScheduleMode: "invalid_mode",
		AvailableCountThreshold:   2,
		AvailableRatioThreshold:   30,
		CheckIntervalMinutes:      6,
		ProxyProbeIntervalMinutes: 1,
		ProxyFailureWindowMinutes: 9,
		ProxyFailureThreshold:     4,
		AlertCooldownMinutes:      12,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled_proxy_schedule_mode")
}

func TestPoolMonitorService_EvaluatePoolThreshold_DisabledBySwitch(t *testing.T) {
	svc := NewPoolMonitorService(&poolMonitorRepoStub{}, &proxyCounterStub{}, nil)
	eval := svc.EvaluatePoolThreshold(&PoolThresholdAlertConfig{
		Enabled:                 false,
		AvailableCountThreshold: 10,
		AvailableRatioThreshold: 90,
	}, 1, 100)
	require.False(t, eval.Triggered)
	require.False(t, eval.CountTriggered)
	require.False(t, eval.RatioTriggered)
}

func TestPoolMonitorService_IncrementProxyTransportFailure(t *testing.T) {
	counter := &proxyCounterStub{count: 5}
	repo := &poolMonitorRepoStub{byPlatform: map[string]*AccountPoolAlertConfig{
		PlatformOpenAI: {
			Platform:                  PlatformOpenAI,
			ProxyFailureEnabled:       true,
			ProxyFailureWindowMinutes: 10,
			ProxyFailureThreshold:     5,
		},
	}}
	svc := NewPoolMonitorService(repo, counter, nil)

	result, err := svc.IncrementProxyTransportFailure(context.Background(), PlatformOpenAI, 101, time.Now())
	require.NoError(t, err)
	require.True(t, result.Enabled)
	require.True(t, result.Triggered)
	require.Equal(t, int64(5), result.Count)
	require.Equal(t, 1, counter.calls)
}

func TestPoolMonitorService_IncrementProxyTransportFailure_DisabledBySwitch(t *testing.T) {
	counter := &proxyCounterStub{count: 10}
	repo := &poolMonitorRepoStub{byPlatform: map[string]*AccountPoolAlertConfig{
		PlatformOpenAI: {
			Platform:                  PlatformOpenAI,
			ProxyFailureEnabled:       false,
			ProxyFailureWindowMinutes: 10,
			ProxyFailureThreshold:     5,
		},
	}}
	svc := NewPoolMonitorService(repo, counter, nil)

	result, err := svc.IncrementProxyTransportFailure(context.Background(), PlatformOpenAI, 101, time.Now())
	require.NoError(t, err)
	require.False(t, result.Enabled)
	require.Equal(t, 0, counter.calls)
}

func TestPoolMonitorService_UnsupportedPlatform(t *testing.T) {
	svc := NewPoolMonitorService(&poolMonitorRepoStub{}, &proxyCounterStub{}, nil)
	_, err := svc.GetConfig(context.Background(), "gemini")
	require.Error(t, err)
	require.Contains(t, err.Error(), "only openai")
}

func TestPoolMonitorService_DisabledProxyScheduleMode_FailOpen(t *testing.T) {
	repo := &poolMonitorRepoStub{failGet: errors.New("db down")}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, nil)
	require.Equal(t, DisabledProxyScheduleModeDirectWithoutProxy, svc.DisabledProxyScheduleMode(context.Background()))
}

func TestPoolMonitorService_DisabledProxyScheduleMode_ReadsConfig(t *testing.T) {
	repo := &poolMonitorRepoStub{byPlatform: map[string]*AccountPoolAlertConfig{
		PlatformOpenAI: {
			Platform:                  PlatformOpenAI,
			DisabledProxyScheduleMode: DisabledProxyScheduleModeExcludeAccount,
		},
	}}
	svc := NewPoolMonitorService(repo, &proxyCounterStub{}, nil)
	require.Equal(t, DisabledProxyScheduleModeExcludeAccount, svc.DisabledProxyScheduleMode(context.Background()))
}

func ptrInt64(v int64) *int64 { return &v }

func TestFilterAccountsByDisabledProxyScheduleMode_DirectMode_KeepsAll(t *testing.T) {
	accounts := []Account{
		{ID: 1, ProxyID: nil},
		{ID: 2, ProxyID: ptrInt64(10), Proxy: &Proxy{ID: 10, Status: StatusActive}},
		{ID: 3, ProxyID: ptrInt64(20), Proxy: &Proxy{ID: 20, Status: "disabled"}},
	}
	result := filterAccountsByDisabledProxyScheduleMode(accounts, DisabledProxyScheduleModeDirectWithoutProxy)
	require.Len(t, result, 3)
}

func TestFilterAccountsByDisabledProxyScheduleMode_ExcludeMode_KeepsNoProxy(t *testing.T) {
	accounts := []Account{
		{ID: 1, ProxyID: nil},
	}
	result := filterAccountsByDisabledProxyScheduleMode(accounts, DisabledProxyScheduleModeExcludeAccount)
	require.Len(t, result, 1)
	require.Equal(t, int64(1), result[0].ID)
}

func TestFilterAccountsByDisabledProxyScheduleMode_ExcludeMode_KeepsActiveProxy(t *testing.T) {
	accounts := []Account{
		{ID: 1, ProxyID: nil},
		{ID: 2, ProxyID: ptrInt64(10), Proxy: &Proxy{ID: 10, Status: StatusActive}},
	}
	result := filterAccountsByDisabledProxyScheduleMode(accounts, DisabledProxyScheduleModeExcludeAccount)
	require.Len(t, result, 2)
}

func TestFilterAccountsByDisabledProxyScheduleMode_ExcludeMode_DropsInactiveProxy(t *testing.T) {
	accounts := []Account{
		{ID: 1, ProxyID: nil},
		{ID: 2, ProxyID: ptrInt64(10), Proxy: &Proxy{ID: 10, Status: StatusActive}},
		{ID: 3, ProxyID: ptrInt64(20), Proxy: &Proxy{ID: 20, Status: "disabled"}},
		{ID: 4, ProxyID: ptrInt64(30), Proxy: nil}, // dangling proxy reference
	}
	result := filterAccountsByDisabledProxyScheduleMode(accounts, DisabledProxyScheduleModeExcludeAccount)
	require.Len(t, result, 2)
	require.Equal(t, int64(1), result[0].ID)
	require.Equal(t, int64(2), result[1].ID)
}

func TestFilterAccountsByDisabledProxyScheduleMode_UnrecognizedMode_KeepsAll(t *testing.T) {
	accounts := []Account{
		{ID: 1, ProxyID: ptrInt64(10), Proxy: &Proxy{ID: 10, Status: "disabled"}},
		{ID: 2, ProxyID: nil},
	}
	result := filterAccountsByDisabledProxyScheduleMode(accounts, "unknown_mode")
	require.Len(t, result, 2)
}
