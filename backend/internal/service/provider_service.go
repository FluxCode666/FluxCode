package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrProviderVersionConflict = infraerrors.Conflict("PROVIDER_VERSION_CONFLICT", "provider was modified; reload it and retry")
	ErrProviderInvalidConfig   = infraerrors.BadRequest("INVALID_PROVIDER_CONFIG", "provider configuration is invalid")
	ErrProviderActivation      = infraerrors.BadRequest("PROVIDER_ACTIVATION_REJECTED", "provider requires at least one valid native capability")
)

type ProviderEndpointInput struct {
	Protocol    ProtocolFamily
	WireProfile WireProfile
	BaseURL     string
	Path        string
	Headers     map[string]string
	AuthType    string
	Enabled     bool
}

type ProviderCapabilityInput struct {
	LogicalModelName        string
	LogicalModelDisplayName string
	Protocol                ProtocolFamily
	UpstreamModel           string
	WireProfile             WireProfile
	FeatureProfile          ProviderFeatureProfile
	Enabled                 bool
}

type ProviderWriteInput struct {
	Name                    string
	BaseURL                 string
	Headers                 map[string]string
	AuthType                string
	APIKey                  *string
	ClearAPIKey             bool
	AllowProtocolConversion bool
	GroupIDs                []int64
	Concurrency             int
	RateMultiplier          *float64
	Endpoints               []ProviderEndpointInput
	Capabilities            []ProviderCapabilityInput
	Version                 int64
}

type ProviderCapabilityTestInput struct {
	CapabilityID int64
	Protocol     ProtocolFamily
	LogicalModel string
}

type ProviderCapabilityTestResult struct {
	ProviderID        int64
	CapabilityID      int64
	Protocol          ProtocolFamily
	LogicalModel      string
	UpstreamModel     string
	StatusCode        int
	Duration          time.Duration
	UpstreamRequestID string
}

type GroupProviderCapability struct {
	ProviderID       int64          `json:"provider_id"`
	ProviderName     string         `json:"provider_name"`
	LogicalModel     string         `json:"logical_model"`
	IngressProtocol  ProtocolFamily `json:"ingress_protocol"`
	UpstreamProtocol ProtocolFamily `json:"upstream_protocol"`
	Tier             RouteTier      `json:"tier"`
	Adapter          string         `json:"adapter,omitempty"`
	AdapterVersion   string         `json:"adapter_version,omitempty"`
	GroupPriority    int            `json:"group_priority"`
	RouteIdentity    string         `json:"route_identity"`
}

type ProviderService struct {
	repository  ProviderAdminRepository
	accountRepo AccountRepository
	forwarder   *ProviderForwarder
	adapters    *apicompat.Registry
	encryptor   SecretEncryptor
	authCache   ProviderAuthCacheInvalidator
}

type ProviderAuthCacheInvalidator interface {
	InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64)
}

const providerAPIKeyCiphertextKey = "api_key_encrypted_v1"

func NewProviderService(
	repository ProviderAdminRepository,
	accountRepo AccountRepository,
	forwarder *ProviderForwarder,
	encryptor SecretEncryptor,
	authCache ProviderAuthCacheInvalidator,
) *ProviderService {
	return &ProviderService{
		repository: repository, accountRepo: accountRepo, forwarder: forwarder,
		adapters: apicompat.NewRegistry(), encryptor: encryptor, authCache: authCache,
	}
}

func (s *ProviderService) List(ctx context.Context) ([]*ProviderAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.repository.List(ctx)
}

func (s *ProviderService) GetByID(ctx context.Context, providerID int64) (*ProviderAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	aggregate, err := s.repository.GetByID(ctx, providerID)
	if err != nil {
		return nil, translateProviderLookupError(err)
	}
	return aggregate, nil
}

