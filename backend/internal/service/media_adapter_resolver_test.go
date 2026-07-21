package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mediaSubmitOnlyAdapter struct {
	name string
}

func (a *mediaSubmitOnlyAdapter) Name() string { return a.name }

func (*mediaSubmitOnlyAdapter) Submit(context.Context, MediaExecutionRequest) (*MediaAsyncSubmission, error) {
	return &MediaAsyncSubmission{}, nil
}

type mediaContentFetchSyncAdapter struct {
	name string
}

func (a *mediaContentFetchSyncAdapter) Name() string { return a.name }

func (*mediaContentFetchSyncAdapter) Generate(context.Context, MediaExecutionRequest) (*MediaGenerateResult, error) {
	return &MediaGenerateResult{}, nil
}

func (*mediaContentFetchSyncAdapter) OpenContent(context.Context, *Account, *MediaArtifact, string) (*MediaContent, error) {
	return &MediaContent{}, nil
}

type mediaReentrantNameAdapter struct {
	registry *MediaAdapterRegistry
	enabled  atomic.Bool
}

func (a *mediaReentrantNameAdapter) Name() string {
	if a.enabled.Load() {
		a.registry.SetRoutingMetrics(nil)
	}
	return "reentrant-image"
}

func (*mediaReentrantNameAdapter) Generate(context.Context, MediaExecutionRequest) (*MediaGenerateResult, error) {
	return &MediaGenerateResult{}, nil
}

func TestMediaAdapterResolverUsesExactBeforeFamily(t *testing.T) {
	t.Parallel()
	registry, resolver := newTestMediaAdapterResolver(t,
		MediaAdapterRegistration{
			Key: "family-image",
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
				Name: "family-image", NativeAsyncMode: NativeAsyncUnsupported,
			}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			FamilyRules: []MediaAdapterFamilyRule{{
				Vendor: "xai", FamilyID: "grok-image",
				Match: func(string) bool { panic("family matcher must not run after an exact match") },
				Capabilities: MediaAdapterRuleCapabilities{
					Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
				},
			}},
		},
		MediaAdapterRegistration{
			Key: "exact-image",
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
				Name: "exact-image", NativeAsyncMode: NativeAsyncRequired,
			}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			ExactRules: []MediaAdapterExactRule{{
				Vendor: "xai", ModelID: "grok-2-image",
				Capabilities: MediaAdapterRuleCapabilities{
					Operations: []MediaOperation{MediaOperationTextToImage}, NativeAsyncUpstream: true,
				},
			}},
		},
	)
	t.Cleanup(func() { require.NotNil(t, registry) })

	resolution := resolver.Resolve(" XAI ", " Grok-2-Image ", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.Equal(t, "exact-image", resolution.ResolvedAdapter)
	require.Equal(t, MediaAdapterMatchedExact, resolution.MatchedBy)
	require.Empty(t, resolution.MatchedFamily)
	require.NotNil(t, resolution.Capabilities)
	require.True(t, resolution.Capabilities.NativeAsyncUpstream)
	require.False(t, resolution.Capabilities.SyncUpstream)
	require.Empty(t, resolution.ReasonCode)
}

func TestMediaAdapterResolverUsesUniqueFamilyMatch(t *testing.T) {
	t.Parallel()
	_, resolver := newTestMediaAdapterResolver(t, MediaAdapterRegistration{
		Key: "family-image",
		Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
			Name: "family-image", NativeAsyncMode: NativeAsyncOptional,
		}),
		SupportedOperations: []MediaOperation{MediaOperationTextToImage},
		FamilyRules: []MediaAdapterFamilyRule{{
			Vendor: "google", FamilyID: "nano-banana",
			Match: func(modelID string) bool { return strings.HasPrefix(modelID, "nano-banana-") },
			Capabilities: MediaAdapterRuleCapabilities{
				Operations:   []MediaOperation{MediaOperationTextToImage},
				SyncUpstream: true, NativeAsyncUpstream: true,
			},
		}},
	})

	resolution := resolver.Resolve(" GOOGLE ", " Nano-Banana-Pro ", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.Equal(t, "family-image", resolution.ResolvedAdapter)
	require.Equal(t, MediaAdapterMatchedFamily, resolution.MatchedBy)
	require.Equal(t, "nano-banana", resolution.MatchedFamily)
	require.Equal(t, NativeAsyncOptional, resolution.CompatibilityAsyncMode())
}

