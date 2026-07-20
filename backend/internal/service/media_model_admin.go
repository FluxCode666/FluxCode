package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const mediaModelRegistryRefreshTimeout = 5 * time.Second

var (
	ErrMediaModelDefinitionNotFound = infraerrors.NotFound("MEDIA_MODEL_NOT_FOUND", "media model not found")
	ErrMediaModelIDConflict         = infraerrors.Conflict("MEDIA_MODEL_ID_CONFLICT", "media model id is already in use")
	ErrMediaModelIDImmutable        = infraerrors.BadRequest("MEDIA_MODEL_ID_IMMUTABLE", "media model id cannot be changed")
	ErrMediaModelAliasConflict      = infraerrors.Conflict("MEDIA_MODEL_ALIAS_CONFLICT", "media model alias is already in use")
	ErrMediaModelScopeModelNotFound = infraerrors.BadRequest("MEDIA_MODEL_SCOPE_MODEL_NOT_FOUND", "media model scope contains an unknown or disabled model")
	ErrMediaGroupRequired           = infraerrors.BadRequest("MEDIA_GROUP_REQUIRED", "media model scopes are only available for media groups")
)

// MediaModelAdminRecord is the management representation of one canonical
// media model and its aliases. Aliases are persisted atomically with the
// definition by MediaModelAdminRepository.
type MediaModelAdminRecord struct {
	Definition MediaModelDefinition
	Aliases    []string
}

// MediaModelAdminRepository owns transactional CRUD for definitions and
// aliases. Inputs have already been normalized and validated by the service.
type MediaModelAdminRepository interface {
	ListAdmin(ctx context.Context) ([]MediaModelAdminRecord, error)
	GetAdminByID(ctx context.Context, id int64) (*MediaModelAdminRecord, error)
	CreateAdmin(ctx context.Context, record MediaModelAdminRecord) (*MediaModelAdminRecord, error)
	UpdateAdmin(ctx context.Context, id int64, record MediaModelAdminRecord) (*MediaModelAdminRecord, error)
	DeleteAdmin(ctx context.Context, id int64) error
}

type MediaModelAdminService struct {
	models   MediaModelAdminRepository
	scopes   GroupMediaModelScopeRepository
	groups   GroupRepository
	registry *MediaModelRegistry
}

func NewMediaModelAdminService(
	models MediaModelAdminRepository,
	scopes GroupMediaModelScopeRepository,
	groups GroupRepository,
	registry *MediaModelRegistry,
) *MediaModelAdminService {
	return &MediaModelAdminService{models: models, scopes: scopes, groups: groups, registry: registry}
}

func (s *MediaModelAdminService) List(ctx context.Context) ([]MediaModelAdminRecord, error) {
	if s == nil || s.models == nil {
		return nil, fmt.Errorf("media model admin repository is nil")
	}
	items, err := s.models.ListAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("list media models: %w", err)
	}
	if items == nil {
		items = []MediaModelAdminRecord{}
	}
	return items, nil
}

func (s *MediaModelAdminService) GetByID(ctx context.Context, id int64) (*MediaModelAdminRecord, error) {
	if id <= 0 {
		return nil, invalidMediaModelInput("media model id must be positive")
	}
	if s == nil || s.models == nil {
		return nil, fmt.Errorf("media model admin repository is nil")
	}
	item, err := s.models.GetAdminByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get media model: %w", err)
	}
	return item, nil
}

func (s *MediaModelAdminService) Create(ctx context.Context, input MediaModelAdminRecord) (*MediaModelAdminRecord, error) {
	normalized, err := normalizeAndValidateMediaModelAdminRecord(input)
	if err != nil {
		return nil, err
	}
	if s == nil || s.models == nil {
		return nil, fmt.Errorf("media model admin repository is nil")
	}
	created, err := s.models.CreateAdmin(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("create media model: %w", err)
	}
	s.refreshRegistryAfterCommit(ctx)
	return created, nil
}

func (s *MediaModelAdminService) Update(ctx context.Context, id int64, input MediaModelAdminRecord) (*MediaModelAdminRecord, error) {
	if id <= 0 {
		return nil, invalidMediaModelInput("media model id must be positive")
	}
	normalized, err := normalizeAndValidateMediaModelAdminRecord(input)
	if err != nil {
		return nil, err
	}
	if s == nil || s.models == nil {
		return nil, fmt.Errorf("media model admin repository is nil")
	}
	existing, err := s.models.GetAdminByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get media model before update: %w", err)
	}
	if existing == nil {
		return nil, ErrMediaModelDefinitionNotFound
	}
	if normalizeMediaModelID(existing.Definition.ModelID) != normalized.Definition.ModelID {
		return nil, ErrMediaModelIDImmutable
	}
	updated, err := s.models.UpdateAdmin(ctx, id, normalized)
	if err != nil {
		return nil, fmt.Errorf("update media model: %w", err)
	}
	s.refreshRegistryAfterCommit(ctx)
	return updated, nil
}

