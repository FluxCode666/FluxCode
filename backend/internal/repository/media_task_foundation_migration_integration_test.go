//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"

	entmigrate "github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entschema "entgo.io/ent/dialect/sql/schema"
)

func TestMediaTaskFoundationMigrationCatalogContract(t *testing.T) {
	ctx := context.Background()
	body, err := migrations.FS.ReadFile("128_media_task_foundation.sql")
	require.NoError(t, err)

	for attempt := 1; attempt <= 2; attempt++ {
		_, err := integrationDB.ExecContext(ctx, string(body))
		require.NoError(t, err, "direct migration re-application %d", attempt)
	}

	tx := testTx(t)

	t.Run("column types nullability and defaults", func(t *testing.T) {
		for _, column := range []struct {
			table           string
			name            string
			dataType        string
			maxLen          int
			nullable        bool
			defaultPresent  bool
			defaultContains string
		}{
			{table: "media_tasks", name: "public_id", dataType: "character varying", maxLen: 64},
			{table: "media_tasks", name: "progress", dataType: "integer", defaultPresent: true, defaultContains: "0"},
			{table: "media_tasks", name: "retry_count", dataType: "integer", defaultPresent: true, defaultContains: "0"},
			{table: "media_tasks", name: "upstream_task_id", dataType: "text", nullable: true},
			{table: "media_tasks", name: "error_message", dataType: "text", defaultPresent: true, defaultContains: "''::text"},
			{table: "media_tasks", name: "request_spec", dataType: "jsonb", defaultPresent: true, defaultContains: "'{}'::jsonb"},
			{table: "media_tasks", name: "candidate_snapshot", dataType: "jsonb", defaultPresent: true, defaultContains: "'[]'::jsonb"},
			{table: "media_tasks", name: "precharged_amount", dataType: "numeric", defaultPresent: true, defaultContains: "0"},
			{table: "media_tasks", name: "additional_charged_amount", dataType: "numeric", defaultPresent: true, defaultContains: "0"},
			{table: "media_tasks", name: "claim_token", dataType: "character varying", maxLen: 64, defaultPresent: true, defaultContains: "''::character varying"},
			{table: "media_tasks", name: "lease_until", dataType: "timestamp with time zone", nullable: true},
			{table: "media_artifacts", name: "position", dataType: "integer", defaultPresent: true, defaultContains: "0"},
			{table: "media_artifacts", name: "width", dataType: "integer", nullable: true},
			{table: "media_artifacts", name: "height", dataType: "integer", nullable: true},
			{table: "media_artifacts", name: "duration_seconds", dataType: "double precision", nullable: true},
			{table: "media_artifacts", name: "object_key", dataType: "text", nullable: true},
			{table: "media_artifacts", name: "public_url", dataType: "text", nullable: true},
			{table: "media_artifacts", name: "upstream_reference", dataType: "text", nullable: true},
			{table: "media_artifacts", name: "expires_at", dataType: "timestamp with time zone", nullable: true},
			{table: "media_model_definitions", name: "operations", dataType: "jsonb", defaultPresent: true, defaultContains: "'[]'::jsonb"},
			{table: "media_model_definitions", name: "constraints", dataType: "jsonb", defaultPresent: true, defaultContains: "'{}'::jsonb"},
			{table: "media_model_definitions", name: "enabled", dataType: "boolean", defaultPresent: true, defaultContains: "true"},
			{table: "groups", name: "allow_video_generation", dataType: "boolean", defaultPresent: true, defaultContains: "false"},
			{table: "groups", name: "media_cross_platform_enabled", dataType: "boolean", defaultPresent: true, defaultContains: "false"},
		} {
			requireColumn(t, tx, column.table, column.name, column.dataType, column.maxLen, column.nullable)
			requireCatalogColumnDefault(t, tx, column.table, column.name, column.defaultPresent, column.defaultContains)
		}
	})

	t.Run("ent metadata matches catalog types", func(t *testing.T) {
		for _, tt := range []struct {
			table   string
			name    string
			columns []*entschema.Column
		}{
			{table: "media_tasks", name: "progress", columns: entmigrate.MediaTasksColumns},
			{table: "media_tasks", name: "retry_count", columns: entmigrate.MediaTasksColumns},
			{table: "media_tasks", name: "upstream_task_id", columns: entmigrate.MediaTasksColumns},
			{table: "media_tasks", name: "error_message", columns: entmigrate.MediaTasksColumns},
			{table: "media_artifacts", name: "position", columns: entmigrate.MediaArtifactsColumns},
			{table: "media_artifacts", name: "width", columns: entmigrate.MediaArtifactsColumns},
			{table: "media_artifacts", name: "height", columns: entmigrate.MediaArtifactsColumns},
			{table: "media_artifacts", name: "object_key", columns: entmigrate.MediaArtifactsColumns},
			{table: "media_artifacts", name: "public_url", columns: entmigrate.MediaArtifactsColumns},
		} {
			var catalogType string
			err := tx.QueryRowContext(ctx, `
SELECT data_type
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = $1
  AND column_name = $2
`, tt.table, tt.name).Scan(&catalogType)
			require.NoError(t, err, "query catalog type for %s.%s", tt.table, tt.name)

			entColumn := findMigrationEntColumn(t, tt.columns, tt.name)
			require.Equal(t, catalogType, entColumn.SchemaType[dialect.Postgres], "Ent/SQL type mismatch for %s.%s", tt.table, tt.name)
		}
	})

	t.Run("check foreign key and unique constraints", func(t *testing.T) {
		requireConstraintDefinitionContains(t, tx, "media_tasks", "media_tasks_media_type_check", "CHECK", "media_type", "image", "video")
		requireConstraintDefinitionContains(t, tx, "media_tasks", "media_tasks_native_async_mode_check", "CHECK", "native_async_mode", "unsupported", "optional", "required")
		requireConstraintDefinitionContains(t, tx, "media_tasks", "media_tasks_status_check", "CHECK", "status", "queued", "in_progress", "completed", "failed")
		requireConstraintDefinitionContains(t, tx, "media_tasks", "media_tasks_stage_check", "CHECK", "stage", "scheduling", "submitting", "generating", "polling", "storing", "settling")
		requireConstraintDefinitionContains(t, tx, "media_tasks", "media_tasks_progress_check", "CHECK", "progress", ">= 0", "<= 100")
		requireConstraintDefinitionContains(t, tx, "media_artifacts", "media_artifacts_media_type_check", "CHECK", "media_type", "image", "video")
		requireConstraintDefinitionContains(t, tx, "media_model_definitions", "media_model_definitions_media_type_check", "CHECK", "media_type", "image", "video")

		requireConstraintDefinitionContains(t, tx, "media_tasks", "media_tasks_public_id_key", "UNIQUE", "public_id")
		requireConstraintDefinitionContains(t, tx, "media_artifacts", "media_artifacts_task_id_direction_position_key", "UNIQUE", "task_id", "direction", "position")
		requireConstraintDefinitionContains(t, tx, "media_model_definitions", "media_model_definitions_model_id_key", "UNIQUE", "model_id")
		requireConstraintDefinitionContains(t, tx, "media_artifacts", "media_artifacts_task_id_fkey", "FOREIGN KEY", "task_id", "REFERENCES media_tasks", "ON DELETE CASCADE")
	})

	t.Run("named and partial indexes", func(t *testing.T) {
		requireIndexDefinitionContains(t, tx, "media_tasks", "idx_media_tasks_user_created", "user_id", "created_at DESC")
		requireIndexDefinitionContains(t, tx, "media_tasks", "idx_media_tasks_status_lease", "status", "lease_until")
		requireIndexDefinitionContains(t, tx, "media_tasks", "idx_media_tasks_account", "account_id", "WHERE", "account_id IS NOT NULL")
		requireIndexDefinitionContains(t, tx, "media_tasks", "idx_media_tasks_idempotency", "CREATE UNIQUE INDEX", "user_id", "api_key_id", "idempotency_key", "WHERE", "<>")
		requireIndexDefinitionContains(t, tx, "media_model_definitions", "idx_media_model_definitions_enabled", "enabled", "media_type")
	})

	t.Run("artifacts contain metadata only", func(t *testing.T) {
		rows, err := tx.QueryContext(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'media_artifacts'
ORDER BY ordinal_position
`)
		require.NoError(t, err)
		defer rows.Close()

		var actual []string
		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			actual = append(actual, name)
		}
		require.NoError(t, rows.Err())
		require.Equal(t, []string{
			"id", "task_id", "direction", "position", "media_type", "content_type",
			"size_bytes", "checksum_sha256", "width", "height", "duration_seconds",
			"resolution", "fps", "storage_status", "object_key", "public_url",
			"upstream_reference", "expires_at", "created_at", "updated_at",
		}, actual)

		var binaryColumns int
		err = tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'media_artifacts'
  AND data_type IN ('bytea', 'bit', 'bit varying')
`).Scan(&binaryColumns)
		require.NoError(t, err)
		require.Zero(t, binaryColumns, "media_artifacts must not store media binary content")
	})
}

