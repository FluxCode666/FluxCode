package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorRunnerDefaultDisabledSkipsChecks(t *testing.T) {
	runner := newChannelMonitorRunner(&channelMonitorRunnerSvcStub{}, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: false},
	})

	require.False(t, runner.canRun(context.Background()))
}

func TestChannelMonitorRunnerStartDefaultDisabledSkipsScheduling(t *testing.T) {
	svc := &channelMonitorRunnerSvcStub{
		monitors: []*ChannelMonitor{
			{
				ID:              10,
				Name:            "disabled-startup",
				Enabled:         true,
				IntervalSeconds: 60,
			},
		},
	}
	runner := newChannelMonitorRunner(svc, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: false},
	})
	defer runner.Stop()

	runner.Start()

	require.Equal(t, 0, svc.listCalls)
	require.Empty(t, runner.tasks)
}

func TestChannelMonitorRunnerEnabledCanRun(t *testing.T) {
	runner := newChannelMonitorRunner(&channelMonitorRunnerSvcStub{}, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: true},
	})

	require.True(t, runner.canRun(context.Background()))
}

func TestChannelMonitorRunnerStartsWhenSettingEnabledAfterDefaultDisabledStartup(t *testing.T) {
	settingRepo := &channelMonitorSettingsRepoStub{
		values: map[string]string{
			SettingKeyChannelMonitorEnabled:                "false",
			SettingKeyChannelMonitorDefaultIntervalSeconds: "60",
		},
	}
	settingService := NewSettingService(settingRepo, &config.Config{})
	monitorRepo := &channelMonitorRunnerRepoStub{
		monitors: []*ChannelMonitor{
			{
				ID:              42,
				Name:            "post-enable",
				Enabled:         true,
				IntervalSeconds: 60,
			},
		},
	}
	monitorService := &ChannelMonitorService{repo: monitorRepo}

	runner := ProvideChannelMonitorRunner(monitorService, settingService)
	defer runner.Stop()

	require.Equal(t, 0, monitorRepo.listEnabledCalls)

	err := settingService.UpdateSettings(context.Background(), &SystemSettings{
		ChannelMonitorEnabled:                true,
		ChannelMonitorDefaultIntervalSeconds: 60,
	})

	require.NoError(t, err)
	require.Equal(t, 1, monitorRepo.listEnabledCalls)
}

type channelMonitorRuntimeStub struct {
	settings ChannelMonitorRuntimeSettings
}

func (s *channelMonitorRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntimeSettings {
	return s.settings
}

type channelMonitorRunnerSvcStub struct {
	listCalls int
	monitors  []*ChannelMonitor
}

func (s *channelMonitorRunnerSvcStub) ListEnabledMonitors(context.Context) ([]*ChannelMonitor, error) {
	s.listCalls++
	return s.monitors, nil
}

func (s *channelMonitorRunnerSvcStub) RunCheck(context.Context, int64) ([]*CheckResult, error) {
	return nil, nil
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