func (s *ProviderService) Create(ctx context.Context, input ProviderWriteInput) (*ProviderAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := validateProviderWriteInput(input, false); err != nil {
		return nil, err
	}
	credentials := map[string]any{}
	if input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
		var err error
		credentials, err = s.encryptProviderAPIKey(strings.TrimSpace(*input.APIKey))
		if err != nil {
			return nil, err
		}
	}
	concurrency := input.Concurrency
	if concurrency <= 0 {
		concurrency = 3
	}
	account := &Account{
		Name: strings.TrimSpace(input.Name), Platform: PlatformProvider, Type: AccountTypeAPIKey,
		Credentials: credentials, Extra: map[string]any{}, Concurrency: concurrency, Priority: 50,
		RateMultiplier: input.RateMultiplier, Status: StatusActive, Schedulable: true,
		AutoPauseOnExpired: true,
	}
	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create provider account: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.accountRepo.Delete(context.WithoutCancel(ctx), account.ID)
		}
	}()

	profile := NewProviderProfile(account.ID, input.Name)
	profile.AllowProtocolConversion = input.AllowProtocolConversion
	profile.Connection = ProviderConnectionConfig{
		BaseURL: strings.TrimSpace(input.BaseURL), Headers: input.Headers, AuthType: strings.TrimSpace(input.AuthType),
	}
	if err := s.repository.SaveProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("create provider profile: %w", err)
	}
	if err := s.replaceConfiguration(ctx, nil, profile, input); err != nil {
		return nil, err
	}
	if err := s.accountRepo.BindGroups(ctx, account.ID, uniquePositiveIDs(input.GroupIDs)); err != nil {
		return nil, fmt.Errorf("bind provider groups: %w", err)
	}
	cleanup = false
	return s.GetByID(ctx, account.ID)
}

func (s *ProviderService) Update(ctx context.Context, providerID int64, input ProviderWriteInput) (*ProviderAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if input.Version <= 0 {
		return nil, infraerrors.BadRequest("PROVIDER_VERSION_REQUIRED", "provider version is required")
	}
	if err := validateProviderWriteInput(input, true); err != nil {
		return nil, err
	}
	existing, err := s.GetByID(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if existing.Profile.Version != input.Version {
		return nil, ErrProviderVersionConflict
	}

	profile := *existing.Profile
	profile.Name = strings.TrimSpace(input.Name)
	profile.AllowProtocolConversion = input.AllowProtocolConversion
	profile.Connection = ProviderConnectionConfig{
		BaseURL: strings.TrimSpace(input.BaseURL), Headers: input.Headers, AuthType: strings.TrimSpace(input.AuthType),
	}
	previousStatus := profile.Status
	profile.Status = ProviderStatusDraft
	profile.Version++
	profile.UpdatedAt = time.Now()
	if err := s.repository.UpdateProfileIfVersion(ctx, &profile, input.Version); err != nil {
		return nil, err
	}

	account := cloneProviderAccount(existing.Account)
	account.Name = profile.Name
	account.Platform = PlatformProvider
	account.Type = AccountTypeAPIKey
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if input.ClearAPIKey {
		apiKey = ""
	} else if input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
		apiKey = strings.TrimSpace(*input.APIKey)
	}
	account.Credentials, err = s.encryptProviderAPIKey(apiKey)
	if err != nil {
		return nil, err
	}
	if input.Concurrency > 0 {
		account.Concurrency = input.Concurrency
	}
	if input.RateMultiplier != nil {
		account.RateMultiplier = input.RateMultiplier
	}
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("update provider account: %w", err)
	}
	if err := s.replaceConfiguration(ctx, existing, &profile, input); err != nil {
		return nil, err
	}
	if err := s.accountRepo.BindGroups(ctx, providerID, uniquePositiveIDs(input.GroupIDs)); err != nil {
		return nil, fmt.Errorf("bind provider groups: %w", err)
	}
	if previousStatus == ProviderStatusActive {
		configured, getErr := s.repository.GetByID(ctx, providerID)
		if getErr != nil {
			return nil, getErr
		}
		configured.Profile.Status = ProviderStatusActive
		if validateErr := configured.Profile.Validate(); validateErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrProviderActivation, validateErr)
		}
		configured.Profile.Version = profile.Version + 1
		configured.Profile.UpdatedAt = time.Now()
		if err := s.repository.UpdateProfileIfVersion(ctx, configured.Profile, profile.Version); err != nil {
			return nil, err
		}
	}
	return s.GetByID(ctx, providerID)
}

