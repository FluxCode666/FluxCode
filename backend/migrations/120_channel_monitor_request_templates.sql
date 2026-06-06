-- 120_channel_monitor_request_templates.sql
-- Request templates for channel monitor custom headers and body overrides.

CREATE TABLE IF NOT EXISTS channel_monitor_request_templates (
    id                 BIGSERIAL    PRIMARY KEY,
    name               VARCHAR(100) NOT NULL,
    provider           VARCHAR(20)  NOT NULL,
    api_mode           VARCHAR(32)  NOT NULL DEFAULT 'chat_completions',
    description        VARCHAR(500) NOT NULL DEFAULT '',
    extra_headers      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    body_override_mode VARCHAR(10)  NOT NULL DEFAULT 'off',
    body_override      JSONB        NULL,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitor_request_templates_provider_check
        CHECK (provider IN ('openai', 'anthropic', 'gemini')),
    CONSTRAINT channel_monitor_request_templates_api_mode_check
        CHECK (api_mode IN ('chat_completions', 'responses')),
    CONSTRAINT channel_monitor_request_templates_body_mode_check
        CHECK (body_override_mode IN ('off', 'merge', 'replace'))
);

CREATE UNIQUE INDEX IF NOT EXISTS channel_monitor_request_templates_provider_name
    ON channel_monitor_request_templates (provider, name);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_templates_provider_api_mode
    ON channel_monitor_request_templates (provider, api_mode);

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS template_id BIGINT NULL;
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS extra_headers JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS body_override_mode VARCHAR(10) NOT NULL DEFAULT 'off';
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS body_override JSONB NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'channel_monitors_template_id_fkey'
          AND table_name = 'channel_monitors'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_template_id_fkey
            FOREIGN KEY (template_id)
            REFERENCES channel_monitor_request_templates (id)
            ON DELETE SET NULL;
    END IF;
END $$;
