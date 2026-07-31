package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/ent/grouproutesnapshot"
	"github.com/Wei-Shaw/sub2api/ent/logicalmodel"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/providermodelcapability"
	"github.com/Wei-Shaw/sub2api/ent/providerprofile"
	"github.com/Wei-Shaw/sub2api/ent/providerprotocolendpoint"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type providerRepository struct {
	client    *dbent.Client
	encryptor service.SecretEncryptor
}

func (r *providerRepository) ListGroupRouteSnapshots(ctx context.Context, groupID int64) ([]service.GroupRouteSnapshot, error) {
	if r == nil || r.client == nil || groupID <= 0 {
		return nil, errors.New("invalid group route snapshot query")
	}
	items, err := r.client.GroupRouteSnapshot.Query().Where(grouproutesnapshot.GroupIDEQ(groupID)).Order(dbent.Desc(grouproutesnapshot.FieldVersion)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.GroupRouteSnapshot, 0, len(items))
	for _, item := range items {
		result = append(result, groupRouteSnapshotEntityToService(item))
	}
	return result, nil
}

func (r *providerRepository) CreateGroupRouteSnapshot(ctx context.Context, groupID int64, manifest, shadowDiff map[string]any) (*service.GroupRouteSnapshot, error) {
	if r == nil || r.client == nil || groupID <= 0 {
		return nil, errors.New("invalid group route snapshot")
	}
	version := int64(1)
	latest, err := r.client.GroupRouteSnapshot.Query().Where(grouproutesnapshot.GroupIDEQ(groupID)).Order(dbent.Desc(grouproutesnapshot.FieldVersion)).First(ctx)
	if err == nil {
		version = latest.Version + 1
	} else if !dbent.IsNotFound(err) {
		return nil, err
	}
	item, err := r.client.GroupRouteSnapshot.Create().SetGroupID(groupID).SetVersion(version).SetStatus("review_required").SetManifest(manifest).SetShadowDiff(shadowDiff).Save(ctx)
	if err != nil {
		return nil, err
	}
	result := groupRouteSnapshotEntityToService(item)
	return &result, nil
}

func (r *providerRepository) ApproveGroupRouteSnapshot(ctx context.Context, groupID, version, reviewerID int64) (*service.GroupRouteSnapshot, error) {
	item, err := r.client.GroupRouteSnapshot.Query().Where(grouproutesnapshot.GroupIDEQ(groupID), grouproutesnapshot.VersionEQ(version)).Only(ctx)
	if err != nil {
		return nil, err
	}
	if item.Status != "review_required" && item.Status != "draft" {
		return nil, errors.New("group route snapshot cannot be approved from its current status")
	}
	update := item.Update().SetStatus("approved").SetApprovedAt(time.Now())
	if reviewerID > 0 {
		update.SetApprovedBy(reviewerID)
	}
	item, err = update.Save(ctx)
	if err != nil {
		return nil, err
	}
	result := groupRouteSnapshotEntityToService(item)
	return &result, nil
}