func (s *ProviderService) Activate(ctx context.Context, providerID, expectedVersion int64) (*ProviderAggregate, error) {
	return s.setStatus(ctx, providerID, expectedVersion, ProviderStatusActive)
}

func (s *ProviderService) Disable(ctx context.Context, providerID, expectedVersion int64) (*ProviderAggregate, error) {
	return s.setStatus(ctx, providerID, expectedVersion, ProviderStatusDisabled)
}

func (s *ProviderService) setStatus(ctx context.Context, providerID, expectedVersion int64, status ProviderStatus) (*ProviderAggregate, error) {
	aggregate, err := s.GetByID(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if expectedVersion <= 0 || aggregate.Profile.Version != expectedVersion {
		return nil, ErrProviderVersionConflict
	}
	profile := *aggregate.Profile
	profile.Status = status
	profile.Version++
	profile.UpdatedAt = time.Now()
	if status == ProviderStatusActive {
		profile.Capabilities = aggregate.Profile.Capabilities
		if err := validateProviderActivation(aggregate); err != nil {
			return nil, err
		}
	}
	if err := s.repository.UpdateProfileIfVersion(ctx, &profile, expectedVersion); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, providerID)
}

func (s *ProviderService) TestCapability(ctx context.Context, providerID int64, input ProviderCapabilityTestInput) (*ProviderCapabilityTestResult, error) {
	if s == nil || s.forwarder == nil {
		return nil, errors.New("provider forwarder is not configured")
	}
	aggregate, err := s.GetByID(ctx, providerID)
	if err != nil {
		return nil, err
	}
	capability, endpoint, model, err := selectProviderCapability(aggregate, input)
	if err != nil {
		return nil, err
	}
	routeCapability := ProviderRouteCapability{
		Profile: aggregate.Profile, Account: aggregate.Account, Endpoint: endpoint,
		LogicalModel: model, Capability: capability,
	}
	candidate := NewNativeRouteCandidate(routeCapability, capability.Protocol)
	body := providerCapabilityTestBody(capability.Protocol, model.Name)
	forwardInput := ProviderForwardInput{Candidate: candidate, Body: body, Headers: make(http.Header)}
	var result *ProviderForwardResult
	switch capability.Protocol {
	case ProtocolChatCompletions:
		result, err = s.forwarder.ForwardChat(ctx, forwardInput)
	case ProtocolResponses:
		result, err = s.forwarder.ForwardResponses(ctx, forwardInput)
	case ProtocolAnthropicMessages:
		result, err = s.forwarder.ForwardMessages(ctx, forwardInput)
	case ProtocolEmbeddings:
		result, err = s.forwarder.ForwardEmbeddings(ctx, forwardInput)
	default:
		err = ErrProviderInvalidConfig
	}
	if err != nil {
		return nil, fmt.Errorf("test provider capability: %w", err)
	}
	return &ProviderCapabilityTestResult{
		ProviderID: providerID, CapabilityID: capability.ID, Protocol: capability.Protocol,
		LogicalModel: model.Name, UpstreamModel: capability.UpstreamModel,
		StatusCode: result.StatusCode, Duration: result.Duration, UpstreamRequestID: result.UpstreamRequestID,
	}, nil
}

