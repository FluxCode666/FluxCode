package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

type providerRouteRepositoryStub struct {
	capabilities []ProviderRouteCapability
}

func (s *providerRouteRepositoryStub) SaveProfile(context.Context, *ProviderProfile) error {
	return nil
}

func (s *providerRouteRepositoryStub) SaveEndpoint(context.Context, *ProviderProtocolEndpoint) error {
	return nil
}

func (s *providerRouteRepositoryStub) SaveCapability(context.Context, *ProviderModelCapability) error {
	return nil
}

func (s *providerRouteRepositoryStub) UpsertLogicalModel(context.Context, *LogicalModel) error {
	return nil
}

func (s *providerRouteRepositoryStub) GetByID(context.Context, int64) (*ProviderAggregate, error) {
	return nil, ErrProviderNotFound
}

func (s *providerRouteRepositoryStub) ListRouteCapabilities(_ context.Context, filter ProviderCapabilityFilter) ([]ProviderRouteCapability, error) {
	result := make([]ProviderRouteCapability, 0, len(s.capabilities))
	for _, capability := range s.capabilities {
		if capability.LogicalModel.Name != filter.LogicalModel {
			continue
		}
		if filter.Protocol != "" && capability.Capability.Protocol != filter.Protocol {
			continue
		}
		result = append(result, capability)
	}
	return result, nil
}

func routeCapability(providerID, capabilityID int64, protocol ProtocolFamily, allowConversion bool) ProviderRouteCapability {
	account := &Account{
		ID: providerID, Name: "provider", Platform: "must-not-affect-routing", Type: "custom",
		Status: StatusActive, Schedulable: true, Concurrency: 10, Priority: 20,
	}
	profile := NewProviderProfile(providerID, "provider")
	profile.Status = ProviderStatusActive
	profile.AllowProtocolConversion = allowConversion
	profile.Version = 3
	endpoint := &ProviderProtocolEndpoint{
		ID: providerID*10 + int64(protocol[0]), ProviderID: providerID, Protocol: protocol,
		WireProfile: WireProfileCanonical, Enabled: true, Version: 2,
	}
	capability := ProviderModelCapability{
		ID: capabilityID, ProviderID: providerID, LogicalModelID: 99, Protocol: protocol,
		UpstreamModel: "upstream-model", WireProfile: WireProfileCanonical,
		FeatureProfile: FeatureProfileText, Enabled: true, Version: 7,
	}
	if protocol == ProtocolEmbeddings {
		capability.FeatureProfile = FeatureProfileEmbeddings
	}
	return ProviderRouteCapability{
		Profile: profile, Account: account, Endpoint: endpoint,
		LogicalModel: LogicalModel{ID: 99, Name: "model-a", Enabled: true, Version: 4},
		Capability:   capability, GroupPriority: 10,
	}
}

func TestProviderRouteResolverPrefersNativeTier(t *testing.T) {
	repo := &providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{
		routeCapability(1, 101, ProtocolChatCompletions, true),
		routeCapability(2, 202, ProtocolResponses, false),
	}}
	resolver := NewProviderRouteResolver(repo, apicompat.NewRegistry())

	resolution, err := resolver.Resolve(context.Background(), ProviderRouteRequest{
		GroupID: 12, LogicalModel: "model-a", IngressProtocol: ProtocolResponses,
		RawRequest: []byte(`{"model":"model-a","input":"hello"}`),
	})

	require.NoError(t, err)
	require.Equal(t, RouteTierNative, resolution.Tier)
	require.Len(t, resolution.Candidates, 1)
	require.Equal(t, int64(2), resolution.Candidates[0].Identity.ProviderID)
	require.Equal(t, ProtocolResponses, resolution.Candidates[0].Identity.UpstreamProtocol)
	require.Empty(t, resolution.Candidates[0].Identity.Adapter)
}

func TestProviderRouteResolverUsesConversionOnlyAfterNativeRoutesAreExhausted(t *testing.T) {
	native := routeCapability(2, 202, ProtocolResponses, false)
	repo := &providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{
		routeCapability(1, 101, ProtocolChatCompletions, true),
		native,
	}}
	resolver := NewProviderRouteResolver(repo, apicompat.NewRegistry())
	nativeID := NewRouteIdentity(native, ProtocolResponses, "", "")

	resolution, err := resolver.Resolve(context.Background(), ProviderRouteRequest{
		GroupID: 12, LogicalModel: "model-a", IngressProtocol: ProtocolResponses,
		RawRequest:     []byte(`{"model":"model-a","input":"hello","store":false}`),
		ExcludedRoutes: map[RouteIdentity]struct{}{nativeID: {}},
	})

	require.NoError(t, err)
	require.Equal(t, RouteTierConversion, resolution.Tier)
	require.Len(t, resolution.Candidates, 1)
	require.Equal(t, int64(1), resolution.Candidates[0].Identity.ProviderID)
	require.Equal(t, ProtocolChatCompletions, resolution.Candidates[0].Identity.UpstreamProtocol)
	require.Equal(t, apicompat.ProfileNonStreamText, resolution.Candidates[0].AdapterProfile)
}