func (r *providerRepository) ActivateGroupRouteSnapshot(ctx context.Context, groupID, version int64) (*service.GroupRouteCutover, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	snapshot, err := tx.GroupRouteSnapshot.Query().Where(grouproutesnapshot.GroupIDEQ(groupID), grouproutesnapshot.VersionEQ(version)).Only(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot.Status != "approved" && snapshot.Status != "active" {
		return nil, errors.New("group route snapshot must be approved before activation")
	}
	if err := r.validateGroupRouteSnapshotManifest(ctx, tx.Client(), groupID, snapshot.Manifest); err != nil {
		return nil, err
	}
	groupEntity, err := tx.Group.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	previous := int64(0)
	if groupEntity.ActiveRouteSnapshotVersion != nil {
		previous = *groupEntity.ActiveRouteSnapshotVersion
	}
	groupUpdate := groupEntity.Update().SetActiveRouteSnapshotVersion(version)
	if previous > 0 {
		groupUpdate.SetPreviousRouteSnapshotVersion(previous)
	} else {
		groupUpdate.ClearPreviousRouteSnapshotVersion()
	}
	if _, err = groupUpdate.Save(ctx); err != nil {
		return nil, err
	}
	if previous > 0 && previous != version {
		_, _ = tx.GroupRouteSnapshot.Update().Where(grouproutesnapshot.GroupIDEQ(groupID), grouproutesnapshot.VersionEQ(previous)).SetStatus("superseded").Save(ctx)
	}
	if _, err = snapshot.Update().SetStatus("active").Save(ctx); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.GroupRouteCutover{GroupID: groupID, ActiveVersion: version, PreviousVersion: previous}, nil
}

func (r *providerRepository) validateGroupRouteSnapshotManifest(
	ctx context.Context,
	client *dbent.Client,
	groupID int64,
	manifest map[string]any,
) error {
	rawRoutes, ok := manifest["routes"]
	if !ok {
		return errors.New("group route snapshot has no routes")
	}
	encoded, err := json.Marshal(rawRoutes)
	if err != nil {
		return fmt.Errorf("encode group route snapshot routes: %w", err)
	}
	var routes []struct {
		RouteIdentity   string                 `json:"route_identity"`
		LogicalModel    string                 `json:"logical_model"`
		IngressProtocol service.ProtocolFamily `json:"ingress_protocol"`
	}
	if err := json.Unmarshal(encoded, &routes); err != nil {
		return fmt.Errorf("decode group route snapshot routes: %w", err)
	}
	if len(routes) == 0 {
		return errors.New("group route snapshot has no routes")
	}
	txRepo := &providerRepository{client: client, encryptor: r.encryptor}
	capabilities, err := txRepo.ListGroupCapabilities(ctx, groupID)
	if err != nil {
		return fmt.Errorf("validate group route snapshot capabilities: %w", err)
	}
	for _, route := range routes {
		expected := strings.TrimSpace(route.RouteIdentity)
		logicalModel := strings.TrimSpace(route.LogicalModel)
		if expected == "" || logicalModel == "" || !route.IngressProtocol.IsValid() {
			return errors.New("group route snapshot contains an invalid route")
		}
		matched := false
		for _, capability := range capabilities {
			if !strings.EqualFold(capability.LogicalModel.Name, logicalModel) {
				continue
			}
			adapter, adapterVersion := "", ""
			if capability.Capability.Protocol != route.IngressProtocol {
				if capability.Profile == nil || !capability.Profile.AllowProtocolConversion ||
					capability.Capability.Protocol == service.ProtocolEmbeddings || route.IngressProtocol == service.ProtocolEmbeddings {
					continue
				}
				adapter, adapterVersion = service.ProviderAdapterIdentity(route.IngressProtocol, capability.Capability.Protocol)
			}
			identity := service.NewRouteIdentity(capability, route.IngressProtocol, adapter, adapterVersion)
			if identity.String() == expected {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("group route snapshot route is no longer eligible: %s", expected)
		}
	}
	return nil
}

func (r *providerRepository) RollbackGroupRouteSnapshot(ctx context.Context, groupID int64) (*service.GroupRouteCutover, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	groupEntity, err := tx.Group.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	current := int64(0)
	if groupEntity.ActiveRouteSnapshotVersion != nil {
		current = *groupEntity.ActiveRouteSnapshotVersion
	}
	previous := int64(0)
	if groupEntity.PreviousRouteSnapshotVersion != nil {
		previous = *groupEntity.PreviousRouteSnapshotVersion
	}
	update := groupEntity.Update()
	if previous > 0 {
		update.SetActiveRouteSnapshotVersion(previous).SetPreviousRouteSnapshotVersion(current)
		_, _ = tx.GroupRouteSnapshot.Update().Where(grouproutesnapshot.GroupIDEQ(groupID), grouproutesnapshot.VersionEQ(previous)).SetStatus("active").Save(ctx)
	} else {
		update.ClearActiveRouteSnapshotVersion()
		if current > 0 {
			update.SetPreviousRouteSnapshotVersion(current)
		}
	}
	if _, err = update.Save(ctx); err != nil {
		return nil, err
	}
	if current > 0 {
		_, _ = tx.GroupRouteSnapshot.Update().Where(grouproutesnapshot.GroupIDEQ(groupID), grouproutesnapshot.VersionEQ(current)).SetStatus("superseded").Save(ctx)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &service.GroupRouteCutover{GroupID: groupID, ActiveVersion: previous, PreviousVersion: current}, nil
}

func groupRouteSnapshotEntityToService(item *dbent.GroupRouteSnapshot) service.GroupRouteSnapshot {
	return service.GroupRouteSnapshot{ID: item.ID, GroupID: item.GroupID, Version: item.Version, Status: item.Status, Manifest: item.Manifest, ShadowDiff: item.ShadowDiff, ApprovedBy: item.ApprovedBy, ApprovedAt: item.ApprovedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func NewProviderRepository(client *dbent.Client, encryptor service.SecretEncryptor) *providerRepository {
	return &providerRepository{client: client, encryptor: encryptor}
}

func (r *providerRepository) List(ctx context.Context) ([]*service.ProviderAggregate, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("provider repository is not initialized")
	}
	profiles, err := r.client.ProviderProfile.Query().Order(dbent.Desc(providerprofile.FieldUpdatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	aggregates, err := r.loadProviderAggregates(ctx, profiles)
	if err != nil {
		return nil, err
	}
	result := make([]*service.ProviderAggregate, 0, len(profiles))
	for _, profile := range profiles {
		if aggregate, ok := aggregates[profile.ID]; ok {
			result = append(result, aggregate)
		}
	}
	return result, nil
}

func (r *providerRepository) UpdateProfileIfVersion(ctx context.Context, profile *service.ProviderProfile, expectedVersion int64) error {
	if r == nil || r.client == nil {
		return errors.New("provider repository is not initialized")
	}
	if profile == nil || expectedVersion <= 0 || profile.Version <= expectedVersion {
		return errors.New("invalid provider profile version update")
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	update := r.client.ProviderProfile.Update().
		Where(providerprofile.IDEQ(profile.ID), providerprofile.VersionEQ(expectedVersion)).
		SetDisplayName(profile.Name).
		SetStatus(string(profile.Status)).
		SetAllowProtocolConversion(profile.AllowProtocolConversion).
		SetDefaultHeaders(sanitizeStoredHeaders(profile.Connection.Headers)).
		SetVersion(profile.Version)
	if baseURL := strings.TrimSpace(profile.Connection.BaseURL); baseURL != "" {
		update.SetBaseURL(baseURL)
	} else {
		update.ClearBaseURL()
	}
	if authType := strings.TrimSpace(profile.Connection.AuthType); authType != "" {
		update.SetAuthType(authType)
	} else {
		update.ClearAuthType()
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if affected != 1 {
		return service.ErrProviderVersionConflict
	}
	return enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventProviderChanged, &profile.ID, nil, nil)
}

func (r *providerRepository) ListGroupCapabilities(ctx context.Context, groupID int64) ([]service.ProviderRouteCapability, error) {
	if r == nil || r.client == nil || groupID <= 0 {
		return nil, errors.New("invalid provider group capability query")
	}
	memberships, err := r.client.AccountGroup.Query().Where(accountgroup.GroupIDEQ(groupID)).All(ctx)
	if err != nil {
		return nil, err
	}
	providerIDs := make([]int64, 0, len(memberships))
	for _, membership := range memberships {
		providerIDs = append(providerIDs, membership.AccountID)
	}
	aggregates, err := r.loadProviderAggregatesByIDs(ctx, providerIDs)
	if err != nil {
		return nil, err
	}
	result := make([]service.ProviderRouteCapability, 0)
	for _, membership := range memberships {
		aggregate, ok := aggregates[membership.AccountID]
		if !ok {
			continue
		}
		if aggregate.Profile.Status != service.ProviderStatusActive || !aggregate.Account.IsSchedulable() {
			continue
		}
		for _, capability := range aggregate.Profile.Capabilities {
			if !capability.Enabled {
				continue
			}
			model, ok := aggregate.LogicalModels[capability.LogicalModelID]
			if !ok || !model.Enabled {
				continue
			}
			endpoint := matchingProviderEndpoint(aggregate.Profile, capability)
			if endpoint == nil || !endpoint.Enabled {
				continue
			}
			result = append(result, service.ProviderRouteCapability{
				Profile: aggregate.Profile, Account: aggregate.Account, Endpoint: endpoint,
				LogicalModel: model, Capability: capability, GroupPriority: membership.Priority,
			})
		}
	}
	return result, nil
}

func (r *providerRepository) SaveProfile(ctx context.Context, profile *service.ProviderProfile) error {
	if r == nil || r.client == nil {
		return errors.New("provider repository is not initialized")
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	create := r.client.ProviderProfile.Create().
		SetID(profile.ID).
		SetDisplayName(profile.Name).
		SetStatus(string(profile.Status)).
		SetAllowProtocolConversion(profile.AllowProtocolConversion).
		SetDefaultHeaders(sanitizeStoredHeaders(profile.Connection.Headers)).
		SetVersion(profile.Version)
	if baseURL := strings.TrimSpace(profile.Connection.BaseURL); baseURL != "" {
		create.SetBaseURL(baseURL)
	}
	if authType := strings.TrimSpace(profile.Connection.AuthType); authType != "" {
		create.SetAuthType(authType)
	}
	if !profile.CreatedAt.IsZero() {
		create.SetCreatedAt(profile.CreatedAt)
	}
	if !profile.UpdatedAt.IsZero() {
		create.SetUpdatedAt(profile.UpdatedAt)
	}
	if err := create.OnConflictColumns(providerprofile.FieldID).UpdateNewValues().Exec(ctx); err != nil {
		return err
	}
	return enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventProviderChanged, &profile.ID, nil, nil)
}

func (r *providerRepository) SaveEndpoint(ctx context.Context, endpoint *service.ProviderProtocolEndpoint) error {
	if r == nil || r.client == nil {
		return errors.New("provider repository is not initialized")
	}
	if endpoint == nil || endpoint.ProviderID <= 0 || !endpoint.Protocol.IsValid() {
		return errors.New("invalid provider protocol endpoint")
	}
	if endpoint.WireProfile == "" {
		endpoint.WireProfile = service.WireProfileCanonical
	}
	if !endpoint.WireProfile.IsValid() {
		return fmt.Errorf("invalid wire profile %q", endpoint.WireProfile)
	}
	if endpoint.Version <= 0 {
		return errors.New("endpoint version must be positive")
	}
	endpointPath := strings.TrimSpace(endpoint.Path)
	if endpointPath == "" {
		endpointPath = endpoint.Protocol.DefaultPath()
	}
	create := r.client.ProviderProtocolEndpoint.Create().
		SetProviderID(endpoint.ProviderID).
		SetProtocolFamily(string(endpoint.Protocol)).
		SetWireProfile(string(endpoint.WireProfile)).
		SetPath(endpointPath).
		SetHeaders(sanitizeStoredHeaders(endpoint.Headers)).
		SetEnabled(endpoint.Enabled).
		SetVersion(endpoint.Version)
	if baseURL := strings.TrimSpace(endpoint.BaseURL); baseURL != "" {
		create.SetBaseURL(baseURL)
	}
	if authType := strings.TrimSpace(endpoint.AuthType); authType != "" {
		create.SetAuthType(authType)
	}
	id, err := create.
		OnConflictColumns(providerprotocolendpoint.FieldProviderID, providerprotocolendpoint.FieldProtocolFamily).
		UpdateNewValues().
		ID(ctx)
	if err == nil {
		endpoint.ID = id
		err = enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventProviderChanged, &endpoint.ProviderID, nil, nil)
	}
	return err
}

func (r *providerRepository) SaveCapability(ctx context.Context, capability *service.ProviderModelCapability) error {
	if r == nil || r.client == nil {
		return errors.New("provider repository is not initialized")
	}
	if capability == nil {
		return errors.New("provider capability is nil")
	}
	if capability.WireProfile == "" {
		capability.WireProfile = service.WireProfileCanonical
	}
	if err := capability.Validate(); err != nil {
		return err
	}
	create := r.client.ProviderModelCapability.Create().
		SetProviderID(capability.ProviderID).
		SetLogicalModelID(capability.LogicalModelID).
		SetNillableEndpointID(capability.EndpointID).
		SetProtocolFamily(string(capability.Protocol)).
		SetUpstreamModel(strings.TrimSpace(capability.UpstreamModel)).
		SetWireProfile(string(capability.WireProfile)).
		SetFeatureProfile(string(capability.FeatureProfile)).
		SetEnabled(capability.Enabled).
		SetLegacyCompatibility(capability.LegacyCompatibility).
		SetVersion(capability.Version)
	id, err := create.
		OnConflictColumns(
			providermodelcapability.FieldProviderID,
			providermodelcapability.FieldLogicalModelID,
			providermodelcapability.FieldProtocolFamily,
		).
		UpdateNewValues().
		ID(ctx)
	if err == nil {
		capability.ID = id
		err = enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventCapabilityChanged, &capability.ProviderID, nil, map[string]any{
			"capability_id": capability.ID,
			"version":       capability.Version,
		})
	}
	return err
}

func (r *providerRepository) UpsertLogicalModel(ctx context.Context, model *service.LogicalModel) error {
	if r == nil || r.client == nil {
		return errors.New("provider repository is not initialized")
	}
	if model == nil {
		return errors.New("logical model is nil")
	}
	if err := model.Validate(); err != nil {
		return err
	}
	create := r.client.LogicalModel.Create().
		SetName(strings.TrimSpace(model.Name)).
		SetDisplayName(strings.TrimSpace(model.DisplayName)).
		SetEnabled(model.Enabled).
		SetVersion(model.Version)
	id, err := create.OnConflictColumns(logicalmodel.FieldName).UpdateNewValues().ID(ctx)
	if err == nil {
		model.ID = id
		err = enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventLogicalModelChanged, nil, nil, map[string]any{
			"logical_model_id": model.ID,
			"version":          model.Version,
		})
	}
	return err
}

func (r *providerRepository) GetByID(ctx context.Context, providerID int64) (*service.ProviderAggregate, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("provider repository is not initialized")
	}
	profileEntity, err := r.client.ProviderProfile.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	accountEntity, err := r.client.Account.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	endpoints, err := r.client.ProviderProtocolEndpoint.Query().
		Where(providerprotocolendpoint.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	capabilities, err := r.client.ProviderModelCapability.Query().
		Where(providermodelcapability.ProviderIDEQ(providerID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	logicalModels, err := loadLogicalModels(ctx, r.client, capabilities)
	if err != nil {
		return nil, err
	}
	profile := providerProfileEntityToService(profileEntity)
	profile.Endpoints = make([]service.ProviderProtocolEndpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		profile.Endpoints = append(profile.Endpoints, providerEndpointEntityToService(endpoint))
	}
	profile.Capabilities = make([]service.ProviderModelCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		profile.Capabilities = append(profile.Capabilities, providerCapabilityEntityToService(capability))
	}
	return &service.ProviderAggregate{
		Profile:       profile,
		Account:       accountEntityToService(accountEntity),
		LogicalModels: logicalModels,
	}, nil
}

func (r *providerRepository) ListRouteCapabilities(ctx context.Context, filter service.ProviderCapabilityFilter) ([]service.ProviderRouteCapability, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("provider repository is not initialized")
	}
	if filter.GroupID <= 0 || strings.TrimSpace(filter.LogicalModel) == "" ||
		(filter.Protocol != "" && !filter.Protocol.IsValid()) ||
		(filter.SnapshotVersion > 0 && !filter.IngressProtocol.IsValid()) {
		return nil, errors.New("invalid provider capability filter")
	}
	var approvedRoutes map[string]struct{}
	if filter.SnapshotVersion > 0 {
		var err error
		approvedRoutes, err = r.loadActiveSnapshotRouteIdentities(ctx, filter.GroupID, filter.SnapshotVersion)
		if err != nil {
			return nil, err
		}
	}
	model, err := r.client.LogicalModel.Query().
		Where(logicalmodel.NameEqualFold(strings.TrimSpace(filter.LogicalModel)), logicalmodel.EnabledEQ(true)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return []service.ProviderRouteCapability{}, nil
	}
	if err != nil {
		return nil, err
	}
	memberships, err := r.client.AccountGroup.Query().
		Where(accountgroup.GroupIDEQ(filter.GroupID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	providerIDs := make([]int64, 0, len(memberships))
	groupPriorities := make(map[int64]int, len(memberships))
	for _, membership := range memberships {
		providerIDs = append(providerIDs, membership.AccountID)
		groupPriorities[membership.AccountID] = membership.Priority
	}
	if len(providerIDs) == 0 {
		return []service.ProviderRouteCapability{}, nil
	}
	capabilityPredicates := []predicate.ProviderModelCapability{
		providermodelcapability.ProviderIDIn(providerIDs...),
		providermodelcapability.LogicalModelIDEQ(model.ID),
		providermodelcapability.EnabledEQ(true),
	}
	if filter.Protocol != "" {
		capabilityPredicates = append(capabilityPredicates, providermodelcapability.ProtocolFamilyEQ(string(filter.Protocol)))
	}
	capabilities, err := r.client.ProviderModelCapability.Query().Where(capabilityPredicates...).All(ctx)
	if err != nil {
		return nil, err
	}
	capabilityProviderIDs := make([]int64, 0, len(capabilities))
	for _, capability := range capabilities {
		capabilityProviderIDs = append(capabilityProviderIDs, capability.ProviderID)
	}
	aggregates, err := r.loadProviderAggregatesByIDs(ctx, capabilityProviderIDs)
	if err != nil {
		return nil, err
	}
	result := make([]service.ProviderRouteCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		aggregate, ok := aggregates[capability.ProviderID]
		if !ok {
			continue
		}
		if aggregate.Profile.Status != service.ProviderStatusActive {
			continue
		}
		if filter.OnlySchedulable && !aggregate.Account.IsSchedulable() {
			continue
		}
		serviceCapability := providerCapabilityEntityToService(capability)
		endpoint := matchingProviderEndpoint(aggregate.Profile, serviceCapability)
		if endpoint == nil || !endpoint.Enabled {
			continue
		}
		routeCapability := service.ProviderRouteCapability{
			Profile:       aggregate.Profile,
			Account:       aggregate.Account,
			Endpoint:      endpoint,
			LogicalModel:  service.LogicalModel{ID: model.ID, Name: model.Name, DisplayName: model.DisplayName, Enabled: model.Enabled, Version: model.Version},
			Capability:    serviceCapability,
			GroupPriority: groupPriorities[capability.ProviderID],
		}
		if approvedRoutes != nil {
			adapter, adapterVersion := "", ""
			if serviceCapability.Protocol != filter.IngressProtocol {
				adapter, adapterVersion = service.ProviderAdapterIdentity(filter.IngressProtocol, serviceCapability.Protocol)
			}
			identity := service.NewRouteIdentity(routeCapability, filter.IngressProtocol, adapter, adapterVersion)
			if _, approved := approvedRoutes[identity.String()]; !approved {
				continue
			}
		}
		result = append(result, routeCapability)
	}
	return result, nil
}

func (r *providerRepository) loadActiveSnapshotRouteIdentities(ctx context.Context, groupID, version int64) (map[string]struct{}, error) {
	snapshot, err := r.client.GroupRouteSnapshot.Query().Where(
		grouproutesnapshot.GroupIDEQ(groupID),
		grouproutesnapshot.VersionEQ(version),
		grouproutesnapshot.StatusEQ("active"),
	).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("load active provider route snapshot: %w", err)
	}
	rawRoutes, ok := snapshot.Manifest["routes"]
	if !ok {
		return nil, errors.New("active provider route snapshot has no routes")
	}
	encoded, err := json.Marshal(rawRoutes)
	if err != nil {
		return nil, fmt.Errorf("encode provider route snapshot: %w", err)
	}
	var routes []struct {
		RouteIdentity string `json:"route_identity"`
	}
	if err := json.Unmarshal(encoded, &routes); err != nil {
		return nil, fmt.Errorf("decode provider route snapshot: %w", err)
	}
	identities := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		identity := strings.TrimSpace(route.RouteIdentity)
		if identity == "" {
			return nil, errors.New("active provider route snapshot contains an unversioned route")
		}
		identities[identity] = struct{}{}
	}
	if len(identities) == 0 {
		return nil, errors.New("active provider route snapshot has no versioned routes")
	}
	return identities, nil
}

func (r *providerRepository) loadProviderAggregatesByIDs(ctx context.Context, providerIDs []int64) (map[int64]*service.ProviderAggregate, error) {
	providerIDs = uniquePositiveInt64s(providerIDs)
	if len(providerIDs) == 0 {
		return map[int64]*service.ProviderAggregate{}, nil
	}
	profiles, err := r.client.ProviderProfile.Query().Where(providerprofile.IDIn(providerIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	return r.loadProviderAggregates(ctx, profiles)
}

func (r *providerRepository) loadProviderAggregates(ctx context.Context, profiles []*dbent.ProviderProfile) (map[int64]*service.ProviderAggregate, error) {
	providerIDs := make([]int64, 0, len(profiles))
	for _, profile := range profiles {
		providerIDs = append(providerIDs, profile.ID)
	}
	providerIDs = uniquePositiveInt64s(providerIDs)
	result := make(map[int64]*service.ProviderAggregate, len(providerIDs))
	if len(providerIDs) == 0 {
		return result, nil
	}

	accounts, err := r.client.Account.Query().Where(account.IDIn(providerIDs...)).WithProxy().All(ctx)
	if err != nil {
		return nil, err
	}
	accountByID := make(map[int64]*service.Account, len(accounts))
	for _, entity := range accounts {
		providerAccount := accountEntityToService(entity)
		if err := r.decryptProviderCredential(providerAccount); err != nil {
			return nil, err
		}
		if entity.Edges.Proxy != nil {
			providerAccount.Proxy = proxyEntityToService(entity.Edges.Proxy)
		}
		accountByID[entity.ID] = providerAccount
	}
	for _, entity := range profiles {
		providerAccount, ok := accountByID[entity.ID]
		if !ok {
			continue
		}
		profile := providerProfileEntityToService(entity)
		profile.Endpoints = []service.ProviderProtocolEndpoint{}
		profile.Capabilities = []service.ProviderModelCapability{}
		result[entity.ID] = &service.ProviderAggregate{
			Profile:       profile,
			Account:       providerAccount,
			LogicalModels: map[int64]service.LogicalModel{},
		}
	}

	endpoints, err := r.client.ProviderProtocolEndpoint.Query().
		Where(providerprotocolendpoint.ProviderIDIn(providerIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	for _, entity := range endpoints {
		if aggregate, ok := result[entity.ProviderID]; ok {
			aggregate.Profile.Endpoints = append(aggregate.Profile.Endpoints, providerEndpointEntityToService(entity))
		}
	}

	capabilities, err := r.client.ProviderModelCapability.Query().
		Where(providermodelcapability.ProviderIDIn(providerIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	logicalModels, err := loadLogicalModels(ctx, r.client, capabilities)
	if err != nil {
		return nil, err
	}
	for _, entity := range capabilities {
		aggregate, ok := result[entity.ProviderID]
		if !ok {
			continue
		}
		aggregate.Profile.Capabilities = append(aggregate.Profile.Capabilities, providerCapabilityEntityToService(entity))
		if model, ok := logicalModels[entity.LogicalModelID]; ok {
			aggregate.LogicalModels[entity.LogicalModelID] = model
		}
	}
	return result, nil
}

func (r *providerRepository) decryptProviderCredential(account *service.Account) error {
	if account == nil || strings.TrimSpace(account.GetCredential("api_key")) != "" {
		return nil
	}
	ciphertext := strings.TrimSpace(account.GetCredential("api_key_encrypted_v1"))
	if ciphertext == "" {
		return nil
	}
	if r.encryptor == nil {
		return errors.New("provider credential decryption is not configured")
	}
	plaintext, err := r.encryptor.Decrypt(ciphertext)
	if err != nil {
		return fmt.Errorf("decrypt provider credential: %w", err)
	}
	if account.Credentials == nil {
		account.Credentials = map[string]any{}
	}
	account.Credentials["api_key"] = plaintext
	return nil
}

func matchingProviderEndpoint(profile *service.ProviderProfile, capability service.ProviderModelCapability) *service.ProviderProtocolEndpoint {
	for i := range profile.Endpoints {
		candidate := &profile.Endpoints[i]
		if capability.EndpointID != nil && candidate.ID == *capability.EndpointID {
			return candidate
		}
		if capability.EndpointID == nil && candidate.Protocol == capability.Protocol && candidate.Enabled {
			return candidate
		}
	}
	return nil
}

func sanitizeStoredHeaders(headers map[string]string) map[string]string {
	result := make(map[string]string, len(headers))
	for name, value := range headers {
		if service.IsAllowedProviderHeader(name) {
			result[name] = value
		}
	}
	return result
}

func providerProfileEntityToService(entity *dbent.ProviderProfile) *service.ProviderProfile {
	profile := &service.ProviderProfile{
		ID:                      entity.ID,
		Name:                    entity.DisplayName,
		Status:                  service.ProviderStatus(entity.Status),
		AllowProtocolConversion: entity.AllowProtocolConversion,
		Connection: service.ProviderConnectionConfig{
			Headers: entity.DefaultHeaders,
		},
		Version:   entity.Version,
		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.UpdatedAt,
	}
	if entity.BaseURL != nil {
		profile.Connection.BaseURL = *entity.BaseURL
	}
	if entity.AuthType != nil {
		profile.Connection.AuthType = *entity.AuthType
	}
	return profile
}

func providerEndpointEntityToService(entity *dbent.ProviderProtocolEndpoint) service.ProviderProtocolEndpoint {
	endpoint := service.ProviderProtocolEndpoint{
		ID:          entity.ID,
		ProviderID:  entity.ProviderID,
		Protocol:    service.ProtocolFamily(entity.ProtocolFamily),
		WireProfile: service.WireProfile(entity.WireProfile),
		Path:        entity.Path,
		Headers:     entity.Headers,
		Enabled:     entity.Enabled,
		Version:     entity.Version,
	}
	if entity.BaseURL != nil {
		endpoint.BaseURL = *entity.BaseURL
	}
	if entity.AuthType != nil {
		endpoint.AuthType = *entity.AuthType
	}
	return endpoint
}

func providerCapabilityEntityToService(entity *dbent.ProviderModelCapability) service.ProviderModelCapability {
	return service.ProviderModelCapability{
		ID:                  entity.ID,
		ProviderID:          entity.ProviderID,
		LogicalModelID:      entity.LogicalModelID,
		EndpointID:          entity.EndpointID,
		Protocol:            service.ProtocolFamily(entity.ProtocolFamily),
		UpstreamModel:       entity.UpstreamModel,
		WireProfile:         service.WireProfile(entity.WireProfile),
		FeatureProfile:      service.ProviderFeatureProfile(entity.FeatureProfile),
		Enabled:             entity.Enabled,
		LegacyCompatibility: entity.LegacyCompatibility,
		Version:             entity.Version,
	}
}

func loadLogicalModels(ctx context.Context, client *dbent.Client, capabilities []*dbent.ProviderModelCapability) (map[int64]service.LogicalModel, error) {
	ids := make([]int64, 0, len(capabilities))
	seen := make(map[int64]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if _, ok := seen[capability.LogicalModelID]; ok {
			continue
		}
		seen[capability.LogicalModelID] = struct{}{}
		ids = append(ids, capability.LogicalModelID)
	}
	result := make(map[int64]service.LogicalModel, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	entities, err := client.LogicalModel.Query().Where(logicalmodel.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, entity := range entities {
		result[entity.ID] = service.LogicalModel{
			ID:          entity.ID,
			Name:        entity.Name,
			DisplayName: entity.DisplayName,
			Enabled:     entity.Enabled,
			Version:     entity.Version,
		}
	}
	return result, nil
}