func (s *ProviderService) ListGroupCapabilities(ctx context.Context, groupID int64) ([]GroupProviderCapability, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	declared, err := s.repository.ListGroupCapabilities(ctx, groupID)
	if err != nil {
		return nil, err
	}
	nativeKeys := make(map[string]struct{}, len(declared))
	rows := make([]GroupProviderCapability, 0, len(declared))
	for _, capability := range declared {
		key := capability.LogicalModel.Name + "\x00" + string(capability.Capability.Protocol)
		nativeKeys[key] = struct{}{}
		rows = append(rows, groupCapabilityRow(capability, capability.Capability.Protocol, RouteTierNative, "", ""))
	}
	for _, capability := range declared {
		if capability.Profile == nil || !capability.Profile.AllowProtocolConversion || capability.Capability.Protocol == ProtocolEmbeddings {
			continue
		}
		for _, ingress := range []ProtocolFamily{ProtocolChatCompletions, ProtocolResponses, ProtocolAnthropicMessages} {
			if _, hasNative := nativeKeys[capability.LogicalModel.Name+"\x00"+string(ingress)]; hasNative {
				continue
			}
			if !s.adapters.HasDirection(apicompat.Protocol(ingress), apicompat.Protocol(capability.Capability.Protocol)) {
				continue
			}
			rows = append(rows, groupCapabilityRow(
				capability, ingress, RouteTierConversion,
				adapterDirectionName(ingress, capability.Capability.Protocol), adapterRegistryContractVersion,
			))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := fmt.Sprintf("%s/%s/%s/%020d", rows[i].LogicalModel, rows[i].IngressProtocol, rows[i].Tier, rows[i].ProviderID)
		right := fmt.Sprintf("%s/%s/%s/%020d", rows[j].LogicalModel, rows[j].IngressProtocol, rows[j].Tier, rows[j].ProviderID)
		return left < right
	})
	return rows, nil
}

func (s *ProviderService) ListGroupRouteSnapshots(ctx context.Context, groupID int64) ([]GroupRouteSnapshot, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.repository.ListGroupRouteSnapshots(ctx, groupID)
}

func (s *ProviderService) CreateGroupShadowSnapshot(ctx context.Context, groupID int64) (*GroupRouteSnapshot, error) {
	rows, err := s.ListGroupCapabilities(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, infraerrors.BadRequest("PROVIDER_ROUTE_SNAPSHOT_EMPTY", "group has no active provider route capabilities")
	}
	routes := make([]map[string]any, 0, len(rows))
	nativeCount, conversionCount := 0, 0
	for _, row := range rows {
		routes = append(routes, map[string]any{
			"provider_id": row.ProviderID, "logical_model": row.LogicalModel,
			"ingress_protocol": row.IngressProtocol, "upstream_protocol": row.UpstreamProtocol,
			"tier": row.Tier, "adapter": row.Adapter, "adapter_version": row.AdapterVersion,
			"route_identity": row.RouteIdentity,
		})
		if row.Tier == RouteTierNative {
			nativeCount++
		} else {
			conversionCount++
		}
	}
	manifest := map[string]any{"group_id": groupID, "routes": routes, "route_count": len(routes), "generated_at": time.Now().UTC().Format(time.RFC3339)}
	shadowDiff := map[string]any{
		"native_routes": nativeCount, "conversion_routes": conversionCount,
		"review_required": true,
		"note":            "legacy platform/type eligibility is intentionally not inferred; approve declared routes explicitly",
	}
	return s.repository.CreateGroupRouteSnapshot(ctx, groupID, manifest, shadowDiff)
}

func (s *ProviderService) ApproveGroupRouteSnapshot(ctx context.Context, groupID, version, reviewerID int64) (*GroupRouteSnapshot, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.repository.ApproveGroupRouteSnapshot(ctx, groupID, version, reviewerID)
}

func (s *ProviderService) ActivateGroupRouteSnapshot(ctx context.Context, groupID, version int64) (*GroupRouteCutover, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	result, err := s.repository.ActivateGroupRouteSnapshot(ctx, groupID, version)
	if err == nil && s.authCache != nil {
		s.authCache.InvalidateAuthCacheByGroupID(context.WithoutCancel(ctx), groupID)
	}
	return result, err
}

