package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// PoolMonitorService handles account pool monitoring config and proxy failure windows.
type PoolMonitorService struct {
	repo    PoolAlertConfigRepository
	cache   PoolAlertConfigCache
	counter ProxyTransportFailureCounter
}

func NewPoolMonitorService(
	repo PoolAlertConfigRepository,
	counter ProxyTransportFailureCounter,
	cache PoolAlertConfigCache,
) *PoolMonitorService {
	return &PoolMonitorService{repo: repo, cache: cache, counter: counter}
}

func (s *PoolMonitorService) GetConfig(ctx context.Context, platform string) (*AccountPoolAlertConfig, error) {
	platform = normalizePlatform(platform)
	if platform == "" {
		platform = PlatformOpenAI
	}
	if !isSupportedPoolMonitorPlatform(platform) {
		return nil, infraerrors.BadRequest("UNSUPPORTED_PLATFORM", "only openai is supported currently")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("POOL_MONITOR_SERVICE_NOT_READY", "pool monitor service not initialized")
	}

	if s.cache != nil {
		cached, err := s.cache.GetByPlatform(ctx, platform)
		if err == nil && cached != nil {
			return normalizePoolAlertConfig(cached), nil
		}
	}

	cfg := defaultPoolAlertConfig(platform)
	saved, err := s.repo.GetByPlatform(ctx, platform)
	if err != nil {
		return nil, fmt.Errorf("get pool alert config: %w", err)
	}
	if saved != nil {
		cfg = normalizePoolAlertConfig(saved)
	}

	if s.cache != nil {
		_ = s.cache.Set(ctx, cfg)
	}
	return cfg, nil
}

func (s *PoolMonitorService) UpdateConfig(ctx context.Context, cfg *AccountPoolAlertConfig) (*AccountPoolAlertConfig, error) {
	if cfg == nil {
		return nil, infraerrors.BadRequest("INVALID_POOL_ALERT_CONFIG", "config is required")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("POOL_MONITOR_SERVICE_NOT_READY", "pool monitor service not initialized")
	}

	normalized, err := normalizeAndValidatePoolAlertConfigForUpdate(cfg)
	if err != nil {
		return nil, err
	}

	previous := defaultPoolAlertConfig(normalized.Platform)
	if oldCfg, err := s.repo.GetByPlatform(ctx, normalized.Platform); err == nil && oldCfg != nil {
		previous = normalizePoolAlertConfig(oldCfg)
	}

	if err := s.repo.Upsert(ctx, normalized); err != nil {
		return nil, fmt.Errorf("save pool alert config: %w", err)
	}

	if s.cache != nil {
		if err := s.cache.Set(ctx, normalized); err != nil {
			rollbackErr := s.repo.Upsert(ctx, previous)
			if rollbackErr != nil {
				return nil, infraerrors.ServiceUnavailable("POOL_MONITOR_CACHE_UPDATE_ROLLBACK_FAILED", "pool monitor cache update failed; rollback failed").
					WithCause(fmt.Errorf("cache=%v rollback=%w", err, rollbackErr))
			}
			_ = s.cache.Set(ctx, previous)
			return nil, infraerrors.ServiceUnavailable("POOL_MONITOR_CACHE_UPDATE_FAILED", "failed to update pool monitor cache").WithCause(err)
		}
	}

	return normalized, nil
}

func (s *PoolMonitorService) GetPoolThresholdConfig(ctx context.Context, platform string) (*PoolThresholdAlertConfig, error) {
	cfg, err := s.GetConfig(ctx, platform)
	if err != nil {
		return nil, err
	}
	return &PoolThresholdAlertConfig{
		Enabled:                 cfg.PoolThresholdEnabled,
		AvailableCountThreshold: cfg.AvailableCountThreshold,
		AvailableRatioThreshold: cfg.AvailableRatioThreshold,
		CheckIntervalMinutes:    cfg.CheckIntervalMinutes,
	}, nil
}

func (s *PoolMonitorService) GetProxyFailureConfig(ctx context.Context, platform string) (*ProxyFailureAlertConfig, error) {
	cfg, err := s.GetConfig(ctx, platform)
	if err != nil {
		return nil, err
	}
	return &ProxyFailureAlertConfig{
		Enabled:                   cfg.ProxyFailureEnabled,
		ProxyFailureWindowMinutes: cfg.ProxyFailureWindowMinutes,
		ProxyFailureThreshold:     cfg.ProxyFailureThreshold,
	}, nil
}

// DisabledProxyScheduleMode returns disabled-proxy scheduling mode.
// Fail-open on config errors to avoid accidental full scheduling interruption.
func (s *PoolMonitorService) DisabledProxyScheduleMode(ctx context.Context) string {
	if s == nil {
		return DisabledProxyScheduleModeDirectWithoutProxy
	}
	cfg, err := s.GetConfig(ctx, PlatformOpenAI)
	if err != nil || cfg == nil {
		return DisabledProxyScheduleModeDirectWithoutProxy
	}
	return normalizeDisabledProxyScheduleMode(cfg.DisabledProxyScheduleMode)
}

