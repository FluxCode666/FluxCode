package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingService_CodexImageGenerationBridgeSetting(t *testing.T) {
	repo := &openAIImagesSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{
		Gateway: config.GatewayConfig{CodexImageGenerationBridgeEnabled: true},
	})

	fallback := svc.parseSettings(map[string]string{})
	require.True(t, fallback.CodexImageGenerationBridgeEnabled)

	disabled := svc.parseSettings(map[string]string{
		SettingKeyCodexImageGenerationBridgeEnabled: "false",
	})
	require.False(t, disabled.CodexImageGenerationBridgeEnabled)

	enabled := svc.parseSettings(map[string]string{
		SettingKeyCodexImageGenerationBridgeEnabled: "true",
	})
	require.True(t, enabled.CodexImageGenerationBridgeEnabled)

	invalidFallsBackToConfig := svc.parseSettings(map[string]string{
		SettingKeyCodexImageGenerationBridgeEnabled: "not-a-bool",
	})
	require.True(t, invalidFallsBackToConfig.CodexImageGenerationBridgeEnabled)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		CodexImageGenerationBridgeEnabled: true,
		MediaSyncTimeoutBillingPolicy:     MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:             MediaVideoStorageModeHybrid,
	})
	require.NoError(t, err)
	require.Equal(t, "true", repo.values[SettingKeyCodexImageGenerationBridgeEnabled])
}

func TestSettingService_OpenAIUsageDebugLogSettingRefreshesCache(t *testing.T) {
	codexCLICfgCache.Store((*cachedCodexCLIConfig)(nil))
	t.Cleanup(func() { codexCLICfgCache.Store((*cachedCodexCLIConfig)(nil)) })

	repo := &openAIImagesSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	defaults := svc.parseSettings(map[string]string{})
	require.False(t, defaults.OpenAIUsageDebugLogEnabled)
	require.False(t, resolveOpenAIUsageDebugLogEnabled())

	enabled := svc.parseSettings(map[string]string{
		SettingKeyOpenAIUsageDebugLogEnabled: "true",
	})
	require.True(t, enabled.OpenAIUsageDebugLogEnabled)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		OpenAIUsageDebugLogEnabled:    true,
		CodexPassthroughUAVersion:     true,
		MediaSyncTimeoutBillingPolicy: MediaTimeoutBillingPolicyPenalty,
		MediaVideoStorageMode:         MediaVideoStorageModeHybrid,
	})
	require.NoError(t, err)
	require.Equal(t, "true", repo.values[SettingKeyOpenAIUsageDebugLogEnabled])
	require.True(t, resolveOpenAIUsageDebugLogEnabled())
}
