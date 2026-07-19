-- Public model-performance rollups use complete UTC hour buckets.
-- One row represents either all groups (group_id IS NULL) or one group.

CREATE TABLE IF NOT EXISTS model_performance_metrics_hourly (
    id BIGSERIAL PRIMARY KEY,

    bucket_start TIMESTAMPTZ NOT NULL,
    model VARCHAR(100) NOT NULL,
    group_id BIGINT,

    success_count BIGINT NOT NULL DEFAULT 0,
    valid_failure_count BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    total_first_token_ms BIGINT NOT NULL DEFAULT 0,
    first_token_count BIGINT NOT NULL DEFAULT 0,

    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- PostgreSQL treats NULL values as distinct in ordinary unique constraints.
-- Coalescing the nullable group dimension guarantees a single all-groups row.
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_performance_metrics_hourly_unique_dim
    ON model_performance_metrics_hourly (
        bucket_start,
        model,
        COALESCE(group_id, 0)
    );

-- Public queries use the all-groups row for the model card and detail summary.
CREATE INDEX IF NOT EXISTS idx_model_performance_metrics_hourly_model_bucket
    ON model_performance_metrics_hourly (model, bucket_start DESC)
    WHERE group_id IS NULL;

-- Per-group detail and selected-group cards use the group-specific rows.
CREATE INDEX IF NOT EXISTS idx_model_performance_metrics_hourly_model_group_bucket
    ON model_performance_metrics_hourly (model, group_id, bucket_start DESC)
    WHERE group_id IS NOT NULL;

-- Cleanup deletes by bucket independently of model or group.
CREATE INDEX IF NOT EXISTS idx_model_performance_metrics_hourly_bucket
    ON model_performance_metrics_hourly (bucket_start DESC);

COMMENT ON TABLE model_performance_metrics_hourly IS
    'UTC hourly model performance aggregates for public model pricing pages.';

-- This progress marker is intentionally independent from aggregate rows: a period with no
-- samples still advances the aggregation cursor, avoiding repeated seven-day backfills.
CREATE TABLE IF NOT EXISTS model_performance_metrics_aggregation_watermark (
    id INT PRIMARY KEY DEFAULT 1,
    last_aggregated_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT model_performance_metrics_aggregation_watermark_singleton CHECK (id = 1)
);

INSERT INTO model_performance_metrics_aggregation_watermark (id, last_aggregated_at, updated_at)
VALUES (1, NULL, NOW())
ON CONFLICT (id) DO NOTHING;