func (s *MediaModelAdminService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return invalidMediaModelInput("media model id must be positive")
	}
	if s == nil || s.models == nil {
		return fmt.Errorf("media model admin repository is nil")
	}
	if err := s.models.DeleteAdmin(ctx, id); err != nil {
		return fmt.Errorf("delete media model: %w", err)
	}
	s.refreshRegistryAfterCommit(ctx)
	return nil
}

func (s *MediaModelAdminService) GetGroupScopes(ctx context.Context, groupID int64) ([]string, error) {
	if err := s.requireMediaGroup(ctx, groupID); err != nil {
		return nil, err
	}
	if s.scopes == nil {
		return nil, fmt.Errorf("group media model scope repository is nil")
	}
	// Disabled definitions cannot be written back by ReplaceGroupScopes and are
	// ignored by the scheduler, so the management API only exposes effective
	// grants. This prevents an invisible disabled scope from making later edits
	// fail validation.
	modelIDs, err := s.scopes.ListEnabledMediaModelIDs(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group media model scopes: %w", err)
	}
	if modelIDs == nil {
		modelIDs = []string{}
	}
	return modelIDs, nil
}

func (s *MediaModelAdminService) ReplaceGroupScopes(ctx context.Context, groupID int64, modelIDs []string) ([]string, error) {
	if err := s.requireMediaGroup(ctx, groupID); err != nil {
		return nil, err
	}
	normalized, err := normalizeMediaModelScopeIDs(modelIDs)
	if err != nil {
		return nil, err
	}
	if s.scopes == nil {
		return nil, fmt.Errorf("group media model scope repository is nil")
	}
	if err := s.scopes.ReplaceMediaModelScopes(ctx, groupID, normalized); err != nil {
		return nil, fmt.Errorf("replace group media model scopes: %w", err)
	}
	return s.GetGroupScopes(ctx, groupID)
}

func (s *MediaModelAdminService) requireMediaGroup(ctx context.Context, groupID int64) error {
	if groupID <= 0 {
		return invalidMediaModelInput("group id must be positive")
	}
	if s == nil || s.groups == nil {
		return fmt.Errorf("group repository is nil")
	}
	group, err := s.groups.GetByIDLite(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get group for media model scopes: %w", err)
	}
	if group == nil {
		return ErrGroupNotFound
	}
	if group.Platform != PlatformMedia {
		return ErrMediaGroupRequired
	}
	return nil
}

func (s *MediaModelAdminService) refreshRegistry(ctx context.Context) error {
	if s == nil || s.registry == nil {
		return fmt.Errorf("media model registry is nil")
	}
	// The database write has already committed. Detach client cancellation so
	// a disconnect in this narrow window cannot leave this process serving a
	// stale registry indefinitely; retain request values and apply a hard bound.
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaModelRegistryRefreshTimeout)
	defer cancel()
	if err := s.registry.Refresh(refreshCtx); err != nil {
		return fmt.Errorf("refresh media model registry: %w", err)
	}
	return nil
}

func (s *MediaModelAdminService) refreshRegistryAfterCommit(ctx context.Context) {
	if err := s.refreshRegistry(ctx); err != nil {
		// The database mutation has committed and cannot truthfully be reported as
		// failed. Keep the API result successful; the periodic registry refresher
		// retries this process and every other instance.
		slog.Error("media_model_registry_refresh_after_commit_failed", "error", err)
	}
}