func (s *ProviderService) RollbackGroupRouteSnapshot(ctx context.Context, groupID int64) (*GroupRouteCutover, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	result, err := s.repository.RollbackGroupRouteSnapshot(ctx, groupID)
	if err == nil && s.authCache != nil {
		s.authCache.InvalidateAuthCacheByGroupID(context.WithoutCancel(ctx), groupID)
	}
	return result, err
}

func (s *ProviderService) encryptProviderAPIKey(apiKey string) (map[string]any, error) {
	credentials := map[string]any{}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return credentials, nil
	}
	if s.encryptor == nil {
		return nil, errors.New("provider credential encryption is not configured")
	}
	ciphertext, err := s.encryptor.Encrypt(apiKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt provider credential: %w", err)
	}
	credentials[providerAPIKeyCiphertextKey] = ciphertext
	return credentials, nil
}

func (s *ProviderService) replaceConfiguration(ctx context.Context, existing *ProviderAggregate, profile *ProviderProfile, input ProviderWriteInput) error {
	endpointByProtocol := make(map[ProtocolFamily]*ProviderProtocolEndpoint, len(input.Endpoints))
	for _, item := range input.Endpoints {
		wireProfile := item.WireProfile
		if wireProfile == "" {
			wireProfile = WireProfileCanonical
		}
		version := int64(1)
		if existing != nil {
			for i := range existing.Profile.Endpoints {
				if existing.Profile.Endpoints[i].Protocol == item.Protocol {
					version = existing.Profile.Endpoints[i].Version + 1
				}
			}
		}
		endpoint := &ProviderProtocolEndpoint{
			ProviderID: profile.ID, Protocol: item.Protocol, WireProfile: wireProfile,
			BaseURL: strings.TrimSpace(item.BaseURL), Path: strings.TrimSpace(item.Path), Headers: item.Headers,
			AuthType: strings.TrimSpace(item.AuthType), Enabled: item.Enabled, Version: version,
		}
		if err := s.repository.SaveEndpoint(ctx, endpoint); err != nil {
			return fmt.Errorf("save provider endpoint %s: %w", item.Protocol, err)
		}
		endpointByProtocol[item.Protocol] = endpoint
	}
	if existing != nil {
		for i := range existing.Profile.Endpoints {
			old := existing.Profile.Endpoints[i]
			if _, retained := endpointByProtocol[old.Protocol]; retained {
				continue
			}
			old.Enabled = false
			old.Version++
			if err := s.repository.SaveEndpoint(ctx, &old); err != nil {
				return fmt.Errorf("disable removed provider endpoint: %w", err)
			}
		}
	}

	retainedCapabilities := make(map[string]struct{}, len(input.Capabilities))
	for _, item := range input.Capabilities {
		model := &LogicalModel{
			Name: strings.TrimSpace(item.LogicalModelName), DisplayName: strings.TrimSpace(item.LogicalModelDisplayName),
			Enabled: true, Version: 1,
		}
		if existing != nil {
			for _, oldModel := range existing.LogicalModels {
				if strings.EqualFold(oldModel.Name, model.Name) {
					model.Version = oldModel.Version
					if model.DisplayName == "" {
						model.DisplayName = oldModel.DisplayName
					}
				}
			}
		}
		if err := s.repository.UpsertLogicalModel(ctx, model); err != nil {
			return fmt.Errorf("save logical model %s: %w", model.Name, err)
		}
		endpoint := endpointByProtocol[item.Protocol]
		if endpoint == nil || !endpoint.Enabled {
			return fmt.Errorf("%w: capability %s/%s requires an enabled endpoint", ErrProviderInvalidConfig, model.Name, item.Protocol)
		}
		wireProfile := item.WireProfile
		if wireProfile == "" {
			wireProfile = endpoint.WireProfile
		}
		version := int64(1)
		if existing != nil {
			for i := range existing.Profile.Capabilities {
				old := existing.Profile.Capabilities[i]
				oldModel := existing.LogicalModels[old.LogicalModelID]
				if strings.EqualFold(oldModel.Name, model.Name) && old.Protocol == item.Protocol {
					version = old.Version + 1
				}
			}
		}
		capability := &ProviderModelCapability{
			ProviderID: profile.ID, LogicalModelID: model.ID, EndpointID: &endpoint.ID,
			Protocol: item.Protocol, UpstreamModel: strings.TrimSpace(item.UpstreamModel),
			WireProfile: wireProfile, FeatureProfile: item.FeatureProfile,
			Enabled: item.Enabled, Version: version,
		}
		if err := s.repository.SaveCapability(ctx, capability); err != nil {
			return fmt.Errorf("save provider capability %s/%s: %w", model.Name, item.Protocol, err)
		}
		retainedCapabilities[providerCapabilityKey(model.Name, item.Protocol)] = struct{}{}
	}
	if existing != nil {
		for i := range existing.Profile.Capabilities {
			old := existing.Profile.Capabilities[i]
			model := existing.LogicalModels[old.LogicalModelID]
			if _, retained := retainedCapabilities[providerCapabilityKey(model.Name, old.Protocol)]; retained {
				continue
			}
			old.Enabled = false
			old.Version++
			if err := s.repository.SaveCapability(ctx, &old); err != nil {
				return fmt.Errorf("disable removed provider capability: %w", err)
			}
		}
	}
	return nil
}

