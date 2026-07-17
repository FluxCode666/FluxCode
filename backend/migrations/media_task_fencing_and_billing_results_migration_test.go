package migrations

import (
	"testing"

	entmigrate "github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
)

func TestMediaTaskFencingAndBillingResultsMigrationContract(t *testing.T) {
	body, err := FS.ReadFile("130_media_task_fencing_and_billing_results.sql")
	require.NoError(t, err)
	sql := string(body)
	require.Contains(t, sql, "claim_token VARCHAR(64) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "additional_charged_amount NUMERIC(20,8) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "DROP INDEX IF EXISTS idx_media_artifacts_task")

	claimToken := findEntColumn(t, entmigrate.MediaTasksColumns, "claim_token")
	require.Equal(t, int64(64), claimToken.Size)
	additional := findEntColumn(t, entmigrate.MediaTasksColumns, "additional_charged_amount")
	require.Equal(t, "numeric(20,8)", additional.SchemaType[dialect.Postgres])
}
