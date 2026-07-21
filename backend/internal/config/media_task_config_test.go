package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestMediaTaskConfigDefaults(t *testing.T) {
	cfg := loadConfigWithYAML(t, "")

	require.True(t, cfg.MediaTasks.Enabled)
	require.Equal(t, 4, cfg.MediaTasks.WorkerCount)
	require.Equal(t, 7200, cfg.MediaTasks.TaskTimeoutSeconds)
	require.Equal(t, 120, cfg.MediaTasks.LeaseTTLSeconds)
	require.Equal(t, 30, cfg.MediaTasks.LeaseRenewIntervalSeconds)
	require.Equal(t, 2, cfg.MediaTasks.PollIntervalSeconds)
	require.Equal(t, 15, cfg.MediaTasks.RecoveryIntervalSeconds)
	require.Equal(t, 100, cfg.MediaTasks.RecoveryBatchSize)
	require.Equal(t, 1000, cfg.MediaTasks.StreamBlockMilliseconds)
	require.Equal(t, 90, cfg.MediaTasks.ContentProxyTimeoutSeconds)
	require.Equal(t, int64(2147483648), cfg.MediaTasks.MaxContentBytes)
	require.Equal(t, "./data/generated", cfg.MediaTasks.LocalStoragePath)
}

func TestMediaTaskConfigLoadsDeploymentOverrides(t *testing.T) {
	cfg := loadConfigWithYAML(t, `
media_tasks:
  enabled: false
  worker_count: 8
  task_timeout_seconds: 3600
  lease_ttl_seconds: 90
  lease_renew_interval_seconds: 20
  poll_interval_seconds: 3
  recovery_interval_seconds: 45
  recovery_batch_size: 50
  stream_block_milliseconds: 750
  content_proxy_timeout_seconds: 60
  max_content_bytes: 1048576
  local_storage_path: /tmp/fluxcode-generated
`)

	require.False(t, cfg.MediaTasks.Enabled)
	require.Equal(t, 8, cfg.MediaTasks.WorkerCount)
	require.Equal(t, 3600, cfg.MediaTasks.TaskTimeoutSeconds)
	require.Equal(t, 90, cfg.MediaTasks.LeaseTTLSeconds)
	require.Equal(t, 20, cfg.MediaTasks.LeaseRenewIntervalSeconds)
	require.Equal(t, 3, cfg.MediaTasks.PollIntervalSeconds)
	require.Equal(t, 45, cfg.MediaTasks.RecoveryIntervalSeconds)
	require.Equal(t, 50, cfg.MediaTasks.RecoveryBatchSize)
	require.Equal(t, 750, cfg.MediaTasks.StreamBlockMilliseconds)
	require.Equal(t, 60, cfg.MediaTasks.ContentProxyTimeoutSeconds)
	require.Equal(t, int64(1048576), cfg.MediaTasks.MaxContentBytes)
	require.Equal(t, "/tmp/fluxcode-generated", cfg.MediaTasks.LocalStoragePath)
}

func TestMediaTaskConfigRejectsNonPositiveDeploymentValues(t *testing.T) {
	tests := []struct {
		name  string
		field string
	}{
		{name: "worker count", field: "worker_count"},
		{name: "task timeout", field: "task_timeout_seconds"},
		{name: "lease TTL", field: "lease_ttl_seconds"},
		{name: "lease renew interval", field: "lease_renew_interval_seconds"},
		{name: "poll interval", field: "poll_interval_seconds"},
		{name: "recovery interval", field: "recovery_interval_seconds"},
		{name: "recovery batch size", field: "recovery_batch_size"},
		{name: "stream block", field: "stream_block_milliseconds"},
		{name: "content proxy timeout", field: "content_proxy_timeout_seconds"},
		{name: "content size limit", field: "max_content_bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfigWithYAMLError(t, "media_tasks:\n  "+tt.field+": 0\n")
			require.ErrorContains(t, err, "media_tasks."+tt.field)
		})
	}
}

func TestMediaTaskConfigRejectsRenewIntervalNotBelowLease(t *testing.T) {
	_, err := loadConfigWithYAMLError(t, `
media_tasks:
  lease_ttl_seconds: 30
  lease_renew_interval_seconds: 30
`)
	require.ErrorContains(t, err, "media_tasks.lease_renew_interval_seconds")
}

func TestMediaTaskConfigRejectsEmptyLocalStoragePath(t *testing.T) {
	_, err := loadConfigWithYAMLError(t, "media_tasks:\n  local_storage_path: '   '\n")
	require.ErrorContains(t, err, "media_tasks.local_storage_path")
}

func loadConfigWithYAML(t *testing.T, yaml string) *Config {
	t.Helper()
	cfg, err := loadConfigWithYAMLError(t, yaml)
	require.NoError(t, err)
	return cfg
}

func loadConfigWithYAMLError(t *testing.T, yaml string) (*Config, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	t.Setenv("DATA_DIR", t.TempDir())
	path := filepath.Join(os.Getenv("DATA_DIR"), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	return Load()
}
