package repository

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPoolAlertConfigCacheBackwardCompatibility_DefaultsMissingProxyProbeSwitch(t *testing.T) {
	cfg := &service.AccountPoolAlertConfig{}
	raw := map[string]json.RawMessage{
		"allow_schedule_when_proxy_disabled": json.RawMessage("false"),
	}

	applyPoolAlertConfigCacheBackwardCompatibility(cfg, raw)

	require.True(t, cfg.ProxyActiveProbeEnabled)
	require.Equal(t, service.DisabledProxyScheduleModeExcludeAccount, cfg.DisabledProxyScheduleMode)
}

func TestPoolAlertConfigCacheBackwardCompatibility_RespectsExplicitScheduleMode(t *testing.T) {
	cfg := &service.AccountPoolAlertConfig{}
	raw := map[string]json.RawMessage{
		"disabled_proxy_schedule_mode": json.RawMessage(`"direct_without_proxy"`),
	}

	applyPoolAlertConfigCacheBackwardCompatibility(cfg, raw)

	require.Equal(t, "", cfg.DisabledProxyScheduleMode)
}
