package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

var (
	ErrProviderNotFound                = errors.New("provider not found")
	ErrNoProviderRoute                 = errors.New("no provider route")
	ErrProviderContinuationUnavailable = errors.New("provider continuation route is unavailable")
)

const (
	adapterResponsesToChat         = "responses_to_chat_completions"
	adapterRegistryContractVersion = "v1"
)

type ProviderRouteRequest struct {
	GroupID           int64
	SnapshotVersion   int64
	LogicalModel      string
	IngressProtocol   ProtocolFamily
	RawRequest        []byte
	ExcludedRoutes    map[RouteIdentity]struct{}
	ExcludedProviders map[int64]struct{}
	RequiredRoute     *RouteIdentity
}

type ProviderRouteRejection struct {
	Route            RouteIdentity
	Tier             RouteTier
	LogicalModel     string
	UpstreamModel    string
	WireProfile      WireProfile
	ConversionUsed   bool
	IngressProtocol  ProtocolFamily
	UpstreamProtocol ProtocolFamily
	Reason           string
	Detail           string
}

type ProviderRouteResolution struct {
	Tier       RouteTier
	Candidates []RouteCandidate
	Rejections []ProviderRouteRejection
}

type ProviderRouteResolver struct {
	repository ProviderRepository
	adapters   *apicompat.Registry
}

func NewProviderRouteResolver(repository ProviderRepository, adapters *apicompat.Registry) *ProviderRouteResolver {
	if adapters == nil {
		adapters = apicompat.NewRegistry()
	}
	return &ProviderRouteResolver{repository: repository, adapters: adapters}
}

func (r *ProviderRouteResolver) Resolve(ctx context.Context, request ProviderRouteRequest) (ProviderRouteResolution, error) {
	if r == nil || r.repository == nil {
		return ProviderRouteResolution{}, errors.New("provider route resolver is not initialized")
	}
	if request.GroupID <= 0 || strings.TrimSpace(request.LogicalModel) == "" || !request.IngressProtocol.IsValid() {
		return ProviderRouteResolution{}, errors.New("invalid provider route request")
	}

	declared, err := r.repository.ListRouteCapabilities(ctx, ProviderCapabilityFilter{
		GroupID:         request.GroupID,
		SnapshotVersion: request.SnapshotVersion,
		IngressProtocol: request.IngressProtocol,
		LogicalModel:    strings.TrimSpace(request.LogicalModel),
		OnlySchedulable: true,
	})
	if err != nil {
		return ProviderRouteResolution{}, fmt.Errorf("list provider route capabilities: %w", err)
	}

	resolution := ProviderRouteResolution{}
	native := make([]RouteCandidate, 0, len(declared))
	for _, capability := range declared {
		if capability.Capability.Protocol != request.IngressProtocol {
			continue
		}
		if reason := providerRouteCapabilityIneligible(capability); reason != "" {
			resolution.Rejections = append(resolution.Rejections, providerRouteRejection(
				capability, request.IngressProtocol, RouteTierNative, reason, "",
			))
			continue
		}
		if err := validateProviderNativeRequest(
			capability.Capability.Protocol,
			capability.Capability.FeatureProfile,
			request.RawRequest,
		); err != nil {
			resolution.Rejections = append(resolution.Rejections, providerRouteRejection(
				capability, request.IngressProtocol, RouteTierNative, "feature_profile_mismatch", err.Error(),
			))
			continue
		}
		native = append(native, NewNativeRouteCandidate(capability, request.IngressProtocol))
	}
	native = FilterRouteCandidates(native, request.ExcludedRoutes, request.ExcludedProviders)
	if request.RequiredRoute != nil {
		native = filterRequiredProviderRoute(native, *request.RequiredRoute)
	}
	if len(native) > 0 {
		sortRoutesByIdentity(native)
		resolution.Tier = RouteTierNative
		resolution.Candidates = native
		return resolution, nil
	}
	if request.RequiredRoute != nil {
		return resolution, ErrProviderContinuationUnavailable
	}

	if request.IngressProtocol == ProtocolEmbeddings {
		return resolution, fmt.Errorf("%w: embeddings require a native capability", ErrNoProviderRoute)
	}

	converted := make([]RouteCandidate, 0, len(declared))
	for _, capability := range declared {
		if capability.Capability.Protocol == request.IngressProtocol || !capability.Capability.Protocol.IsConversational() {
			continue
		}
		if reason := providerRouteCapabilityIneligible(capability); reason != "" {
			resolution.Rejections = append(resolution.Rejections, providerRouteRejection(
				capability, request.IngressProtocol, RouteTierConversion, reason, "",
			))
			continue
		}
		if capability.Profile == nil || !capability.Profile.AllowProtocolConversion {
			resolution.Rejections = append(resolution.Rejections, providerRouteRejection(
				capability, request.IngressProtocol, RouteTierConversion, "conversion_disabled", "",
			))
			continue
		}

		from, to := apiCompatProtocol(request.IngressProtocol), apiCompatProtocol(capability.Capability.Protocol)
		compatibility := r.adapters.CheckRequest(from, to, request.RawRequest)
		if !compatibility.Compatible {
			resolution.Rejections = append(resolution.Rejections, providerRouteRejection(
				capability,
				request.IngressProtocol,
				RouteTierConversion,
				compatibility.ReasonCode,
				compatibility.Detail,
			))
			continue
		}
		if !providerFeatureSupportsAdapterProfile(capability.Capability.FeatureProfile, compatibility.Profile) {
			resolution.Rejections = append(resolution.Rejections, providerRouteRejection(
				capability,
				request.IngressProtocol,
				RouteTierConversion,
				"feature_profile_mismatch",
				string(compatibility.Profile),
			))
			continue
		}
		adapterName := adapterDirectionName(request.IngressProtocol, capability.Capability.Protocol)
		converted = append(converted, NewConversionRouteCandidate(
			capability,
			request.IngressProtocol,
			adapterName,
			adapterRegistryContractVersion,
			string(compatibility.Profile),
		))
	}
	converted = FilterRouteCandidates(converted, request.ExcludedRoutes, request.ExcludedProviders)
	if len(converted) == 0 {
		return resolution, fmt.Errorf("%w: model %s has no eligible %s path", ErrNoProviderRoute, request.LogicalModel, request.IngressProtocol)
	}
	sortRoutesByIdentity(converted)
	resolution.Tier = RouteTierConversion
	resolution.Candidates = converted
	return resolution, nil
}

