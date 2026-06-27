package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

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

	items, page, err := repo.List(ctx, service.GeneratedImageListParams{
		PaginationParams: pagination.PaginationParams{Page: 1, PageSize: 1},
	})
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

func TestGeneratedImageRepositoryListEnrichesNamesAndFilters(t *testing.T) {
	repo, client, _ := newGeneratedImageEntRepo(t)
	ctx := context.Background()

	alice, err := client.User.Create().
		SetEmail("alice@example.com").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	bob, err := client.User.Create().
		SetEmail("bob@example.com").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)

	aliceKey, err := client.APIKey.Create().
		SetUserID(alice.ID).
		SetKey("sk-alice").
		SetName("Alice Image Key").
		Save(ctx)
	require.NoError(t, err)
	bobKey, err := client.APIKey.Create().
		SetUserID(bob.ID).
		SetKey("sk-bob").
		SetName("Bob Image Key").
		Save(ctx)
	require.NoError(t, err)

	imageGroup, err := client.Group.Create().
		SetName("Images").
		SetPlatform("openai").
		Save(ctx)
	require.NoError(t, err)
	textGroup, err := client.Group.Create().
		SetName("Text").
		SetPlatform("openai").
		Save(ctx)
	require.NoError(t, err)

	aliceAccount, err := client.Account.Create().
		SetName("OpenAI Image Account").
		SetPlatform("openai").
		SetType("apikey").
		AddGroupIDs(imageGroup.ID).
		Save(ctx)
	require.NoError(t, err)
	bobAccount, err := client.Account.Create().
		SetName("OpenAI Text Account").
		SetPlatform("openai").
		SetType("apikey").
		AddGroupIDs(textGroup.ID).
		Save(ctx)
	require.NoError(t, err)

	createdAt := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	_, err = repo.Create(ctx, &service.GeneratedImage{
		Provider:       service.GeneratedImageProviderOpenAI,
		UserID:         alice.ID,
		APIKeyID:       aliceKey.ID,
		AccountID:      aliceAccount.ID,
		RequestID:      "req_alice",
		Model:          "gpt-image-2",
		Prompt:         "draw alice",
		ResponseFormat: "url",
		Source:         "upstream_url",
		ContentType:    "image/png",
		ImageData:      []byte("alice-image"),
		SizeBytes:      len("alice-image"),
		CreatedAt:      createdAt,
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, &service.GeneratedImage{
		Provider:       service.GeneratedImageProviderOpenAI,
		UserID:         bob.ID,
		APIKeyID:       bobKey.ID,
		AccountID:      bobAccount.ID,
		RequestID:      "req_bob",
		Model:          "gpt-image-2",
		Prompt:         "draw bob",
		ResponseFormat: "b64_json",
		Source:         "b64_json",
		ContentType:    "image/png",
		ImageData:      []byte("bob-image"),
		SizeBytes:      len("bob-image"),
		CreatedAt:      createdAt.Add(48 * time.Hour),
	})
	require.NoError(t, err)

	endAt := createdAt.Add(24 * time.Hour)
	items, page, err := repo.List(ctx, service.GeneratedImageListParams{
		PaginationParams: pagination.PaginationParams{Page: 1, PageSize: 10},
		UserEmail:        "alice@",
		GroupID:          imageGroup.ID,
		StartAt:          &createdAt,
		EndAt:            &endAt,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, items, 1)
	require.Equal(t, "alice@example.com", items[0].UserEmail)
	require.Equal(t, "Alice Image Key", items[0].APIKeyName)
	require.Equal(t, "OpenAI Image Account", items[0].AccountName)
	require.Equal(t, []string{"Images"}, items[0].AccountGroups)
}
