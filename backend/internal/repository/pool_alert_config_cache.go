package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const poolAlertConfigCacheHashKey = "pool_alert:config"

type poolAlertConfigCache struct {
	rdb *redis.Client
}

func NewPoolAlertConfigCache(rdb *redis.Client) service.PoolAlertConfigCache {
	return &poolAlertConfigCache{rdb: rdb}
}

func (c *poolAlertConfigCache) GetByPlatform(ctx context.Context, platform string) (*service.AccountPoolAlertConfig, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	platform = normalizePoolAlertCacheText(platform)
	if platform == "" {
		return nil, nil
	}

	payload, err := c.rdb.HGet(ctx, poolAlertConfigCacheHashKey, platform).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}

	var cfg service.AccountPoolAlertConfig
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		return nil, err
	}
	// Backward compatibility: old cache payloads may miss newly-added bool fields.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &raw); err == nil {
		applyPoolAlertConfigCacheBackwardCompatibility(&cfg, raw)
	}
	cfg.Platform = platform
	cfg.AlertEmails = normalizeAlertEmails(cfg.AlertEmails)
	return &cfg, nil
}

func applyPoolAlertConfigCacheBackwardCompatibility(cfg *service.AccountPoolAlertConfig, raw map[string]json.RawMessage) {
	if cfg == nil {
		return
	}
	if !hasAnyRawJSONKey(raw, "proxy_active_probe_enabled", "ProxyActiveProbeEnabled") {
		cfg.ProxyActiveProbeEnabled = true
	}
	if !hasAnyRawJSONKey(raw, "disabled_proxy_schedule_mode", "DisabledProxyScheduleMode") {
		cfg.DisabledProxyScheduleMode = service.DisabledProxyScheduleModeDirectWithoutProxy
		if allow, ok := readRawJSONBool(raw, "allow_schedule_when_proxy_disabled", "AllowScheduleWhenProxyDisabled"); ok && !allow {
			cfg.DisabledProxyScheduleMode = service.DisabledProxyScheduleModeExcludeAccount
		}
	}
}

func hasAnyRawJSONKey(raw map[string]json.RawMessage, keys ...string) bool {
	if len(raw) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := raw[key]; ok {
			return true
		}
	}
	return false
}

func readRawJSONBool(raw map[string]json.RawMessage, keys ...string) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	for _, key := range keys {
		v, ok := raw[key]
		if !ok {
			continue
		}
		var parsed bool
		if err := json.Unmarshal(v, &parsed); err == nil {
			return parsed, true
		}
	}
	return false, false
}

func (c *poolAlertConfigCache) Set(ctx context.Context, cfg *service.AccountPoolAlertConfig) error {
	if c == nil || c.rdb == nil || cfg == nil {
		return nil
	}
	platform := normalizePoolAlertCacheText(cfg.Platform)
	if platform == "" {
		return nil
	}

	clone := *cfg
	clone.Platform = platform
	clone.AlertEmails = normalizeAlertEmails(clone.AlertEmails)

	payload, err := json.Marshal(clone)
	if err != nil {
		return err
	}
	return c.rdb.HSet(ctx, poolAlertConfigCacheHashKey, platform, string(payload)).Err()
}

func (c *poolAlertConfigCache) Delete(ctx context.Context, platform string) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	platform = normalizePoolAlertCacheText(platform)
	if platform == "" {
		return nil
	}
	return c.rdb.HDel(ctx, poolAlertConfigCacheHashKey, platform).Err()
}

func normalizePoolAlertCacheText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
