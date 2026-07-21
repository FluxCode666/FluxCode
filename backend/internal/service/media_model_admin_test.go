package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type mediaModelAdminServiceStore struct {
	record     *MediaModelAdminRecord
	onCreate   func()
	listErr    error
	writeCount int
}

type mediaModelAdminScopeStore struct {
	modelIDs         []string
	listAllCount     int
	listEnabledCount int
	replaceCount     int
}

func (s *mediaModelAdminScopeStore) ListMediaModelIDs(context.Context, int64) ([]string, error) {
	s.listAllCount++
	return append([]string(nil), s.modelIDs...), nil
}

func (s *mediaModelAdminScopeStore) ListEnabledMediaModelIDs(context.Context, int64) ([]string, error) {
	s.listEnabledCount++
	return append([]string(nil), s.modelIDs...), nil
}

func (s *mediaModelAdminScopeStore) ReplaceMediaModelScopes(_ context.Context, _ int64, modelIDs []string) error {
	s.replaceCount++
	s.modelIDs = append([]string(nil), modelIDs...)
	return nil
}

type mediaModelAdminGroupStore struct {
	GroupRepository
	group *Group
}

func (s *mediaModelAdminGroupStore) GetByIDLite(context.Context, int64) (*Group, error) {
	if s.group == nil {
		return nil, ErrGroupNotFound
	}
	copy := *s.group
	return &copy, nil
}

type mediaModelAdminRegistryStore struct {
	definitions []MediaModelDefinition
	aliases     []MediaModelAlias
}

func (s *mediaModelAdminRegistryStore) ListEnabled(context.Context) ([]MediaModelDefinition, error) {
	items := make([]MediaModelDefinition, len(s.definitions))
	for index, definition := range s.definitions {
		items[index] = cloneMediaModelDefinition(definition)
	}
	return items, nil
}

func (s *mediaModelAdminRegistryStore) ListAll(context.Context) ([]MediaModelAlias, error) {
	return append([]MediaModelAlias(nil), s.aliases...), nil
}

func cloneMediaModelAdminServiceRecord(record MediaModelAdminRecord) MediaModelAdminRecord {
	record.Definition = cloneMediaModelDefinition(record.Definition)
	record.AdapterResolution = cloneMediaAdapterResolution(record.AdapterResolution)
	record.Aliases = append([]string(nil), record.Aliases...)
	return record
}

func (s *mediaModelAdminServiceStore) ListEnabled(ctx context.Context) ([]MediaModelDefinition, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.record == nil || !s.record.Definition.Enabled {
		return []MediaModelDefinition{}, nil
	}
	return []MediaModelDefinition{cloneMediaModelDefinition(s.record.Definition)}, nil
}

func (s *mediaModelAdminServiceStore) ListAll(ctx context.Context) ([]MediaModelAlias, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.record == nil || !s.record.Definition.Enabled {
		return []MediaModelAlias{}, nil
	}
	aliases := make([]MediaModelAlias, 0, len(s.record.Aliases))
	for _, alias := range s.record.Aliases {
		aliases = append(aliases, MediaModelAlias{RequestedModelID: alias, ModelDefinitionID: s.record.Definition.ID})
	}
	return aliases, nil
}

func (s *mediaModelAdminServiceStore) ListAdmin(context.Context) ([]MediaModelAdminRecord, error) {
	if s.record == nil {
		return []MediaModelAdminRecord{}, nil
	}
	return []MediaModelAdminRecord{cloneMediaModelAdminServiceRecord(*s.record)}, nil
}

func (s *mediaModelAdminServiceStore) GetAdminByID(context.Context, int64) (*MediaModelAdminRecord, error) {
	if s.record == nil {
		return nil, ErrMediaModelDefinitionNotFound
	}
	copy := cloneMediaModelAdminServiceRecord(*s.record)
	return &copy, nil
}

func (s *mediaModelAdminServiceStore) CreateAdmin(_ context.Context, record MediaModelAdminRecord) (*MediaModelAdminRecord, error) {
	s.writeCount++
	record.Definition.ID = 1
	stored := cloneMediaModelAdminServiceRecord(record)
	s.record = &stored
	if s.onCreate != nil {
		s.onCreate()
	}
	copy := cloneMediaModelAdminServiceRecord(record)
	return &copy, nil
}

