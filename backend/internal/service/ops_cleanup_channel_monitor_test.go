package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpsCleanupRunsChannelMonitorMaintenance(t *testing.T) {
	monitorRepo := &opsCleanupChannelMonitorRepoStub{}
	monitorSvc := &ChannelMonitorService{repo: monitorRepo}
	cleanup := &OpsCleanupService{
		db:                &sql.DB{},
		cfg:               &config.Config{},
		channelMonitorSvc: monitorSvc,
	}

	_, err := cleanup.runCleanupOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, monitorRepo.loadWatermarkCalls)
	require.Equal(t, 1, monitorRepo.deleteHistoryCalls)
	require.Equal(t, 1, monitorRepo.deleteRollupsCalls)
}

func TestModelPerformanceRetentionDaysNeverDropsBelowPublicWindow(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0, modelPerformanceRetentionDays(0))
	require.Equal(t, 7, modelPerformanceRetentionDays(1))
	require.Equal(t, 7, modelPerformanceRetentionDays(7))
	require.Equal(t, 30, modelPerformanceRetentionDays(30))
}

func TestOpsCleanupRetainsModelPerformanceMetricsForAtLeastSevenDays(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	startedAt := time.Now().UTC()
	modelPerformanceCutoff := startedAt.AddDate(0, 0, -7)
	mock.ExpectExec("ops_metrics_hourly").
		WithArgs(sqlmock.AnyArg(), 5000).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("model_performance_metrics_hourly").
		WithArgs(timeInRange{from: modelPerformanceCutoff, to: modelPerformanceCutoff.Add(time.Second)}, 5000).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ops_metrics_daily").
		WithArgs(sqlmock.AnyArg(), 5000).
		WillReturnResult(sqlmock.NewResult(0, 0))

	cleanup := &OpsCleanupService{
		db: db,
		cfg: &config.Config{Ops: config.OpsConfig{Cleanup: config.OpsCleanupConfig{
			HourlyMetricsRetentionDays: 1,
		}}},
	}

	_, err = cleanup.runCleanupOnce(context.Background())
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

type timeInRange struct {
	from time.Time
	to   time.Time
}

func (m timeInRange) Match(value driver.Value) bool {
	actual, ok := value.(time.Time)
	return ok && !actual.Before(m.from) && !actual.After(m.to)
}

type opsCleanupChannelMonitorRepoStub struct {
	channelMonitorRunnerRepoStub
	loadWatermarkCalls int
	deleteHistoryCalls int
	deleteRollupsCalls int
}

func (s *opsCleanupChannelMonitorRepoStub) LoadAggregationWatermark(context.Context) (*time.Time, error) {
	s.loadWatermarkCalls++
	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	return &yesterday, nil
}

func (s *opsCleanupChannelMonitorRepoStub) DeleteHistoryBefore(context.Context, time.Time) (int64, error) {
	s.deleteHistoryCalls++
	return 0, nil
}

func (s *opsCleanupChannelMonitorRepoStub) DeleteRollupsBefore(context.Context, time.Time) (int64, error) {
	s.deleteRollupsCalls++
	return 0, nil
}
