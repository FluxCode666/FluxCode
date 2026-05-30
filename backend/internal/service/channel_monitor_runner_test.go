package service

import (
	"context"
	"testing"

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
