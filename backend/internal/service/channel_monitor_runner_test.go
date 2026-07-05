package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorRunnerDefaultDisabledSkipsChecks(t *testing.T) {
	runner := newChannelMonitorRunner(&channelMonitorRunnerSvcStub{}, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: false},
	})

	require.False(t, runner.canRun(context.Background()))
}

func TestChannelMonitorRunnerStartDefaultDisabledSchedulesButSkipsChecks(t *testing.T) {
	svc := &channelMonitorRunnerSvcStub{
		monitors: []*ChannelMonitor{
			{
				ID:              10,
				Name:            "disabled-startup",
				Enabled:         true,
				IntervalSeconds: 60,
				JitterSeconds:   5,
			},
		},
	}
	runner := newChannelMonitorRunner(svc, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: false},
	})
	defer runner.Stop()

	runner.Start()
	time.Sleep(20 * time.Millisecond)

	require.Equal(t, 1, svc.listCalls)
	require.Equal(t, 0, svc.runCalls)
	require.Len(t, runner.tasks, 1)
	require.Equal(t, 5*time.Second, runner.tasks[10].jitter)
}

func TestScheduledMonitorNextDelayNoJitter(t *testing.T) {
	task := &scheduledMonitor{
		interval: 60 * time.Second,
		jitter:   0,
	}

	require.Equal(t, 60*time.Second, task.nextDelay())
}

func TestScheduledMonitorNextDelayWithJitterStaysInRange(t *testing.T) {
	task := &scheduledMonitor{
		interval: 60 * time.Second,
		jitter:   10 * time.Second,
	}

	for i := 0; i < 100; i++ {
		delay := task.nextDelay()
		require.GreaterOrEqual(t, delay, 50*time.Second)
		require.LessOrEqual(t, delay, 70*time.Second)
	}
}

func TestScheduledMonitorNextDelayClampsToMinimum(t *testing.T) {
	task := &scheduledMonitor{
		interval: 15 * time.Second,
		jitter:   20 * time.Second,
	}

	for i := 0; i < 100; i++ {
		delay := task.nextDelay()
		require.GreaterOrEqual(t, delay, 15*time.Second)
	}
}

func TestChannelMonitorRunnerEnabledCanRun(t *testing.T) {
	runner := newChannelMonitorRunner(&channelMonitorRunnerSvcStub{}, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: true},
	})

	require.True(t, runner.canRun(context.Background()))
}

func TestChannelMonitorRunnerStartsWhenSettingEnabledAfterDefaultDisabledStartup(t *testing.T) {
	svc := &channelMonitorRunnerSvcStub{
		monitors: []*ChannelMonitor{
			{
				ID:              42,
				Name:            "post-enable",
				Enabled:         true,
				IntervalSeconds: 1,
			},
		},
	}
	runtime := &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: false},
	}
	runner := newChannelMonitorRunner(svc, runtime)
	defer runner.Stop()

	runner.Start()

	require.Eventually(t, func() bool {
		return svc.ListCalls() == 1 && len(runner.tasks) == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 0, svc.RunCalls())

	runtime.SetEnabled(true)

	require.Eventually(t, func() bool {
		return svc.RunCalls() > 0
	}, 1500*time.Millisecond, 20*time.Millisecond)
	require.Equal(t, 1, svc.ListCalls())
}

type channelMonitorRuntimeStub struct {
	mu       sync.RWMutex
	settings ChannelMonitorRuntimeSettings
}

func (s *channelMonitorRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *channelMonitorRuntimeStub) SetEnabled(enabled bool) {
	s.mu.Lock()
	s.settings.Enabled = enabled
	s.mu.Unlock()
}

type channelMonitorRunnerSvcStub struct {
	mu        sync.Mutex
	listCalls int
	runCalls  int
	monitors  []*ChannelMonitor
}

