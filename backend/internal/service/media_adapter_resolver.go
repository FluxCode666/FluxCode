package service

import (
	"errors"
	"fmt"
	"strings"
)

type MediaAdapterResolutionStatus string

const (
	MediaAdapterResolutionReady                 MediaAdapterResolutionStatus = "ready"
	MediaAdapterResolutionInvalidDefinition     MediaAdapterResolutionStatus = "invalid_definition"
	MediaAdapterResolutionUnresolved            MediaAdapterResolutionStatus = "unresolved"
	MediaAdapterResolutionAmbiguous             MediaAdapterResolutionStatus = "ambiguous"
	MediaAdapterResolutionImplementationMissing MediaAdapterResolutionStatus = "implementation_missing"
	MediaAdapterResolutionCapabilityMismatch    MediaAdapterResolutionStatus = "capability_mismatch"
)

type MediaAdapterMatchType string

const (
	MediaAdapterMatchedExact  MediaAdapterMatchType = "exact"
	MediaAdapterMatchedFamily MediaAdapterMatchType = "family"
)

type MediaAdapterCapabilities struct {
	Operations          []MediaOperation
	SyncUpstream        bool
	NativeAsyncUpstream bool
	ContentFetch        bool
}

type MediaAdapterRuleCapabilities struct {
	Operations          []MediaOperation
	SyncUpstream        bool
	NativeAsyncUpstream bool
}

type MediaAdapterExactRule struct {
	Vendor       string
	ModelID      string
	Capabilities MediaAdapterRuleCapabilities
}

type MediaAdapterFamilyRule struct {
	Vendor       string
	FamilyID     string
	Match        func(modelID string) bool
	Capabilities MediaAdapterRuleCapabilities
}

type MediaAdapterRegistration struct {
	Key                 string
	Adapter             MediaAdapter
	SupportedOperations []MediaOperation
	ExactRules          []MediaAdapterExactRule
	FamilyRules         []MediaAdapterFamilyRule
}

type MediaAdapterResolution struct {
	Status          MediaAdapterResolutionStatus
	ResolvedAdapter string
	MatchedBy       MediaAdapterMatchType
	MatchedFamily   string
	Capabilities    *MediaAdapterCapabilities
	ReasonCode      string
}

func (r MediaAdapterResolution) IsReady() bool {
	return r.Status == MediaAdapterResolutionReady && r.Capabilities != nil
}

func (r MediaAdapterResolution) CompatibilityAsyncMode() NativeAsyncMode {
	if !r.IsReady() {
		return NativeAsyncUnsupported
	}
	switch {
	case r.Capabilities.SyncUpstream && r.Capabilities.NativeAsyncUpstream:
		return NativeAsyncOptional
	case r.Capabilities.NativeAsyncUpstream:
		return NativeAsyncRequired
	default:
		return NativeAsyncUnsupported
	}
}

type mediaAdapterExactRuleKey struct {
	vendor  string
	modelID string
}

type mediaAdapterRuleEntry struct {
	adapterKey          string
	supportedOperations []MediaOperation
	capabilities        MediaAdapterRuleCapabilities
}

type mediaAdapterFamilyEntry struct {
	mediaAdapterRuleEntry
	familyID string
	match    func(modelID string) bool
}

type MediaAdapterResolver struct {
	registry *MediaAdapterRegistry
	exact    map[mediaAdapterExactRuleKey]mediaAdapterRuleEntry
	families map[string][]mediaAdapterFamilyEntry
}

func NewMediaAdapterResolver(registry *MediaAdapterRegistry) *MediaAdapterResolver {
	resolver := &MediaAdapterResolver{
		registry: registry,
		exact:    make(map[mediaAdapterExactRuleKey]mediaAdapterRuleEntry),
		families: make(map[string][]mediaAdapterFamilyEntry),
	}
	if registry == nil {
		return resolver
	}
	for _, registration := range registry.Registrations() {
		for _, rule := range registration.ExactRules {
			key := mediaAdapterExactRuleKey{vendor: rule.Vendor, modelID: rule.ModelID}
			if _, exists := resolver.exact[key]; exists {
				continue
			}
			resolver.exact[key] = mediaAdapterRuleEntry{
				adapterKey:          registration.Key,
				supportedOperations: cloneMediaOperations(registration.SupportedOperations),
				capabilities:        cloneMediaAdapterRuleCapabilities(rule.Capabilities),
			}
		}
		for _, rule := range registration.FamilyRules {
			resolver.families[rule.Vendor] = append(resolver.families[rule.Vendor], mediaAdapterFamilyEntry{
				mediaAdapterRuleEntry: mediaAdapterRuleEntry{
					adapterKey:          registration.Key,
					supportedOperations: cloneMediaOperations(registration.SupportedOperations),
					capabilities:        cloneMediaAdapterRuleCapabilities(rule.Capabilities),
				},
				familyID: rule.FamilyID,
				match:    rule.Match,
			})
		}
	}
	return resolver
}

