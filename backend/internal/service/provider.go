package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProtocolFamily is the client-visible and upstream protocol family used by
// provider routing. It deliberately excludes vendor/platform identity.
type ProtocolFamily string

const (
	ProtocolChatCompletions   ProtocolFamily = "chat_completions"
	ProtocolResponses         ProtocolFamily = "responses"
	ProtocolAnthropicMessages ProtocolFamily = "anthropic_messages"
	ProtocolEmbeddings        ProtocolFamily = "embeddings"
)

func (p ProtocolFamily) IsValid() bool {
	switch p {
	case ProtocolChatCompletions, ProtocolResponses, ProtocolAnthropicMessages, ProtocolEmbeddings:
		return true
	default:
		return false
	}
}

func (p ProtocolFamily) IsConversational() bool {
	return p == ProtocolChatCompletions || p == ProtocolResponses || p == ProtocolAnthropicMessages
}

func (p ProtocolFamily) DefaultPath() string {
	switch p {
	case ProtocolChatCompletions:
		return "/v1/chat/completions"
	case ProtocolResponses:
		return "/v1/responses"
	case ProtocolAnthropicMessages:
		return "/v1/messages"
	case ProtocolEmbeddings:
		return "/v1/embeddings"
	default:
		return ""
	}
}

type WireProfile string

const (
	WireProfileCanonical           WireProfile = "canonical_v1"
	WireProfileNewAPIMessages      WireProfile = "newapi_messages_v1"
	WireProfileSiliconFlowMessages WireProfile = "siliconflow_messages_v1"
)

func (p WireProfile) IsValid() bool {
	switch p {
	case WireProfileCanonical, WireProfileNewAPIMessages, WireProfileSiliconFlowMessages:
		return true
	default:
		return false
	}
}

type ProviderFeatureProfile string

const (
	FeatureProfileText       ProviderFeatureProfile = "text_v1"
	FeatureProfileStreamText ProviderFeatureProfile = "stream_text_v1"
	FeatureProfileTools      ProviderFeatureProfile = "function_tools_v1"
	FeatureProfileEmbeddings ProviderFeatureProfile = "embeddings_v1"
)

func (p ProviderFeatureProfile) IsValid() bool {
	switch p {
	case FeatureProfileText, FeatureProfileStreamText, FeatureProfileTools, FeatureProfileEmbeddings:
		return true
	default:
		return false
	}
}

type ProviderStatus string

const (
	ProviderStatusDraft          ProviderStatus = "draft"
	ProviderStatusActive         ProviderStatus = "active"
	ProviderStatusDisabled       ProviderStatus = "disabled"
	ProviderStatusReviewRequired ProviderStatus = "review_required"
)

func (s ProviderStatus) IsValid() bool {
	switch s {
	case ProviderStatusDraft, ProviderStatusActive, ProviderStatusDisabled, ProviderStatusReviewRequired:
		return true
	default:
		return false
	}
}

// ProviderProfile is a one-to-one extension of Account. ID is intentionally
// the backing account ID so existing scheduling, usage and billing references
// remain stable throughout the migration.
type ProviderProfile struct {
	ID                      int64
	Name                    string
	Status                  ProviderStatus
	AllowProtocolConversion bool
	Connection              ProviderConnectionConfig
	Version                 int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
	Endpoints               []ProviderProtocolEndpoint
	Capabilities            []ProviderModelCapability
}

func NewProviderProfile(accountID int64, name string) *ProviderProfile {
	now := time.Now()
	return &ProviderProfile{
		ID:                      accountID,
		Name:                    strings.TrimSpace(name),
		Status:                  ProviderStatusDraft,
		AllowProtocolConversion: false,
		Version:                 1,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
}

func (p *ProviderProfile) Validate() error {
	if p == nil {
		return errors.New("provider profile is nil")
	}
	if p.ID <= 0 {
		return errors.New("provider id must be positive")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("provider name is required")
	}
	if !p.Status.IsValid() {
		return fmt.Errorf("invalid provider status %q", p.Status)
	}
	if p.Version <= 0 {
		return errors.New("provider version must be positive")
	}
	if p.Status == ProviderStatusActive {
		hasNative := false
		for i := range p.Capabilities {
			if p.Capabilities[i].Enabled && p.Capabilities[i].Validate() == nil {
				hasNative = true
				break
			}
		}
		if !hasNative {
			return errors.New("active provider requires at least one valid native capability")
		}
	}
	return nil
}

type ProviderConnectionConfig struct {
	BaseURL  string
	Headers  map[string]string
	AuthType string
}

type ProviderProtocolEndpoint struct {
	ID          int64
	ProviderID  int64
	Protocol    ProtocolFamily
	WireProfile WireProfile
	BaseURL     string
	Path        string
	Headers     map[string]string
	AuthType    string
	Enabled     bool
	Version     int64
}

type LogicalModel struct {
	ID          int64
	Name        string
	DisplayName string
	Enabled     bool
	Version     int64
}

func (m LogicalModel) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("logical model name is required")
	}
	if m.Version <= 0 {
		return errors.New("logical model version must be positive")
	}
	return nil
}

