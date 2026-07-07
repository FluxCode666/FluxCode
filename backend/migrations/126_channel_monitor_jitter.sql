-- 126_channel_monitor_jitter.sql
-- Add per-monitor scheduling jitter. A value of 0 preserves fixed interval behavior.

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS jitter_seconds INTEGER NOT NULL DEFAULT 0;

ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_jitter_check;

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_jitter_check
    CHECK (jitter_seconds >= 0 AND interval_seconds - jitter_seconds >= 15);