func (s *PoolMonitorService) EvaluatePoolThreshold(cfg *PoolThresholdAlertConfig, availableCount, baseCount int) PoolThresholdEvaluation {
	evaluation := PoolThresholdEvaluation{
		AvailableCount:   availableCount,
		BaseAccountCount: baseCount,
	}
	if cfg == nil || !cfg.Enabled {
		return evaluation
	}

	if cfg.AvailableCountThreshold > 0 && availableCount < cfg.AvailableCountThreshold {
		evaluation.CountTriggered = true
	}

	if baseCount > 0 {
		evaluation.AvailableRatioPct = float64(availableCount) * 100 / float64(baseCount)
		if cfg.AvailableRatioThreshold > 0 && evaluation.AvailableRatioPct < float64(cfg.AvailableRatioThreshold) {
			evaluation.RatioTriggered = true
		}
	}

	evaluation.Triggered = evaluation.CountTriggered || evaluation.RatioTriggered
	return evaluation
}

func (s *PoolMonitorService) IncrementProxyTransportFailure(ctx context.Context, platform string, proxyID int64, now time.Time) (*ProxyFailureCheckResult, error) {
	if s == nil || s.counter == nil || proxyID <= 0 {
		return &ProxyFailureCheckResult{Enabled: false}, nil
	}

	cfg, err := s.GetProxyFailureConfig(ctx, platform)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return &ProxyFailureCheckResult{Enabled: false}, nil
	}

	windowMinutes := cfg.ProxyFailureWindowMinutes
	threshold := cfg.ProxyFailureThreshold
	if windowMinutes <= 0 || threshold <= 0 {
		return &ProxyFailureCheckResult{Enabled: false}, nil
	}

	window := time.Duration(windowMinutes) * time.Minute
	count, err := s.counter.IncrementAndCount(ctx, normalizePlatform(platform), proxyID, window, now)
	if err != nil {
		return nil, err
	}

	return &ProxyFailureCheckResult{
		Enabled:       true,
		Triggered:     count >= int64(threshold),
		Count:         count,
		WindowMinutes: windowMinutes,
		Threshold:     threshold,
	}, nil
}

func normalizePoolAlertConfig(cfg *AccountPoolAlertConfig) *AccountPoolAlertConfig {
	if cfg == nil {
		return nil
	}
	platform := normalizePlatform(cfg.Platform)
	if platform == "" {
		platform = PlatformOpenAI
	}

	base := defaultPoolAlertConfig(platform)
	base.PoolThresholdEnabled = cfg.PoolThresholdEnabled
	base.ProxyFailureEnabled = cfg.ProxyFailureEnabled
	base.ProxyActiveProbeEnabled = cfg.ProxyActiveProbeEnabled
	base.DisabledProxyScheduleMode = normalizeDisabledProxyScheduleMode(cfg.DisabledProxyScheduleMode)
	base.AvailableCountThreshold = cfg.AvailableCountThreshold
	base.AvailableRatioThreshold = cfg.AvailableRatioThreshold
	base.CheckIntervalMinutes = cfg.CheckIntervalMinutes
	base.ProxyProbeIntervalMinutes = cfg.ProxyProbeIntervalMinutes
	base.ProxyFailureWindowMinutes = cfg.ProxyFailureWindowMinutes
	base.ProxyFailureThreshold = cfg.ProxyFailureThreshold
	base.AlertEmails = normalizeAlertRecipients(cfg.AlertEmails)
	base.AlertCooldownMinutes = cfg.AlertCooldownMinutes
	base.CreatedAt = cfg.CreatedAt
	base.UpdatedAt = cfg.UpdatedAt

	if base.AvailableCountThreshold < 0 {
		base.AvailableCountThreshold = 0
	}
	if base.AvailableRatioThreshold < 0 {
		base.AvailableRatioThreshold = 0
	}
	if base.AvailableRatioThreshold > 100 {
		base.AvailableRatioThreshold = 100
	}
	if base.CheckIntervalMinutes < 1 {
		base.CheckIntervalMinutes = 1
	}
	if base.ProxyProbeIntervalMinutes < 1 {
		base.ProxyProbeIntervalMinutes = defaultProxyProbeIntervalMinutes
	}
	if base.ProxyFailureWindowMinutes < 0 {
		base.ProxyFailureWindowMinutes = 0
	}
	if base.ProxyFailureThreshold < 0 {
		base.ProxyFailureThreshold = 0
	}
	if base.AlertCooldownMinutes < 0 {
		base.AlertCooldownMinutes = 0
	}
	return base
}

