package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newMediaGroupRepoSQLite(t *testing.T) (*groupRepository, *dbent.Client) {
	t.Helper()

	dsn := fmt.Sprintf("file:group_media_flags_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return newGroupRepositoryWithSQL(client, db), client
}

func TestGroupRepositoryMediaFlagsRoundTripSQLite(t *testing.T) {
	repo, client := newMediaGroupRepoSQLite(t)
	ctx := context.Background()
	group := &service.Group{
		Name:                      "media-round-trip",
		Platform:                  service.PlatformOpenAI,
		Status:                    service.StatusActive,
		SubscriptionType:          service.SubscriptionTypeStandard,
		RateMultiplier:            1,
		AllowImageGeneration:      true,
		AllowVideoGeneration:      true,
		MediaCrossPlatformEnabled: true,
	}

	require.NoError(t, repo.Create(ctx, group))
	createdEntity, err := client.Group.Get(ctx, group.ID)
	require.NoError(t, err)
	require.True(t, createdEntity.AllowVideoGeneration)
	require.True(t, createdEntity.MediaCrossPlatformEnabled)

	got, err := repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.True(t, got.AllowImageGeneration)
	require.True(t, got.AllowVideoGeneration)
	require.True(t, got.MediaCrossPlatformEnabled)

	listed, err := repo.ListActive(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.True(t, listed[0].AllowVideoGeneration)
	require.True(t, listed[0].MediaCrossPlatformEnabled)

	got.AllowImageGeneration = false
	got.AllowVideoGeneration = false
	got.MediaCrossPlatformEnabled = false
	require.NoError(t, repo.Update(ctx, got))

	updatedEntity, err := client.Group.Get(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, updatedEntity.AllowImageGeneration)
	require.False(t, updatedEntity.AllowVideoGeneration)
	require.False(t, updatedEntity.MediaCrossPlatformEnabled)

	updated, err := repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, updated.AllowImageGeneration)
	require.False(t, updated.AllowVideoGeneration)
	require.False(t, updated.MediaCrossPlatformEnabled)
}

func TestAPIKeyRepositoryPreloadedGroupIncludesMediaFlagsSQLite(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "media-flags-apikey@test.com")

	group, err := client.Group.Create().
		SetName("media-api-key").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetAllowImageGeneration(true).
		SetAllowVideoGeneration(true).
		SetMediaCrossPlatformEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	key := &service.APIKey{
		UserID:  user.ID,
		Key:     "sk-media-flags",
		Name:    "Media Flags",
		GroupID: &group.ID,
		Status:  service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.NotNil(t, got.Group)
	require.True(t, got.Group.AllowImageGeneration)
	require.True(t, got.Group.AllowVideoGeneration)
	require.True(t, got.Group.MediaCrossPlatformEnabled)
}