func (r *MediaAdapterResolver) Resolve(vendor, modelID string, operations []MediaOperation) MediaAdapterResolution {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	modelID = normalizeMediaModelID(modelID)
	if r == nil {
		return mediaAdapterResolutionFailure(MediaAdapterResolutionUnresolved, "MEDIA_ADAPTER_UNRESOLVED")
	}
	if exact, ok := r.exact[mediaAdapterExactRuleKey{vendor: vendor, modelID: modelID}]; ok {
		return r.resolveRule(exact, MediaAdapterMatchedExact, "", operations)
	}
	matches := make([]mediaAdapterFamilyEntry, 0, 1)
	for _, family := range r.families[vendor] {
		if family.match(modelID) {
			matches = append(matches, family)
		}
	}
	if len(matches) == 0 {
		return mediaAdapterResolutionFailure(MediaAdapterResolutionUnresolved, "MEDIA_ADAPTER_UNRESOLVED")
	}
	if len(matches) != 1 {
		return mediaAdapterResolutionFailure(MediaAdapterResolutionAmbiguous, "MEDIA_ADAPTER_AMBIGUOUS")
	}
	match := matches[0]
	return r.resolveRule(match.mediaAdapterRuleEntry, MediaAdapterMatchedFamily, match.familyID, operations)
}

func (r *MediaAdapterResolver) resolveRule(
	entry mediaAdapterRuleEntry,
	matchedBy MediaAdapterMatchType,
	matchedFamily string,
	requestedOperations []MediaOperation,
) MediaAdapterResolution {
	base := MediaAdapterResolution{
		ResolvedAdapter: entry.adapterKey,
		MatchedBy:       matchedBy,
		MatchedFamily:   matchedFamily,
	}
	if r.registry == nil {
		base.Status = MediaAdapterResolutionImplementationMissing
		base.ReasonCode = "MEDIA_ADAPTER_IMPLEMENTATION_MISSING"
		return base
	}
	adapter, err := r.registry.Resolve(entry.adapterKey)
	if err != nil || isNilMediaAdapter(adapter) {
		base.Status = MediaAdapterResolutionImplementationMissing
		base.ReasonCode = "MEDIA_ADAPTER_IMPLEMENTATION_MISSING"
		return base
	}

	_, syncImplemented := adapter.(MediaSyncGenerator)
	_, submitImplemented := adapter.(MediaAsyncSubmitter)
	_, pollImplemented := adapter.(MediaAsyncPoller)
	_, contentFetchImplemented := adapter.(MediaContentFetcher)
	capabilities := &MediaAdapterCapabilities{
		Operations:          intersectMediaOperations(entry.capabilities.Operations, entry.supportedOperations),
		SyncUpstream:        entry.capabilities.SyncUpstream && syncImplemented,
		NativeAsyncUpstream: entry.capabilities.NativeAsyncUpstream && submitImplemented && pollImplemented,
		ContentFetch:        contentFetchImplemented,
	}
	base.Capabilities = capabilities
	if !mediaOperationsAreSubset(requestedOperations, capabilities.Operations) ||
		(!capabilities.SyncUpstream && !capabilities.NativeAsyncUpstream) {
		base.Status = MediaAdapterResolutionCapabilityMismatch
		base.ReasonCode = "MEDIA_ADAPTER_CAPABILITY_MISMATCH"
		return base
	}
	base.Status = MediaAdapterResolutionReady
	return base
}

func mediaAdapterResolutionFailure(status MediaAdapterResolutionStatus, reasonCode string) MediaAdapterResolution {
	return MediaAdapterResolution{Status: status, ReasonCode: reasonCode}
}