func TestMediaAdapterResolverReportsUnresolvedAndAmbiguous(t *testing.T) {
	t.Parallel()
	_, resolver := newTestMediaAdapterResolver(t,
		MediaAdapterRegistration{
			Key: "family-a",
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
				Name: "family-a", NativeAsyncMode: NativeAsyncUnsupported,
			}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			FamilyRules: []MediaAdapterFamilyRule{{
				Vendor: "xai", FamilyID: "grok-a",
				Match: func(modelID string) bool { return strings.HasPrefix(modelID, "grok-") },
				Capabilities: MediaAdapterRuleCapabilities{
					Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
				},
			}},
		},
		MediaAdapterRegistration{
			Key: "family-b",
			Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
				Name: "family-b", NativeAsyncMode: NativeAsyncUnsupported,
			}),
			SupportedOperations: []MediaOperation{MediaOperationTextToImage},
			FamilyRules: []MediaAdapterFamilyRule{{
				Vendor: "xai", FamilyID: "grok-b",
				Match: func(modelID string) bool { return strings.HasSuffix(modelID, "-image") },
				Capabilities: MediaAdapterRuleCapabilities{
					Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
				},
			}},
		},
	)

	unresolved := resolver.Resolve("unknown", "unknown-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionUnresolved, unresolved.Status)
	require.Equal(t, "MEDIA_ADAPTER_UNRESOLVED", unresolved.ReasonCode)
	require.Empty(t, unresolved.ResolvedAdapter)
	require.Empty(t, unresolved.MatchedBy)
	require.Nil(t, unresolved.Capabilities)

	ambiguous := resolver.Resolve("xai", "grok-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionAmbiguous, ambiguous.Status)
	require.Equal(t, "MEDIA_ADAPTER_AMBIGUOUS", ambiguous.ReasonCode)
	require.Empty(t, ambiguous.ResolvedAdapter)
	require.Empty(t, ambiguous.MatchedBy)
	require.Nil(t, ambiguous.Capabilities)
}

func TestMediaAdapterResolverDoesNotUseAccountProviderOrUpstreamModel(t *testing.T) {
	t.Parallel()
	_, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("xai-image", "xai", "grok-2-image"))

	for _, irrelevant := range []struct {
		accountProvider string
		upstreamModel   string
	}{
		{accountProvider: "openai", upstreamModel: "provider/model-a"},
		{accountProvider: "custom", upstreamModel: "renamed-model-b"},
	} {
		require.NotEmpty(t, irrelevant.accountProvider)
		require.NotEmpty(t, irrelevant.upstreamModel)
		resolution := resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage})
		require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
		require.Equal(t, "xai-image", resolution.ResolvedAdapter)
	}
}

func TestMediaAdapterRegistryRejectsDuplicateExactKey(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.RegisterDefinition(exactImageRegistration("first-image", "xai", "grok-2-image")))
	before := registry.Registrations()

	err := registry.RegisterDefinition(exactImageRegistration("second-image", " XAI ", " Grok-2-Image "))
	require.ErrorContains(t, err, "duplicate media adapter exact rule")
	require.Equal(t, len(before), len(registry.Registrations()))
	resolved, resolveErr := registry.Resolve("first-image")
	require.NoError(t, resolveErr)
	require.Equal(t, "first-image", resolved.Name())
	_, resolveErr = registry.Resolve("second-image")
	require.ErrorIs(t, resolveErr, ErrMediaAdapterNotFound)
}

