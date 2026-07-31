package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newProviderRepositorySQLite(t *testing.T) (service.ProviderRepository, *dbent.Client) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return NewProviderRepository(client, nil), client
}

func TestProviderRepositoryReturnsOnlyDeclaredModelProtocolCombinations(t *testing.T) {
	repo, client := newProviderRepositorySQLite(t)
	ctx := context.Background()

	group, err := client.Group.Create().SetName("mixed-provider-pool").SetPlatform("legacy").Save(ctx)
	require.NoError(t, err)
	accountA, err := client.Account.Create().
		SetName("provider-a").SetPlatform("newapi").SetType("api_key").AddGroupIDs(group.ID).Save(ctx)
	require.NoError(t, err)
	accountB, err := client.Account.Create().
		SetName("provider-b").SetPlatform("siliconflow").SetType("api_key").AddGroupIDs(group.ID).Save(ctx)
	require.NoError(t, err)

	modelX := &service.LogicalModel{Name: "model-x", DisplayName: "Model X", Enabled: true, Version: 1}
	modelY := &service.LogicalModel{Name: "model-y", DisplayName: "Model Y", Enabled: true, Version: 1}
	require.NoError(t, repo.UpsertLogicalModel(ctx, modelX))
	require.NoError(t, repo.UpsertLogicalModel(ctx, modelY))

	capA := service.ProviderModelCapability{
		ProviderID: accountA.ID, LogicalModelID: modelX.ID, Protocol: service.ProtocolResponses,
		UpstreamModel: "upstream-x", WireProfile: service.WireProfileCanonical,
		FeatureProfile: service.FeatureProfileText, Enabled: true, Version: 1,
	}
	capB := service.ProviderModelCapability{
		ProviderID: accountB.ID, LogicalModelID: modelY.ID, Protocol: service.ProtocolChatCompletions,
		UpstreamModel: "upstream-y", WireProfile: service.WireProfileCanonical,
		FeatureProfile: service.FeatureProfileText, Enabled: true, Version: 1,
	}
	profileA := service.NewProviderProfile(accountA.ID, accountA.Name)
	profileA.Status = service.ProviderStatusActive
	profileA.Capabilities = []service.ProviderModelCapability{capA}
	profileB := service.NewProviderProfile(accountB.ID, accountB.Name)
	profileB.Status = service.ProviderStatusActive
	profileB.Capabilities = []service.ProviderModelCapability{capB}
	require.NoError(t, repo.SaveProfile(ctx, profileA))
	require.NoError(t, repo.SaveProfile(ctx, profileB))

	endpointA := &service.ProviderProtocolEndpoint{
		ProviderID: accountA.ID, Protocol: service.ProtocolResponses, WireProfile: service.WireProfileCanonical,
		Path: service.ProtocolResponses.DefaultPath(), Enabled: true, Version: 1,
	}
	endpointB := &service.ProviderProtocolEndpoint{
		ProviderID: accountB.ID, Protocol: service.ProtocolChatCompletions, WireProfile: service.WireProfileCanonical,
		Path: service.ProtocolChatCompletions.DefaultPath(), Enabled: true, Version: 1,
	}
	require.NoError(t, repo.SaveEndpoint(ctx, endpointA))
	require.NoError(t, repo.SaveEndpoint(ctx, endpointB))
	capA.EndpointID = &endpointA.ID
	capB.EndpointID = &endpointB.ID
	require.NoError(t, repo.SaveCapability(ctx, &capA))
	require.NoError(t, repo.SaveCapability(ctx, &capB))

	declared, err := repo.ListRouteCapabilities(ctx, service.ProviderCapabilityFilter{
		GroupID: group.ID, LogicalModel: "model-y", Protocol: service.ProtocolChatCompletions,
	})
	require.NoError(t, err)
	require.Len(t, declared, 1)
	require.Equal(t, accountB.ID, declared[0].Profile.ID)
	require.Equal(t, "upstream-y", declared[0].Capability.UpstreamModel)

	undeclared, err := repo.ListRouteCapabilities(ctx, service.ProviderCapabilityFilter{
		GroupID: group.ID, LogicalModel: "model-x", Protocol: service.ProtocolChatCompletions,
	})
	require.NoError(t, err)
	require.Empty(t, undeclared, "the repository must not infer a Cartesian product")

	allDeclaredProtocols, err := repo.ListRouteCapabilities(ctx, service.ProviderCapabilityFilter{
		GroupID: group.ID, LogicalModel: "model-x", OnlySchedulable: true,
	})
	require.NoError(t, err)
	require.Len(t, allDeclaredProtocols, 1)
	require.Equal(t, service.ProtocolResponses, allDeclaredProtocols[0].Capability.Protocol)
	require.Equal(t, 50, allDeclaredProtocols[0].GroupPriority)
}

