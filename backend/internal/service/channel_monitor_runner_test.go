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

type channelMonitorRunnerSvcStub struct{}

func (s *channelMonitorRunnerSvcStub) ListEnabledMonitors(context.Context) ([]*ChannelMonitor, error) {
	return nil, nil
}

func (s *channelMonitorRunnerSvcStub) RunCheck(context.Context, int64) ([]*CheckResult, error) {
	return nil, nil
}