func (s *ProviderService) ready() error {
	if s == nil || s.repository == nil || s.accountRepo == nil {
		return errors.New("provider service is not initialized")
	}
	return nil
}

func validateProviderWriteInput(input ProviderWriteInput, update bool) error {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.BaseURL) == "" {
		return fmt.Errorf("%w: name and base_url are required", ErrProviderInvalidConfig)
	}
	if input.Concurrency < 0 || (input.RateMultiplier != nil && *input.RateMultiplier < 0) {
		return fmt.Errorf("%w: concurrency and rate_multiplier cannot be negative", ErrProviderInvalidConfig)
	}
	if input.ClearAPIKey && input.APIKey != nil {
		return fmt.Errorf("%w: api_key and clear_api_key are mutually exclusive", ErrProviderInvalidConfig)
	}
	seenEndpoints := make(map[ProtocolFamily]struct{}, len(input.Endpoints))
	for _, endpoint := range input.Endpoints {
		if !endpoint.Protocol.IsValid() {
			return fmt.Errorf("%w: invalid endpoint protocol %q", ErrProviderInvalidConfig, endpoint.Protocol)
		}
		if _, duplicate := seenEndpoints[endpoint.Protocol]; duplicate {
			return fmt.Errorf("%w: duplicate endpoint protocol %q", ErrProviderInvalidConfig, endpoint.Protocol)
		}
		seenEndpoints[endpoint.Protocol] = struct{}{}
	}
	seenCapabilities := make(map[string]struct{}, len(input.Capabilities))
	for _, capability := range input.Capabilities {
		if strings.TrimSpace(capability.LogicalModelName) == "" || strings.TrimSpace(capability.UpstreamModel) == "" || !capability.Protocol.IsValid() || !capability.FeatureProfile.IsValid() {
			return fmt.Errorf("%w: capability model, upstream model, protocol and feature profile are required", ErrProviderInvalidConfig)
		}
		key := providerCapabilityKey(capability.LogicalModelName, capability.Protocol)
		if _, duplicate := seenCapabilities[key]; duplicate {
			return fmt.Errorf("%w: duplicate capability %s", ErrProviderInvalidConfig, key)
		}
		seenCapabilities[key] = struct{}{}
	}
	_ = update
	return nil
}