func TestMediaTaskFencingAndBillingResultsMigrationUpgradeContract(t *testing.T) {
	ctx := context.Background()
	body, err := migrations.FS.ReadFile("130_media_task_fencing_and_billing_results.sql")
	require.NoError(t, err)
	tx := testTx(t)

	_, err = tx.ExecContext(ctx, `
ALTER TABLE media_tasks
    DROP COLUMN claim_token,
    DROP COLUMN additional_charged_amount;
CREATE INDEX IF NOT EXISTS idx_media_artifacts_task ON media_artifacts(task_id, direction, position);
`)
	require.NoError(t, err)

	for attempt := 1; attempt <= 2; attempt++ {
		_, err = tx.ExecContext(ctx, string(body))
		require.NoError(t, err, "upgrade migration re-application %d", attempt)
	}

	requireColumn(t, tx, "media_tasks", "claim_token", "character varying", 64, false)
	requireCatalogColumnDefault(t, tx, "media_tasks", "claim_token", true, "''::character varying")
	requireColumn(t, tx, "media_tasks", "additional_charged_amount", "numeric", 0, false)
	requireCatalogColumnDefault(t, tx, "media_tasks", "additional_charged_amount", true, "0")

	var redundantIndexes int
	err = tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_indexes
WHERE schemaname = 'public'
  AND tablename = 'media_artifacts'
  AND indexname = 'idx_media_artifacts_task'
`).Scan(&redundantIndexes)
	require.NoError(t, err)
	require.Zero(t, redundantIndexes)
}

func requireCatalogColumnDefault(t *testing.T, tx *sql.Tx, table, column string, present bool, contains string) {
	t.Helper()
	var columnDefault sql.NullString
	err := tx.QueryRowContext(context.Background(), `
SELECT column_default
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = $1
  AND column_name = $2
`, table, column).Scan(&columnDefault)
	require.NoError(t, err, "query default for %s.%s", table, column)
	require.Equal(t, present, columnDefault.Valid, "default presence mismatch for %s.%s", table, column)
	if present {
		require.Contains(t, columnDefault.String, contains, "default mismatch for %s.%s", table, column)
	}
}

func requireConstraintDefinitionContains(t *testing.T, tx *sql.Tx, table, constraint string, fragments ...string) {
	t.Helper()
	var definition string
	err := tx.QueryRowContext(context.Background(), `
SELECT pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid = $1::regclass
  AND conname = $2
`, "public."+table, constraint).Scan(&definition)
	require.NoError(t, err, "query constraint %s.%s", table, constraint)
	for _, fragment := range fragments {
		require.Contains(t, definition, fragment, "constraint definition mismatch for %s.%s", table, constraint)
	}
}

func requireIndexDefinitionContains(t *testing.T, tx *sql.Tx, table, index string, fragments ...string) {
	t.Helper()
	var definition string
	err := tx.QueryRowContext(context.Background(), `
SELECT indexdef
FROM pg_indexes
WHERE schemaname = 'public'
  AND tablename = $1
  AND indexname = $2
`, table, index).Scan(&definition)
	require.NoError(t, err, "query index %s.%s", table, index)
	for _, fragment := range fragments {
		require.Contains(t, definition, fragment, "index definition mismatch for %s.%s", table, index)
	}
}

func findMigrationEntColumn(t *testing.T, columns []*entschema.Column, name string) *entschema.Column {
	t.Helper()
	for _, column := range columns {
		if column.Name == name {
			return column
		}
	}
	t.Fatalf("Ent column %q not found", name)
	return nil
}
