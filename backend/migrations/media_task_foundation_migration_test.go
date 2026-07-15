package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaTaskFoundationMigrationContainsRequiredObjects(t *testing.T) {
	body, err := FS.ReadFile("128_media_task_foundation.sql")
	require.NoError(t, err)
	sql := string(body)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS media_tasks",
		"CREATE TABLE IF NOT EXISTS media_artifacts",
		"CREATE TABLE IF NOT EXISTS media_model_definitions",
		"allow_video_generation",
		"media_cross_platform_enabled",
		"public_id",
		"idempotency_key",
		"settlement_plan",
		"candidate_snapshot",
		"lease_until",
		"version",
		"UNIQUE (task_id, direction, position)",
		"idx_media_tasks_user_created",
		"idx_media_tasks_status_lease",
		"idx_media_tasks_account",
		"idx_media_tasks_idempotency",
		"idx_media_artifacts_task",
		"idx_media_model_definitions_enabled",
	} {
		require.Contains(t, sql, fragment)
	}
}