func filterRequiredProviderRoute(candidates []RouteCandidate, required RouteIdentity) []RouteCandidate {
	filtered := make([]RouteCandidate, 0, 1)
	for _, candidate := range candidates {
		if candidate.Identity == required {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func providerRouteCapabilityIneligible(capability ProviderRouteCapability) string {
	switch {
	case capability.Profile == nil || capability.Profile.Status != ProviderStatusActive:
		return "provider_inactive"
	case capability.Account == nil || !capability.Account.IsSchedulable():
		return "provider_unschedulable"
	case capability.Account.ProxyID != nil:
		// Provider transports must never silently bypass an assigned proxy. The
		// destination-pinned proxy transport is not part of the minimum release,
		// so these routes fail closed until it is available.
		return "provider_proxy_unsupported"
	case capability.Endpoint == nil || !capability.Endpoint.Enabled:
		return "endpoint_disabled"
	case !capability.Capability.Enabled:
		return "capability_disabled"
	case capability.Capability.Validate() != nil:
		return "capability_invalid"
	case !capability.LogicalModel.Enabled:
		return "logical_model_disabled"
	default:
		return ""
	}
}

func providerRouteRejection(
	capability ProviderRouteCapability,
	ingress ProtocolFamily,
	tier RouteTier,
	reason string,
	detail string,
) ProviderRouteRejection {
	return ProviderRouteRejection{
		Route: NewRouteIdentity(capability, ingress, "", ""), Tier: tier,
		LogicalModel:    capability.LogicalModel.Name,
		UpstreamModel:   capability.Capability.UpstreamModel,
		WireProfile:     capability.Capability.WireProfile,
		ConversionUsed:  tier == RouteTierConversion,
		IngressProtocol: ingress, UpstreamProtocol: capability.Capability.Protocol,
		Reason: reason, Detail: detail,
	}
}

func apiCompatProtocol(protocol ProtocolFamily) apicompat.Protocol {
	return apicompat.Protocol(protocol)
}

func adapterDirectionName(from, to ProtocolFamily) string {
	if from == ProtocolResponses && to == ProtocolChatCompletions {
		return adapterResponsesToChat
	}
	return string(from) + "_to_" + string(to)
}

// ProviderAdapterIdentity exposes the versioned adapter portion of a route
// identity so persistence can enforce an approved snapshot without owning
// adapter compatibility policy.
func ProviderAdapterIdentity(from, to ProtocolFamily) (string, string) {
	return adapterDirectionName(from, to), adapterRegistryContractVersion
}

func providerFeatureSupportsAdapterProfile(feature ProviderFeatureProfile, profile apicompat.CompatibilityProfile) bool {
	switch profile {
	case apicompat.ProfileNonStreamText:
		return feature == FeatureProfileText || feature == FeatureProfileStreamText || feature == FeatureProfileTools
	case apicompat.ProfileStreamText:
		return feature == FeatureProfileStreamText
	case apicompat.ProfileFunctionTools:
		return feature == FeatureProfileTools
	default:
		return false
	}
}