func TestProviderRepositoryPersistsConversionDefaultAndSanitizesHeaders(t *testing.T) {
	repo, client := newProviderRepositorySQLite(t)
	ctx := context.Background()
	account, err := client.Account.Create().SetName("provider").SetPlatform("custom").SetType("api_key").Save(ctx)
	require.NoError(t, err)

	profile := service.NewProviderProfile(account.ID, account.Name)
	profile.Connection.Headers = map[string]string{"X-Tenant": "one", "Authorization": "must-not-persist"}
	require.NoError(t, repo.SaveProfile(ctx, profile))

	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.False(t, got.Profile.AllowProtocolConversion)
	require.Equal(t, "one", got.Profile.Connection.Headers["X-Tenant"])
	require.NotContains(t, got.Profile.Connection.Headers, "Authorization")
}

func TestProviderRepositoryRollbackToLegacyClearsActivePointer(t *testing.T) {
	repo, client := newProviderRepositorySQLite(t)
	ctx := context.Background()
	groupEntity, err := client.Group.Create().SetName("provider-cutover").SetPlatform("legacy").Save(ctx)
	require.NoError(t, err)

	snapshot, err := repo.(*providerRepository).CreateGroupRouteSnapshot(ctx, groupEntity.ID, map[string]any{"route_count": 1}, map[string]any{"review_required": true})
	require.NoError(t, err)
	require.Equal(t, "review_required", snapshot.Status)
	approved, err := repo.(*providerRepository).ApproveGroupRouteSnapshot(ctx, groupEntity.ID, snapshot.Version, 99)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Status)
	_, err = client.GroupRouteSnapshot.UpdateOneID(snapshot.ID).SetStatus("active").Save(ctx)
	require.NoError(t, err)
	_, err = client.Group.UpdateOneID(groupEntity.ID).SetActiveRouteSnapshotVersion(snapshot.Version).Save(ctx)
	require.NoError(t, err)

	rollback, err := repo.(*providerRepository).RollbackGroupRouteSnapshot(ctx, groupEntity.ID)
	require.NoError(t, err)
	require.Zero(t, rollback.ActiveVersion)
	reloaded, err := client.Group.Get(ctx, groupEntity.ID)
	require.NoError(t, err)
	require.Nil(t, reloaded.ActiveRouteSnapshotVersion)
}

