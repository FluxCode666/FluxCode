package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUpsertModelPerformanceHourlyMetricsUsesPublicModelAndValidFailuresOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	end := start.Add(time.Hour)
	mock.ExpectExec(`(?s)INSERT INTO model_performance_metrics_hourly`).
		WithArgs(start.UTC(), end.UTC()).
		WillReturnResult(sqlmock.NewResult(0, 4))

	repo := &opsRepository{db: db}
	require.NoError(t, repo.UpsertModelPerformanceHourlyMetrics(context.Background(), start, end))
	require.NoError(t, mock.ExpectationsWereMet())

	// requested_model is what visitors see; never split this public metric by the
	// upstream mapping stored in model.
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "COALESCE(NULLIF(TRIM(ul.requested_model), ''), NULLIF(TRIM(ul.model), ''))")
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "COALESCE(NULLIF(TRIM(el.requested_model), ''), NULLIF(TRIM(el.model), ''))")
	// User-side limitations, including expired API keys classified by the error
	// logger, do not enter the model availability denominator.
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "COALESCE(el.is_business_limited, FALSE) = FALSE")
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "el.status_code >= 400")
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "WHERE group_id IS NOT NULL")
	// The deletion and INSERT are one statement. Thus an overlap recomputation
	// also removes stale rows when an error is later reclassified as a business
	// limitation and no longer produces a combined aggregate row.
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "WITH deleted_window AS")
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "DELETE FROM model_performance_metrics_hourly")
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "WHERE bucket_start >= $1 AND bucket_start < $2")
	require.Contains(t, modelPerformanceHourlyUpsertSQL, "ON CONFLICT (bucket_start, model, COALESCE(group_id, 0)) DO UPDATE SET")
}

func TestModelPerformanceAggregationWatermarkRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	watermark := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT last_aggregated_at
FROM model_performance_metrics_aggregation_watermark
WHERE id = 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"last_aggregated_at"}).AddRow(watermark))
	mock.ExpectExec(`(?s)INSERT INTO model_performance_metrics_aggregation_watermark`).
		WithArgs(watermark).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &opsRepository{db: db}
	got, err := repo.GetModelPerformanceMetricsAggregationWatermark(context.Background())
	require.NoError(t, err)
	require.Equal(t, watermark, *got)
	require.NoError(t, repo.UpdateModelPerformanceMetricsAggregationWatermark(context.Background(), watermark))
	require.NoError(t, mock.ExpectationsWereMet())
}