func validateProviderActivation(aggregate *ProviderAggregate) error {
	if aggregate == nil || aggregate.Profile == nil {
		return ErrProviderActivation
	}
	for _, capability := range aggregate.Profile.Capabilities {
		if !capability.Enabled || capability.Validate() != nil {
			continue
		}
		model, ok := aggregate.LogicalModels[capability.LogicalModelID]
		if !ok || !model.Enabled {
			continue
		}
		for _, endpoint := range aggregate.Profile.Endpoints {
			if endpoint.Enabled && endpoint.Protocol == capability.Protocol && (capability.EndpointID == nil || endpoint.ID == *capability.EndpointID) {
				return nil
			}
		}
	}
	return ErrProviderActivation
}

func selectProviderCapability(aggregate *ProviderAggregate, input ProviderCapabilityTestInput) (ProviderModelCapability, *ProviderProtocolEndpoint, LogicalModel, error) {
	for _, capability := range aggregate.Profile.Capabilities {
		model := aggregate.LogicalModels[capability.LogicalModelID]
		if input.CapabilityID > 0 && capability.ID != input.CapabilityID {
			continue
		}
		if input.Protocol != "" && capability.Protocol != input.Protocol {
			continue
		}
		if strings.TrimSpace(input.LogicalModel) != "" && !strings.EqualFold(model.Name, strings.TrimSpace(input.LogicalModel)) {
			continue
		}
		if !capability.Enabled || !model.Enabled {
			continue
		}
		for i := range aggregate.Profile.Endpoints {
			endpoint := &aggregate.Profile.Endpoints[i]
			if endpoint.Enabled && endpoint.Protocol == capability.Protocol && (capability.EndpointID == nil || endpoint.ID == *capability.EndpointID) {
				return capability, endpoint, model, nil
			}
		}
	}
	return ProviderModelCapability{}, nil, LogicalModel{}, infraerrors.BadRequest("PROVIDER_CAPABILITY_NOT_FOUND", "no enabled matching capability was found")
}

func providerCapabilityTestBody(protocol ProtocolFamily, logicalModel string) []byte {
	model := strings.ReplaceAll(logicalModel, `"`, "")
	switch protocol {
	case ProtocolChatCompletions:
		return []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, model))
	case ProtocolResponses:
		return []byte(fmt.Sprintf(`{"model":%q,"input":"ping","max_output_tokens":1}`, model))
	case ProtocolAnthropicMessages:
		return []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, model))
	case ProtocolEmbeddings:
		return []byte(fmt.Sprintf(`{"model":%q,"input":"ping"}`, model))
	default:
		return nil
	}
}

func groupCapabilityRow(capability ProviderRouteCapability, ingress ProtocolFamily, tier RouteTier, adapter, version string) GroupProviderCapability {
	providerID, name := capability.Capability.ProviderID, ""
	if capability.Profile != nil {
		providerID, name = capability.Profile.ID, capability.Profile.Name
	}
	identity := NewRouteIdentity(capability, ingress, adapter, version)
	return GroupProviderCapability{
		ProviderID: providerID, ProviderName: name, LogicalModel: capability.LogicalModel.Name,
		IngressProtocol: ingress, UpstreamProtocol: capability.Capability.Protocol,
		Tier: tier, Adapter: adapter, AdapterVersion: version, GroupPriority: capability.GroupPriority,
		RouteIdentity: identity.String(),
	}
}

func providerCapabilityKey(model string, protocol ProtocolFamily) string {
	return strings.ToLower(strings.TrimSpace(model)) + "\x00" + string(protocol)
}

func uniquePositiveIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func cloneProviderAccount(account *Account) *Account {
	if account == nil {
		return &Account{}
	}
	cloned := *account
	cloned.Credentials = make(map[string]any, len(account.Credentials))
	for key, value := range account.Credentials {
		cloned.Credentials[key] = value
	}
	return &cloned
}

func translateProviderLookupError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrProviderNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
		return infraerrors.NotFound("PROVIDER_NOT_FOUND", "provider not found")
	}
	return err
}
