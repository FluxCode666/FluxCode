package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type mediaModelRepoStub struct {
	items []MediaModelDefinition
	err   error
}

type periodicMediaModelRepoStub struct {
	mu    sync.RWMutex
	items []MediaModelDefinition
}

func (s *periodicMediaModelRepoStub) ListEnabled(context.Context) ([]MediaModelDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]MediaModelDefinition(nil), s.items...), nil
}

func (s *periodicMediaModelRepoStub) set(items ...MediaModelDefinition) {
	s.mu.Lock()
	s.items = append([]MediaModelDefinition(nil), items...)
	s.mu.Unlock()
}

func (s *mediaModelRepoStub) ListEnabled(context.Context) ([]MediaModelDefinition, error) {
	return s.items, s.err
}

func TestMediaModelRegistryPeriodicRefreshLoadsExternalChanges(t *testing.T) {
	first := validImageModelDefinition()
	second := validImageModelDefinition()
	second.ModelID = "external-image"
	repo := &periodicMediaModelRepoStub{items: []MediaModelDefinition{first}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, first, second)
	require.NoError(t, registry.Refresh(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry.StartPeriodicRefresh(ctx, 5*time.Millisecond)

	repo.set(second)
	require.Eventually(t, func() bool {
		resolved, err := registry.Resolve(second.ModelID, MediaOperationTextToImage)
		return err == nil && resolved.ModelID == second.ModelID
	}, time.Second, 5*time.Millisecond)
}

type mediaModelAliasRepoStub struct {
	items []MediaModelAlias
	err   error
}

func (s *mediaModelAliasRepoStub) ListAll(context.Context) ([]MediaModelAlias, error) {
	return s.items, s.err
}

type mediaModelRepositoryWithAliasesStub struct {
	mediaModelRepoStub
	mediaModelAliasRepoStub
}

func validImageModelDefinition() MediaModelDefinition {
	return MediaModelDefinition{
		ModelID:          "fake-image",
		Vendor:           "fake-vendor",
		MediaType:        MediaTypeImage,
		Operations:       []MediaOperation{MediaOperationTextToImage, MediaOperationImageToImage, MediaOperationImageEdit},
		Constraints:      json.RawMessage(`{}`),
		BillingUnit:      "image",
		DefaultAdapter:   "fake-adapter",
		DefaultAsyncMode: NativeAsyncOptional,
		Enabled:          true,
	}
}

func validVideoModelDefinition() MediaModelDefinition {
	return MediaModelDefinition{
		ModelID:   "fake-video",
		Vendor:    "fake-vendor",
		MediaType: MediaTypeVideo,
		Operations: []MediaOperation{
			MediaOperationTextToVideo,
			MediaOperationImageToVideo,
			MediaOperationReferenceVideo,
			MediaOperationVideoExtend,
			MediaOperationVideoRemix,
		},
		Constraints:      json.RawMessage(`{}`),
		BillingUnit:      "second",
		DefaultAdapter:   "fake-adapter",
		DefaultAsyncMode: NativeAsyncOptional,
		Enabled:          true,
	}
}

func testMediaAdapterRegistrationForDefinitions(
	key string,
	mode NativeAsyncMode,
	definitions ...MediaModelDefinition,
) MediaAdapterRegistration {
	operationSet := map[MediaOperation]struct{}{}
	exactRules := make([]MediaAdapterExactRule, 0, len(definitions))
	for _, definition := range definitions {
		operations := append([]MediaOperation(nil), definition.Operations...)
		for _, operation := range operations {
			operationSet[operation] = struct{}{}
		}
		exactRules = append(exactRules, MediaAdapterExactRule{
			Vendor:  definition.Vendor,
			ModelID: definition.ModelID,
			Capabilities: MediaAdapterRuleCapabilities{
				Operations:          operations,
				SyncUpstream:        mode != NativeAsyncRequired,
				NativeAsyncUpstream: mode != NativeAsyncUnsupported,
			},
		})
	}
	operations := make([]MediaOperation, 0, len(operationSet))
	for operation := range operationSet {
		operations = append(operations, operation)
	}
	slices.Sort(operations)
	return MediaAdapterRegistration{
		Key: key,
		Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
			Name: key, NativeAsyncMode: mode,
		}),
		SupportedOperations: operations,
		ExactRules:          exactRules,
	}
}

func newReadyMediaModelRegistry(
	t *testing.T,
	repo MediaModelDefinitionRepository,
	aliasRepo MediaModelAliasRepository,
	mode NativeAsyncMode,
	definitions ...MediaModelDefinition,
) *MediaModelRegistry {
	t.Helper()
	_, resolver := newTestMediaAdapterResolver(t,
		testMediaAdapterRegistrationForDefinitions("test-model-adapter", mode, definitions...),
	)
	if aliasRepo != nil {
		return NewMediaModelRegistryWithResolver(repo, resolver, aliasRepo)
	}
	return NewMediaModelRegistryWithResolver(repo, resolver)
}

