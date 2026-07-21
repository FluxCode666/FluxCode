package service

import (
	"sync"
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

func TestAtomicMediaTaskMetricsTracksFixedRoutingDimensions(t *testing.T) {
	metrics := NewAtomicMediaTaskMetrics()
	statuses := []MediaAdapterResolutionStatus{
		MediaAdapterResolutionInvalidDefinition,
		MediaAdapterResolutionUnresolved,
		MediaAdapterResolutionAmbiguous,
		MediaAdapterResolutionImplementationMissing,
		MediaAdapterResolutionCapabilityMismatch,
	}
	for _, status := range statuses {
		metrics.IncrementAdapterResolutionFailure(status)
		metrics.IncrementAdapterResolutionFailure(status)
	}
	metrics.IncrementAdapterResolutionFailure(MediaAdapterResolutionReady)
	metrics.IncrementAdapterResolutionFailure(MediaAdapterResolutionStatus("unknown"))
	metrics.IncrementCandidateCapabilityMismatch()
	metrics.IncrementHistoricalAdapterAliasResolution()

	for _, status := range statuses {
		require.Equal(t, int64(2), metrics.AdapterResolutionFailures(status))
	}
	require.Zero(t, metrics.AdapterResolutionFailures(MediaAdapterResolutionReady))
	require.Zero(t, metrics.AdapterResolutionFailures(MediaAdapterResolutionStatus("unknown")))
	require.Equal(t, int64(1), metrics.CandidateCapabilityMismatches())
	require.Equal(t, int64(1), metrics.HistoricalAdapterAliasResolutions())

	var nilMetrics *AtomicMediaTaskMetrics
	nilMetrics.IncrementAdapterResolutionFailure(MediaAdapterResolutionUnresolved)
	nilMetrics.IncrementCandidateCapabilityMismatch()
	nilMetrics.IncrementHistoricalAdapterAliasResolution()
	require.Zero(t, nilMetrics.AdapterResolutionFailures(MediaAdapterResolutionUnresolved))
	require.Zero(t, nilMetrics.CandidateCapabilityMismatches())
	require.Zero(t, nilMetrics.HistoricalAdapterAliasResolutions())
}

func TestAtomicMediaTaskMetricsRoutingCountersAreConcurrentSafe(t *testing.T) {
	metrics := NewAtomicMediaTaskMetrics()
	const goroutines = 32
	const iterations = 200
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				metrics.IncrementAdapterResolutionFailure(MediaAdapterResolutionUnresolved)
				metrics.IncrementCandidateCapabilityMismatch()
				metrics.IncrementHistoricalAdapterAliasResolution()
			}
		}()
	}
	wg.Wait()

	expected := int64(goroutines * iterations)
	require.Equal(t, expected, metrics.AdapterResolutionFailures(MediaAdapterResolutionUnresolved))
	require.Equal(t, expected, metrics.CandidateCapabilityMismatches())
	require.Equal(t, expected, metrics.HistoricalAdapterAliasResolutions())
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
