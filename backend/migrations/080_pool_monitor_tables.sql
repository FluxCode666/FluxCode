-- 080_pool_monitor_tables.sql
-- Account pool alert configs + proxy usage metrics (consolidated from old project migrations 037-048)
-- PostgreSQL 15+

-- 1) account_pool_alert_configs
CREATE TABLE IF NOT EXISTS account_pool_alert_configs (
    platform                        VARCHAR(50) PRIMARY KEY,
    available_count_threshold       INT NOT NULL DEFAULT 0,
    available_ratio_threshold       INT NOT NULL DEFAULT 20,
    check_interval_minutes          INT NOT NULL DEFAULT 5,
    proxy_failure_window_minutes    INT NOT NULL DEFAULT 10,
    proxy_failure_threshold         INT NOT NULL DEFAULT 5,
    alert_emails                    JSONB NOT NULL DEFAULT '[]'::jsonb,
    alert_cooldown_minutes          INT NOT NULL DEFAULT 5,

    -- switches (from 038)
    pool_threshold_enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    proxy_failure_enabled           BOOLEAN NOT NULL DEFAULT TRUE,

    -- proxy active probing (from 042, 043, 044)
    proxy_active_probe_enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    proxy_probe_interval_minutes    INT NOT NULL DEFAULT 5,

    -- disabled proxy schedule mode (from 047)
    disabled_proxy_schedule_mode    VARCHAR(32) NOT NULL DEFAULT 'direct_without_proxy',

    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_account_pool_alert_count_nonneg
        CHECK (available_count_threshold >= 0),
    CONSTRAINT chk_account_pool_alert_ratio_range
        CHECK (available_ratio_threshold >= 0 AND available_ratio_threshold <= 100),
    CONSTRAINT chk_account_pool_alert_interval_positive
        CHECK (check_interval_minutes >= 1),
    CONSTRAINT chk_account_pool_alert_proxy_window_nonneg
        CHECK (proxy_failure_window_minutes >= 0),
    CONSTRAINT chk_account_pool_alert_proxy_threshold_nonneg
        CHECK (proxy_failure_threshold >= 0),
    CONSTRAINT chk_account_pool_alert_cooldown_nonneg
        CHECK (alert_cooldown_minutes >= 0),
    CONSTRAINT chk_account_pool_alert_configs_proxy_probe_interval_minutes
        CHECK (proxy_probe_interval_minutes >= 1),
    CONSTRAINT account_pool_alert_configs_disabled_proxy_schedule_mode_check
        CHECK (disabled_proxy_schedule_mode IN ('direct_without_proxy', 'exclude_account'))
);

-- Seed default OpenAI config
INSERT INTO account_pool_alert_configs (
    platform,
    available_count_threshold,
    available_ratio_threshold,
    check_interval_minutes,
    proxy_failure_window_minutes,
    proxy_failure_threshold,
    alert_emails,
    alert_cooldown_minutes
)
VALUES (
    'openai', 0, 20, 5, 10, 5, '[]'::jsonb, 5
)
ON CONFLICT (platform) DO NOTHING;

-- 2) proxy_usage_metrics_hourly
CREATE TABLE IF NOT EXISTS proxy_usage_metrics_hourly (
    bucket_start   TIMESTAMPTZ NOT NULL,
    platform       VARCHAR(20) NOT NULL,
    proxy_id       BIGINT NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    total_count    BIGINT NOT NULL DEFAULT 0,
    success_count  BIGINT NOT NULL DEFAULT 0,
    failure_count  BIGINT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_start, platform, proxy_id),
    CONSTRAINT chk_proxy_usage_metrics_non_negative
        CHECK (total_count >= 0 AND success_count >= 0 AND failure_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_proxy_usage_metrics_hourly_platform_bucket
    ON proxy_usage_metrics_hourly (platform, bucket_start DESC);

CREATE INDEX IF NOT EXISTS idx_proxy_usage_metrics_hourly_platform_proxy_bucket
    ON proxy_usage_metrics_hourly (platform, proxy_id, bucket_start DESC);

-- 3) Normalize legacy proxy 'disabled' status to 'inactive'
UPDATE proxies
SET status = 'inactive',
    updated_at = NOW()
WHERE status = 'disabled';
