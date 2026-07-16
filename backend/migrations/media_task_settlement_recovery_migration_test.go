package migrations

import (
	"testing"

	entmigrate "github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
)

func TestMediaTaskSettlementRecoveryMigrationContract(t *testing.T) {
	body, err := FS.ReadFile("129_media_task_settlement_recovery.sql")
	require.NoError(t, err)
	require.Contains(t, string(body), "ALTER TABLE media_tasks ADD COLUMN IF NOT EXISTS settlement_recovery JSONB NULL")

	column := findEntColumn(t, entmigrate.MediaTasksColumns, "settlement_recovery")
	require.Equal(t, "jsonb", column.SchemaType[dialect.Postgres])
	require.True(t, column.Nullable)
}
