package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingServiceChannelMonitorDefaultsDisabled(t *testing.T) {
	repo := &channelMonitorSettingsRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.ChannelMonitorEnabled)
}

func TestSettingServiceUpdateChannelMonitorFields(t *testing.T) {
	repo := &channelMonitorSettingsRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		ChannelMonitorEnabled:                true,
		ChannelMonitorDefaultIntervalSeconds: 120,
	})

	require.NoError(t, err)
	require.Equal(t, "true", repo.updates[SettingKeyChannelMonitorEnabled])
	require.Equal(t, "120", repo.updates[SettingKeyChannelMonitorDefaultIntervalSeconds])
}

type channelMonitorSettingsRepoStub struct {
	values  map[string]string
	updates map[string]string
}

func (s *channelMonitorSettingsRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *channelMonitorSettingsRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *channelMonitorSettingsRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *channelMonitorSettingsRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *channelMonitorSettingsRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for key, value := range settings {
		s.updates[key] = value
	}
	return nil
}

func (s *channelMonitorSettingsRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}

func (s *channelMonitorSettingsRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}