type ProviderModelCapability struct {
	ID                  int64
	ProviderID          int64
	LogicalModelID      int64
	Protocol            ProtocolFamily
	UpstreamModel       string
	WireProfile         WireProfile
	FeatureProfile      ProviderFeatureProfile
	EndpointID          *int64
	Enabled             bool
	Version             int64
	LegacyCompatibility bool
}

func (c ProviderModelCapability) Validate() error {
	if c.ProviderID <= 0 {
		return errors.New("provider id must be positive")
	}
	if c.LogicalModelID <= 0 {
		return errors.New("logical model id must be positive")
	}
	if !c.Protocol.IsValid() {
		return fmt.Errorf("invalid protocol family %q", c.Protocol)
	}
	if strings.TrimSpace(c.UpstreamModel) == "" {
		return errors.New("upstream model is required")
	}
	if c.WireProfile == "" {
		c.WireProfile = WireProfileCanonical
	}
	if !c.WireProfile.IsValid() {
		return fmt.Errorf("invalid wire profile %q", c.WireProfile)
	}
	if !c.FeatureProfile.IsValid() {
		return fmt.Errorf("invalid feature profile %q", c.FeatureProfile)
	}
	if c.Protocol == ProtocolEmbeddings && c.FeatureProfile != FeatureProfileEmbeddings {
		return errors.New("embeddings protocol requires the embeddings feature profile")
	}
	if c.Protocol.IsConversational() && c.FeatureProfile == FeatureProfileEmbeddings {
		return errors.New("conversational protocols cannot use the embeddings feature profile")
	}
	if c.Version <= 0 {
		return errors.New("capability version must be positive")
	}
	return nil
}

type ProviderAggregate struct {
	Profile       *ProviderProfile
	Account       *Account
	LogicalModels map[int64]LogicalModel
}

type ProviderRouteCapability struct {
	Profile       *ProviderProfile
	Account       *Account
	Endpoint      *ProviderProtocolEndpoint
	LogicalModel  LogicalModel
	Capability    ProviderModelCapability
	GroupPriority int
}

type ProviderCapabilityFilter struct {
	GroupID         int64
	SnapshotVersion int64
	LogicalModel    string
	Protocol        ProtocolFamily
	IngressProtocol ProtocolFamily
	OnlySchedulable bool
}

type ProviderRepository interface {
	SaveProfile(ctx context.Context, profile *ProviderProfile) error
	SaveEndpoint(ctx context.Context, endpoint *ProviderProtocolEndpoint) error
	SaveCapability(ctx context.Context, capability *ProviderModelCapability) error
	UpsertLogicalModel(ctx context.Context, model *LogicalModel) error
	GetByID(ctx context.Context, providerID int64) (*ProviderAggregate, error)
	ListRouteCapabilities(ctx context.Context, filter ProviderCapabilityFilter) ([]ProviderRouteCapability, error)
}

// ProviderAdminRepository extends the hot-path repository with management
// projections. Keeping these methods separate lets routing tests use small
// fail-closed stubs without depending on admin-only queries.
type ProviderAdminRepository interface {
	ProviderRepository
	List(ctx context.Context) ([]*ProviderAggregate, error)
	ListGroupCapabilities(ctx context.Context, groupID int64) ([]ProviderRouteCapability, error)
	UpdateProfileIfVersion(ctx context.Context, profile *ProviderProfile, expectedVersion int64) error
	ListGroupRouteSnapshots(ctx context.Context, groupID int64) ([]GroupRouteSnapshot, error)
	CreateGroupRouteSnapshot(ctx context.Context, groupID int64, manifest, shadowDiff map[string]any) (*GroupRouteSnapshot, error)
	ApproveGroupRouteSnapshot(ctx context.Context, groupID, version, reviewerID int64) (*GroupRouteSnapshot, error)
	ActivateGroupRouteSnapshot(ctx context.Context, groupID, version int64) (*GroupRouteCutover, error)
	RollbackGroupRouteSnapshot(ctx context.Context, groupID int64) (*GroupRouteCutover, error)
}

type GroupRouteSnapshot struct {
	ID         int64
	GroupID    int64
	Version    int64
	Status     string
	Manifest   map[string]any
	ShadowDiff map[string]any
	ApprovedBy *int64
	ApprovedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type GroupRouteCutover struct {
	GroupID         int64
	ActiveVersion   int64
	PreviousVersion int64
}

func (a *ProviderAggregate) Validate() error {
	if a == nil || a.Profile == nil || a.Account == nil {
		return errors.New("provider aggregate requires profile and account")
	}
	if a.Profile.ID != a.Account.ID {
		return errors.New("provider id must match backing account id")
	}
	return a.Profile.Validate()
}
