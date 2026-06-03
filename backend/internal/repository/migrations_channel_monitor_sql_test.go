package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorMigrationsDefineTablesAndDefaults(t *testing.T) {
	cases := map[string][]string{
		"118_channel_monitors.sql": {
			"CREATE TABLE IF NOT EXISTS channel_monitors",
			"CREATE TABLE IF NOT EXISTS channel_monitor_histories",
			"channel_monitor_enabled",
			"channel_monitor_default_interval_seconds",
			"false",
			"60",
			"api_mode",
		},
		"119_channel_monitor_aggregation.sql": {
			"CREATE TABLE IF NOT EXISTS channel_monitor_daily_rollups",
			"CREATE TABLE IF NOT EXISTS channel_monitor_aggregation_watermark",
		},
		"120_channel_monitor_request_templates.sql": {
			"CREATE TABLE IF NOT EXISTS channel_monitor_request_templates",
			"body_override_mode",
			"extra_headers",
		},
		"121_seed_channel_monitor_templates.sql": {
			"channel_monitor_request_templates",
			"chat_completions",
			"responses",
			"ON CONFLICT",
		},
	}

	for filename, expectedFragments := range cases {
		t.Run(filename, func(t *testing.T) {
			content, err := migrations.FS.ReadFile(filename)
			require.NoError(t, err)
			sql := string(content)
			for _, fragment := range expectedFragments {
				require.Contains(t, sql, fragment)
			}
			require.NotContains(t, strings.ToLower(sql), "drop table")
		})
	}
}