func normalizeAndValidateMediaModelAdminRecord(input MediaModelAdminRecord) (MediaModelAdminRecord, error) {
	definition := input.Definition
	definition.ID = 0
	definition.ModelID = normalizeMediaModelID(definition.ModelID)
	definition.Vendor = strings.ToLower(strings.TrimSpace(definition.Vendor))
	definition.MediaType = MediaType(strings.ToLower(strings.TrimSpace(string(definition.MediaType))))
	definition.BillingUnit = strings.ToLower(strings.TrimSpace(definition.BillingUnit))
	definition.DefaultAdapter = normalizeMediaAdapterName(definition.DefaultAdapter)
	definition.DefaultAsyncMode = NativeAsyncMode(strings.ToLower(strings.TrimSpace(string(definition.DefaultAsyncMode))))
	definition.Operations = normalizeMediaOperations(definition.Operations)

	if !isValidMediaModelIdentifier(definition.ModelID) {
		return MediaModelAdminRecord{}, invalidMediaModelInput("model_id has invalid format")
	}
	if !isValidMediaSimpleIdentifier(definition.Vendor, 64) {
		return MediaModelAdminRecord{}, invalidMediaModelInput("vendor has invalid format")
	}
	if !isValidMediaSimpleIdentifier(definition.BillingUnit, 32) {
		return MediaModelAdminRecord{}, invalidMediaModelInput("billing_unit has invalid format")
	}
	if !isValidMediaAdapterName(definition.DefaultAdapter) || len(definition.DefaultAdapter) > 64 {
		return MediaModelAdminRecord{}, invalidMediaModelInput("default_adapter has invalid format")
	}

	constraints, err := normalizeMediaModelConstraints(definition.Constraints)
	if err != nil {
		return MediaModelAdminRecord{}, invalidMediaModelInput(err.Error())
	}
	definition.Constraints = constraints

	// Registry validation intentionally rejects disabled records because its
	// repository only returns enabled rows. Validate the same routing metadata
	// here while preserving the requested enabled state for persistence.
	validationDefinition := definition
	validationDefinition.Enabled = true
	if err := validateMediaModelDefinition(validationDefinition); err != nil {
		return MediaModelAdminRecord{}, invalidMediaModelInput(err.Error())
	}

	aliases, err := normalizeMediaAliases(input.Aliases, definition.ModelID)
	if err != nil {
		return MediaModelAdminRecord{}, err
	}
	return MediaModelAdminRecord{Definition: definition, Aliases: aliases}, nil
}

func normalizeMediaOperations(operations []MediaOperation) []MediaOperation {
	normalized := make([]MediaOperation, len(operations))
	for index, operation := range operations {
		normalized[index] = MediaOperation(strings.ToLower(strings.TrimSpace(string(operation))))
	}
	return normalized
}

func normalizeMediaModelConstraints(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte(`{}`)
	}
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return nil, fmt.Errorf("constraints must be a JSON object")
	}
	_, err := decodeMediaModelConstraints(trimmed)
	if err != nil {
		return nil, err
	}
	compact := &bytes.Buffer{}
	if err := json.Compact(compact, trimmed); err != nil {
		return nil, fmt.Errorf("constraints contain invalid JSON")
	}
	return append(json.RawMessage(nil), compact.Bytes()...), nil
}

func normalizeMediaAliases(aliases []string, canonicalModelID string) ([]string, error) {
	if aliases == nil {
		return []string{}, nil
	}
	normalized := make([]string, 0, len(aliases))
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		alias = normalizeMediaModelID(alias)
		if !isValidMediaModelIdentifier(alias) {
			return nil, invalidMediaModelInput("alias has invalid format")
		}
		if alias == canonicalModelID {
			return nil, invalidMediaModelInput("alias cannot equal model_id")
		}
		if _, exists := seen[alias]; exists {
			return nil, invalidMediaModelInput("aliases must be unique")
		}
		seen[alias] = struct{}{}
		normalized = append(normalized, alias)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeMediaModelScopeIDs(modelIDs []string) ([]string, error) {
	if modelIDs == nil {
		return []string{}, nil
	}
	normalized := make([]string, 0, len(modelIDs))
	seen := make(map[string]struct{}, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = normalizeMediaModelID(modelID)
		if !isValidMediaModelIdentifier(modelID) {
			return nil, invalidMediaModelInput("model_ids contains an invalid model id")
		}
		if _, exists := seen[modelID]; exists {
			return nil, invalidMediaModelInput("model_ids must be unique")
		}
		seen[modelID] = struct{}{}
		normalized = append(normalized, modelID)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func isValidMediaModelIdentifier(value string) bool {
	if value == "" || len(value) > 128 || !isLowerAlphaNumeric(rune(value[0])) {
		return false
	}
	for _, r := range value {
		if isLowerAlphaNumeric(r) || r == '-' || r == '_' || r == '.' || r == '/' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func isValidMediaSimpleIdentifier(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength || !isLowerAlphaNumeric(rune(value[0])) {
		return false
	}
	for _, r := range value {
		if isLowerAlphaNumeric(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func isLowerAlphaNumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func invalidMediaModelInput(message string) error {
	return infraerrors.BadRequest("INVALID_MEDIA_MODEL", message)
}
