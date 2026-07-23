package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newModelPerformanceAggregationService(t *testing.T, repo OpsRepository) (*OpsAggregationService, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return NewOpsAggregationService(repo, nil, db, nil, &config.Config{Ops: config.OpsConfig{
		Enabled:     true,
		Aggregation: config.OpsAggregationConfig{Enabled: true},
	}}), mock
}

func expectModelPerformanceAggregationLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_try_advisory_lock($1)")).
		WithArgs(hashAdvisoryLockID(opsAggModelPerformanceHourlyLeaderLockKey)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(hashAdvisoryLockID(opsAggModelPerformanceHourlyLeaderLockKey)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestOpsAggregationModelPerformanceFirstRunBackfillsSevenDays(t *testing.T) {
	var windows [][2]time.Time
	var watermarkUpdates []time.Time
	var heartbeats []*OpsUpsertJobHeartbeatInput
	repo := &opsRepoMock{
		UpsertModelPerformanceHourlyMetricsFn: func(_ context.Context, start, end time.Time) error {
			windows = append(windows, [2]time.Time{start, end})
			return nil
		},
		UpdateModelPerformanceMetricsAggregationWatermarkFn: func(_ context.Context, at time.Time) error {
			watermarkUpdates = append(watermarkUpdates, at)
			return nil
		},
		UpsertJobHeartbeatFn: func(_ context.Context, input *OpsUpsertJobHeartbeatInput) error {
			heartbeats = append(heartbeats, input)
			return nil
		},
	}
	svc, mock := newModelPerformanceAggregationService(t, repo)
	expectModelPerformanceAggregationLock(mock)

	now := time.Date(2026, time.July, 20, 12, 34, 56, 0, time.UTC)
	svc.aggregateModelPerformanceHourlyAt(now)

	end := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	start := end.Add(-opsAggModelPerformanceBackfillWindow)
	require.Len(t, windows, 7)
	require.Equal(t, start, windows[0][0])
	require.Equal(t, end, windows[len(windows)-1][1])
	for i, window := range windows {
		require.Equal(t, opsAggHourlyChunk, window[1].Sub(window[0]))
		if i > 0 {
			require.Equal(t, windows[i-1][1], window[0])
		}
	}
	require.Equal(t, []time.Time{end}, watermarkUpdates)
	require.Len(t, heartbeats, 1)
	require.Equal(t, opsAggModelPerformanceHourlyJobName, heartbeats[0].JobName)
	require.NotNil(t, heartbeats[0].LastSuccessAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsAggregationModelPerformanceRecomputesWatermarkOverlap(t *testing.T) {
	var windows [][2]time.Time
	var watermarkUpdates []time.Time
	watermark := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	repo := &opsRepoMock{
		GetModelPerformanceMetricsAggregationWatermarkFn: func(context.Context) (*time.Time, error) {
			return &watermark, nil
		},
		UpsertModelPerformanceHourlyMetricsFn: func(_ context.Context, start, end time.Time) error {
			windows = append(windows, [2]time.Time{start, end})
			return nil
		},
		UpdateModelPerformanceMetricsAggregationWatermarkFn: func(_ context.Context, at time.Time) error {
			watermarkUpdates = append(watermarkUpdates, at)
			return nil
		},
	}
	svc, mock := newModelPerformanceAggregationService(t, repo)
	expectModelPerformanceAggregationLock(mock)

	now := time.Date(2026, time.July, 20, 13, 34, 56, 0, time.UTC)
	svc.aggregateModelPerformanceHourlyAt(now)

	end := time.Date(2026, time.July, 20, 13, 0, 0, 0, time.UTC)
	require.Equal(t, [][2]time.Time{{watermark.Add(-opsAggHourlyOverlap), end}}, windows)
	require.Equal(t, []time.Time{end}, watermarkUpdates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpsAggregationModelPerformanceFailureDoesNotAdvanceWatermarkAndRecordsHeartbeat(t *testing.T) {
	var watermarkUpdates []time.Time
	var heartbeats []*OpsUpsertJobHeartbeatInput
	repo := &opsRepoMock{
		UpsertModelPerformanceHourlyMetricsFn: func(context.Context, time.Time, time.Time) error {
			return errors.New("model performance upsert failed")
		},
		UpdateModelPerformanceMetricsAggregationWatermarkFn: func(_ context.Context, at time.Time) error {
			watermarkUpdates = append(watermarkUpdates, at)
			return nil
		},
		UpsertJobHeartbeatFn: func(_ context.Context, input *OpsUpsertJobHeartbeatInput) error {
			heartbeats = append(heartbeats, input)
			return nil
		},
	}
	svc, mock := newModelPerformanceAggregationService(t, repo)
	expectModelPerformanceAggregationLock(mock)

	svc.aggregateModelPerformanceHourlyAt(time.Date(2026, time.July, 20, 12, 34, 56, 0, time.UTC))

	require.Empty(t, watermarkUpdates)
	require.Len(t, heartbeats, 1)
	require.Equal(t, opsAggModelPerformanceHourlyJobName, heartbeats[0].JobName)
	require.NotNil(t, heartbeats[0].LastErrorAt)
	require.NotNil(t, heartbeats[0].LastError)
	require.Nil(t, heartbeats[0].LastSuccessAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