func (s *channelMonitorRunnerSvcStub) ListEnabledMonitors(context.Context) ([]*ChannelMonitor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	return s.monitors, nil
}

func (s *channelMonitorRunnerSvcStub) RunCheck(context.Context, int64) ([]*CheckResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runCalls++
	return nil, nil
}

func (s *channelMonitorRunnerSvcStub) ListCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
}

func (s *channelMonitorRunnerSvcStub) RunCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runCalls
}

type channelMonitorRunnerRepoStub struct {
	listEnabledCalls int
	monitors         []*ChannelMonitor
}

func (s *channelMonitorRunnerRepoStub) Create(context.Context, *ChannelMonitor) error {
	panic("unexpected Create call")
}

func (s *channelMonitorRunnerRepoStub) GetByID(context.Context, int64) (*ChannelMonitor, error) {
	return nil, ErrChannelMonitorNotFound
}

func (s *channelMonitorRunnerRepoStub) Update(context.Context, *ChannelMonitor) error {
	panic("unexpected Update call")
}

func (s *channelMonitorRunnerRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *channelMonitorRunnerRepoStub) List(context.Context, ChannelMonitorListParams) ([]*ChannelMonitor, int64, error) {
	panic("unexpected List call")
}

func (s *channelMonitorRunnerRepoStub) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	s.listEnabledCalls++
	return s.monitors, nil
}

func (s *channelMonitorRunnerRepoStub) MarkChecked(context.Context, int64, time.Time) error {
	panic("unexpected MarkChecked call")
}

func (s *channelMonitorRunnerRepoStub) InsertHistoryBatch(context.Context, []*ChannelMonitorHistoryRow) error {
	panic("unexpected InsertHistoryBatch call")
}

func (s *channelMonitorRunnerRepoStub) DeleteHistoryBefore(context.Context, time.Time) (int64, error) {
	panic("unexpected DeleteHistoryBefore call")
}

func (s *channelMonitorRunnerRepoStub) ListHistory(context.Context, int64, string, int) ([]*ChannelMonitorHistoryEntry, error) {
	panic("unexpected ListHistory call")
}

func (s *channelMonitorRunnerRepoStub) ListLatestPerModel(context.Context, int64) ([]*ChannelMonitorLatest, error) {
	panic("unexpected ListLatestPerModel call")
}

func (s *channelMonitorRunnerRepoStub) ComputeAvailability(context.Context, int64, int) ([]*ChannelMonitorAvailability, error) {
	panic("unexpected ComputeAvailability call")
}

func (s *channelMonitorRunnerRepoStub) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*ChannelMonitorLatest, error) {
	panic("unexpected ListLatestForMonitorIDs call")
}

func (s *channelMonitorRunnerRepoStub) ComputeAvailabilityForMonitors(context.Context, []int64, int) (map[int64][]*ChannelMonitorAvailability, error) {
	panic("unexpected ComputeAvailabilityForMonitors call")
}

func (s *channelMonitorRunnerRepoStub) ListRecentHistoryForMonitors(context.Context, []int64, map[int64]string, int) (map[int64][]*ChannelMonitorHistoryEntry, error) {
	panic("unexpected ListRecentHistoryForMonitors call")
}

func (s *channelMonitorRunnerRepoStub) UpsertDailyRollupsFor(context.Context, time.Time) (int64, error) {
	panic("unexpected UpsertDailyRollupsFor call")
}

func (s *channelMonitorRunnerRepoStub) DeleteRollupsBefore(context.Context, time.Time) (int64, error) {
	panic("unexpected DeleteRollupsBefore call")
}

func (s *channelMonitorRunnerRepoStub) LoadAggregationWatermark(context.Context) (*time.Time, error) {
	panic("unexpected LoadAggregationWatermark call")
}

func (s *channelMonitorRunnerRepoStub) UpdateAggregationWatermark(context.Context, time.Time) error {
	panic("unexpected UpdateAggregationWatermark call")
}