func TestProviderRouteResolverConversionDefaultsClosed(t *testing.T) {
	repo := &providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{
		routeCapability(1, 101, ProtocolChatCompletions, false),
	}}
	resolver := NewProviderRouteResolver(repo, apicompat.NewRegistry())

	_, err := resolver.Resolve(context.Background(), ProviderRouteRequest{
		GroupID: 12, LogicalModel: "model-a", IngressProtocol: ProtocolResponses,
		RawRequest: []byte(`{"model":"model-a","input":"hello"}`),
	})

	require.ErrorIs(t, err, ErrNoProviderRoute)
}

func TestProviderRouteResolverEmbeddingsAreNativeOnly(t *testing.T) {
	repo := &providerRouteRepositoryStub{capabilities: []ProviderRouteCapability{
		routeCapability(1, 101, ProtocolChatCompletions, true),
	}}
	resolver := NewProviderRouteResolver(repo, apicompat.NewRegistry())

	_, err := resolver.Resolve(context.Background(), ProviderRouteRequest{
		GroupID: 12, LogicalModel: "model-a", IngressProtocol: ProtocolEmbeddings,
		RawRequest: []byte(`{"model":"model-a","input":"hello"}`),
	})

	require.ErrorIs(t, err, ErrNoProviderRoute)
}

func TestProviderRouteResolverFiltersNativeRoutesByFeatureProfile(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		supportedProfile ProviderFeatureProfile
	}{
		{
			name: "stream", body: `{"model":"model-a","messages":[],"stream":true}`,
			supportedProfile: FeatureProfileStreamText,
		},
		{
			name: "tools", body: `{"model":"model-a","messages":[],"tools":[{"type":"function","function":{"name":"lookup"}}]}`,
			supportedProfile: FeatureProfileTools,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsupported := routeCapability(1, 101, ProtocolChatCompletions, false)
			unsupported.Capability.FeatureProfile = FeatureProfileText
			supported := routeCapability(2, 202, ProtocolChatCompletions, false)
			supported.Capability.FeatureProfile = tt.supportedProfile
			resolver := NewProviderRouteResolver(&providerRouteRepositoryStub{
				capabilities: []ProviderRouteCapability{unsupported, supported},
			}, apicompat.NewRegistry())

			resolution, err := resolver.Resolve(context.Background(), ProviderRouteRequest{
				GroupID: 12, LogicalModel: "model-a", IngressProtocol: ProtocolChatCompletions,
				RawRequest: []byte(tt.body),
			})

			require.NoError(t, err)
			require.Len(t, resolution.Candidates, 1)
			require.Equal(t, NewNativeRouteCandidate(supported, ProtocolChatCompletions).Identity, resolution.Candidates[0].Identity)
			require.Len(t, resolution.Rejections, 1)
			require.Equal(t, "feature_profile_mismatch", resolution.Rejections[0].Reason)
		})
	}
}

func TestRouteIdentityIsStableAndPlatformIndependent(t *testing.T) {
	left := routeCapability(8, 808, ProtocolChatCompletions, false)
	right := left
	right.Account = cloneAccountForProviderRoute(left.Account)
	right.Account.Platform = "another-platform"
	right.Account.Type = "another-type"

	leftID := NewRouteIdentity(left, ProtocolResponses, "responses_to_chat", "v1")
	rightID := NewRouteIdentity(right, ProtocolResponses, "responses_to_chat", "v1")

	require.Equal(t, leftID, rightID)
	require.Equal(t, leftID.String(), rightID.String())
	require.NotEmpty(t, leftID.String())
}

func cloneAccountForProviderRoute(account *Account) *Account {
	if account == nil {
		return nil
	}
	cloned := *account
	return &cloned
}

func TestRouteLevelExclusionDoesNotExcludeSiblingCapability(t *testing.T) {
	now := time.Now()
	first := routeCapability(1, 101, ProtocolChatCompletions, false)
	second := routeCapability(1, 102, ProtocolResponses, false)
	first.Account.LastUsedAt = &now
	second.Account.LastUsedAt = &now
	candidates := []RouteCandidate{
		NewNativeRouteCandidate(first, ProtocolChatCompletions),
		NewNativeRouteCandidate(second, ProtocolResponses),
	}

	remaining := FilterRouteCandidates(candidates, map[RouteIdentity]struct{}{candidates[0].Identity: {}}, nil)

	require.Len(t, remaining, 1)
	require.Equal(t, int64(102), remaining[0].Identity.CapabilityID)
}