func TestMediaAdapterRegistryRejectsCapabilitiesBeyondImplementation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		registration MediaAdapterRegistration
		errorText    string
	}{
		{
			name: "sync requires MediaSyncGenerator",
			registration: registrationWithCapabilities(
				"async-only", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "async-only", NativeAsyncMode: NativeAsyncRequired}),
				MediaAdapterRuleCapabilities{Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true},
			),
			errorText: "MediaSyncGenerator",
		},
		{
			name: "native async requires submit and poll",
			registration: registrationWithCapabilities(
				"sync-only", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync-only", NativeAsyncMode: NativeAsyncUnsupported}),
				MediaAdapterRuleCapabilities{Operations: []MediaOperation{MediaOperationTextToImage}, NativeAsyncUpstream: true},
			),
			errorText: "MediaAsyncSubmitter and MediaAsyncPoller",
		},
		{
			name: "submit without poll is insufficient",
			registration: registrationWithCapabilities(
				"submit-only", &mediaSubmitOnlyAdapter{name: "submit-only"},
				MediaAdapterRuleCapabilities{Operations: []MediaOperation{MediaOperationTextToImage}, NativeAsyncUpstream: true},
			),
			errorText: "MediaAsyncSubmitter and MediaAsyncPoller",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registry := NewMediaAdapterRegistry()
			err := registry.RegisterDefinition(tt.registration)
			require.ErrorContains(t, err, tt.errorText)
			require.Empty(t, registry.Registrations())
			_, resolveErr := registry.Resolve(tt.registration.Key)
			require.ErrorIs(t, resolveErr, ErrMediaAdapterNotFound)
		})
	}
}

func TestMediaAdapterRegistryRejectsDefinitionWithoutExecutionPath(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	registration := registrationWithCapabilities(
		"optional", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "optional", NativeAsyncMode: NativeAsyncOptional}),
		MediaAdapterRuleCapabilities{Operations: []MediaOperation{MediaOperationTextToImage}},
	)

	err := registry.RegisterDefinition(registration)
	require.ErrorContains(t, err, "execution path")
	require.Empty(t, registry.Registrations())
	_, resolveErr := registry.Resolve("optional")
	require.ErrorIs(t, resolveErr, ErrMediaAdapterNotFound)
}

func TestMediaAdapterRegistrationOperationsAreSourceOfTruth(t *testing.T) {
	t.Parallel()
	registration := MediaAdapterRegistration{
		Key: "image",
		Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
			Name: "image", NativeAsyncMode: NativeAsyncUnsupported,
		}),
		SupportedOperations: []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit},
		ExactRules: []MediaAdapterExactRule{{
			Vendor: "vendor", ModelID: "image-model",
			Capabilities: MediaAdapterRuleCapabilities{
				Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
			},
		}},
	}
	_, resolver := newTestMediaAdapterResolver(t, registration)

	ready := resolver.Resolve("vendor", "image-model", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, ready.Status)
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, ready.Capabilities.Operations)

	mismatch := resolver.Resolve("vendor", "image-model", []MediaOperation{MediaOperationImageEdit})
	require.Equal(t, MediaAdapterResolutionCapabilityMismatch, mismatch.Status)
	require.Equal(t, "image", mismatch.ResolvedAdapter)
	require.Equal(t, MediaAdapterMatchedExact, mismatch.MatchedBy)
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, mismatch.Capabilities.Operations)

	invalid := registration
	invalid.Key = "invalid-image"
	invalid.Adapter = NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "invalid-image", NativeAsyncMode: NativeAsyncUnsupported})
	invalid.ExactRules[0].ModelID = "invalid-image-model"
	invalid.ExactRules[0].Capabilities.Operations = []MediaOperation{MediaOperationImageToImage}
	registry := NewMediaAdapterRegistry()
	err := registry.RegisterDefinition(invalid)
	require.ErrorContains(t, err, "not declared in supported operations")
}