func normalizeAndValidateMediaAdapterRegistration(registration MediaAdapterRegistration) (MediaAdapterRegistration, error) {
	normalized := cloneMediaAdapterRegistration(registration)
	normalized.Key = normalizeMediaAdapterName(normalized.Key)
	if normalized.Key == "" || isNilMediaAdapter(normalized.Adapter) {
		return MediaAdapterRegistration{}, errors.New("media adapter name and implementation are required")
	}
	if !isValidMediaAdapterName(normalized.Key) || len(normalized.Key) > 64 {
		return MediaAdapterRegistration{}, fmt.Errorf("media adapter key %q has invalid format", normalized.Key)
	}
	if normalizeMediaAdapterName(normalized.Adapter.Name()) != normalized.Key {
		return MediaAdapterRegistration{}, fmt.Errorf(
			"media adapter key %q does not match implementation name %q",
			normalized.Key,
			normalized.Adapter.Name(),
		)
	}

	supported, supportedSet, err := validateMediaAdapterOperations(
		"supported operations",
		normalized.SupportedOperations,
		nil,
	)
	if err != nil {
		return MediaAdapterRegistration{}, err
	}
	normalized.SupportedOperations = supported
	if len(normalized.ExactRules) == 0 && len(normalized.FamilyRules) == 0 {
		return MediaAdapterRegistration{}, errors.New("media adapter registration has no exact or family rules")
	}

	localExact := make(map[mediaAdapterExactRuleKey]struct{}, len(normalized.ExactRules))
	for index := range normalized.ExactRules {
		rule := &normalized.ExactRules[index]
		rule.Vendor = strings.ToLower(strings.TrimSpace(rule.Vendor))
		rule.ModelID = normalizeMediaModelID(rule.ModelID)
		if !isValidMediaSimpleIdentifier(rule.Vendor, 64) {
			return MediaAdapterRegistration{}, fmt.Errorf("media adapter exact rule vendor %q has invalid format", rule.Vendor)
		}
		if !isValidMediaModelIdentifier(rule.ModelID) {
			return MediaAdapterRegistration{}, fmt.Errorf("media adapter exact rule model id %q has invalid format", rule.ModelID)
		}
		key := mediaAdapterExactRuleKey{vendor: rule.Vendor, modelID: rule.ModelID}
		if _, exists := localExact[key]; exists {
			return MediaAdapterRegistration{}, fmt.Errorf("duplicate media adapter exact rule: vendor=%q model_id=%q", rule.Vendor, rule.ModelID)
		}
		localExact[key] = struct{}{}
		rule.Capabilities, err = normalizeAndValidateMediaAdapterRuleCapabilities(
			normalized.Adapter,
			rule.Capabilities,
			supportedSet,
		)
		if err != nil {
			return MediaAdapterRegistration{}, fmt.Errorf("validate media adapter exact rule %q/%q: %w", rule.Vendor, rule.ModelID, err)
		}
	}

	for index := range normalized.FamilyRules {
		rule := &normalized.FamilyRules[index]
		rule.Vendor = strings.ToLower(strings.TrimSpace(rule.Vendor))
		rule.FamilyID = strings.ToLower(strings.TrimSpace(rule.FamilyID))
		if !isValidMediaSimpleIdentifier(rule.Vendor, 64) {
			return MediaAdapterRegistration{}, fmt.Errorf("media adapter family rule vendor %q has invalid format", rule.Vendor)
		}
		if !isValidMediaSimpleIdentifier(rule.FamilyID, 64) {
			return MediaAdapterRegistration{}, fmt.Errorf("media adapter family id %q has invalid format", rule.FamilyID)
		}
		if rule.Match == nil {
			return MediaAdapterRegistration{}, fmt.Errorf("media adapter family %q matcher is required", rule.FamilyID)
		}
		rule.Capabilities, err = normalizeAndValidateMediaAdapterRuleCapabilities(
			normalized.Adapter,
			rule.Capabilities,
			supportedSet,
		)
		if err != nil {
			return MediaAdapterRegistration{}, fmt.Errorf("validate media adapter family rule %q/%q: %w", rule.Vendor, rule.FamilyID, err)
		}
	}
	return normalized, nil
}

