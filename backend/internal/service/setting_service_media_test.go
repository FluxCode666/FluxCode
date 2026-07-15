package service

import (
	"context"
	"math"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type mediaSettingRepoStub struct {
	values  map[string]string
	updates map[string]string
}

func newMediaSettingRepoStub(values map[string]string) *mediaSettingRepoStub {
	if values == nil {
		values = map[string]string{}
	}
	return &mediaSettingRepoStub{values: values}
}

func (s *mediaSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *mediaSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *mediaSettingRepoStub) Set(ctx context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *mediaSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (s *mediaSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	for key, value := range settings {
		s.updates[key] = value
		s.values[key] = value
	}
	return nil
}

func (s *mediaSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	values := make(map[string]string, len(s.values))
	for key, value := range s.values {
		values[key] = value
	}
	return values, nil
}

func (s *mediaSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestSettingServiceMediaDefaults(t *testing.T) {
	svc := NewSettingService(newMediaSettingRepoStub(nil), &config.Config{})

	settings := svc.parseSettings(map[string]string{})

	require.Equal(t, 240, settings.MediaSyncWaitTimeoutSeconds)
	require.False(t, settings.MediaSyncTimeoutFallbackAsyncEnabled)
	require.Equal(t, MediaTimeoutBillingPolicyPenalty, settings.MediaSyncTimeoutBillingPolicy)
	require.Equal(t, 0.8, settings.MediaSyncTimeoutPenaltyRatio)
	require.Equal(t, MediaVideoStorageModeHybrid, settings.MediaVideoStorageMode)
	require.True(t, settings.MediaVideoProxyFallbackEnabled)
}

func TestSettingServiceMediaStoredValuesRoundTrip(t *testing.T) {
	svc := NewSettingService(newMediaSettingRepoStub(nil), &config.Config{})

	settings := svc.parseSettings(map[string]string{
		SettingKeyMediaSyncWaitTimeoutSeconds:          "0",
		SettingKeyMediaSyncTimeoutFallbackAsyncEnabled: "true",
		SettingKeyMediaSyncTimeoutBillingPolicy:        MediaTimeoutBillingPolicyRefund,
		SettingKeyMediaSyncTimeoutPenaltyRatio:         "1",
		SettingKeyMediaVideoStorageMode:                MediaVideoStorageModeHybrid,
		SettingKeyMediaVideoProxyFallbackEnabled:       "false",
	})

	require.Equal(t, 0, settings.MediaSyncWaitTimeoutSeconds)
	require.True(t, settings.MediaSyncTimeoutFallbackAsyncEnabled)
	require.Equal(t, MediaTimeoutBillingPolicyRefund, settings.MediaSyncTimeoutBillingPolicy)
	require.Equal(t, 1.0, settings.MediaSyncTimeoutPenaltyRatio)
	require.Equal(t, MediaVideoStorageModeHybrid, settings.MediaVideoStorageMode)
	require.False(t, settings.MediaVideoProxyFallbackEnabled)
}

func TestSettingServiceMediaInvalidStoredValuesUseSafeDefaults(t *testing.T) {
	svc := NewSettingService(newMediaSettingRepoStub(nil), &config.Config{})

	settings := svc.parseSettings(map[string]string{
		SettingKeyMediaSyncWaitTimeoutSeconds:          "-1",
		SettingKeyMediaSyncTimeoutFallbackAsyncEnabled: "sometimes",
		SettingKeyMediaSyncTimeoutBillingPolicy:        "charge",
		SettingKeyMediaSyncTimeoutPenaltyRatio:         "NaN",
		SettingKeyMediaVideoStorageMode:                "local",
		SettingKeyMediaVideoProxyFallbackEnabled:       "sometimes",
	})

	require.Equal(t, 240, settings.MediaSyncWaitTimeoutSeconds)
	require.False(t, settings.MediaSyncTimeoutFallbackAsyncEnabled)
	require.Equal(t, MediaTimeoutBillingPolicyPenalty, settings.MediaSyncTimeoutBillingPolicy)
	require.Equal(t, 0.8, settings.MediaSyncTimeoutPenaltyRatio)
	require.Equal(t, MediaVideoStorageModeHybrid, settings.MediaVideoStorageMode)
	require.True(t, settings.MediaVideoProxyFallbackEnabled)
}

func TestSettingServiceMediaUpdatePersistsExplicitZeroAndFalse(t *testing.T) {
	repo := newMediaSettingRepoStub(nil)
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		MediaSyncWaitTimeoutSeconds:          0,
		MediaSyncTimeoutFallbackAsyncEnabled: true,
		MediaSyncTimeoutBillingPolicy:        MediaTimeoutBillingPolicyRefund,
		MediaSyncTimeoutPenaltyRatio:         0,
		MediaVideoStorageMode:                MediaVideoStorageModeHybrid,
		MediaVideoProxyFallbackEnabled:       false,
	})

	require.NoError(t, err)
	require.Equal(t, "0", repo.updates[SettingKeyMediaSyncWaitTimeoutSeconds])
	require.Equal(t, "true", repo.updates[SettingKeyMediaSyncTimeoutFallbackAsyncEnabled])
	require.Equal(t, MediaTimeoutBillingPolicyRefund, repo.updates[SettingKeyMediaSyncTimeoutBillingPolicy])
	require.Equal(t, "0", repo.updates[SettingKeyMediaSyncTimeoutPenaltyRatio])
	require.Equal(t, MediaVideoStorageModeHybrid, repo.updates[SettingKeyMediaVideoStorageMode])
	require.Equal(t, "false", repo.updates[SettingKeyMediaVideoProxyFallbackEnabled])
}

func TestSettingServiceRejectsInvalidMediaSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SystemSettings)
	}{
		{name: "negative timeout", mutate: func(s *SystemSettings) { s.MediaSyncWaitTimeoutSeconds = -1 }},
		{name: "empty policy", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutBillingPolicy = "" }},
		{name: "blank policy", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutBillingPolicy = " \t\n " }},
		{name: "unknown policy", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutBillingPolicy = "charge" }},
		{name: "negative ratio", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutPenaltyRatio = -0.01 }},
		{name: "ratio above one", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutPenaltyRatio = 1.01 }},
		{name: "NaN ratio", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutPenaltyRatio = math.NaN() }},
		{name: "positive infinity ratio", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutPenaltyRatio = math.Inf(1) }},
		{name: "negative infinity ratio", mutate: func(s *SystemSettings) { s.MediaSyncTimeoutPenaltyRatio = math.Inf(-1) }},
		{name: "empty storage", mutate: func(s *SystemSettings) { s.MediaVideoStorageMode = "" }},
		{name: "blank storage", mutate: func(s *SystemSettings) { s.MediaVideoStorageMode = " \t\n " }},
		{name: "unsupported storage", mutate: func(s *SystemSettings) { s.MediaVideoStorageMode = "object" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMediaSettingRepoStub(nil)
			svc := NewSettingService(repo, &config.Config{})
			settings := validMediaSystemSettings()
			tt.mutate(settings)

			err := svc.UpdateSettings(context.Background(), settings)

			require.Error(t, err)
			require.Nil(t, repo.updates)
		})
	}
}

