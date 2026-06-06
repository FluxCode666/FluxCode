-- 119_channel_monitor_aggregation.sql
-- Daily rollups for channel monitor history and a singleton aggregation watermark.

CREATE TABLE IF NOT EXISTS channel_monitor_daily_rollups (
    id                    BIGSERIAL PRIMARY KEY,
    monitor_id            BIGINT       NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE,
    model                 VARCHAR(200) NOT NULL,
    bucket_date           DATE         NOT NULL,
    total_checks          INT          NOT NULL DEFAULT 0,
    ok_count              INT          NOT NULL DEFAULT 0,
    operational_count     INT          NOT NULL DEFAULT 0,
    degraded_count        INT          NOT NULL DEFAULT 0,
    failed_count          INT          NOT NULL DEFAULT 0,
    error_count           INT          NOT NULL DEFAULT 0,
    sum_latency_ms        BIGINT       NOT NULL DEFAULT 0,
    count_latency         INT          NOT NULL DEFAULT 0,
    sum_ping_latency_ms   BIGINT       NOT NULL DEFAULT 0,
    count_ping_latency    INT          NOT NULL DEFAULT 0,
    computed_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_monitor_daily_rollups_unique
    ON channel_monitor_daily_rollups (monitor_id, model, bucket_date);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_daily_rollups_bucket
    ON channel_monitor_daily_rollups (bucket_date);

CREATE TABLE IF NOT EXISTS channel_monitor_aggregation_watermark (
    id                   INT          PRIMARY KEY DEFAULT 1,
    last_aggregated_date DATE,
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitor_aggregation_watermark_singleton CHECK (id = 1)
);

INSERT INTO channel_monitor_aggregation_watermark (id, last_aggregated_date, updated_at)
VALUES (1, NULL, NOW())
ON CONFLICT (id) DO NOTHING;
