package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const accountPoolAlertConfigsTable = "account_pool_alert_configs"

type poolAlertConfigRepository struct {
	db *sql.DB
}

func NewPoolAlertConfigRepository(db *sql.DB) service.PoolAlertConfigRepository {
	return &poolAlertConfigRepository{db: db}
}

func (r *poolAlertConfigRepository) GetByPlatform(ctx context.Context, platform string) (*service.AccountPoolAlertConfig, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	platform = normalizePoolAlertText(platform)
	if platform == "" {
		return nil, nil
	}

	const query = `
SELECT
	platform,
	pool_threshold_enabled,
	proxy_failure_enabled,
	proxy_active_probe_enabled,
	disabled_proxy_schedule_mode,
	available_count_threshold,
	available_ratio_threshold,
	check_interval_minutes,
	proxy_probe_interval_minutes,
	proxy_failure_window_minutes,
	proxy_failure_threshold,
	alert_emails,
	alert_cooldown_minutes,
	created_at,
	updated_at
FROM account_pool_alert_configs
WHERE platform = $1
`

	row := r.db.QueryRowContext(ctx, query, platform)
	cfg, err := scanPoolAlertConfigRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query %s by platform: %w", accountPoolAlertConfigsTable, err)
	}
	return cfg, nil
}

func (r *poolAlertConfigRepository) Upsert(ctx context.Context, cfg *service.AccountPoolAlertConfig) error {
	if r == nil || r.db == nil || cfg == nil {
		return nil
	}
	platform := normalizePoolAlertText(cfg.Platform)
	if platform == "" {
		return nil
	}

	alertEmailsJSON, err := json.Marshal(normalizeAlertEmails(cfg.AlertEmails))
	if err != nil {
		alertEmailsJSON = []byte("[]")
	}
	now := time.Now()

	const upsert = `
INSERT INTO account_pool_alert_configs (
	platform,
	pool_threshold_enabled,
	proxy_failure_enabled,
	proxy_active_probe_enabled,
	disabled_proxy_schedule_mode,
	available_count_threshold,
	available_ratio_threshold,
	check_interval_minutes,
	proxy_probe_interval_minutes,
	proxy_failure_window_minutes,
	proxy_failure_threshold,
	alert_emails,
	alert_cooldown_minutes,
	created_at,
	updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
ON CONFLICT (platform) DO UPDATE
SET
	pool_threshold_enabled = EXCLUDED.pool_threshold_enabled,
	proxy_failure_enabled = EXCLUDED.proxy_failure_enabled,
	proxy_active_probe_enabled = EXCLUDED.proxy_active_probe_enabled,
	disabled_proxy_schedule_mode = EXCLUDED.disabled_proxy_schedule_mode,
	available_count_threshold = EXCLUDED.available_count_threshold,
	available_ratio_threshold = EXCLUDED.available_ratio_threshold,
	check_interval_minutes = EXCLUDED.check_interval_minutes,
	proxy_probe_interval_minutes = EXCLUDED.proxy_probe_interval_minutes,
	proxy_failure_window_minutes = EXCLUDED.proxy_failure_window_minutes,
	proxy_failure_threshold = EXCLUDED.proxy_failure_threshold,
	alert_emails = EXCLUDED.alert_emails,
	alert_cooldown_minutes = EXCLUDED.alert_cooldown_minutes,
	updated_at = EXCLUDED.updated_at
`

	_, err = r.db.ExecContext(
		ctx,
		upsert,
		platform,
		cfg.PoolThresholdEnabled,
		cfg.ProxyFailureEnabled,
		cfg.ProxyActiveProbeEnabled,
		cfg.DisabledProxyScheduleMode,
		cfg.AvailableCountThreshold,
		cfg.AvailableRatioThreshold,
		cfg.CheckIntervalMinutes,
		cfg.ProxyProbeIntervalMinutes,
		cfg.ProxyFailureWindowMinutes,
		cfg.ProxyFailureThreshold,
		alertEmailsJSON,
		cfg.AlertCooldownMinutes,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert %s: %w", accountPoolAlertConfigsTable, err)
	}
	return nil
}

type poolAlertConfigScanner interface {
	Scan(dest ...any) error
}

func scanPoolAlertConfigRow(scanner poolAlertConfigScanner) (*service.AccountPoolAlertConfig, error) {
	var cfg service.AccountPoolAlertConfig
	var rawAlertEmails []byte
	if err := scanner.Scan(
		&cfg.Platform,
		&cfg.PoolThresholdEnabled,
		&cfg.ProxyFailureEnabled,
		&cfg.ProxyActiveProbeEnabled,
		&cfg.DisabledProxyScheduleMode,
		&cfg.AvailableCountThreshold,
		&cfg.AvailableRatioThreshold,
		&cfg.CheckIntervalMinutes,
		&cfg.ProxyProbeIntervalMinutes,
		&cfg.ProxyFailureWindowMinutes,
		&cfg.ProxyFailureThreshold,
		&rawAlertEmails,
		&cfg.AlertCooldownMinutes,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	); err != nil {
		return nil, err
	}
	cfg.Platform = normalizePoolAlertText(cfg.Platform)
	cfg.AlertEmails = []string{}
	if len(rawAlertEmails) > 0 {
		if err := json.Unmarshal(rawAlertEmails, &cfg.AlertEmails); err != nil {
			cfg.AlertEmails = []string{}
		}
	}
	cfg.AlertEmails = normalizeAlertEmails(cfg.AlertEmails)
	return &cfg, nil
}

func normalizePoolAlertText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeAlertEmails(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		addr := strings.TrimSpace(item)
		if addr == "" {
			continue
		}
		key := strings.ToLower(addr)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, addr)
	}
	if out == nil {
		return []string{}
	}
	return out
}