func TestMediaModelRegistryPublishesValidModelsAndUnavailableTombstones(t *testing.T) {
	ready := validImageModelDefinition()
	ready.ID, ready.ModelID, ready.Vendor = 1, "grok-2-image", "xai"
	ready.Operations = []MediaOperation{MediaOperationTextToImage}
	unavailable := validImageModelDefinition()
	unavailable.ID, unavailable.ModelID, unavailable.Vendor = 2, "unknown-image", "unknown"
	unavailable.Operations = []MediaOperation{MediaOperationTextToImage}
	repo := &mediaModelRepositoryWithAliasesStub{
		mediaModelRepoStub: mediaModelRepoStub{items: []MediaModelDefinition{ready, unavailable}},
		mediaModelAliasRepoStub: mediaModelAliasRepoStub{items: []MediaModelAlias{{
			RequestedModelID: "unknown-alias", ModelDefinitionID: 2,
		}}},
	}
	_, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("xai-image", "xai", "grok-2-image"))
	registry := NewMediaModelRegistryWithResolver(repo, resolver)
	require.NoError(t, registry.Refresh(context.Background()))

	definition, err := registry.Resolve("grok-2-image", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "xai-image", definition.AdapterResolution.ResolvedAdapter)
	require.Equal(t, "xai-image", definition.DefaultAdapter)
	require.Equal(t, NativeAsyncUnsupported, definition.DefaultAsyncMode)

	for _, modelID := range []string{"unknown-image", "unknown-alias"} {
		_, err = registry.Resolve(modelID, MediaOperationTextToImage)
		require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
		require.Equal(t, "MEDIA_MODEL_ADAPTER_UNAVAILABLE", infraerrors.Reason(err))
		appErr := infraerrors.FromError(err)
		require.Equal(t, "unknown-image", appErr.Metadata["model_id"])
		require.Equal(t, string(MediaAdapterResolutionUnresolved), appErr.Metadata["resolution_status"])
		require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", appErr.Metadata["reason_code"])
	}

	_, err = registry.CanonicalModelID("unknown-alias")
	require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
}

func TestMediaModelRegistryCapabilityMismatchReplacesOldReadyRouteWithTombstone(t *testing.T) {
	ready := validImageModelDefinition()
	ready.ModelID = "changing-image"
	ready.Operations = []MediaOperation{MediaOperationTextToImage}
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{ready}}
	_, resolver := newTestMediaAdapterResolver(t,
		testMediaAdapterRegistrationForDefinitions("changing-adapter", NativeAsyncUnsupported, ready),
	)
	registry := NewMediaModelRegistryWithResolver(repo, resolver)
	require.NoError(t, registry.Refresh(context.Background()))
	_, err := registry.Resolve(ready.ModelID, MediaOperationTextToImage)
	require.NoError(t, err)

	mismatch := ready
	mismatch.Operations = []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit}
	repo.items = []MediaModelDefinition{mismatch}
	require.NoError(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve(ready.ModelID, MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
	require.Equal(t, string(MediaAdapterResolutionCapabilityMismatch), infraerrors.FromError(err).Metadata["resolution_status"])
}

func TestMediaModelRegistryImplementationMissingPreservesWholeSnapshot(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	adapters, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("xai-image", definition.Vendor, definition.ModelID))
	registry := NewMediaModelRegistryWithResolver(repo, resolver)
	require.NoError(t, registry.Refresh(context.Background()))

	adapters.mu.Lock()
	delete(adapters.adapters, "xai-image")
	adapters.mu.Unlock()
	repo.items = []MediaModelDefinition{func() MediaModelDefinition {
		changed := definition
		changed.Constraints = json.RawMessage(`{"max_image_count":2}`)
		return changed
	}()}
	err := registry.Refresh(context.Background())
	require.ErrorContains(t, err, "implementation_missing")

	resolved, resolveErr := registry.Resolve(definition.ModelID, MediaOperationTextToImage)
	require.NoError(t, resolveErr)
	require.JSONEq(t, `{}`, string(resolved.Constraints))
}

