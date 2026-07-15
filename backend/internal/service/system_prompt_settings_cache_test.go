package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type systemPromptSettingsRepoStub struct {
	values                       map[string]string
	updates                      map[string]string
	getMultipleCalls             int
	systemPromptGetMultipleCalls int
}

func (s *systemPromptSettingsRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *systemPromptSettingsRepoStub) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *systemPromptSettingsRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *systemPromptSettingsRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.getMultipleCalls++
	if len(keys) == len(systemPromptSettingKeys) && keys[0] == SettingKeySystemPromptAnthropic {
		s.systemPromptGetMultipleCalls++
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = s.values[key]
	}
	return out, nil
}

func (s *systemPromptSettingsRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	s.updates = make(map[string]string, len(settings))
	if s.values == nil {
		s.values = make(map[string]string, len(settings))
	}
	for key, value := range settings {
		s.updates[key] = value
		s.values[key] = value
	}
	return nil
}

func (s *systemPromptSettingsRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *systemPromptSettingsRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func resetSystemPromptSettingsTestCache(t *testing.T) {
	t.Helper()

	systemPromptSettingsCache.Store((*cachedSystemPromptSettings)(nil))
	t.Cleanup(func() {
		systemPromptSettingsCache.Store((*cachedSystemPromptSettings)(nil))
	})
}

func TestNormalizeSystemPromptConfigRejectsInvalidMode(t *testing.T) {
	_, _, err := NormalizeSystemPromptConfig("prompt", "replace")

	require.Error(t, err)
	require.Equal(t, "INVALID_SYSTEM_PROMPT_MODE", infraerrors.Reason(err))
}

func TestNormalizeSystemPromptConfigClearsPromptWhenInherit(t *testing.T) {
	prompt, mode, err := NormalizeSystemPromptConfig("unused prompt", SystemPromptModeInherit)

	require.NoError(t, err)
	require.Empty(t, prompt)
	require.Equal(t, SystemPromptModeInherit, mode)
}

func TestSettingService_GetSystemPromptSettings_CachesByPlatform(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{values: map[string]string{
		SettingKeySystemPromptAnthropic:     "anthropic prompt",
		SettingKeySystemPromptModeAnthropic: SystemPromptModeAppend,
		SettingKeySystemPromptOpenAI:        "openai prompt",
		SettingKeySystemPromptModeOpenAI:    SystemPromptModeOverride,
	}}
	svc := NewSettingService(repo, &config.Config{})

	first := svc.GetSystemPromptSettings(context.Background())
	second := svc.GetSystemPromptSettings(context.Background())

	require.Equal(t, 1, repo.systemPromptGetMultipleCalls)
	require.Equal(t, "anthropic prompt", first.Prompts[PlatformAnthropic].Prompt)
	require.Equal(t, SystemPromptModeAppend, first.Prompts[PlatformAnthropic].Mode)
	require.Equal(t, SystemPromptSourceSystem, first.Prompts[PlatformAnthropic].Source)
	require.Equal(t, "openai prompt", second.Prompts[PlatformOpenAI].Prompt)
	require.Equal(t, SystemPromptModeOverride, second.Prompts[PlatformOpenAI].Mode)
	require.False(t, first.UserScope.Enabled)
	require.Equal(t, SystemPromptUserScopeAll, first.UserScope.Mode)
}

func TestSettingService_UpdateSettingsRefreshesSystemPromptCache(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{values: map[string]string{
		SettingKeySystemPromptAnthropic:     "old prompt",
		SettingKeySystemPromptModeAnthropic: SystemPromptModeOverride,
	}}
	svc := NewSettingService(repo, &config.Config{})

	require.Equal(t, "old prompt", svc.GetSystemPromptSettings(context.Background()).Prompts[PlatformAnthropic].Prompt)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		SystemPromptAnthropic:         "new prompt",
		SystemPromptModeAnthropic:     SystemPromptModeAppend,
		MediaSyncTimeoutBillingPolicy: MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:         MediaVideoStorageModeHybrid,
	})
	require.NoError(t, err)

	got := svc.GetSystemPromptSettings(context.Background())
	require.Equal(t, 1, repo.systemPromptGetMultipleCalls)
	require.Equal(t, "new prompt", got.Prompts[PlatformAnthropic].Prompt)
	require.Equal(t, SystemPromptModeAppend, got.Prompts[PlatformAnthropic].Mode)
	require.Equal(t, "new prompt", repo.updates[SettingKeySystemPromptAnthropic])
	require.Equal(t, SystemPromptModeAppend, repo.updates[SettingKeySystemPromptModeAnthropic])
}

