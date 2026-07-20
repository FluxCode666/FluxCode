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
	"github.com/Wei-Shaw/sub2api/ent/mediamodelalias"
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

func validMediaModelAdminRecord(modelID string, aliases ...string) service.MediaModelAdminRecord {
	return service.MediaModelAdminRecord{
		Definition: service.MediaModelDefinition{
			ModelID:          modelID,
			Vendor:           "openai",
			MediaType:        service.MediaTypeImage,
			Operations:       []service.MediaOperation{service.MediaOperationTextToImage},
			Constraints:      json.RawMessage(`{"image_sizes":["1024x1024"]}`),
			BillingUnit:      "image",
			DefaultAdapter:   "openai-images",
			DefaultAsyncMode: service.NativeAsyncOptional,
			Enabled:          true,
		},
		Aliases: aliases,
	}
}

func TestMediaModelRepositoryAdminCreatePersistsDefinitionAndAliasesAtomically(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	repo := NewMediaModelRepository(client)

	created, err := repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("gpt-image-2", "gpt-image-latest", "image-current"))
	require.NoError(t, err)
	require.NotZero(t, created.Definition.ID)
	require.Equal(t, []string{"gpt-image-latest", "image-current"}, created.Aliases)

	stored, err := repo.GetAdminByID(context.Background(), created.Definition.ID)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", stored.Definition.ModelID)
	require.Equal(t, []string{"gpt-image-latest", "image-current"}, stored.Aliases)
	require.JSONEq(t, `{"image_sizes":["1024x1024"]}`, string(stored.Definition.Constraints))

	items, err := repo.ListAdmin(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, stored.Aliases, items[0].Aliases)
}

func TestMediaModelRepositoryAdminRejectsCrossNamespaceConflictsAndRollsBack(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	repo := NewMediaModelRepository(client)
	first, err := repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("image-one", "image-one-alias"))
	require.NoError(t, err)
	_, err = repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("image-two", "image-two-alias"))
	require.NoError(t, err)

	conflicting := validMediaModelAdminRecord("image-one", "image-two")
	_, err = repo.UpdateAdmin(context.Background(), first.Definition.ID, conflicting)
	require.ErrorIs(t, err, service.ErrMediaModelAliasConflict)

	stored, err := repo.GetAdminByID(context.Background(), first.Definition.ID)
	require.NoError(t, err)
	require.Equal(t, "image-one", stored.Definition.ModelID)
	require.Equal(t, []string{"image-one-alias"}, stored.Aliases)

	_, err = repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("IMAGE-ONE-ALIAS"))
	require.ErrorIs(t, err, service.ErrMediaModelIDConflict)
}

func TestMediaModelRepositoryAdminUpdateReplacesAliasesInSameTransaction(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	repo := NewMediaModelRepository(client)
	created, err := repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("image-one", "old-alias"))
	require.NoError(t, err)

	updatedInput := validMediaModelAdminRecord("image-one", "new-alias")
	updatedInput.Definition.Enabled = false
	updated, err := repo.UpdateAdmin(context.Background(), created.Definition.ID, updatedInput)
	require.NoError(t, err)
	require.Equal(t, "image-one", updated.Definition.ModelID)
	require.False(t, updated.Definition.Enabled)
	require.Equal(t, []string{"new-alias"}, updated.Aliases)

	oldAliasExists, err := client.MediaModelAlias.Query().Where(mediamodelalias.RequestedModelIDEQ("old-alias")).Exist(context.Background())
	require.NoError(t, err)
	require.False(t, oldAliasExists)
}

func TestMediaModelRepositoryAdminRejectsCanonicalModelIDRename(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	repo := NewMediaModelRepository(client)
	created, err := repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("image-one", "image-alias"))
	require.NoError(t, err)

	_, err = repo.UpdateAdmin(context.Background(), created.Definition.ID, validMediaModelAdminRecord("image-two", "image-alias"))
	require.ErrorIs(t, err, service.ErrMediaModelIDImmutable)

	stored, err := repo.GetAdminByID(context.Background(), created.Definition.ID)
	require.NoError(t, err)
	require.Equal(t, "image-one", stored.Definition.ModelID)
	require.Equal(t, []string{"image-alias"}, stored.Aliases)
}

func TestMediaModelRepositoryAdminDeleteCascadesAliasesAndScopes(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	repo := NewMediaModelRepository(client)
	created, err := repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("image-one", "image-alias"))
	require.NoError(t, err)
	mediaGroup, err := client.Group.Create().SetName("media-group").SetPlatform(service.PlatformMedia).Save(context.Background())
	require.NoError(t, err)
	_, err = client.GroupMediaModelScope.Create().
		SetGroupID(mediaGroup.ID).
		SetModelDefinitionID(created.Definition.ID).
		Save(context.Background())
	require.NoError(t, err)

	require.NoError(t, repo.DeleteAdmin(context.Background(), created.Definition.ID))
	aliasCount, err := client.MediaModelAlias.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, aliasCount)
	scopeCount, err := client.GroupMediaModelScope.Query().Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, scopeCount)
	require.ErrorIs(t, repo.DeleteAdmin(context.Background(), created.Definition.ID), service.ErrMediaModelDefinitionNotFound)
}

func TestMediaModelRepositoryRegistryAliasesExcludeDisabledDefinitions(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	repo := NewMediaModelRepository(client)
	created, err := repo.CreateAdmin(context.Background(), validMediaModelAdminRecord("image-one", "image-alias"))
	require.NoError(t, err)
	disabled := validMediaModelAdminRecord("image-one", "image-alias")
	disabled.Definition.Enabled = false
	_, err = repo.UpdateAdmin(context.Background(), created.Definition.ID, disabled)
	require.NoError(t, err)

	aliases, err := repo.ListAll(context.Background())
	require.NoError(t, err)
	require.Empty(t, aliases)
	registry := service.NewMediaModelRegistry(repo)
	require.NoError(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve("image-alias", service.MediaOperationTextToImage)
	require.ErrorIs(t, err, service.ErrMediaModelNotFound)
}