func TestMediaAdapterRegistryRejectsNameKeyMismatchWithoutMutation(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "implementation-name", NativeAsyncMode: NativeAsyncUnsupported})

	err := registry.Register("configured-name", adapter)
	require.ErrorContains(t, err, "does not match implementation name")
	require.Empty(t, registry.Registrations())
	_, resolveErr := registry.Resolve("configured-name")
	require.ErrorIs(t, resolveErr, ErrMediaAdapterNotFound)
	_, resolveErr = registry.Resolve("implementation-name")
	require.ErrorIs(t, resolveErr, ErrMediaAdapterNotFound)
}

func TestMediaAdapterRegistryRejectsAliasOverwriteDuplicateChainAndCycle(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.Register("canonical-image", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		Name: "canonical-image", NativeAsyncMode: NativeAsyncUnsupported,
	})))
	require.NoError(t, registry.Register("other-image", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		Name: "other-image", NativeAsyncMode: NativeAsyncUnsupported,
	})))

	require.Error(t, registry.RegisterAlias("canonical-image", "other-image"))
	require.Error(t, registry.RegisterAlias("missing-target", "missing-image"))
	require.NoError(t, registry.RegisterAlias("legacy-image", "canonical-image"))
	require.Error(t, registry.RegisterAlias("legacy-image", "canonical-image"))
	require.Error(t, registry.RegisterAlias("alias-chain", "legacy-image"))
	require.Error(t, registry.RegisterAlias("canonical-image", "legacy-image"))
	require.Error(t, registry.RegisterAlias("same", "same"))
	require.Error(t, registry.Register("legacy-image", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		Name: "legacy-image", NativeAsyncMode: NativeAsyncUnsupported,
	})))
	legacyDefinition := exactImageRegistration("legacy-image", "vendor", "legacy-model")
	require.Error(t, registry.RegisterDefinition(legacyDefinition))

	canonical, aliased := registry.CanonicalKey("legacy-image")
	require.Equal(t, "canonical-image", canonical)
	require.True(t, aliased)
}

func TestMediaAdapterRegistryAndResolverReturnDeepCopies(t *testing.T) {
	t.Parallel()
	registration := exactImageRegistration("canonical-image", "xai", "grok-2-image")
	registration.FamilyRules = []MediaAdapterFamilyRule{{
		Vendor: "xai", FamilyID: "grok-family",
		Match: func(modelID string) bool { return strings.HasPrefix(modelID, "grok-") },
		Capabilities: MediaAdapterRuleCapabilities{
			Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
		},
	}}
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.RegisterDefinition(registration))
	registration.SupportedOperations[0] = MediaOperationVideoRemix
	registration.ExactRules[0].Vendor = "caller-changed"
	registration.ExactRules[0].Capabilities.Operations[0] = MediaOperationVideoRemix
	registration.FamilyRules[0].Vendor = "caller-changed"
	registration.FamilyRules[0].Capabilities.Operations[0] = MediaOperationVideoRemix
	resolver := NewMediaAdapterResolver(registry)

	registrations := registry.Registrations()
	require.Len(t, registrations, 1)
	registrations[0].SupportedOperations[0] = MediaOperationVideoRemix
	registrations[0].ExactRules[0].Vendor = "changed"
	registrations[0].ExactRules[0].Capabilities.Operations[0] = MediaOperationVideoRemix
	registrations[0].FamilyRules[0].Vendor = "changed"
	registrations[0].FamilyRules[0].Capabilities.Operations[0] = MediaOperationVideoRemix

	unchanged := registry.Registrations()
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, unchanged[0].SupportedOperations)
	require.Equal(t, "xai", unchanged[0].ExactRules[0].Vendor)
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, unchanged[0].ExactRules[0].Capabilities.Operations)
	require.Equal(t, "xai", unchanged[0].FamilyRules[0].Vendor)
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, unchanged[0].FamilyRules[0].Capabilities.Operations)

	resolution := resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	resolution.Capabilities.Operations[0] = MediaOperationVideoRemix
	resolution = resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, resolution.Capabilities.Operations)
}