func (s *mediaModelAdminServiceStore) UpdateAdmin(_ context.Context, _ int64, record MediaModelAdminRecord) (*MediaModelAdminRecord, error) {
	s.writeCount++
	record.Definition.ID = 1
	if s.record != nil {
		record.LegacyDefaultAdapter = s.record.LegacyDefaultAdapter
		record.LegacyDefaultAsyncMode = s.record.LegacyDefaultAsyncMode
	}
	stored := cloneMediaModelAdminServiceRecord(record)
	s.record = &stored
	copy := cloneMediaModelAdminServiceRecord(record)
	return &copy, nil
}

func (s *mediaModelAdminServiceStore) DeleteAdmin(context.Context, int64) error {
	s.writeCount++
	s.record = nil
	return nil
}

func newReadyMediaModelAdminService(
	t *testing.T,
	store *mediaModelAdminServiceStore,
	definitions ...MediaModelDefinition,
) (*MediaModelAdminService, *MediaModelRegistry, *MediaAdapterResolver) {
	t.Helper()
	_, resolver := newTestMediaAdapterResolver(t,
		testMediaAdapterRegistrationForDefinitions("openai-images", NativeAsyncOptional, definitions...),
	)
	registry := NewMediaModelRegistryWithResolver(store, resolver)
	require.NoError(t, registry.Refresh(context.Background()))
	return NewMediaModelAdminService(store, nil, nil, registry, resolver), registry, resolver
}

func TestMediaModelAdminRejectsEnabledUnresolvedBeforeWrite(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	resolver := NewMediaAdapterResolver(NewMediaAdapterRegistry())
	registry := NewMediaModelRegistryWithResolver(store, resolver)
	svc := NewMediaModelAdminService(store, nil, nil, registry, resolver)
	record := validMediaModelAdminServiceRecord()
	record.Definition.Enabled = true

	_, err := svc.Create(context.Background(), record)
	require.Error(t, err)
	require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", infraerrors.Reason(err))
	require.Nil(t, store.record)
	require.Zero(t, store.writeCount)
}

func TestMediaModelAdminRejectsEnabledUnresolvedUpdateBeforeWrite(t *testing.T) {
	existing := validMediaModelAdminServiceRecord()
	existing.Definition.ID = 1
	store := &mediaModelAdminServiceStore{record: &existing}
	resolver := NewMediaAdapterResolver(NewMediaAdapterRegistry())
	registry := NewMediaModelRegistryWithResolver(store, resolver)
	svc := NewMediaModelAdminService(store, nil, nil, registry, resolver)

	_, err := svc.Update(context.Background(), 1, validMediaModelAdminServiceRecord())
	require.Error(t, err)
	require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", infraerrors.Reason(err))
	require.Zero(t, store.writeCount)
	require.Equal(t, "image-model", store.record.Definition.ModelID)
}

func TestMediaModelAdminAllowsDisabledUnresolvedAndReturnsDiagnostics(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	resolver := NewMediaAdapterResolver(NewMediaAdapterRegistry())
	registry := NewMediaModelRegistryWithResolver(store, resolver)
	svc := NewMediaModelAdminService(store, nil, nil, registry, resolver)
	record := validMediaModelAdminServiceRecord()
	record.Definition.Enabled = false

	created, err := svc.Create(context.Background(), record)
	require.NoError(t, err)
	require.Equal(t, 1, store.writeCount)
	require.Equal(t, MediaAdapterResolutionUnresolved, created.AdapterResolution.Status)
	require.Equal(t, created.AdapterResolution, created.Definition.AdapterResolution)
	require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", created.AdapterResolution.ReasonCode)
}

