package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newGeneratedImageEntRepo(t *testing.T) (service.GeneratedImageStore, *dbent.Client, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:generated_images?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewGeneratedImageRepository(client), client, db
}

func TestGeneratedImageRepositoryCreateListAndGetContent(t *testing.T) {
	repo, _, db := newGeneratedImageEntRepo(t)
	ctx := context.Background()

	var generatedTableCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'generated_images'").Scan(&generatedTableCount))
	require.Equal(t, 1, generatedTableCount)

	first, err := repo.Create(ctx, &service.GeneratedImage{
		Provider:       service.GeneratedImageProviderOpenAI,
		UserID:         12,
		APIKeyID:       34,
		AccountID:      56,
		RequestID:      "req_1",
		Model:          "gpt-image-2",
		Prompt:         "draw a cat",
		RevisedPrompt:  "cute cat",
		ResponseFormat: "b64_json",
		Source:         "b64_json",
		ContentType:    "image/png",
		ImageData:      []byte("first-image"),
		SizeBytes:      len("first-image"),
	})
	require.NoError(t, err)
	require.NotZero(t, first.ID)

	second, err := repo.Create(ctx, &service.GeneratedImage{
		Provider:       "gemini",
		UserID:         13,
		APIKeyID:       35,
		AccountID:      57,
		RequestID:      "req_2",
		Model:          "gpt-image-2",
		Prompt:         "draw a dog",
		ResponseFormat: "url",
		Source:         "upstream_url",
		ContentType:    "image/webp",
		ImageData:      []byte("second-image"),
		SizeBytes:      len("second-image"),
	})
	require.NoError(t, err)

	items, page, err := repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, int64(2), page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 1, page.PageSize)
	require.Len(t, items, 1)
	require.Equal(t, second.ID, items[0].ID)
	require.Equal(t, "gemini", items[0].Provider)
	require.Empty(t, items[0].ImageData, "列表接口不应返回完整图片内容")

	data, contentType, err := repo.GetContent(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, []byte("first-image"), data)
	require.Equal(t, "image/png", contentType)
}
