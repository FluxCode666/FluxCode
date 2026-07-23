package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// modelPerformanceHourlyUpsertSQL rebuilds a closed UTC interval from source
// logs. Each source is first rolled up into an all-groups row and, where a
// group is known, a group row. Combining those contributions before the final
// UPSERT makes retries overwrite totals instead of incrementing them.
const modelPerformanceHourlyUpsertSQL = `
WITH deleted_window AS (
  DELETE FROM model_performance_metrics_hourly
  WHERE bucket_start >= $1 AND bucket_start < $2
),
usage_base AS (
  SELECT
    date_trunc('hour', ul.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket_start,
    COALESCE(NULLIF(TRIM(ul.requested_model), ''), NULLIF(TRIM(ul.model), '')) AS model,
    ul.group_id,
    COALESCE(ul.output_tokens, 0)::bigint AS output_tokens,
    ul.duration_ms,
    ul.first_token_ms
  FROM usage_logs ul
  WHERE ul.created_at >= $1 AND ul.created_at < $2
    AND COALESCE(NULLIF(TRIM(ul.requested_model), ''), NULLIF(TRIM(ul.model), '')) IS NOT NULL
),
usage_rollups AS (
  SELECT
    bucket_start,
    model,
    NULL::bigint AS group_id,
    COUNT(*)::bigint AS success_count,
    COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(duration_ms) FILTER (WHERE duration_ms IS NOT NULL), 0)::bigint AS total_duration_ms,
    COALESCE(SUM(first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL), 0)::bigint AS total_first_token_ms,
    COUNT(*) FILTER (WHERE first_token_ms IS NOT NULL)::bigint AS first_token_count
  FROM usage_base
  GROUP BY bucket_start, model

  UNION ALL

  SELECT
    bucket_start,
    model,
    group_id,
    COUNT(*)::bigint AS success_count,
    COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(duration_ms) FILTER (WHERE duration_ms IS NOT NULL), 0)::bigint AS total_duration_ms,
    COALESCE(SUM(first_token_ms) FILTER (WHERE first_token_ms IS NOT NULL), 0)::bigint AS total_first_token_ms,
    COUNT(*) FILTER (WHERE first_token_ms IS NOT NULL)::bigint AS first_token_count
  FROM usage_base
  WHERE group_id IS NOT NULL
  GROUP BY bucket_start, model, group_id
),
error_base AS (
  SELECT
    date_trunc('hour', el.created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket_start,
    COALESCE(NULLIF(TRIM(el.requested_model), ''), NULLIF(TRIM(el.model), '')) AS model,
    el.group_id
  FROM ops_error_logs el
  WHERE el.created_at >= $1 AND el.created_at < $2
    AND COALESCE(el.is_business_limited, FALSE) = FALSE
    AND el.status_code >= 400
    AND el.is_count_tokens = FALSE
    AND COALESCE(NULLIF(TRIM(el.requested_model), ''), NULLIF(TRIM(el.model), '')) IS NOT NULL
),
error_rollups AS (
  SELECT
    bucket_start,
    model,
    NULL::bigint AS group_id,
    COUNT(*)::bigint AS valid_failure_count
  FROM error_base
  GROUP BY bucket_start, model

  UNION ALL

  SELECT
    bucket_start,
    model,
    group_id,
    COUNT(*)::bigint AS valid_failure_count
  FROM error_base
  WHERE group_id IS NOT NULL
  GROUP BY bucket_start, model, group_id
),
combined AS (
  SELECT
    bucket_start,
    model,
    group_id,
    COALESCE(SUM(success_count), 0)::bigint AS success_count,
    COALESCE(SUM(valid_failure_count), 0)::bigint AS valid_failure_count,
    COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(total_duration_ms), 0)::bigint AS total_duration_ms,
    COALESCE(SUM(total_first_token_ms), 0)::bigint AS total_first_token_ms,
    COALESCE(SUM(first_token_count), 0)::bigint AS first_token_count
  FROM (
    SELECT
      bucket_start,
      model,
      group_id,
      success_count,
      0::bigint AS valid_failure_count,
      output_tokens,
      total_duration_ms,
      total_first_token_ms,
      first_token_count
    FROM usage_rollups

    UNION ALL

    SELECT
      bucket_start,
      model,
      group_id,
      0::bigint AS success_count,
      valid_failure_count,
      0::bigint AS output_tokens,
      0::bigint AS total_duration_ms,
      0::bigint AS total_first_token_ms,
      0::bigint AS first_token_count
    FROM error_rollups
  ) contributions
  GROUP BY bucket_start, model, group_id
)
INSERT INTO model_performance_metrics_hourly (
  bucket_start,
  model,
  group_id,
  success_count,
  valid_failure_count,
  output_tokens,
  total_duration_ms,
  total_first_token_ms,
  first_token_count,
  computed_at
)
SELECT
  bucket_start,
  model,
  group_id,
  success_count,
  valid_failure_count,
  output_tokens,
  total_duration_ms,
  total_first_token_ms,
  first_token_count,
  NOW()
FROM combined
ON CONFLICT (bucket_start, model, COALESCE(group_id, 0)) DO UPDATE SET
  success_count = EXCLUDED.success_count,
  valid_failure_count = EXCLUDED.valid_failure_count,
  output_tokens = EXCLUDED.output_tokens,
  total_duration_ms = EXCLUDED.total_duration_ms,
  total_first_token_ms = EXCLUDED.total_first_token_ms,
  first_token_count = EXCLUDED.first_token_count,
  computed_at = NOW()
`

func (r *opsRepository) UpsertModelPerformanceHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	_, err := r.db.ExecContext(ctx, modelPerformanceHourlyUpsertSQL, startTime.UTC(), endTime.UTC())
	return err
}

func (r *opsRepository) GetModelPerformanceMetricsAggregationWatermark(ctx context.Context) (*time.Time, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT last_aggregated_at
FROM model_performance_metrics_aggregation_watermark
WHERE id = 1`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) || !value.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	watermark := value.Time.UTC()
	return &watermark, nil
}

func (r *opsRepository) UpdateModelPerformanceMetricsAggregationWatermark(ctx context.Context, lastAggregatedAt time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if lastAggregatedAt.IsZero() {
		return fmt.Errorf("model performance aggregation watermark is required")
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO model_performance_metrics_aggregation_watermark (
  id,
  last_aggregated_at,
  updated_at
) VALUES (1, $1, NOW())
ON CONFLICT (id) DO UPDATE SET
  last_aggregated_at = EXCLUDED.last_aggregated_at,
  updated_at = NOW()`, lastAggregatedAt.UTC())
	return err
}