func TestMediaAdapterRegistryValidateDoesNotCallAdapterUnderLock(t *testing.T) {
	registry := NewMediaAdapterRegistry()
	adapter := &mediaReentrantNameAdapter{registry: registry}
	require.NoError(t, registry.RegisterDefinition(MediaAdapterRegistration{
		Key:                 "reentrant-image",
		Adapter:             adapter,
		SupportedOperations: []MediaOperation{MediaOperationTextToImage},
		ExactRules: []MediaAdapterExactRule{{
			Vendor: "vendor", ModelID: "reentrant-image",
			Capabilities: MediaAdapterRuleCapabilities{
				Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
			},
		}},
	}))
	adapter.enabled.Store(true)

	done := make(chan error, 1)
	go func() {
		done <- registry.Validate()
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("MediaAdapterRegistry.Validate called adapter.Name while holding the registry lock")
	}
}

func TestMediaAdapterRegistryValidateUsesLiveAdapterCapabilities(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.RegisterDefinition(registrationWithCapabilities(
		"native-image",
		NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "native-image", NativeAsyncMode: NativeAsyncRequired}),
		MediaAdapterRuleCapabilities{
			Operations: []MediaOperation{MediaOperationTextToImage}, NativeAsyncUpstream: true,
		},
	)))
	registry.mu.Lock()
	registry.adapters["native-image"] = NewFakeMediaAdapter(FakeMediaAdapterOptions{
		Name: "native-image", NativeAsyncMode: NativeAsyncUnsupported,
	})
	registry.mu.Unlock()

	err := registry.Validate()
	require.ErrorContains(t, err, "MediaAsyncSubmitter and MediaAsyncPoller")
}

func TestMediaAdapterRegistryValidateRejectsDuplicateRegistrationKey(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.RegisterDefinition(exactImageRegistration("image", "vendor", "image-model")))
	registry.mu.Lock()
	duplicate := cloneMediaAdapterRegistration(registry.registrations[0])
	duplicate.ExactRules[0].ModelID = "other-image-model"
	registry.registrations = append(registry.registrations, duplicate)
	registry.mu.Unlock()

	err := registry.Validate()
	require.ErrorContains(t, err, "duplicate media adapter registration key")
}

func TestMediaAdapterRegistryValidateRejectsNonCanonicalStoredRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*MediaAdapterRegistration)
	}{
		{name: "exact vendor", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].Vendor = " VENDOR " }},
		{name: "exact model", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].ModelID = " IMAGE-MODEL " }},
		{name: "family vendor", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].Vendor = " VENDOR " }},
		{name: "family id", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].FamilyID = " IMAGE-FAMILY " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registration := exactImageRegistration("image", "vendor", "image-model")
			registration.FamilyRules = []MediaAdapterFamilyRule{{
				Vendor: "vendor", FamilyID: "image-family",
				Match: func(modelID string) bool { return strings.HasPrefix(modelID, "image-") },
				Capabilities: MediaAdapterRuleCapabilities{
					Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
				},
			}}
			registry := NewMediaAdapterRegistry()
			require.NoError(t, registry.RegisterDefinition(registration))
			registry.mu.Lock()
			tt.mutate(&registry.registrations[0])
			registry.mu.Unlock()

			err := registry.Validate()
			require.ErrorContains(t, err, "is not canonical")
		})
	}
}

func TestMediaAdapterResolverUsesImmutableRuleSnapshot(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	oldResolver := NewMediaAdapterResolver(registry)
	require.Equal(t, MediaAdapterResolutionUnresolved, oldResolver.Resolve(
		"xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage},
	).Status)

	require.NoError(t, registry.RegisterDefinition(exactImageRegistration("xai-image", "xai", "grok-2-image")))
	require.Equal(t, MediaAdapterResolutionUnresolved, oldResolver.Resolve(
		"xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage},
	).Status)
	newResolver := NewMediaAdapterResolver(registry)
	require.Equal(t, MediaAdapterResolutionReady, newResolver.Resolve(
		"xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage},
	).Status)
}