func TestSettingService_GetSystemPromptSettings_CachesUserScope(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{values: map[string]string{
		SettingKeySystemPromptUserScopeEnabled: "true",
		SettingKeySystemPromptUserScopeMode:    SystemPromptUserScopeWhitelist,
		SettingKeySystemPromptUserScopeUserIDs: "[101,202,101]",
	}}
	svc := NewSettingService(repo, &config.Config{})

	first := svc.GetSystemPromptSettings(context.Background())
	second := svc.GetSystemPromptSettings(context.Background())

	require.Equal(t, 1, repo.systemPromptGetMultipleCalls)
	require.True(t, first.UserScope.Enabled)
	require.Equal(t, SystemPromptUserScopeWhitelist, first.UserScope.Mode)
	require.Equal(t, []int64{101, 202}, first.UserScope.UserIDs)
	require.Equal(t, first.UserScope, second.UserScope)
}

func TestSettingService_UpdateSettingsRefreshesSystemPromptUserScopeCache(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{values: map[string]string{
		SettingKeySystemPromptUserScopeEnabled: "false",
		SettingKeySystemPromptUserScopeMode:    SystemPromptUserScopeAll,
		SettingKeySystemPromptUserScopeUserIDs: "[]",
	}}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		SystemPromptUserScopeEnabled:  true,
		SystemPromptUserScopeMode:     SystemPromptUserScopeBlacklist,
		SystemPromptUserScopeUserIDs:  []int64{7, 3, 7},
		MediaSyncTimeoutBillingPolicy: MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:         MediaVideoStorageModeHybrid,
	})
	require.NoError(t, err)

	got := svc.GetSystemPromptSettings(context.Background())
	require.True(t, got.UserScope.Enabled)
	require.Equal(t, SystemPromptUserScopeBlacklist, got.UserScope.Mode)
	require.Equal(t, []int64{3, 7}, got.UserScope.UserIDs)
	require.Equal(t, "true", repo.updates[SettingKeySystemPromptUserScopeEnabled])
	require.Equal(t, SystemPromptUserScopeBlacklist, repo.updates[SettingKeySystemPromptUserScopeMode])
	require.JSONEq(t, `[3,7]`, repo.updates[SettingKeySystemPromptUserScopeUserIDs])
}

func TestSettingService_UpdateSettingsClearsUserScopeIDsWhenAllUsers(t *testing.T) {
	resetSystemPromptSettingsTestCache(t)

	repo := &systemPromptSettingsRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		SystemPromptUserScopeEnabled:  true,
		SystemPromptUserScopeMode:     SystemPromptUserScopeAll,
		SystemPromptUserScopeUserIDs:  []int64{7, 3},
		MediaSyncTimeoutBillingPolicy: MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:         MediaVideoStorageModeHybrid,
	})
	require.NoError(t, err)

	got := svc.GetSystemPromptSettings(context.Background())
	require.True(t, got.UserScope.Enabled)
	require.Equal(t, SystemPromptUserScopeAll, got.UserScope.Mode)
	require.Empty(t, got.UserScope.UserIDs)
	require.JSONEq(t, `[]`, repo.updates[SettingKeySystemPromptUserScopeUserIDs])
}
