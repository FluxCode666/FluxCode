package migrations

import (
	"testing"

	entschema "entgo.io/ent/dialect/sql/schema"
	entmigrate "github.com/Wei-Shaw/sub2api/ent/migrate"
	"github.com/stretchr/testify/require"
)

func TestMediaUnifiedRoutingStorageMigrationContract(t *testing.T) {
	body, err := FS.ReadFile("131_media_unified_routing_storage.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, fragment := range []string{
		"vendor VARCHAR(64) NOT NULL DEFAULT ''",
		"default_adapter VARCHAR(64) NOT NULL DEFAULT ''",
		"default_async_mode VARCHAR(16) NOT NULL DEFAULT 'unsupported'",
		"storage_provider VARCHAR(32) NOT NULL DEFAULT 'legacy'",
		"CREATE TABLE IF NOT EXISTS media_model_aliases",
		"requested_model_id VARCHAR(128) NOT NULL UNIQUE",
		"model_definition_id BIGINT NOT NULL REFERENCES media_model_definitions(id) ON DELETE CASCADE",
		"CREATE TABLE IF NOT EXISTS group_media_model_scopes",
		"group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE",
		"UNIQUE (group_id, model_definition_id)",
		"idx_media_model_aliases_definition",
		"idx_group_media_model_scopes_definition",
	} {
		require.Contains(t, sql, fragment)
	}
}

func TestMediaUnifiedRoutingStorageEntSchemaContract(t *testing.T) {
	for _, tt := range []struct {
		columns      []*entschema.Column
		column       string
		size         int64
		defaultValue any
	}{
		{columns: entmigrate.MediaModelDefinitionsColumns, column: "vendor", size: 64, defaultValue: ""},
		{columns: entmigrate.MediaModelDefinitionsColumns, column: "default_adapter", size: 64, defaultValue: ""},
		{columns: entmigrate.MediaModelDefinitionsColumns, column: "default_async_mode", size: 16, defaultValue: "unsupported"},
		{columns: entmigrate.MediaArtifactsColumns, column: "storage_provider", size: 32, defaultValue: "legacy"},
		{columns: entmigrate.MediaModelAliasesColumns, column: "requested_model_id", size: 128},
		{columns: entmigrate.GroupMediaModelScopesColumns, column: "group_id"},
		{columns: entmigrate.GroupMediaModelScopesColumns, column: "model_definition_id"},
	} {
		t.Run(tt.column, func(t *testing.T) {
			column := findEntColumn(t, tt.columns, tt.column)
			require.Equal(t, tt.size, column.Size)
			if tt.defaultValue != nil {
				require.Equal(t, tt.defaultValue, column.Default)
			}
		})
	}

	require.True(t, findEntColumn(t, entmigrate.MediaModelAliasesColumns, "requested_model_id").Unique)
	require.Contains(t, entmigrate.GroupMediaModelScopesTable.Indexes, &entschema.Index{
		Name:    "groupmediamodelscope_group_id_model_definition_id",
		Unique:  true,
		Columns: []*entschema.Column{findEntColumn(t, entmigrate.GroupMediaModelScopesColumns, "group_id"), findEntColumn(t, entmigrate.GroupMediaModelScopesColumns, "model_definition_id")},
	})
}