func TestProviderRepositoryActiveSnapshotRejectsDriftAndRollbackRestoresPreviousRoutes(t *testing.T) {
	repo, client := newProviderRepositorySQLite(t)
	providerRepo := repo.(*providerRepository)
	ctx := context.Background()
	groupEntity, err := client.Group.Create().SetName("snapshot-routes").SetPlatform("legacy").Save(ctx)
	require.NoError(t, err)
	accountEntity, err := client.Account.Create().
		SetName("provider").SetPlatform(service.PlatformProvider).SetType("api_key").AddGroupIDs(groupEntity.ID).Save(ctx)
	require.NoError(t, err)
	model := &service.LogicalModel{Name: "model-a", DisplayName: "Model A", Enabled: true, Version: 1}
	require.NoError(t, repo.UpsertLogicalModel(ctx, model))
	profile := service.NewProviderProfile(accountEntity.ID, accountEntity.Name)
	require.NoError(t, repo.SaveProfile(ctx, profile))
	endpoint := &service.ProviderProtocolEndpoint{
		ProviderID: accountEntity.ID, Protocol: service.ProtocolChatCompletions,
		WireProfile: service.WireProfileCanonical, Path: service.ProtocolChatCompletions.DefaultPath(),
		Enabled: true, Version: 1,
	}
	require.NoError(t, repo.SaveEndpoint(ctx, endpoint))
	capability := &service.ProviderModelCapability{
		ProviderID: accountEntity.ID, LogicalModelID: model.ID, EndpointID: &endpoint.ID,
		Protocol: service.ProtocolChatCompletions, UpstreamModel: "upstream-model",
		WireProfile: service.WireProfileCanonical, FeatureProfile: service.FeatureProfileText,
		Enabled: true, Version: 1,
	}
	require.NoError(t, repo.SaveCapability(ctx, capability))
	profile.Status = service.ProviderStatusActive
	profile.Capabilities = []service.ProviderModelCapability{*capability}
	require.NoError(t, repo.SaveProfile(ctx, profile))

	declared, err := repo.ListRouteCapabilities(ctx, service.ProviderCapabilityFilter{
		GroupID: groupEntity.ID, LogicalModel: model.Name, Protocol: service.ProtocolChatCompletions,
	})
	require.NoError(t, err)
	require.Len(t, declared, 1)
	routeIdentity := service.NewRouteIdentity(declared[0], service.ProtocolChatCompletions, "", "").String()
	routeManifest := map[string]any{
		"route_identity": routeIdentity, "logical_model": model.Name,
		"ingress_protocol": service.ProtocolChatCompletions,
	}

	first, err := providerRepo.CreateGroupRouteSnapshot(ctx, groupEntity.ID, map[string]any{
		"routes": []map[string]any{routeManifest},
	}, map[string]any{})
	require.NoError(t, err)
	_, err = providerRepo.ApproveGroupRouteSnapshot(ctx, groupEntity.ID, first.Version, 1)
	require.NoError(t, err)
	_, err = providerRepo.ActivateGroupRouteSnapshot(ctx, groupEntity.ID, first.Version)
	require.NoError(t, err)

	second, err := providerRepo.CreateGroupRouteSnapshot(ctx, groupEntity.ID, map[string]any{
		"routes": []map[string]any{routeManifest},
	}, map[string]any{})
	require.NoError(t, err)
	_, err = providerRepo.ApproveGroupRouteSnapshot(ctx, groupEntity.ID, second.Version, 1)
	require.NoError(t, err)
	_, err = providerRepo.ActivateGroupRouteSnapshot(ctx, groupEntity.ID, second.Version)
	require.NoError(t, err)

	rollback, err := providerRepo.RollbackGroupRouteSnapshot(ctx, groupEntity.ID)
	require.NoError(t, err)
	require.Equal(t, first.Version, rollback.ActiveVersion)
	filtered, err := repo.ListRouteCapabilities(ctx, service.ProviderCapabilityFilter{
		GroupID: groupEntity.ID, SnapshotVersion: first.Version,
		IngressProtocol: service.ProtocolChatCompletions,
		LogicalModel:    model.Name, Protocol: service.ProtocolChatCompletions,
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)

	stale, err := providerRepo.CreateGroupRouteSnapshot(ctx, groupEntity.ID, map[string]any{
		"routes": []map[string]any{routeManifest},
	}, map[string]any{})
	require.NoError(t, err)
	_, err = providerRepo.ApproveGroupRouteSnapshot(ctx, groupEntity.ID, stale.Version, 1)
	require.NoError(t, err)
	_, err = client.ProviderProtocolEndpoint.UpdateOneID(endpoint.ID).SetVersion(endpoint.Version + 1).Save(ctx)
	require.NoError(t, err)
	_, err = providerRepo.ActivateGroupRouteSnapshot(ctx, groupEntity.ID, stale.Version)
	require.Error(t, err)
	reloaded, reloadErr := client.Group.Get(ctx, groupEntity.ID)
	require.NoError(t, reloadErr)
	require.NotNil(t, reloaded.ActiveRouteSnapshotVersion)
	require.Equal(t, first.Version, *reloaded.ActiveRouteSnapshotVersion)
	filtered, err = repo.ListRouteCapabilities(ctx, service.ProviderCapabilityFilter{
		GroupID: groupEntity.ID, SnapshotVersion: first.Version,
		IngressProtocol: service.ProtocolChatCompletions,
		LogicalModel:    model.Name, Protocol: service.ProtocolChatCompletions,
	})
	require.NoError(t, err)
	require.Empty(t, filtered, "任何版本化配置漂移都必须使已批准路由失效")
}

func TestProviderRepositoryRejectsUnapprovedSnapshotWithoutMovingGroupPointer(t *testing.T) {
	repo, client := newProviderRepositorySQLite(t)
	providerRepo := repo.(*providerRepository)
	ctx := context.Background()
	groupEntity, err := client.Group.Create().SetName("unapproved-snapshot").SetPlatform("legacy").Save(ctx)
	require.NoError(t, err)
	snapshot, err := providerRepo.CreateGroupRouteSnapshot(ctx, groupEntity.ID, map[string]any{
		"routes": []map[string]any{{"route_identity": "route"}},
	}, map[string]any{})
	require.NoError(t, err)

	_, err = providerRepo.ActivateGroupRouteSnapshot(ctx, groupEntity.ID, snapshot.Version)

	require.Error(t, err)
	reloaded, reloadErr := client.Group.Get(ctx, groupEntity.ID)
	require.NoError(t, reloadErr)
	require.Nil(t, reloaded.ActiveRouteSnapshotVersion)
	require.Nil(t, reloaded.PreviousRouteSnapshotVersion)
}