func TestMediaModelAdminEnrichesResolutionAcrossReadAndWriteMethods(t *testing.T) {
	assertReady := func(t *testing.T, record *MediaModelAdminRecord) {
		t.Helper()
		require.NotNil(t, record)
		require.Equal(t, MediaAdapterResolutionReady, record.AdapterResolution.Status)
		require.Equal(t, "openai-images", record.AdapterResolution.ResolvedAdapter)
		require.Equal(t, MediaAdapterMatchedExact, record.AdapterResolution.MatchedBy)
		require.NotNil(t, record.AdapterResolution.Capabilities)
		require.Equal(t, []MediaOperation{MediaOperationTextToImage}, record.AdapterResolution.Capabilities.Operations)
		require.True(t, record.AdapterResolution.Capabilities.SyncUpstream)
		require.True(t, record.AdapterResolution.Capabilities.NativeAsyncUpstream)
		require.Empty(t, record.AdapterResolution.ReasonCode)
		require.Equal(t, record.AdapterResolution, record.Definition.AdapterResolution)
	}

	for _, operation := range []string{"list", "get", "create", "update"} {
		t.Run(operation, func(t *testing.T) {
			definition := validMediaModelAdminServiceRecord().Definition
			definition.ID = 1
			store := &mediaModelAdminServiceStore{}
			if operation == "list" || operation == "get" || operation == "update" {
				record := validMediaModelAdminServiceRecord()
				record.Definition.ID = 1
				store.record = &record
			}
			svc, _, _ := newReadyMediaModelAdminService(t, store, definition)

			switch operation {
			case "list":
				items, err := svc.List(context.Background())
				require.NoError(t, err)
				require.Len(t, items, 1)
				assertReady(t, &items[0])
			case "get":
				item, err := svc.GetByID(context.Background(), 1)
				require.NoError(t, err)
				assertReady(t, item)
			case "create":
				item, err := svc.Create(context.Background(), validMediaModelAdminServiceRecord())
				require.NoError(t, err)
				assertReady(t, item)
			case "update":
				input := validMediaModelAdminServiceRecord()
				input.Aliases = []string{"updated-alias"}
				item, err := svc.Update(context.Background(), 1, input)
				require.NoError(t, err)
				assertReady(t, item)
			}
		})
	}
}

func TestMediaModelAdminIgnoresLegacyRoutingInputFields(t *testing.T) {
	definition := validMediaModelAdminServiceRecord().Definition
	store := &mediaModelAdminServiceStore{}
	svc, _, _ := newReadyMediaModelAdminService(t, store, definition)
	record := validMediaModelAdminServiceRecord()
	record.Definition.DefaultAdapter = "bad adapter input"
	record.Definition.DefaultAsyncMode = NativeAsyncMode("sometimes")

	created, err := svc.Create(context.Background(), record)
	require.NoError(t, err)
	require.Equal(t, "openai-images", created.AdapterResolution.ResolvedAdapter)
	require.Equal(t, NativeAsyncOptional, created.AdapterResolution.CompatibilityAsyncMode())
	require.Empty(t, store.record.Definition.DefaultAdapter)
	require.Empty(t, store.record.Definition.DefaultAsyncMode)
}

func TestMediaModelAdminGroupScopesExposeAndRemoveHistoricalUnavailableModels(t *testing.T) {
	ready := validMediaModelAdminServiceRecord().Definition
	ready.ID = 1
	stale := ready
	stale.ID = 2
	stale.ModelID = "stale-image"
	stale.Vendor = "legacy"
	models := &mediaModelAdminRegistryStore{
		definitions: []MediaModelDefinition{ready, stale},
		aliases:     []MediaModelAlias{{RequestedModelID: "ready-alias", ModelDefinitionID: ready.ID}},
	}
	_, resolver := newTestMediaAdapterResolver(t,
		testMediaAdapterRegistrationForDefinitions("openai-images", NativeAsyncOptional, ready),
	)
	registry := NewMediaModelRegistryWithResolver(models, resolver)
	require.NoError(t, registry.Refresh(context.Background()))
	scopes := &mediaModelAdminScopeStore{modelIDs: []string{ready.ModelID, stale.ModelID}}
	groups := &mediaModelAdminGroupStore{group: &Group{ID: 7, Platform: PlatformMedia}}
	svc := NewMediaModelAdminService(nil, scopes, groups, registry, resolver)

	visible, err := svc.GetGroupScopes(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, []string{ready.ModelID, stale.ModelID}, visible)
	require.Equal(t, 1, scopes.listAllCount)
	require.Zero(t, scopes.listEnabledCount)

	_, err = svc.ReplaceGroupScopes(context.Background(), 7, []string{ready.ModelID, stale.ModelID})
	require.ErrorIs(t, err, ErrMediaModelScopeModelNotFound)
	require.Zero(t, scopes.replaceCount)
	_, err = svc.ReplaceGroupScopes(context.Background(), 7, []string{"ready-alias"})
	require.ErrorIs(t, err, ErrMediaModelScopeModelNotFound)
	require.Zero(t, scopes.replaceCount)

	remaining, err := svc.ReplaceGroupScopes(context.Background(), 7, []string{ready.ModelID})
	require.NoError(t, err)
	require.Equal(t, []string{ready.ModelID}, remaining)
	require.Equal(t, 1, scopes.replaceCount)
	require.Equal(t, []string{ready.ModelID}, scopes.modelIDs)
}