func TestMediaModelRegistryResolutionMetricsAndLogsUseFixedSafeFields(t *testing.T) {
	invalid := validImageModelDefinition()
	invalid.ID, invalid.ModelID = 1, "invalid model"
	invalid.Constraints = json.RawMessage(`{"image_sizes":["do-not-log-credential"]}`)
	unresolved := validImageModelDefinition()
	unresolved.ID, unresolved.ModelID, unresolved.Vendor = 2, "unresolved-image", "unresolved"
	ambiguous := validImageModelDefinition()
	ambiguous.ID, ambiguous.ModelID, ambiguous.Vendor = 3, "ambiguous-image", "ambiguous"
	mismatch := validImageModelDefinition()
	mismatch.ID, mismatch.ModelID, mismatch.Vendor = 4, "mismatch-image", "mismatch"
	mismatch.Operations = []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit}

	familyRegistration := func(key, familyID string) MediaAdapterRegistration {
		return MediaAdapterRegistration{
			Key: key,
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
				Name: key, NativeAsyncMode: NativeAsyncUnsupported,
			}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			FamilyRules: []MediaAdapterFamilyRule{{
				Vendor: "ambiguous", FamilyID: familyID,
				Match: func(modelID string) bool { return modelID == "ambiguous-image" },
				Capabilities: MediaAdapterRuleCapabilities{
					Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
				},
			}},
		}
	}
	mismatchRule := mismatch
	mismatchRule.Operations = []MediaOperation{MediaOperationTextToImage}
	_, resolver := newTestMediaAdapterResolver(t,
		familyRegistration("ambiguous-a", "family-a"),
		familyRegistration("ambiguous-b", "family-b"),
		testMediaAdapterRegistrationForDefinitions("mismatch-adapter", NativeAsyncUnsupported, mismatchRule),
	)
	registry := NewMediaModelRegistryWithResolver(
		&mediaModelRepoStub{items: []MediaModelDefinition{invalid, unresolved, ambiguous, mismatch}}, resolver,
	)
	metrics := NewAtomicMediaTaskMetrics()
	registry.SetRoutingMetrics(metrics)
	var logs bytes.Buffer
	registry.SetLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, registry.Refresh(context.Background()))

	for _, status := range []MediaAdapterResolutionStatus{
		MediaAdapterResolutionInvalidDefinition,
		MediaAdapterResolutionUnresolved,
		MediaAdapterResolutionAmbiguous,
		MediaAdapterResolutionCapabilityMismatch,
	} {
		require.Equal(t, int64(1), metrics.AdapterResolutionFailures(status), status)
	}
	require.Equal(t, int64(0), metrics.AdapterResolutionFailures(MediaAdapterResolutionImplementationMissing))

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	require.Len(t, lines, 4)
	require.NotContains(t, logs.String(), "do-not-log-credential")
	for _, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		require.Equal(t, "media_model_adapter_resolution_unavailable", record["msg"])
		for _, field := range []string{
			"canonical_model_id", "vendor", "adapter_resolution_status", "adapter_key",
			"matched_by", "matched_family", "reason_code",
		} {
			require.Contains(t, record, field)
		}
		require.NotContains(t, record, "constraints")
		require.NotContains(t, record, "request_body")
		require.NotContains(t, record, "credential")
	}

	implemented, missingResolver := newTestMediaAdapterResolver(t,
		exactImageRegistration("missing-adapter", "missing", "missing-image"),
	)
	implemented.mu.Lock()
	delete(implemented.adapters, "missing-adapter")
	implemented.mu.Unlock()
	missing := validImageModelDefinition()
	missing.ModelID, missing.Vendor = "missing-image", "missing"
	missing.Operations = []MediaOperation{MediaOperationTextToImage}
	missing.Constraints = json.RawMessage(`{"image_sizes":["do-not-log-api-key"]}`)
	missingRegistry := NewMediaModelRegistryWithResolver(&mediaModelRepoStub{items: []MediaModelDefinition{missing}}, missingResolver)
	missingRegistry.SetRoutingMetrics(metrics)
	var missingLogs bytes.Buffer
	missingRegistry.SetLogger(slog.New(slog.NewJSONHandler(&missingLogs, nil)))
	require.Error(t, missingRegistry.Refresh(context.Background()))
	require.Equal(t, int64(1), metrics.AdapterResolutionFailures(MediaAdapterResolutionImplementationMissing))
	require.NotContains(t, missingLogs.String(), "do-not-log-api-key")
	var missingRecord map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(missingLogs.Bytes()), &missingRecord))
	require.Equal(t, "ERROR", missingRecord["level"])
	require.Equal(t, string(MediaAdapterResolutionImplementationMissing), missingRecord["adapter_resolution_status"])
	require.Equal(t, "missing-adapter", missingRecord["adapter_key"])
}

