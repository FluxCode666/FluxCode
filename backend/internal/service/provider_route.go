package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type RouteTier string

const (
	RouteTierNative     RouteTier = "native"
	RouteTierConversion RouteTier = "conversion"
)

func (t RouteTier) IsValid() bool {
	return t == RouteTierNative || t == RouteTierConversion
}

// RouteIdentity identifies a concrete provider path. It intentionally contains
// no platform or account-type fields: provider capability declarations are the
// only source of protocol eligibility.
type RouteIdentity struct {
	ProviderID        int64
	ProviderVersion   int64
	CapabilityID      int64
	CapabilityVersion int64
	EndpointID        int64
	EndpointVersion   int64
	IngressProtocol   ProtocolFamily
	UpstreamProtocol  ProtocolFamily
	Adapter           string
	AdapterVersion    string
}

func NewRouteIdentity(
	capability ProviderRouteCapability,
	ingress ProtocolFamily,
	adapter string,
	adapterVersion string,
) RouteIdentity {
	identity := RouteIdentity{
		IngressProtocol:   ingress,
		Adapter:           strings.TrimSpace(adapter),
		AdapterVersion:    strings.TrimSpace(adapterVersion),
		UpstreamProtocol:  capability.Capability.Protocol,
		CapabilityID:      capability.Capability.ID,
		CapabilityVersion: capability.Capability.Version,
	}
	if capability.Profile != nil {
		identity.ProviderID = capability.Profile.ID
		identity.ProviderVersion = capability.Profile.Version
	} else {
		identity.ProviderID = capability.Capability.ProviderID
	}
	if capability.Endpoint != nil {
		identity.EndpointID = capability.Endpoint.ID
		identity.EndpointVersion = capability.Endpoint.Version
	}
	return identity
}

func (i RouteIdentity) String() string {
	return fmt.Sprintf(
		"provider:%d@%d/capability:%d@%d/endpoint:%d@%d/%s->%s/adapter:%s@%s",
		i.ProviderID,
		i.ProviderVersion,
		i.CapabilityID,
		i.CapabilityVersion,
		i.EndpointID,
		i.EndpointVersion,
		i.IngressProtocol,
		i.UpstreamProtocol,
		i.Adapter,
		i.AdapterVersion,
	)
}

type RouteCandidate struct {
	Identity       RouteIdentity
	Tier           RouteTier
	Profile        *ProviderProfile
	Account        *Account
	Endpoint       *ProviderProtocolEndpoint
	LogicalModel   LogicalModel
	Capability     ProviderModelCapability
	GroupPriority  int
	AdapterProfile apicompat.CompatibilityProfile
}

func NewNativeRouteCandidate(capability ProviderRouteCapability, ingress ProtocolFamily) RouteCandidate {
	return RouteCandidate{
		Identity:      NewRouteIdentity(capability, ingress, "", ""),
		Tier:          RouteTierNative,
		Profile:       capability.Profile,
		Account:       capability.Account,
		Endpoint:      capability.Endpoint,
		LogicalModel:  capability.LogicalModel,
		Capability:    capability.Capability,
		GroupPriority: capability.GroupPriority,
	}
}

func NewConversionRouteCandidate(
	capability ProviderRouteCapability,
	ingress ProtocolFamily,
	adapter string,
	adapterVersion string,
	profile string,
) RouteCandidate {
	return RouteCandidate{
		Identity:       NewRouteIdentity(capability, ingress, adapter, adapterVersion),
		Tier:           RouteTierConversion,
		Profile:        capability.Profile,
		Account:        capability.Account,
		Endpoint:       capability.Endpoint,
		LogicalModel:   capability.LogicalModel,
		Capability:     capability.Capability,
		GroupPriority:  capability.GroupPriority,
		AdapterProfile: apicompat.CompatibilityProfile(profile),
	}
}

func FilterRouteCandidates(
	candidates []RouteCandidate,
	excludedRoutes map[RouteIdentity]struct{},
	excludedProviders map[int64]struct{},
) []RouteCandidate {
	filtered := make([]RouteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, excluded := excludedRoutes[candidate.Identity]; excluded {
			continue
		}
		if _, excluded := excludedProviders[candidate.Identity.ProviderID]; excluded {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func sortRoutesByIdentity(candidates []RouteCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Identity.String() < candidates[j].Identity.String()
	})
}