func TestMediaModelPreflightIsReadOnly(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	record := validMediaModelAdminServiceRecord()
	record.Definition.ModelID = "grok-2-image"
	record.Definition.Vendor = "xai"
	record.LegacyDefaultAdapter = "xai-image"
	record.LegacyDefaultAsyncMode = NativeAsyncUnsupported
	store.record = &record
	_, resolver := newTestMediaAdapterResolver(t,
		exactImageRegistration("xai-image", "xai", "grok-2-image"),
	)
	svc := NewMediaModelAdminService(store, nil, nil, nil, resolver)
	report, err := svc.Preflight(context.Background())
	require.NoError(t, err)
	require.True(t, report.Safe)
	require.Len(t, report.Items, 1)
	require.True(t, report.Items[0].LegacyCheckApplicable)
	require.True(t, report.Items[0].LegacyAsyncModeReadable)
	require.Equal(t, MediaAdapterResolutionReady, report.Items[0].Status)
	require.Empty(t, report.Items[0].ReasonCode)
	require.Equal(t, 0, store.writeCount)
}

func TestMediaModelPreflightLegacyCompatibilityMatrix(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		legacyAdapter  string
		legacyMode     NativeAsyncMode
		wantSafe       bool
		wantBlocking   int
		wantApplicable bool
		wantKeyMatch   bool
	}{
		{name: "empty legacy key", enabled: true, wantSafe: true, wantApplicable: false, wantKeyMatch: true},
		{name: "mismatched legacy key", enabled: true, legacyAdapter: "other-image", legacyMode: NativeAsyncOptional, wantSafe: false, wantBlocking: 1, wantApplicable: true},
		{name: "unreadable legacy mode", enabled: true, legacyAdapter: "openai-images", legacyMode: "broken", wantSafe: false, wantBlocking: 1, wantApplicable: true, wantKeyMatch: true},
		{name: "normalized matching key", enabled: true, legacyAdapter: " OpenAI-Images ", legacyMode: NativeAsyncOptional, wantSafe: true, wantApplicable: true, wantKeyMatch: true},
		{name: "disabled unresolved never blocks", enabled: false, legacyAdapter: "broken", legacyMode: "broken", wantSafe: true, wantApplicable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mediaModelAdminServiceStore{}
			record := validMediaModelAdminServiceRecord()
			record.Definition.Enabled = tt.enabled
			record.LegacyDefaultAdapter = tt.legacyAdapter
			record.LegacyDefaultAsyncMode = tt.legacyMode
			store.record = &record
			var resolver *MediaAdapterResolver
			if tt.enabled {
				_, resolver = newTestMediaAdapterResolver(t,
					exactImageRegistration("openai-images", record.Definition.Vendor, record.Definition.ModelID),
				)
			} else {
				resolver = NewMediaAdapterResolver(NewMediaAdapterRegistry())
			}
			svc := NewMediaModelAdminService(store, nil, nil, nil, resolver)

			report, err := svc.Preflight(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantSafe, report.Safe)
			require.Equal(t, tt.wantBlocking, report.BlockingCount)
			require.Len(t, report.Items, 1)
			item := report.Items[0]
			require.Equal(t, tt.wantApplicable, item.LegacyCheckApplicable)
			require.Equal(t, tt.wantKeyMatch, item.AdapterKeyMatches)
			require.Equal(t, tt.legacyAdapter, item.LegacyDefaultAdapter)
			require.Equal(t, !tt.enabled || tt.wantSafe, item.RolloutSafe)
			if tt.enabled {
				require.Equal(t, MediaAdapterResolutionReady, item.Status)
				require.Empty(t, item.ReasonCode)
			} else {
				require.Equal(t, MediaAdapterResolutionUnresolved, item.Status)
				require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", item.ReasonCode)
			}
			legacyMode := NativeAsyncMode(strings.ToLower(strings.TrimSpace(string(tt.legacyMode))))
			wantLegacyReadable := !tt.wantApplicable ||
				legacyMode == NativeAsyncUnsupported ||
				legacyMode == NativeAsyncOptional ||
				legacyMode == NativeAsyncRequired
			require.Equal(t, wantLegacyReadable, item.LegacyAsyncModeReadable)
		})
	}
}

