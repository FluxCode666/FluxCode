package service

import (
	"sync/atomic"
	"time"
)

const (
	mediaMetricTypeCount  = 2
	mediaMetricStageCount = 9
)

type mediaStageMetric struct {
	count        atomic.Int64
	elapsedNanos atomic.Int64
}

// AtomicMediaTaskMetrics 使用固定维度的原子计数器聚合 Worker 指标。
// 未知媒体类型或阶段会被忽略，避免由无界标签创建高基数状态。
type AtomicMediaTaskMetrics struct {
	stages            [mediaMetricTypeCount][mediaMetricStageCount]mediaStageMetric
	recoveries        [mediaMetricTypeCount]atomic.Int64
	duplicates        [mediaMetricTypeCount]atomic.Int64
	storageFailures   [mediaMetricTypeCount]atomic.Int64
	settlementRetries [mediaMetricTypeCount]atomic.Int64
}

func NewAtomicMediaTaskMetrics() *AtomicMediaTaskMetrics {
	return &AtomicMediaTaskMetrics{}
}

func (m *AtomicMediaTaskMetrics) ObserveStage(mediaType MediaType, stage MediaTaskStage, elapsed time.Duration) {
	typeIndex, typeOK := mediaMetricTypeIndex(mediaType)
	stageIndex, stageOK := mediaMetricStageIndex(stage)
	if m == nil || !typeOK || !stageOK || elapsed < 0 {
		return
	}
	metric := &m.stages[typeIndex][stageIndex]
	metric.count.Add(1)
	metric.elapsedNanos.Add(int64(elapsed))
}

func (m *AtomicMediaTaskMetrics) IncrementRecovery(mediaType MediaType) {
	if m == nil {
		return
	}
	m.incrementByMediaType(mediaType, &m.recoveries)
}

func (m *AtomicMediaTaskMetrics) IncrementDuplicateMessage(mediaType MediaType) {
	if m == nil {
		return
	}
	m.incrementByMediaType(mediaType, &m.duplicates)
}

func (m *AtomicMediaTaskMetrics) IncrementStorageFailure(mediaType MediaType) {
	if m == nil {
		return
	}
	m.incrementByMediaType(mediaType, &m.storageFailures)
}

func (m *AtomicMediaTaskMetrics) IncrementSettlementRetry(mediaType MediaType) {
	if m == nil {
		return
	}
	m.incrementByMediaType(mediaType, &m.settlementRetries)
}

func (m *AtomicMediaTaskMetrics) incrementByMediaType(mediaType MediaType, counters *[mediaMetricTypeCount]atomic.Int64) {
	index, ok := mediaMetricTypeIndex(mediaType)
	if !ok {
		return
	}
	counters[index].Add(1)
}

func (m *AtomicMediaTaskMetrics) StageSnapshot(mediaType MediaType, stage MediaTaskStage) (int64, time.Duration) {
	typeIndex, typeOK := mediaMetricTypeIndex(mediaType)
	stageIndex, stageOK := mediaMetricStageIndex(stage)
	if m == nil || !typeOK || !stageOK {
		return 0, 0
	}
	metric := &m.stages[typeIndex][stageIndex]
	return metric.count.Load(), time.Duration(metric.elapsedNanos.Load())
}

func (m *AtomicMediaTaskMetrics) Recoveries() int64 {
	if m == nil {
		return 0
	}
	return sumMediaMetricCounters(m, &m.recoveries)
}

func (m *AtomicMediaTaskMetrics) DuplicateMessages() int64 {
	if m == nil {
		return 0
	}
	return sumMediaMetricCounters(m, &m.duplicates)
}

func (m *AtomicMediaTaskMetrics) StorageFailures() int64 {
	if m == nil {
		return 0
	}
	return sumMediaMetricCounters(m, &m.storageFailures)
}

func (m *AtomicMediaTaskMetrics) SettlementRetries() int64 {
	if m == nil {
		return 0
	}
	return sumMediaMetricCounters(m, &m.settlementRetries)
}

func sumMediaMetricCounters(m *AtomicMediaTaskMetrics, counters *[mediaMetricTypeCount]atomic.Int64) int64 {
	if m == nil {
		return 0
	}
	var total int64
	for index := range counters {
		total += counters[index].Load()
	}
	return total
}

func mediaMetricTypeIndex(mediaType MediaType) (int, bool) {
	switch mediaType {
	case MediaTypeImage:
		return 0, true
	case MediaTypeVideo:
		return 1, true
	default:
		return 0, false
	}
}

func mediaMetricStageIndex(stage MediaTaskStage) (int, bool) {
	switch stage {
	case MediaTaskStageQueued:
		return 0, true
	case MediaTaskStageScheduling:
		return 1, true
	case MediaTaskStageSubmitting:
		return 2, true
	case MediaTaskStageGenerating:
		return 3, true
	case MediaTaskStagePolling:
		return 4, true
	case MediaTaskStageStoring:
		return 5, true
	case MediaTaskStageSettling:
		return 6, true
	case MediaTaskStageCompleted:
		return 7, true
	case MediaTaskStageFailed:
		return 8, true
	default:
		return 0, false
	}
}

var _ MediaTaskMetrics = (*AtomicMediaTaskMetrics)(nil)