func normalizeAndValidatePoolAlertConfigForUpdate(cfg *AccountPoolAlertConfig) (*AccountPoolAlertConfig, error) {
	if cfg == nil {
		return nil, infraerrors.BadRequest("INVALID_POOL_ALERT_CONFIG", "config is required")
	}
	platform := normalizePlatform(cfg.Platform)
	if platform == "" {
		return nil, infraerrors.BadRequest("INVALID_PLATFORM", "platform is required")
	}
	if !isSupportedPoolMonitorPlatform(platform) {
		return nil, infraerrors.BadRequest("UNSUPPORTED_PLATFORM", "only openai is supported currently")
	}
	if cfg.AvailableCountThreshold < 0 {
		return nil, infraerrors.BadRequest("INVALID_AVAILABLE_COUNT_THRESHOLD", "available_count_threshold must be greater than or equal to 0")
	}
	if cfg.AvailableRatioThreshold < 0 || cfg.AvailableRatioThreshold > 100 {
		return nil, infraerrors.BadRequest("INVALID_AVAILABLE_RATIO_THRESHOLD", "available_ratio_threshold must be between 0 and 100")
	}
	if cfg.CheckIntervalMinutes < 1 {
		return nil, infraerrors.BadRequest("INVALID_CHECK_INTERVAL", "check_interval_minutes must be greater than or equal to 1")
	}
	if cfg.ProxyProbeIntervalMinutes < 1 {
		return nil, infraerrors.BadRequest("INVALID_PROXY_PROBE_INTERVAL", "proxy_probe_interval_minutes must be greater than or equal to 1")
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.DisabledProxyScheduleMode))
	if mode == "" {
		mode = defaultDisabledProxyScheduleMode
	}
	if mode != DisabledProxyScheduleModeDirectWithoutProxy && mode != DisabledProxyScheduleModeExcludeAccount {
		return nil, infraerrors.BadRequest("INVALID_DISABLED_PROXY_SCHEDULE_MODE", "disabled_proxy_schedule_mode must be direct_without_proxy or exclude_account")
	}
	if cfg.ProxyFailureWindowMinutes < 0 {
		return nil, infraerrors.BadRequest("INVALID_PROXY_FAILURE_WINDOW", "proxy_failure_window_minutes must be greater than or equal to 0")
	}
	if cfg.ProxyFailureThreshold < 0 {
		return nil, infraerrors.BadRequest("INVALID_PROXY_FAILURE_THRESHOLD", "proxy_failure_threshold must be greater than or equal to 0")
	}
	if cfg.AlertCooldownMinutes < 0 {
		return nil, infraerrors.BadRequest("INVALID_ALERT_COOLDOWN", "alert_cooldown_minutes must be greater than or equal to 0")
	}

	return &AccountPoolAlertConfig{
		Platform:                  platform,
		PoolThresholdEnabled:      cfg.PoolThresholdEnabled,
		ProxyFailureEnabled:       cfg.ProxyFailureEnabled,
		ProxyActiveProbeEnabled:   cfg.ProxyActiveProbeEnabled,
		DisabledProxyScheduleMode: mode,
		AvailableCountThreshold:   cfg.AvailableCountThreshold,
		AvailableRatioThreshold:   cfg.AvailableRatioThreshold,
		CheckIntervalMinutes:      cfg.CheckIntervalMinutes,
		ProxyProbeIntervalMinutes: cfg.ProxyProbeIntervalMinutes,
		ProxyFailureWindowMinutes: cfg.ProxyFailureWindowMinutes,
		ProxyFailureThreshold:     cfg.ProxyFailureThreshold,
		AlertEmails:               normalizeAlertRecipients(cfg.AlertEmails),
		AlertCooldownMinutes:      cfg.AlertCooldownMinutes,
	}, nil
}

func defaultPoolAlertConfig(platform string) *AccountPoolAlertConfig {
	platform = normalizePlatform(platform)
	if platform == "" {
		platform = PlatformOpenAI
	}
	return &AccountPoolAlertConfig{
		Platform:                  platform,
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   defaultProxyActiveProbeEnabled,
		DisabledProxyScheduleMode: defaultDisabledProxyScheduleMode,
		AvailableCountThreshold:   defaultPoolMonitorAvailableCountThreshold,
		AvailableRatioThreshold:   defaultPoolMonitorAvailableRatioThreshold,
		CheckIntervalMinutes:      defaultPoolMonitorCheckIntervalMinutes,
		ProxyProbeIntervalMinutes: defaultProxyProbeIntervalMinutes,
		ProxyFailureWindowMinutes: defaultProxyFailureWindowMinutes,
		ProxyFailureThreshold:     defaultProxyFailureThreshold,
		AlertEmails:               []string{},
		AlertCooldownMinutes:      defaultAlertCooldownMinutes,
	}
}

func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func isSupportedPoolMonitorPlatform(platform string) bool {
	return normalizePlatform(platform) == PlatformOpenAI
}
