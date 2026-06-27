package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestGeneratedImagesMigrationSQLDefinesArchiveTable(t *testing.T) {
	content, err := migrations.FS.ReadFile("122_generated_images.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(strings.ToLower(string(content))), " ")

	expectedFragments := []string{
		"create table if not exists generated_images",
		"provider varchar(32) not null default 'openai'",
		"image_data bytea not null",
		"created_at timestamptz not null default now()",
		"generatedimage_created_at",
		"generatedimage_provider_created_at",
		"generatedimage_user_id_created_at",
		"generatedimage_api_key_id_created_at",
		"generatedimage_account_id_created_at",
		"generatedimage_request_id",
	}
	for _, fragment := range expectedFragments {
		require.Contains(t, sql, fragment)
	}
	require.NotContains(t, sql, "drop table")
}