func TestMediaAdapterRegistryRejectsInvalidRegistrationShapeWithoutMutation(t *testing.T) {
	t.Parallel()
	base := exactImageRegistration("image", "vendor", "image-model")
	familyBase := base
	familyBase.ExactRules = nil
	familyBase.FamilyRules = []MediaAdapterFamilyRule{{
		Vendor: "vendor", FamilyID: "image-family",
		Match: func(modelID string) bool { return strings.HasPrefix(modelID, "image-") },
		Capabilities: MediaAdapterRuleCapabilities{
			Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
		},
	}}
	tests := []struct {
		name   string
		mutate func(*MediaAdapterRegistration)
	}{
		{name: "empty vendor", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].Vendor = "" }},
		{name: "invalid vendor", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].Vendor = "bad.vendor" }},
		{name: "empty model id", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].ModelID = "" }},
		{name: "invalid model id", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].ModelID = "bad model" }},
		{name: "empty supported operations", mutate: func(r *MediaAdapterRegistration) { r.SupportedOperations = nil }},
		{name: "duplicate supported operations", mutate: func(r *MediaAdapterRegistration) {
			r.SupportedOperations = []MediaOperation{MediaOperationTextToImage, MediaOperationTextToImage}
		}},
		{name: "unknown supported operation", mutate: func(r *MediaAdapterRegistration) {
			r.SupportedOperations = []MediaOperation{MediaOperation("unknown")}
		}},
		{name: "empty rule operations", mutate: func(r *MediaAdapterRegistration) { r.ExactRules[0].Capabilities.Operations = nil }},
		{name: "duplicate rule operations", mutate: func(r *MediaAdapterRegistration) {
			r.SupportedOperations = []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit}
			r.ExactRules[0].Capabilities.Operations = []MediaOperation{MediaOperationTextToImage, MediaOperationTextToImage}
		}},
		{name: "unknown rule operation", mutate: func(r *MediaAdapterRegistration) {
			r.ExactRules[0].Capabilities.Operations = []MediaOperation{MediaOperation("unknown")}
		}},
		{name: "no rules", mutate: func(r *MediaAdapterRegistration) { r.ExactRules = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registration := cloneMediaAdapterRegistration(base)
			tt.mutate(&registration)
			registry := NewMediaAdapterRegistry()
			require.Error(t, registry.RegisterDefinition(registration))
			require.Empty(t, registry.Registrations())
			_, err := registry.Resolve(registration.Key)
			require.ErrorIs(t, err, ErrMediaAdapterNotFound)
		})
	}

	familyTests := []struct {
		name   string
		mutate func(*MediaAdapterRegistration)
	}{
		{name: "empty family vendor", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].Vendor = "" }},
		{name: "invalid family vendor", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].Vendor = "bad.vendor" }},
		{name: "empty family id", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].FamilyID = "" }},
		{name: "invalid family id", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].FamilyID = "bad.family" }},
		{name: "nil family matcher", mutate: func(r *MediaAdapterRegistration) { r.FamilyRules[0].Match = nil }},
	}
	for _, tt := range familyTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registration := cloneMediaAdapterRegistration(familyBase)
			tt.mutate(&registration)
			registry := NewMediaAdapterRegistry()
			require.Error(t, registry.RegisterDefinition(registration))
			require.Empty(t, registry.Registrations())
		})
	}
}