func normalizeAndValidateMediaAdapterRuleCapabilities(
	adapter MediaAdapter,
	capabilities MediaAdapterRuleCapabilities,
	supportedSet map[MediaOperation]struct{},
) (MediaAdapterRuleCapabilities, error) {
	operations, _, err := validateMediaAdapterOperations("rule operations", capabilities.Operations, supportedSet)
	if err != nil {
		return MediaAdapterRuleCapabilities{}, err
	}
	capabilities.Operations = operations
	if !capabilities.SyncUpstream && !capabilities.NativeAsyncUpstream {
		return MediaAdapterRuleCapabilities{}, errors.New("media adapter rule has no execution path")
	}
	if capabilities.SyncUpstream {
		if _, ok := adapter.(MediaSyncGenerator); !ok {
			return MediaAdapterRuleCapabilities{}, errors.New("sync upstream requires MediaSyncGenerator")
		}
	}
	if capabilities.NativeAsyncUpstream {
		_, submitOK := adapter.(MediaAsyncSubmitter)
		_, pollOK := adapter.(MediaAsyncPoller)
		if !submitOK || !pollOK {
			return MediaAdapterRuleCapabilities{}, errors.New("native async upstream requires MediaAsyncSubmitter and MediaAsyncPoller")
		}
	}
	return capabilities, nil
}

func validateMediaAdapterOperations(
	label string,
	operations []MediaOperation,
	allowed map[MediaOperation]struct{},
) ([]MediaOperation, map[MediaOperation]struct{}, error) {
	if len(operations) == 0 {
		return nil, nil, fmt.Errorf("media adapter %s are empty", label)
	}
	cloned := cloneMediaOperations(operations)
	seen := make(map[MediaOperation]struct{}, len(cloned))
	for _, operation := range cloned {
		if _, ok := mediaTypeForOperation(operation); !ok {
			return nil, nil, fmt.Errorf("unsupported media adapter operation %q", operation)
		}
		if _, exists := seen[operation]; exists {
			return nil, nil, fmt.Errorf("duplicate media adapter operation %q", operation)
		}
		if allowed != nil {
			if _, ok := allowed[operation]; !ok {
				return nil, nil, fmt.Errorf("media adapter operation %q is not declared in supported operations", operation)
			}
		}
		seen[operation] = struct{}{}
	}
	return cloned, seen, nil
}

func mediaOperationsAreSubset(requested, supported []MediaOperation) bool {
	supportedSet := make(map[MediaOperation]struct{}, len(supported))
	for _, operation := range supported {
		supportedSet[operation] = struct{}{}
	}
	for _, operation := range requested {
		if _, ok := supportedSet[operation]; !ok {
			return false
		}
	}
	return true
}

func intersectMediaOperations(left, right []MediaOperation) []MediaOperation {
	rightSet := make(map[MediaOperation]struct{}, len(right))
	for _, operation := range right {
		rightSet[operation] = struct{}{}
	}
	result := make([]MediaOperation, 0, len(left))
	for _, operation := range left {
		if _, ok := rightSet[operation]; ok {
			result = append(result, operation)
		}
	}
	return result
}

func cloneMediaOperations(operations []MediaOperation) []MediaOperation {
	return append([]MediaOperation(nil), operations...)
}

func cloneMediaAdapterRuleCapabilities(capabilities MediaAdapterRuleCapabilities) MediaAdapterRuleCapabilities {
	capabilities.Operations = cloneMediaOperations(capabilities.Operations)
	return capabilities
}

func cloneMediaAdapterRegistration(registration MediaAdapterRegistration) MediaAdapterRegistration {
	registration.SupportedOperations = cloneMediaOperations(registration.SupportedOperations)
	registration.ExactRules = append([]MediaAdapterExactRule(nil), registration.ExactRules...)
	for index := range registration.ExactRules {
		registration.ExactRules[index].Capabilities = cloneMediaAdapterRuleCapabilities(registration.ExactRules[index].Capabilities)
	}
	registration.FamilyRules = append([]MediaAdapterFamilyRule(nil), registration.FamilyRules...)
	for index := range registration.FamilyRules {
		registration.FamilyRules[index].Capabilities = cloneMediaAdapterRuleCapabilities(registration.FamilyRules[index].Capabilities)
	}
	return registration
}

func cloneMediaAdapterRegistrations(registrations []MediaAdapterRegistration) []MediaAdapterRegistration {
	cloned := make([]MediaAdapterRegistration, len(registrations))
	for index, registration := range registrations {
		cloned[index] = cloneMediaAdapterRegistration(registration)
	}
	return cloned
}
