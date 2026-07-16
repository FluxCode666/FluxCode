package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAtomicMediaTaskMetricsAggregatesCountsAndDurations(t *testing.T) {
	metrics := NewAtomicMediaTaskMetrics()

	metrics.ObserveStage(MediaTypeImage, MediaTaskStagePolling, 10*time.Millisecond)
	metrics.ObserveStage(MediaTypeImage, MediaTaskStagePolling, 15*time.Millisecond)
	metrics.IncrementRecovery(MediaTypeImage)
	metrics.IncrementDuplicateMessage(MediaTypeImage)
	metrics.IncrementStorageFailure(MediaTypeVideo)
	metrics.IncrementSettlementRetry(MediaTypeVideo)

	count, elapsed := metrics.StageSnapshot(MediaTypeImage, MediaTaskStagePolling)
	require.Equal(t, int64(2), count)
	require.Equal(t, 25*time.Millisecond, elapsed)
	require.Equal(t, int64(1), metrics.Recoveries())
	require.Equal(t, int64(1), metrics.DuplicateMessages())
	require.Equal(t, int64(1), metrics.StorageFailures())
	require.Equal(t, int64(1), metrics.SettlementRetries())
}

func TestAtomicMediaTaskMetricsIgnoresUnknownDimensions(t *testing.T) {
	metrics := NewAtomicMediaTaskMetrics()
	metrics.ObserveStage(MediaType("audio"), MediaTaskStage("unknown"), time.Second)
	metrics.IncrementRecovery(MediaType("audio"))

	count, elapsed := metrics.StageSnapshot(MediaTypeImage, MediaTaskStagePolling)
	require.Zero(t, count)
	require.Zero(t, elapsed)
	require.Zero(t, metrics.Recoveries())
}