func TestMediaAdapterAliasResolvesSameImplementationAndCapabilities(t *testing.T) {
	registry, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("canonical-image", "xai", "grok-2-image"))
	require.NoError(t, registry.RegisterAlias("legacy-image", "canonical-image"))
	metrics := NewAtomicMediaTaskMetrics()
	registry.SetRoutingMetrics(metrics)
	var logs bytes.Buffer
	registry.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))

	canonical, err := registry.Resolve("canonical-image")
	require.NoError(t, err)
	legacy, err := registry.Resolve(" LEGACY-IMAGE ")
	require.NoError(t, err)
	require.Same(t, canonical, legacy)
	resolution := resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.Equal(t, "canonical-image", resolution.ResolvedAdapter)
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, resolution.Capabilities.Operations)
	require.True(t, resolution.Capabilities.SyncUpstream)
	require.Equal(t, int64(1), metrics.HistoricalAdapterAliasResolutions())

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	require.Len(t, lines, 1)
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	require.Equal(t, "media_adapter_historical_alias_resolved", record["msg"])
	require.Equal(t, "legacy-image", record["legacy_adapter_key"])
	require.Equal(t, "canonical-image", record["adapter_key"])
	require.NotContains(t, record, "vendor")
	require.NotContains(t, record, "model_id")
}

func TestMediaAdapterRegistryCanonicalKeyPreservesHistoricalLookup(t *testing.T) {
	t.Parallel()
	registry := NewMediaAdapterRegistry()
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "canonical-image", NativeAsyncMode: NativeAsyncUnsupported})
	require.NoError(t, registry.Register("canonical-image", adapter))
	require.NoError(t, registry.RegisterAlias("legacy-image", "canonical-image"))

	canonical, aliased := registry.CanonicalKey(" LEGACY-IMAGE ")
	require.Equal(t, "canonical-image", canonical)
	require.True(t, aliased)
	canonical, aliased = registry.CanonicalKey(" Canonical-Image ")
	require.Equal(t, "canonical-image", canonical)
	require.False(t, aliased)
	canonical, aliased = registry.CanonicalKey(" Unknown-Image ")
	require.Equal(t, "unknown-image", canonical)
	require.False(t, aliased)
	require.Empty(t, registry.Registrations())
	resolved, err := registry.Resolve("legacy-image")
	require.NoError(t, err)
	require.Same(t, adapter, resolved)
	_, err = registry.Resolve("unknown-image")
	require.ErrorIs(t, err, ErrMediaAdapterNotFound)
}

func TestMediaAdapterResolverReportsImplementationMissingWithMatchedKey(t *testing.T) {
	t.Parallel()
	registry, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("xai-image", "xai", "grok-2-image"))
	registry.mu.Lock()
	delete(registry.adapters, "xai-image")
	registry.mu.Unlock()

	resolution := resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionImplementationMissing, resolution.Status)
	require.Equal(t, "MEDIA_ADAPTER_IMPLEMENTATION_MISSING", resolution.ReasonCode)
	require.Equal(t, "xai-image", resolution.ResolvedAdapter)
	require.Equal(t, MediaAdapterMatchedExact, resolution.MatchedBy)
	require.Empty(t, resolution.MatchedFamily)
	require.Nil(t, resolution.Capabilities)
	require.Error(t, registry.Validate())
}

func TestMediaAdapterResolverCapabilityMismatchKeepsResolvedMetadata(t *testing.T) {
	t.Parallel()
	_, resolver := newTestMediaAdapterResolver(t, exactImageRegistration("xai-image", "xai", "grok-2-image"))

	resolution := resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationImageEdit})
	require.Equal(t, MediaAdapterResolutionCapabilityMismatch, resolution.Status)
	require.Equal(t, "MEDIA_ADAPTER_CAPABILITY_MISMATCH", resolution.ReasonCode)
	require.Equal(t, "xai-image", resolution.ResolvedAdapter)
	require.Equal(t, MediaAdapterMatchedExact, resolution.MatchedBy)
	require.Empty(t, resolution.MatchedFamily)
	require.Equal(t, &MediaAdapterCapabilities{
		Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
	}, resolution.Capabilities)
	require.Equal(t, NativeAsyncUnsupported, resolution.CompatibilityAsyncMode())
	resolution.Capabilities.Operations[0] = MediaOperationVideoRemix
	resolution = resolver.Resolve("xai", "grok-2-image", []MediaOperation{MediaOperationImageEdit})
	require.Equal(t, []MediaOperation{MediaOperationTextToImage}, resolution.Capabilities.Operations)
}