func TestSettingServiceRejectsInvalidMediaPenaltyRatio(t *testing.T) {
	repo := newMediaSettingRepoStub(nil)
	svc := NewSettingService(repo, &config.Config{})
	settings := validMediaSystemSettings()
	settings.MediaSyncTimeoutPenaltyRatio = 1.2

	err := svc.UpdateSettings(context.Background(), settings)

	require.Error(t, err)
	require.Nil(t, repo.updates)
}

func TestSettingServiceAcceptsMediaPenaltyRatioBoundaries(t *testing.T) {
	for _, ratio := range []float64{0, 1} {
		t.Run(strconv.FormatFloat(ratio, 'f', -1, 64), func(t *testing.T) {
			repo := newMediaSettingRepoStub(nil)
			svc := NewSettingService(repo, &config.Config{})
			settings := validMediaSystemSettings()
			settings.MediaSyncTimeoutPenaltyRatio = ratio

			err := svc.UpdateSettings(context.Background(), settings)

			require.NoError(t, err)
			require.Equal(t, ratio, mustParseFloat(t, repo.updates[SettingKeyMediaSyncTimeoutPenaltyRatio]))
		})
	}
}

func TestSettingServiceMediaDefaultsInitializeOnlyMissingKeys(t *testing.T) {
	repo := newMediaSettingRepoStub(map[string]string{
		SettingKeyRegistrationEnabled:            "false",
		SettingKeyMediaSyncTimeoutBillingPolicy:  MediaTimeoutBillingPolicyRefund,
		SettingKeyMediaSyncTimeoutPenaltyRatio:   "0.25",
		SettingKeyMediaVideoProxyFallbackEnabled: "false",
	})
	svc := NewSettingService(repo, &config.Config{})

	err := svc.InitializeDefaultSettings(context.Background())

	require.NoError(t, err)
	require.Equal(t, "false", repo.values[SettingKeyRegistrationEnabled])
	require.Equal(t, "240", repo.values[SettingKeyMediaSyncWaitTimeoutSeconds])
	require.Equal(t, "false", repo.values[SettingKeyMediaSyncTimeoutFallbackAsyncEnabled])
	require.Equal(t, MediaTimeoutBillingPolicyRefund, repo.values[SettingKeyMediaSyncTimeoutBillingPolicy])
	require.Equal(t, "0.25", repo.values[SettingKeyMediaSyncTimeoutPenaltyRatio])
	require.Equal(t, MediaVideoStorageModeHybrid, repo.values[SettingKeyMediaVideoStorageMode])
	require.Equal(t, "false", repo.values[SettingKeyMediaVideoProxyFallbackEnabled])
}

func validMediaSystemSettings() *SystemSettings {
	return &SystemSettings{
		MediaSyncWaitTimeoutSeconds:    240,
		MediaSyncTimeoutBillingPolicy:  MediaTimeoutBillingPolicyPenalty,
		MediaSyncTimeoutPenaltyRatio:   0.8,
		MediaVideoStorageMode:          MediaVideoStorageModeHybrid,
		MediaVideoProxyFallbackEnabled: true,
	}
}

func mustParseFloat(t *testing.T, raw string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(raw, 64)
	require.NoError(t, err)
	return value
}
