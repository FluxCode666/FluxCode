package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// modelPerformanceMetricsReader reads only pre-aggregated, public-safe model
// performance rows. It intentionally cannot access request or error details.
type modelPerformanceMetricsReader struct {
	db *sql.DB
}

func NewModelPerformanceMetricsReader(db *sql.DB) service.ModelPerformanceMetricsReader {
	return &modelPerformanceMetricsReader{db: db}
}

func (r *modelPerformanceMetricsReader) ListModelPerformanceSummaries(ctx context.Context, window service.ModelPerformanceWindow, models []string, groupID *int64) (map[string]service.ModelPerformanceMetrics, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil model performance metrics reader")
	}
	if !validModelPerformanceWindow(window) || len(models) == 0 {
		return map[string]service.ModelPerformanceMetrics{}, nil
	}

	query := `
SELECT
  model,
  SUM(success_count)::bigint AS success_count,
  SUM(valid_failure_count)::bigint AS valid_failure_count,
  SUM(output_tokens)::bigint AS output_tokens,
  SUM(total_duration_ms)::bigint AS total_duration_ms,
  SUM(total_first_token_ms)::bigint AS total_first_token_ms,
  SUM(first_token_count)::bigint AS first_token_count
FROM model_performance_metrics_hourly
WHERE bucket_start >= $1
  AND bucket_start < $2
  AND model = ANY($3)`
	args := []any{window.Start.UTC(), window.End.UTC(), pq.Array(models)}
	if groupID == nil {
		query += `
  AND group_id IS NULL`
	} else {
		query += `
  AND group_id = $4`
		args = append(args, *groupID)
	}
	query += `
GROUP BY model
ORDER BY model ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	metrics := make(map[string]service.ModelPerformanceMetrics, len(models))
	for rows.Next() {
		var model string
		var totals modelPerformanceMetricTotals
		if err := rows.Scan(
			&model,
			&totals.successCount,
			&totals.validFailureCount,
			&totals.outputTokens,
			&totals.totalDurationMS,
			&totals.totalFirstTokenMS,
			&totals.firstTokenCount,
		); err != nil {
			return nil, err
		}
		metrics[model] = totals.metrics()
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (r *modelPerformanceMetricsReader) GetModelPerformanceDetail(ctx context.Context, window service.ModelPerformanceWindow, model string, groupIDs []int64) (*service.ModelPerformanceDetail, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil model performance metrics reader")
	}
	detail := &service.ModelPerformanceDetail{Groups: map[int64]service.ModelPerformanceMetrics{}}
	if !validModelPerformanceWindow(window) || model == "" {
		return detail, nil
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
  group_id,
  SUM(success_count)::bigint AS success_count,
  SUM(valid_failure_count)::bigint AS valid_failure_count,
  SUM(output_tokens)::bigint AS output_tokens,
  SUM(total_duration_ms)::bigint AS total_duration_ms,
  SUM(total_first_token_ms)::bigint AS total_first_token_ms,
  SUM(first_token_count)::bigint AS first_token_count
FROM model_performance_metrics_hourly
WHERE bucket_start >= $1
  AND bucket_start < $2
  AND model = $3
  AND (group_id IS NULL OR group_id = ANY($4))
GROUP BY group_id
ORDER BY group_id ASC NULLS FIRST`, window.Start.UTC(), window.End.UTC(), model, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var groupID sql.NullInt64
		var totals modelPerformanceMetricTotals
		if err := rows.Scan(
			&groupID,
			&totals.successCount,
			&totals.validFailureCount,
			&totals.outputTokens,
			&totals.totalDurationMS,
			&totals.totalFirstTokenMS,
			&totals.firstTokenCount,
		); err != nil {
			return nil, err
		}
		if groupID.Valid {
			detail.Groups[groupID.Int64] = totals.metrics()
		} else {
			detail.Overall = totals.metrics()
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	detail.Trend, err = r.listModelPerformanceOverallTrend(ctx, window, model)
	if err != nil {
		return nil, err
	}
	return detail, nil
}

func (r *modelPerformanceMetricsReader) listModelPerformanceOverallTrend(ctx context.Context, window service.ModelPerformanceWindow, model string) ([]service.ModelPerformanceHourlyTrendPoint, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  bucket_start,
  success_count,
  valid_failure_count,
  total_first_token_ms,
  first_token_count
FROM model_performance_metrics_hourly
WHERE bucket_start >= $1
  AND bucket_start < $2
  AND model = $3
  AND group_id IS NULL
ORDER BY bucket_start ASC`, window.Start.UTC(), window.End.UTC(), model)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	byBucket := make(map[time.Time]modelPerformanceMetricTotals)
	for rows.Next() {
		var bucket time.Time
		var totals modelPerformanceMetricTotals
		if err := rows.Scan(&bucket, &totals.successCount, &totals.validFailureCount, &totals.totalFirstTokenMS, &totals.firstTokenCount); err != nil {
			return nil, err
		}
		byBucket[bucket.UTC()] = totals
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points := make([]service.ModelPerformanceHourlyTrendPoint, 0, int(window.End.Sub(window.Start)/time.Hour))
	for bucket := window.Start.UTC(); bucket.Before(window.End.UTC()); bucket = bucket.Add(time.Hour) {
		point := service.ModelPerformanceHourlyTrendPoint{BucketStart: bucket}
		if totals, ok := byBucket[bucket]; ok {
			metrics := totals.metrics()
			point.Availability = metrics.Availability
			point.AverageFirstToken = metrics.AverageFirstToken
		}
		points = append(points, point)
	}
	return points, nil
}

type modelPerformanceMetricTotals struct {
	successCount      int64
	validFailureCount int64
	outputTokens      int64
	totalDurationMS   int64
	totalFirstTokenMS int64
	firstTokenCount   int64
}

func (t modelPerformanceMetricTotals) metrics() service.ModelPerformanceMetrics {
	metrics := service.ModelPerformanceMetrics{}
	if denominator := t.successCount + t.validFailureCount; denominator > 0 {
		availability := float64(t.successCount) * 100 / float64(denominator)
		metrics.Availability = &availability
	}
	if t.totalDurationMS > 0 {
		tps := float64(t.outputTokens) * 1000 / float64(t.totalDurationMS)
		metrics.TPS = &tps
	}
	if t.successCount > 0 {
		averageRequestTime := float64(t.totalDurationMS) / float64(t.successCount)
		metrics.AverageRequestTime = &averageRequestTime
	}
	if t.firstTokenCount > 0 {
		averageFirstToken := float64(t.totalFirstTokenMS) / float64(t.firstTokenCount)
		metrics.AverageFirstToken = &averageFirstToken
	}
	return metrics
}

func validModelPerformanceWindow(window service.ModelPerformanceWindow) bool {
	return !window.Start.IsZero() && !window.End.IsZero() && window.End.After(window.Start)
}
