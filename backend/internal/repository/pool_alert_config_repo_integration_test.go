//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPoolAlertConfigRepository_UpsertAndGetByPlatform_RoundTripsNormalizedFields(t *testing.T) {
	ctx := context.Background()
	repo := NewPoolAlertConfigRepository(integrationDB).(*poolAlertConfigRepository)
	platform := fmt.Sprintf("it-pool-%d", time.Now().UnixNano())

	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM account_pool_alert_configs WHERE platform = $1`, platform)
		require.NoError(t, err)
	})

	err := repo.Upsert(ctx, &service.AccountPoolAlertConfig{
		Platform:                  "  " + platform + "  ",
		PoolThresholdEnabled:      true,
		ProxyFailureEnabled:       true,
		ProxyActiveProbeEnabled:   false,
		DisabledProxyScheduleMode: service.DisabledProxyScheduleModeExcludeAccount,
		AvailableCountThreshold:   3,
		AvailableRatioThreshold:   45,
		CheckIntervalMinutes:      8,
		ProxyProbeIntervalMinutes: 6,
		ProxyFailureWindowMinutes: 12,
		ProxyFailureThreshold:     7,
		AlertEmails:               []string{" ops@example.com ", "OPS@example.com", ""},
		AlertCooldownMinutes:      9,
	})
	require.NoError(t, err)

	cfg, err := repo.GetByPlatform(ctx, " "+platform+" ")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, platform, cfg.Platform)
	require.False(t, cfg.ProxyActiveProbeEnabled)
	require.Equal(t, service.DisabledProxyScheduleModeExcludeAccount, cfg.DisabledProxyScheduleMode)
	require.Equal(t, 3, cfg.AvailableCountThreshold)
	require.Equal(t, 45, cfg.AvailableRatioThreshold)
	require.Equal(t, 8, cfg.CheckIntervalMinutes)
	require.Equal(t, 6, cfg.ProxyProbeIntervalMinutes)
	require.Equal(t, 12, cfg.ProxyFailureWindowMinutes)
	require.Equal(t, 7, cfg.ProxyFailureThreshold)
	require.Equal(t, []string{"ops@example.com"}, cfg.AlertEmails)
	require.Equal(t, 9, cfg.AlertCooldownMinutes)
}

func TestPoolAlertConfigRepository_GetByPlatform_ReturnsNilWhenMissing(t *testing.T) {
	ctx := context.Background()
	repo := NewPoolAlertConfigRepository(integrationDB).(*poolAlertConfigRepository)
	platform := fmt.Sprintf("it-pool-missing-%d", time.Now().UnixNano())

	cfg, err := repo.GetByPlatform(ctx, platform)
	require.NoError(t, err)
	require.Nil(t, cfg)
}
