package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

func newMediaModelRepositoryTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func seedMediaModelDefinition(t *testing.T, client *dbent.Client, modelID string, enabled bool) {
	t.Helper()
	_, err := client.MediaModelDefinition.Create().
		SetModelID(modelID).
		SetMediaType(string(service.MediaTypeImage)).
		SetOperations([]string{string(service.MediaOperationTextToImage)}).
		SetConstraints([]byte(`{"image_sizes":["1024x1024"]}`)).
		SetBillingUnit("image").
		SetEnabled(enabled).
		Save(context.Background())
	require.NoError(t, err)
}

func collectMediaModelIDs(items []service.MediaModelDefinition) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ModelID)
	}
	return ids
}

func TestMediaModelRepositoryListEnabledExcludesDisabled(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	seedMediaModelDefinition(t, client, "enabled-image", true)
	seedMediaModelDefinition(t, client, "disabled-image", false)
	repo := NewMediaModelRepository(client)
	items, err := repo.ListEnabled(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"enabled-image"}, collectMediaModelIDs(items))
}

func TestMediaModelRepositoryListEnabledMapsDefinitionFields(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	createdAt := time.Date(2026, time.July, 15, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	constraints := json.RawMessage(`{"video_durations":[5,10],"min_fps":24}`)
	entity, err := client.MediaModelDefinition.Create().
		SetModelID("video-model").
		SetMediaType(string(service.MediaTypeVideo)).
		SetOperations([]string{string(service.MediaOperationTextToVideo), string(service.MediaOperationImageToVideo)}).
		SetConstraints(constraints).
		SetBillingUnit("second").
		SetVendor("fake-vendor").
		SetDefaultAdapter("fake-adapter").
		SetDefaultAsyncMode(string(service.NativeAsyncOptional)).
		SetEnabled(true).
		SetCreatedAt(createdAt).
		SetUpdatedAt(updatedAt).
		Save(context.Background())
	require.NoError(t, err)

	repo := NewMediaModelRepository(client)
	items, err := repo.ListEnabled(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	require.Equal(t, entity.ID, item.ID)
	require.Equal(t, "video-model", item.ModelID)
	require.Equal(t, "fake-vendor", item.Vendor)
	require.Equal(t, service.MediaTypeVideo, item.MediaType)
	require.Equal(t, []service.MediaOperation{service.MediaOperationTextToVideo, service.MediaOperationImageToVideo}, item.Operations)
	require.JSONEq(t, string(constraints), string(item.Constraints))
	require.Equal(t, "second", item.BillingUnit)
	require.Equal(t, "fake-adapter", item.DefaultAdapter)
	require.Equal(t, service.NativeAsyncOptional, item.DefaultAsyncMode)
	require.True(t, item.Enabled)
	require.Equal(t, createdAt, item.CreatedAt)
	require.Equal(t, updatedAt, item.UpdatedAt)
}

func TestMediaModelAliasRepositoryListAllMapsAliasesInIDOrder(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	first, err := client.MediaModelDefinition.Create().
		SetModelID("first-model").
		SetMediaType(string(service.MediaTypeImage)).
		SetOperations([]string{string(service.MediaOperationTextToImage)}).
		SetConstraints([]byte(`{}`)).
		SetBillingUnit("image").
		Save(context.Background())
	require.NoError(t, err)
	second, err := client.MediaModelDefinition.Create().
		SetModelID("second-model").
		SetMediaType(string(service.MediaTypeImage)).
		SetOperations([]string{string(service.MediaOperationTextToImage)}).
		SetConstraints([]byte(`{}`)).
		SetBillingUnit("image").
		Save(context.Background())
	require.NoError(t, err)
	_, err = client.MediaModelAlias.Create().
		SetRequestedModelID("second-alias").
		SetModelDefinitionID(second.ID).
		Save(context.Background())
	require.NoError(t, err)
	_, err = client.MediaModelAlias.Create().
		SetRequestedModelID("first-alias").
		SetModelDefinitionID(first.ID).
		Save(context.Background())
	require.NoError(t, err)

	aliases, err := NewMediaModelAliasRepository(client).ListAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, []service.MediaModelAlias{
		{RequestedModelID: "second-alias", ModelDefinitionID: second.ID},
		{RequestedModelID: "first-alias", ModelDefinitionID: first.ID},
	}, aliases)
}

func TestMediaModelRepositorySingleArgumentRegistryLoadsAliases(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	definition, err := client.MediaModelDefinition.Create().
		SetModelID("canonical-image").
		SetMediaType(string(service.MediaTypeImage)).
		SetOperations([]string{string(service.MediaOperationTextToImage)}).
		SetConstraints([]byte(`{}`)).
		SetBillingUnit("image").
		SetVendor("fake-vendor").
		SetDefaultAdapter("fake-adapter").
		SetDefaultAsyncMode(string(service.NativeAsyncOptional)).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	_, err = client.MediaModelAlias.Create().
		SetRequestedModelID("image-alias").
		SetModelDefinitionID(definition.ID).
		Save(context.Background())
	require.NoError(t, err)

	registry := service.NewMediaModelRegistry(NewMediaModelRepository(client))
	require.NoError(t, registry.Refresh(context.Background()))
	resolved, err := registry.Resolve("image-alias", service.MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "canonical-image", resolved.ModelID)
}