func validMediaModelAdminServiceRecord() MediaModelAdminRecord {
	return MediaModelAdminRecord{
		Definition: MediaModelDefinition{
			ModelID:     "image-model",
			Vendor:      "openai",
			MediaType:   MediaTypeImage,
			Operations:  []MediaOperation{MediaOperationTextToImage},
			Constraints: json.RawMessage(`{"image_sizes":["1024x1024"]}`),
			BillingUnit: "image",
			Enabled:     true,
		},
		Aliases: []string{"image-alias"},
	}
}

func TestMediaModelAdminServiceRefreshesAfterCommitEvenWhenRequestIsCanceled(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	definition := validMediaModelAdminServiceRecord().Definition
	svc, registry, _ := newReadyMediaModelAdminService(t, store, definition)
	ctx, cancel := context.WithCancel(context.Background())
	store.onCreate = cancel

	_, err := svc.Create(ctx, validMediaModelAdminServiceRecord())
	require.NoError(t, err)
	resolved, err := registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "image-model", resolved.ModelID)
}

func TestMediaModelAdminServiceDoesNotReportCommittedCreateAsFailedWhenRefreshFails(t *testing.T) {
	store := &mediaModelAdminServiceStore{}
	definition := validMediaModelAdminServiceRecord().Definition
	svc, _, _ := newReadyMediaModelAdminService(t, store, definition)
	store.onCreate = func() { store.listErr = errors.New("registry read unavailable") }

	created, err := svc.Create(context.Background(), validMediaModelAdminServiceRecord())
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, "image-model", store.record.Definition.ModelID)
}

func TestMediaModelAdminServiceStrictValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MediaModelAdminRecord)
	}{
		{name: "model id", mutate: func(r *MediaModelAdminRecord) { r.Definition.ModelID = "bad model" }},
		{name: "vendor", mutate: func(r *MediaModelAdminRecord) { r.Definition.Vendor = "bad.vendor" }},
		{name: "media type", mutate: func(r *MediaModelAdminRecord) { r.Definition.MediaType = "audio" }},
		{name: "operation", mutate: func(r *MediaModelAdminRecord) { r.Definition.Operations = []MediaOperation{"unknown"} }},
		{name: "duplicate operation", mutate: func(r *MediaModelAdminRecord) {
			r.Definition.Operations = []MediaOperation{MediaOperationTextToImage, MediaOperationTextToImage}
		}},
		{name: "constraints shape", mutate: func(r *MediaModelAdminRecord) { r.Definition.Constraints = json.RawMessage(`[]`) }},
		{name: "constraints unknown field", mutate: func(r *MediaModelAdminRecord) { r.Definition.Constraints = json.RawMessage(`{"script":"x"}`) }},
		{name: "constraints type", mutate: func(r *MediaModelAdminRecord) {
			r.Definition.Constraints = json.RawMessage(`{"max_image_count":"many"}`)
		}},
		{name: "alias equals canonical", mutate: func(r *MediaModelAdminRecord) { r.Aliases = []string{"image-model"} }},
		{name: "duplicate alias", mutate: func(r *MediaModelAdminRecord) { r.Aliases = []string{"alias", " ALIAS "} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := validMediaModelAdminServiceRecord()
			tt.mutate(&record)
			_, err := normalizeAndValidateMediaModelAdminRecord(record)
			require.Error(t, err)
			require.Equal(t, "INVALID_MEDIA_MODEL", infraerrors.Reason(err))
		})
	}
}
