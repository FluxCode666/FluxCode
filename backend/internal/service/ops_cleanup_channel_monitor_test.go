package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

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
