-- Persist daily subscription grant exhaustion snapshots for the admin dashboard.
-- Rows are refreshed by the dashboard aggregation job and can be rebuilt by backfill/recompute.

CREATE TABLE IF NOT EXISTS subscription_exhaustion_daily_stats (
    bucket_date DATE PRIMARY KEY,
    total_subscriptions BIGINT NOT NULL DEFAULT 0,
    exhausted_subscriptions BIGINT NOT NULL DEFAULT 0,
    exhaustion_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_exhaustion_daily_stats_bucket_date
    ON subscription_exhaustion_daily_stats (bucket_date DESC);

COMMENT ON TABLE subscription_exhaustion_daily_stats IS 'Daily subscription grant exhaustion snapshots for admin dashboard.';
COMMENT ON COLUMN subscription_exhaustion_daily_stats.bucket_date IS 'Local date of the day bucket.';
COMMENT ON COLUMN subscription_exhaustion_daily_stats.total_subscriptions IS 'Number of active subscription grants on this date.';
COMMENT ON COLUMN subscription_exhaustion_daily_stats.exhausted_subscriptions IS 'Number of active subscription grants whose daily, weekly, or monthly quota is exhausted on this date.';
COMMENT ON COLUMN subscription_exhaustion_daily_stats.exhaustion_rate IS 'exhausted_subscriptions / total_subscriptions * 100.';