func TestMediaAdapterResolverDerivesContentFetchFromInterface(t *testing.T) {
	t.Parallel()
	registration := MediaAdapterRegistration{
		Key:                 "content-image",
		Adapter:             &mediaContentFetchSyncAdapter{name: "content-image"},
		SupportedOperations: []MediaOperation{MediaOperationTextToImage},
		ExactRules: []MediaAdapterExactRule{{
			Vendor: "vendor", ModelID: "content-image",
			Capabilities: MediaAdapterRuleCapabilities{
				Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
			},
		}},
	}
	_, resolver := newTestMediaAdapterResolver(t, registration)

	resolution := resolver.Resolve("vendor", "content-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.True(t, resolution.Capabilities.ContentFetch)

	_, resolver = newTestMediaAdapterResolver(t, exactImageRegistration("plain-image", "vendor", "plain-image"))
	resolution = resolver.Resolve("vendor", "plain-image", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionReady, resolution.Status)
	require.False(t, resolution.Capabilities.ContentFetch)
}

func TestMediaAdapterRegistryResolvesHistoricalAlias(t *testing.T) {
	registry := NewMediaAdapterRegistry()
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "canonical-image", NativeAsyncMode: NativeAsyncUnsupported})
	require.NoError(t, registry.Register("canonical-image", adapter))
	require.NoError(t, registry.RegisterAlias("legacy-image", "canonical-image"))
	require.NoError(t, registry.Validate())
	resolved, err := registry.Resolve("legacy-image")
	require.NoError(t, err)
	require.Same(t, adapter, resolved)
	require.Error(t, registry.RegisterAlias("alias-chain", "legacy-image"))
}

func newTestMediaAdapterResolver(
	t *testing.T,
	registrations ...MediaAdapterRegistration,
) (*MediaAdapterRegistry, *MediaAdapterResolver) {
	t.Helper()
	registry := NewMediaAdapterRegistry()
	for _, registration := range registrations {
		require.NoError(t, registry.RegisterDefinition(registration))
	}
	require.NoError(t, registry.Validate())
	return registry, NewMediaAdapterResolver(registry)
}

func exactImageRegistration(key, vendor, modelID string) MediaAdapterRegistration {
	return MediaAdapterRegistration{
		Key: key,
		Adapter: NewFakeMediaAdapter(FakeMediaAdapterOptions{
			Name: key, NativeAsyncMode: NativeAsyncUnsupported,
		}),
		SupportedOperations: []MediaOperation{MediaOperationTextToImage},
		ExactRules: []MediaAdapterExactRule{{
			Vendor: vendor, ModelID: modelID,
			Capabilities: MediaAdapterRuleCapabilities{
				Operations: []MediaOperation{MediaOperationTextToImage}, SyncUpstream: true,
			},
		}},
	}
}

func readyResolution(syncUpstream, nativeAsyncUpstream bool) MediaAdapterResolution {
	return MediaAdapterResolution{
		Status:          MediaAdapterResolutionReady,
		ResolvedAdapter: "test-adapter",
		MatchedBy:       MediaAdapterMatchedExact,
		Capabilities: &MediaAdapterCapabilities{
			Operations:          []MediaOperation{MediaOperationTextToImage},
			SyncUpstream:        syncUpstream,
			NativeAsyncUpstream: nativeAsyncUpstream,
		},
	}
}

func registrationWithCapabilities(key string, adapter MediaAdapter, capabilities MediaAdapterRuleCapabilities) MediaAdapterRegistration {
	return MediaAdapterRegistration{
		Key:                 key,
		Adapter:             adapter,
		SupportedOperations: []MediaOperation{MediaOperationTextToImage},
		ExactRules: []MediaAdapterExactRule{{
			Vendor: "vendor", ModelID: key + "-model", Capabilities: capabilities,
		}},
	}
}