func TestMediaModelDefinitionJSONKeepsV1FieldsAndOmitsAdapterResolution(t *testing.T) {
	definition := validImageModelDefinition()
	definition.AdapterResolution = readyResolution(true, true)
	encoded, err := json.Marshal(definition)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"DefaultAdapter":"fake-adapter"`)
	require.Contains(t, string(encoded), `"DefaultAsyncMode":"optional"`)
	require.NotContains(t, string(encoded), "AdapterResolution")

	cloned := cloneMediaModelDefinition(definition)
	cloned.AdapterResolution.Capabilities.Operations[0] = MediaOperationVideoRemix
	require.Equal(t, MediaOperationTextToImage, definition.AdapterResolution.Capabilities.Operations[0])
}

func TestMediaModelRegistryConcurrentRefreshResolveAndObserverUpdates(t *testing.T) {
	first := validImageModelDefinition()
	second := validImageModelDefinition()
	second.ModelID = "second-image"
	repo := &periodicMediaModelRepoStub{items: []MediaModelDefinition{first}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, first, second)
	require.NoError(t, registry.Refresh(context.Background()))

	metrics := NewAtomicMediaTaskMetrics()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	var wait sync.WaitGroup
	wait.Add(3)
	go func() {
		defer wait.Done()
		for index := 0; index < 100; index++ {
			if index%2 == 0 {
				repo.set(first)
			} else {
				repo.set(second)
			}
			require.NoError(t, registry.Refresh(context.Background()))
		}
	}()
	go func() {
		defer wait.Done()
		for index := 0; index < 200; index++ {
			_, firstErr := registry.Resolve(first.ModelID, MediaOperationTextToImage)
			_, secondErr := registry.Resolve(second.ModelID, MediaOperationTextToImage)
			require.True(t, firstErr == nil || errors.Is(firstErr, ErrMediaModelNotFound))
			require.True(t, secondErr == nil || errors.Is(secondErr, ErrMediaModelNotFound))
		}
	}()
	go func() {
		defer wait.Done()
		for index := 0; index < 100; index++ {
			registry.SetRoutingMetrics(metrics)
			registry.SetLogger(logger)
		}
	}()
	wait.Wait()
}

func TestMediaModelRegistryRefreshResolvesAliases(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ID = 10
	aliases := &mediaModelAliasRepoStub{items: []MediaModelAlias{{
		RequestedModelID:  "  IMAGE-ALIAS  ",
		ModelDefinitionID: definition.ID,
	}}}
	registry := newReadyMediaModelRegistry(
		t, &mediaModelRepoStub{items: []MediaModelDefinition{definition}}, aliases, NativeAsyncOptional, definition,
	)

	require.NoError(t, registry.Refresh(context.Background()))
	resolved, err := registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "fake-image", resolved.ModelID)
}

func TestMediaModelRegistryWithResolverDiscoversAliasesFromRepository(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ID = 10
	repo := &mediaModelRepositoryWithAliasesStub{
		mediaModelRepoStub: mediaModelRepoStub{items: []MediaModelDefinition{definition}},
		mediaModelAliasRepoStub: mediaModelAliasRepoStub{items: []MediaModelAlias{{
			RequestedModelID:  "image-alias",
			ModelDefinitionID: definition.ID,
		}}},
	}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, definition)

	require.NoError(t, registry.Refresh(context.Background()))
	resolved, err := registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "fake-image", resolved.ModelID)

	repo.mediaModelAliasRepoStub.err = errors.New("database unavailable")
	require.Error(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
}

func TestMediaModelRegistryRefreshRejectsInvalidAliasesAndPreservesSnapshot(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ID = 10
	modelRepo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	aliasRepo := &mediaModelAliasRepoStub{items: []MediaModelAlias{{RequestedModelID: "image-alias", ModelDefinitionID: definition.ID}}}
	registry := newReadyMediaModelRegistry(t, modelRepo, aliasRepo, NativeAsyncOptional, definition)
	require.NoError(t, registry.Refresh(context.Background()))

	aliasRepo.err = errors.New("database unavailable")
	require.Error(t, registry.Refresh(context.Background()))
	_, err := registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
	aliasRepo.err = nil

	for _, aliases := range [][]MediaModelAlias{
		{{RequestedModelID: "missing-target", ModelDefinitionID: 99}},
		{{RequestedModelID: "duplicate"}, {RequestedModelID: " DUPLICATE ", ModelDefinitionID: definition.ID}},
		{{RequestedModelID: "invalid alias", ModelDefinitionID: definition.ID}},
	} {
		aliasRepo.items = aliases
		require.Error(t, registry.Refresh(context.Background()))
		_, err := registry.Resolve("image-alias", MediaOperationTextToImage)
		require.NoError(t, err)
	}

	modelRepo.items = []MediaModelDefinition{func() MediaModelDefinition {
		disabled := definition
		disabled.Enabled = false
		return disabled
	}()}
	aliasRepo.items = []MediaModelAlias{{RequestedModelID: "image-alias", ModelDefinitionID: definition.ID}}
	require.Error(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve("image-alias", MediaOperationTextToImage)
	require.NoError(t, err)
}

func TestMediaModelRegistryAliasCanResolveInvalidEmptyCanonicalTombstone(t *testing.T) {
	invalid := validImageModelDefinition()
	invalid.ID = 20
	invalid.ModelID = "   "
	repo := &mediaModelRepositoryWithAliasesStub{
		mediaModelRepoStub: mediaModelRepoStub{items: []MediaModelDefinition{invalid}},
		mediaModelAliasRepoStub: mediaModelAliasRepoStub{items: []MediaModelAlias{{
			RequestedModelID: "invalid-empty-alias", ModelDefinitionID: invalid.ID,
		}}},
	}
	registry := NewMediaModelRegistry(repo)
	require.NoError(t, registry.Refresh(context.Background()))

	for _, modelID := range []string{"", "invalid-empty-alias"} {
		_, err := registry.Resolve(modelID, MediaOperationTextToImage)
		require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
		appErr := infraerrors.FromError(err)
		require.Equal(t, "", appErr.Metadata["model_id"])
		require.Equal(t, string(MediaAdapterResolutionInvalidDefinition), appErr.Metadata["resolution_status"])
	}
}

func TestMediaModelRegistryAliasConflictWithTombstoneCanonicalPreservesSnapshot(t *testing.T) {
	ready := validImageModelDefinition()
	ready.ID = 10
	modelRepo := &mediaModelRepoStub{items: []MediaModelDefinition{ready}}
	aliasRepo := &mediaModelAliasRepoStub{}
	registry := newReadyMediaModelRegistry(t, modelRepo, aliasRepo, NativeAsyncOptional, ready)
	require.NoError(t, registry.Refresh(context.Background()))

	tombstone := validImageModelDefinition()
	tombstone.ID = 11
	tombstone.ModelID = "tombstone-image"
	tombstone.Vendor = "invalid.vendor"
	modelRepo.items = []MediaModelDefinition{tombstone}
	aliasRepo.items = []MediaModelAlias{{RequestedModelID: "tombstone-image", ModelDefinitionID: tombstone.ID}}
	require.ErrorContains(t, registry.Refresh(context.Background()), "conflicts with a canonical model id")

	_, err := registry.Resolve(ready.ModelID, MediaOperationTextToImage)
	require.NoError(t, err)
	_, err = registry.Resolve(tombstone.ModelID, MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
}

func TestMediaModelRegistryRoutingIgnoresLegacyAdapterColumns(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Operations = []MediaOperation{MediaOperationTextToImage}
	definition.DefaultAdapter = " "
	definition.DefaultAsyncMode = NativeAsyncMode("database-garbage")
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncRequired, definition)

	require.NoError(t, registry.Refresh(context.Background()))
	resolved, err := registry.Resolve(definition.ModelID, MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "test-model-adapter", resolved.DefaultAdapter)
	require.Equal(t, NativeAsyncRequired, resolved.DefaultAsyncMode)
}

func TestMediaModelRegistryPublishesInvalidDefinitionTombstones(t *testing.T) {
	tests := []struct {
		name       string
		definition MediaModelDefinition
	}{
		{name: "model id contains spaces", definition: func() MediaModelDefinition {
			d := validImageModelDefinition()
			d.ModelID = "bad model"
			return d
		}()},
		{name: "vendor contains dot", definition: func() MediaModelDefinition {
			d := validImageModelDefinition()
			d.Vendor = "bad.vendor"
			return d
		}()},
		{name: "billing unit empty", definition: func() MediaModelDefinition {
			d := validImageModelDefinition()
			d.BillingUnit = ""
			return d
		}()},
		{name: "billing unit contains spaces", definition: func() MediaModelDefinition {
			d := validImageModelDefinition()
			d.BillingUnit = "per image"
			return d
		}()},
		{name: "billing unit unsupported", definition: func() MediaModelDefinition {
			d := validImageModelDefinition()
			d.BillingUnit = "request"
			return d
		}()},
		{name: "billing unit mismatches media type", definition: func() MediaModelDefinition {
			d := validImageModelDefinition()
			d.BillingUnit = "second"
			return d
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{tt.definition}})
			require.NoError(t, registry.Refresh(context.Background()))
			_, err := registry.Resolve(tt.definition.ModelID, MediaOperationTextToImage)
			require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
			require.Equal(t, string(MediaAdapterResolutionInvalidDefinition), infraerrors.FromError(err).Metadata["resolution_status"])
		})
	}
}

func TestValidateMediaModelDefinitionBaseRequiresVideoSecondBilling(t *testing.T) {
	definition := validVideoModelDefinition()
	require.NoError(t, validateMediaModelDefinitionBase(definition))

	definition.BillingUnit = MediaBillingUnitImage
	require.EqualError(t, validateMediaModelDefinitionBase(definition), `video model billing unit must be "second"`)
}

func TestMediaModelRegistryResolverDefinitionDoesNotRequireEnabled(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Enabled = false
	_, resolver := newTestMediaAdapterResolver(t,
		testMediaAdapterRegistrationForDefinitions("disabled-diagnostic-adapter", NativeAsyncOptional, definition),
	)

	resolution := resolver.ResolveDefinition(definition)
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.Equal(t, "disabled-diagnostic-adapter", resolution.ResolvedAdapter)
}

func TestMediaModelRegistryResolveRouteRequestChecksCapability(t *testing.T) {
	definition := validImageModelDefinition()
	registry := newReadyMediaModelRegistry(
		t, &mediaModelRepoStub{items: []MediaModelDefinition{definition}}, nil, NativeAsyncOptional, definition,
	)
	require.NoError(t, registry.Refresh(context.Background()))

	_, err := registry.ResolveRouteRequest(MediaRouteRequest{
		RequestedModel: "fake-image",
		Operation:      MediaOperationTextToImage,
		Capability:     MediaTypeVideo,
	})
	require.ErrorIs(t, err, ErrMediaCapabilityUnsupported)
}

func TestMediaModelRegistryValidateOperation(t *testing.T) {
	definition := MediaModelDefinition{
		ModelID:          "fake-image",
		Vendor:           "fake-vendor",
		MediaType:        MediaTypeImage,
		Operations:       []MediaOperation{MediaOperationTextToImage},
		Constraints:      json.RawMessage(`{"image_sizes":["1024x1024"],"max_image_count":2}`),
		BillingUnit:      "image",
		DefaultAdapter:   "fake-adapter",
		DefaultAsyncMode: NativeAsyncOptional,
		Enabled:          true,
	}
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, definition)
	require.NoError(t, registry.Refresh(context.Background()))

	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	_, err = registry.Resolve("fake-image", MediaOperationTextToVideo)
	require.ErrorIs(t, err, ErrMediaOperationUnsupported)
	_, err = registry.Resolve("missing", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
	err = registry.ValidateSpec("fake-image", MediaOperationTextToImage, MediaSpec{Image: &ImageSpec{
		Prompt: "cat", Size: "2048x2048", Count: 1,
	}})
	require.ErrorIs(t, err, ErrMediaSpecOutsideModelConstraints)
}

func TestMediaModelDefinitionSupportsExactOperation(t *testing.T) {
	definition := validImageModelDefinition()
	require.True(t, definition.Supports(MediaOperationTextToImage))
	require.False(t, definition.Supports(MediaOperationTextToVideo))
}

func TestMediaModelRegistryRefreshNormalizesAndReplacesSnapshot(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ModelID = "  Fake-Image  "
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, definition)

	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
	require.NoError(t, registry.Refresh(context.Background()))
	resolved, err := registry.Resolve("  FAKE-IMAGE ", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "fake-image", resolved.ModelID)

	repo.items = nil
	require.NoError(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve("fake-image", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
}

func TestMediaModelRegistryRefreshPreservesSnapshotOnFailure(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ID = 7
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, definition)
	require.NoError(t, registry.Refresh(context.Background()))

	repo.err = errors.New("database unavailable")
	require.Error(t, registry.Refresh(context.Background()))
	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)

	repo.err = nil
	duplicate := definition
	duplicate.ModelID = " FAKE-IMAGE "
	repo.items = []MediaModelDefinition{definition, duplicate}
	require.ErrorContains(t, registry.Refresh(context.Background()), "duplicate media model id")
	_, err = registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)

	duplicate.ModelID = "other-image"
	repo.items = []MediaModelDefinition{definition, duplicate}
	require.ErrorContains(t, registry.Refresh(context.Background()), "duplicate media model definition id")
	_, err = registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
}

func TestMediaModelRegistryRefreshStrictlyDecodesConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints json.RawMessage
		wantError   bool
	}{
		{name: "unknown field typo", constraints: json.RawMessage(`{"max_image_counts":2}`), wantError: true},
		{name: "second top-level value", constraints: json.RawMessage(`{} {}`), wantError: true},
		{name: "trailing non-whitespace garbage", constraints: json.RawMessage(`{} trailing`), wantError: true},
		{name: "missing constraints", constraints: nil},
		{name: "empty object", constraints: json.RawMessage(`{}`)},
		{name: "empty object with trailing whitespace", constraints: json.RawMessage("{}  \n\t ")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := validImageModelDefinition()
			definition.Constraints = tt.constraints
			repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
			if tt.wantError {
				registry := NewMediaModelRegistry(repo)
				require.NoError(t, registry.Refresh(context.Background()))
				_, err := registry.Resolve(definition.ModelID, MediaOperationTextToImage)
				require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
				require.Equal(t, string(MediaAdapterResolutionInvalidDefinition), infraerrors.FromError(err).Metadata["resolution_status"])
				return
			}
			registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, definition)

			require.NoError(t, registry.Refresh(context.Background()))
		})
	}
}

func TestMediaModelRegistryRefreshRedactsInvalidConstraintValues(t *testing.T) {
	tests := []struct {
		name        string
		constraints json.RawMessage
		secretValue string
	}{
		{
			name:        "integer overflow",
			constraints: json.RawMessage(`{"max_image_count":987654321098765432109876543210987654321}`),
			secretValue: "987654321098765432109876543210987654321",
		},
		{
			name:        "fractional number for integer",
			constraints: json.RawMessage(`{"max_image_count":731.125}`),
			secretValue: "731.125",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid := validImageModelDefinition()
			invalid.ModelID = "invalid-image"
			invalid.Constraints = tt.constraints
			err := validateMediaModelDefinitionBase(invalid)
			require.Error(t, err)
			require.Contains(t, err.Error(), "max_image_count")
			require.NotContains(t, err.Error(), tt.secretValue)

			registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{invalid}})
			require.NoError(t, registry.Refresh(context.Background()))
			_, err = registry.Resolve("invalid-image", MediaOperationTextToImage)
			require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
		})
	}
}

func TestMediaModelRegistryRefreshUnknownConstraintPublishesRedactedTombstone(t *testing.T) {
	invalid := validImageModelDefinition()
	invalid.ModelID = "invalid-image"
	invalid.Constraints = json.RawMessage(`{"max_refererence_images":"do-not-leak-this-value"}`)
	err := validateMediaModelDefinitionBase(invalid)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode media model constraints")
	require.NotContains(t, err.Error(), "do-not-leak-this-value")

	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{invalid}})
	var logs bytes.Buffer
	registry.SetLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, registry.Refresh(context.Background()))
	require.NotContains(t, logs.String(), "do-not-leak-this-value")
	_, err = registry.Resolve("invalid-image", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
}

func TestMediaModelRegistryRefreshPublishesInvalidDefinitionTombstones(t *testing.T) {
	tests := []struct {
		name       string
		definition MediaModelDefinition
	}{
		{name: "empty model id", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.ModelID = "  "
			return definition
		}()},
		{name: "unknown media type", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.MediaType = MediaType("audio")
			return definition
		}()},
		{name: "no operations", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Operations = nil
			return definition
		}()},
		{name: "unknown operation", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Operations = []MediaOperation{"unknown"}
			return definition
		}()},
		{name: "operation mismatches media type", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Operations = []MediaOperation{MediaOperationTextToVideo}
			return definition
		}()},
		{name: "malformed constraints", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Constraints = json.RawMessage(`{`)
			return definition
		}()},
		{name: "negative constraint", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Constraints = json.RawMessage(`{"max_image_count":-1}`)
			return definition
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{tt.definition}})
			require.NoError(t, registry.Refresh(context.Background()))
			_, err := registry.Resolve(tt.definition.ModelID, MediaOperationTextToImage)
			require.ErrorIs(t, err, ErrMediaModelAdapterUnavailable)
			require.Equal(t, string(MediaAdapterResolutionInvalidDefinition), infraerrors.FromError(err).Metadata["resolution_status"])
		})
	}

	first := validImageModelDefinition()
	first.ModelID = "fake-image"
	second := validImageModelDefinition()
	second.ModelID = "  FAKE-IMAGE "
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{first, second}})
	require.Error(t, registry.Refresh(context.Background()))
}

func TestMediaModelRegistryRejectsDisabledDefinitionFromEnabledRepositoryWithoutPublishing(t *testing.T) {
	ready := validImageModelDefinition()
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{ready}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, ready)
	require.NoError(t, registry.Refresh(context.Background()))

	disabled := ready
	disabled.ModelID = "disabled-image"
	disabled.Enabled = false
	repo.items = []MediaModelDefinition{disabled}
	err := registry.Refresh(context.Background())
	require.ErrorContains(t, err, "disabled model returned by enabled model repository")
	_, err = registry.Resolve(ready.ModelID, MediaOperationTextToImage)
	require.NoError(t, err)
	_, err = registry.Resolve(disabled.ModelID, MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
}

func TestMediaModelRegistryResolveReturnsDetachedDefinition(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Constraints = json.RawMessage(`{"image_sizes":["1024x1024"]}`)
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := newReadyMediaModelRegistry(t, repo, nil, NativeAsyncOptional, definition)
	require.NoError(t, registry.Refresh(context.Background()))

	repo.items[0].Operations[0] = MediaOperationTextToVideo
	repo.items[0].Constraints[0] = '['
	resolved, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	originalResolutionOperation := resolved.AdapterResolution.Capabilities.Operations[0]
	resolved.Operations[0] = MediaOperationTextToVideo
	resolved.Constraints[0] = '['
	resolved.AdapterResolution.Capabilities.Operations[0] = MediaOperationVideoRemix

	resolvedAgain, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, MediaOperationTextToImage, resolvedAgain.Operations[0])
	require.JSONEq(t, `{"image_sizes":["1024x1024"]}`, string(resolvedAgain.Constraints))
	require.Equal(t, originalResolutionOperation, resolvedAgain.AdapterResolution.Capabilities.Operations[0])
}

func TestMediaModelRegistryValidateImageConstraints(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Constraints = json.RawMessage(`{
		"image_sizes":["1024x1024"],
		"max_image_count":2,
		"max_reference_images":1
	}`)
	registry := newReadyMediaModelRegistry(
		t, &mediaModelRepoStub{items: []MediaModelDefinition{definition}}, nil, NativeAsyncOptional, definition,
	)
	require.NoError(t, registry.Refresh(context.Background()))

	tests := []struct {
		name string
		spec MediaSpec
		err  error
	}{
		{name: "allowed", spec: MediaSpec{Image: &ImageSpec{Size: "1024x1024", Count: 2, InputArtifactIDs: []int64{1}}}},
		{name: "unspecified optional size", spec: MediaSpec{Image: &ImageSpec{Count: 1}}},
		{name: "size", spec: MediaSpec{Image: &ImageSpec{Size: "2048x2048", Count: 1}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "count", spec: MediaSpec{Image: &ImageSpec{Size: "1024x1024", Count: 3}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "references", spec: MediaSpec{Image: &ImageSpec{Size: "1024x1024", Count: 1, InputArtifactIDs: []int64{1, 2}}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "wrong spec kind", spec: MediaSpec{Video: &VideoSpec{}}, err: ErrMediaSpecOutsideModelConstraints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSpec("fake-image", MediaOperationTextToImage, tt.spec)
			if tt.err == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestValidateMediaSpecAgainstFrozenDefinition(t *testing.T) {
	frozen := validImageModelDefinition()
	frozen.Constraints = json.RawMessage(`{"image_sizes":["1024x1024"]}`)
	refreshed := cloneMediaModelDefinition(frozen)
	refreshed.Constraints = json.RawMessage(`{"image_sizes":["512x512"]}`)
	spec := MediaSpec{Image: &ImageSpec{Prompt: "cat", Count: 1, Size: "1024x1024"}}

	require.NoError(t, validateMediaSpecAgainstDefinition(frozen, spec))
	require.ErrorIs(t, validateMediaSpecAgainstDefinition(refreshed, spec), ErrMediaSpecOutsideModelConstraints)
}

func TestMediaModelRegistryValidateVideoConstraints(t *testing.T) {
	definition := validVideoModelDefinition()
	definition.Constraints = json.RawMessage(`{
		"video_durations":[5,10],
		"video_resolutions":["720p"],
		"min_fps":24,
		"max_fps":30,
		"max_reference_images":2
	}`)
	registry := newReadyMediaModelRegistry(
		t, &mediaModelRepoStub{items: []MediaModelDefinition{definition}}, nil, NativeAsyncOptional, definition,
	)
	require.NoError(t, registry.Refresh(context.Background()))

	tests := []struct {
		name string
		spec MediaSpec
		err  error
	}{
		{name: "allowed", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "720p", FPS: 24, ReferenceArtifactIDs: []int64{1, 2}}}},
		{name: "unspecified optional fields", spec: MediaSpec{Video: &VideoSpec{}}},
		{name: "duration", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 6, Resolution: "720p", FPS: 24}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "resolution", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "1080p", FPS: 24}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "fps below minimum", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "720p", FPS: 23}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "fps above maximum", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "720p", FPS: 31}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "references", spec: MediaSpec{Video: &VideoSpec{ReferenceArtifactIDs: []int64{1, 2, 3}}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "wrong spec kind", spec: MediaSpec{Image: &ImageSpec{}}, err: ErrMediaSpecOutsideModelConstraints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSpec("fake-video", MediaOperationTextToVideo, tt.spec)
			if tt.err == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestMediaModelRegistryConstraintFieldsAreOptional(t *testing.T) {
	image := validImageModelDefinition()
	video := validVideoModelDefinition()
	registry := newReadyMediaModelRegistry(
		t, &mediaModelRepoStub{items: []MediaModelDefinition{image, video}}, nil, NativeAsyncOptional, image, video,
	)
	require.NoError(t, registry.Refresh(context.Background()))

	require.NoError(t, registry.ValidateSpec("fake-image", MediaOperationTextToImage, MediaSpec{Image: &ImageSpec{
		Size: "unrestricted", Count: 100, InputArtifactIDs: []int64{1, 2, 3},
	}}))
	require.NoError(t, registry.ValidateSpec("fake-video", MediaOperationTextToVideo, MediaSpec{Video: &VideoSpec{
		DurationSeconds: 999, Resolution: "unrestricted", FPS: 999, ReferenceArtifactIDs: []int64{1, 2, 3},
	}}))
}
