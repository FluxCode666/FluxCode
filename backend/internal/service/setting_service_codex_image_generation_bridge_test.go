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
	})
	require.NoError(t, err)
	require.Equal(t, "true", repo.values[SettingKeyCodexImageGenerationBridgeEnabled])
}
