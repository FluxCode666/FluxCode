-- 118_channel_monitors.sql
-- Channel monitor base tables and default settings.

CREATE TABLE IF NOT EXISTS channel_monitors (
    id                BIGSERIAL PRIMARY KEY,
    name              VARCHAR(100) NOT NULL,
    provider          VARCHAR(20)  NOT NULL,
    api_mode          VARCHAR(32)  NOT NULL DEFAULT 'chat_completions',
    endpoint          VARCHAR(500) NOT NULL,
    api_key_encrypted TEXT         NOT NULL,
    primary_model     VARCHAR(200) NOT NULL,
    extra_models      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    group_name        VARCHAR(100) NOT NULL DEFAULT '',
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    interval_seconds  INT          NOT NULL,
    last_checked_at   TIMESTAMPTZ,
    created_by        BIGINT       NOT NULL DEFAULT 0,
    template_id       BIGINT       NULL,
    extra_headers     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off',
    body_override     JSONB        NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitors_provider_check CHECK (provider IN ('openai', 'anthropic', 'gemini')),
    CONSTRAINT channel_monitors_api_mode_check CHECK (api_mode IN ('chat_completions', 'responses')),
    CONSTRAINT channel_monitors_interval_check CHECK (interval_seconds BETWEEN 15 AND 3600),
    CONSTRAINT channel_monitors_body_mode_check CHECK (body_override_mode IN ('off', 'merge', 'replace'))
);

CREATE INDEX IF NOT EXISTS idx_channel_monitors_enabled_last_checked
    ON channel_monitors (enabled, last_checked_at);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_provider
    ON channel_monitors (provider);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_provider_api_mode
    ON channel_monitors (provider, api_mode);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_name
    ON channel_monitors (group_name);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_template_id
    ON channel_monitors (template_id)
    WHERE template_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS channel_monitor_histories (
    id              BIGSERIAL PRIMARY KEY,
    monitor_id      BIGINT       NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE,
    model           VARCHAR(200) NOT NULL,
    status          VARCHAR(20)  NOT NULL,
    latency_ms      INT,
    ping_latency_ms INT,
    message         VARCHAR(500) NOT NULL DEFAULT '',
    checked_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitor_histories_status_check
        CHECK (status IN ('operational', 'degraded', 'failed', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_histories_monitor_model_checked
    ON channel_monitor_histories (monitor_id, model, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_histories_checked_at
    ON channel_monitor_histories (checked_at);

INSERT INTO settings (key, value)
VALUES
    ('channel_monitor_enabled', 'false'),
    ('channel_monitor_default_interval_seconds', '60')
ON CONFLICT (key) DO NOTHING;
