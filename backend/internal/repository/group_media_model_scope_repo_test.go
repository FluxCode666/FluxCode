package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/groupmediamodelscope"
	"github.com/Wei-Shaw/sub2api/ent/mediamodeldefinition"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMediaModelScopeRepositoryReplacesCanonicalWhitelist(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	mediaGroup, err := client.Group.Create().SetName("media").SetPlatform(service.PlatformMedia).Save(context.Background())
	require.NoError(t, err)
	seedMediaModelDefinition(t, client, "image-model", true)
	seedMediaModelDefinition(t, client, "video-model", true)

	repo := NewGroupMediaModelScopeRepository(client)
	require.NoError(t, repo.ReplaceMediaModelScopes(context.Background(), mediaGroup.ID, []string{"VIDEO-MODEL", "image-model"}))
	modelIDs, err := repo.ListMediaModelIDs(context.Background(), mediaGroup.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"image-model", "video-model"}, modelIDs)

	require.NoError(t, repo.ReplaceMediaModelScopes(context.Background(), mediaGroup.ID, []string{"video-model"}))
	modelIDs, err = repo.ListEnabledMediaModelIDs(context.Background(), mediaGroup.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"video-model"}, modelIDs)
}

func TestGroupMediaModelScopeRepositoryHistoricalDisabledScopeRemainsVisibleUntilRemoved(t *testing.T) {
	ctx := context.Background()
	client := newMediaModelRepositoryTestClient(t)
	mediaGroup, err := client.Group.Create().SetName("media").SetPlatform(service.PlatformMedia).Save(ctx)
	require.NoError(t, err)
	seedMediaModelDefinition(t, client, "ready-image", true)
	seedMediaModelDefinition(t, client, "stale-image", true)
	repo := NewGroupMediaModelScopeRepository(client)
	require.NoError(t, repo.ReplaceMediaModelScopes(ctx, mediaGroup.ID, []string{"ready-image", "stale-image"}))
	stale, err := client.MediaModelDefinition.Query().Where(mediamodeldefinition.ModelIDEQ("stale-image")).Only(ctx)
	require.NoError(t, err)
	_, err = client.MediaModelDefinition.UpdateOneID(stale.ID).SetEnabled(false).Save(ctx)
	require.NoError(t, err)

	all, err := repo.ListMediaModelIDs(ctx, mediaGroup.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"ready-image", "stale-image"}, all)
	enabled, err := repo.ListEnabledMediaModelIDs(ctx, mediaGroup.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"ready-image"}, enabled)

	require.NoError(t, repo.ReplaceMediaModelScopes(ctx, mediaGroup.ID, []string{"ready-image"}))
	all, err = repo.ListMediaModelIDs(ctx, mediaGroup.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"ready-image"}, all)
	exists, err := client.GroupMediaModelScope.Query().Where(
		groupmediamodelscope.GroupIDEQ(mediaGroup.ID),
		groupmediamodelscope.ModelDefinitionIDEQ(stale.ID),
	).Exist(ctx)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestGroupMediaModelScopeRepositoryRejectsNonMediaGroup(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	textGroup, err := client.Group.Create().SetName("openai").SetPlatform(service.PlatformOpenAI).Save(context.Background())
	require.NoError(t, err)
	seedMediaModelDefinition(t, client, "image-model", true)

	repo := NewGroupMediaModelScopeRepository(client)
	err = repo.ReplaceMediaModelScopes(context.Background(), textGroup.ID, []string{"image-model"})
	require.ErrorIs(t, err, service.ErrMediaGroupRequired)
}

func TestGroupMediaModelScopeRepositoryUnknownOrDisabledModelDoesNotClearExistingScopes(t *testing.T) {
	client := newMediaModelRepositoryTestClient(t)
	mediaGroup, err := client.Group.Create().SetName("media").SetPlatform(service.PlatformMedia).Save(context.Background())
	require.NoError(t, err)
	seedMediaModelDefinition(t, client, "enabled-image", true)
	seedMediaModelDefinition(t, client, "disabled-image", false)
	repo := NewGroupMediaModelScopeRepository(client)
	require.NoError(t, repo.ReplaceMediaModelScopes(context.Background(), mediaGroup.ID, []string{"enabled-image"}))

	err = repo.ReplaceMediaModelScopes(context.Background(), mediaGroup.ID, []string{"disabled-image"})
	require.ErrorIs(t, err, service.ErrMediaModelScopeModelNotFound)
	modelIDs, listErr := repo.ListMediaModelIDs(context.Background(), mediaGroup.ID)
	require.NoError(t, listErr)
	require.Equal(t, []string{"enabled-image"}, modelIDs)

	err = repo.ReplaceMediaModelScopes(context.Background(), mediaGroup.ID, []string{"unknown-image"})
	require.ErrorIs(t, err, service.ErrMediaModelScopeModelNotFound)
	modelIDs, listErr = repo.ListMediaModelIDs(context.Background(), mediaGroup.ID)
	require.NoError(t, listErr)
	require.Equal(t, []string{"enabled-image"}, modelIDs)
}
