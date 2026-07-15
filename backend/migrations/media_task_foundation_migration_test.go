package migrations

import (
	"testing"

	entmigrate "github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entschema "entgo.io/ent/dialect/sql/schema"
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

func TestMediaTaskFoundationEntSchemaUsesMigrationPostgresTypes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		columns []*entschema.Column
		column  string
		want    string
	}{
		{name: "task progress", columns: entmigrate.MediaTasksColumns, column: "progress", want: "integer"},
		{name: "task retry count", columns: entmigrate.MediaTasksColumns, column: "retry_count", want: "integer"},
		{name: "task upstream ID", columns: entmigrate.MediaTasksColumns, column: "upstream_task_id", want: "text"},
		{name: "task error message", columns: entmigrate.MediaTasksColumns, column: "error_message", want: "text"},
		{name: "artifact position", columns: entmigrate.MediaArtifactsColumns, column: "position", want: "integer"},
		{name: "artifact width", columns: entmigrate.MediaArtifactsColumns, column: "width", want: "integer"},
		{name: "artifact height", columns: entmigrate.MediaArtifactsColumns, column: "height", want: "integer"},
		{name: "artifact object key", columns: entmigrate.MediaArtifactsColumns, column: "object_key", want: "text"},
		{name: "artifact public URL", columns: entmigrate.MediaArtifactsColumns, column: "public_url", want: "text"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			column := findEntColumn(t, tt.columns, tt.column)
			require.Equal(t, tt.want, column.SchemaType[dialect.Postgres])
		})
	}
}

func findEntColumn(t *testing.T, columns []*entschema.Column, name string) *entschema.Column {
	t.Helper()
	for _, column := range columns {
		if column.Name == name {
			return column
		}
	}
	t.Fatalf("Ent column %q not found", name)
	return nil
}
