package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrMediaModelNotFound               = errors.New("media model not found")
	ErrMediaOperationUnsupported        = errors.New("media operation unsupported")
	ErrMediaCapabilityUnsupported       = errors.New("media capability unsupported")
	ErrMediaSpecOutsideModelConstraints = errors.New("media spec outside model constraints")
	ErrMediaModelAdapterUnavailable     = infraerrors.ServiceUnavailable(
		"MEDIA_MODEL_ADAPTER_UNAVAILABLE",
		"media model adapter is unavailable",
	)
)

type MediaModelDefinition struct {
	ID                int64
	ModelID           string
	Vendor            string
	MediaType         MediaType
	Operations        []MediaOperation
	Constraints       json.RawMessage
	BillingUnit       string
	DefaultAdapter    string
	DefaultAsyncMode  NativeAsyncMode
	AdapterResolution MediaAdapterResolution `json:"-"`
	Enabled           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type MediaModelConstraints struct {
	ImageSizes         []string `json:"image_sizes,omitempty"`
	MaxImageCount      int      `json:"max_image_count,omitempty"`
	VideoDurations     []int    `json:"video_durations,omitempty"`
	VideoResolutions   []string `json:"video_resolutions,omitempty"`
	MinFPS             int      `json:"min_fps,omitempty"`
	MaxFPS             int      `json:"max_fps,omitempty"`
	MaxReferenceImages int      `json:"max_reference_images,omitempty"`
}

func (d MediaModelDefinition) Supports(operation MediaOperation) bool {
	for _, candidate := range d.Operations {
		if candidate == operation {
			return true
		}
	}
	return false
}

type MediaModelDefinitionRepository interface {
	ListEnabled(ctx context.Context) ([]MediaModelDefinition, error)
}

type MediaModelAlias struct {
	RequestedModelID  string
	ModelDefinitionID int64
}

type MediaModelAliasRepository interface {
	ListAll(ctx context.Context) ([]MediaModelAlias, error)
}

type mediaModelUnavailableTombstone struct {
	CanonicalModelID string
	Resolution       MediaAdapterResolution
}

type mediaModelRegistrySnapshot struct {
	models             map[string]MediaModelDefinition
	aliases            map[string]string
	unavailableModels  map[string]mediaModelUnavailableTombstone
	unavailableAliases map[string]string
}

type MediaModelRegistry struct {
	repo           MediaModelDefinitionRepository
	aliasRepo      MediaModelAliasRepository
	resolver       *MediaAdapterResolver
	routingMetrics MediaRoutingMetrics
	logger         *slog.Logger
	snapshot       atomic.Value // mediaModelRegistrySnapshot
	refreshMu      sync.Mutex
	observerMu     sync.RWMutex
	startOnce      sync.Once
}

func NewMediaModelRegistry(repo MediaModelDefinitionRepository, aliasRepos ...MediaModelAliasRepository) *MediaModelRegistry {
	return NewMediaModelRegistryWithResolver(repo, NewMediaAdapterResolver(NewMediaAdapterRegistry()), aliasRepos...)
}

func NewMediaModelRegistryWithResolver(
	repo MediaModelDefinitionRepository,
	resolver *MediaAdapterResolver,
	aliasRepos ...MediaModelAliasRepository,
) *MediaModelRegistry {
	if resolver == nil {
		resolver = NewMediaAdapterResolver(NewMediaAdapterRegistry())
	}
	registry := &MediaModelRegistry{repo: repo, resolver: resolver, logger: slog.Default()}
	if len(aliasRepos) > 0 {
		registry.aliasRepo = aliasRepos[0]
	} else if aliasRepo, ok := repo.(MediaModelAliasRepository); ok {
		// The production composition root supplies the model repository as the
		// sole constructor argument. Reuse its alias capability when available so
		// aliases cannot be silently skipped by that wiring path.
		registry.aliasRepo = aliasRepo
	}
	registry.snapshot.Store(mediaModelRegistrySnapshot{
		models:             map[string]MediaModelDefinition{},
		aliases:            map[string]string{},
		unavailableModels:  map[string]mediaModelUnavailableTombstone{},
		unavailableAliases: map[string]string{},
	})
	return registry
}

func (r *MediaModelRegistry) Refresh(ctx context.Context) error {
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	if r.repo == nil {
		return errors.New("media model repository is nil")
	}
	definitions, err := r.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	aliases, err := r.listAliases(ctx)
	if err != nil {
		return err
	}

	// Validate the repository's global identity contract before resolving any
	// individual model. A duplicate canonical ID/definition ID, or a disabled
	// row returned by ListEnabled, invalidates the entire refresh and must leave
	// the previous snapshot untouched.
	normalizedDefinitions := make([]MediaModelDefinition, len(definitions))
	allCanonicalIDs := make(map[string]struct{}, len(definitions))
	ids := make(map[int64]string, len(definitions))
	for index, definition := range definitions {
		definition.ModelID = normalizeMediaModelID(definition.ModelID)
		definition.Vendor = strings.ToLower(strings.TrimSpace(definition.Vendor))
		definition.BillingUnit = strings.ToLower(strings.TrimSpace(definition.BillingUnit))
		definition.Operations = append([]MediaOperation(nil), definition.Operations...)
		definition.Constraints = append(json.RawMessage(nil), definition.Constraints...)
		definition.AdapterResolution = cloneMediaAdapterResolution(definition.AdapterResolution)
		if _, exists := allCanonicalIDs[definition.ModelID]; exists {
			return fmt.Errorf("duplicate media model id %q", definition.ModelID)
		}
		allCanonicalIDs[definition.ModelID] = struct{}{}
		if definition.ID > 0 {
			if _, exists := ids[definition.ID]; exists {
				return fmt.Errorf("duplicate media model definition id %d", definition.ID)
			}
			ids[definition.ID] = definition.ModelID
		}
		normalizedDefinitions[index] = definition
	}

	items := make(map[string]MediaModelDefinition, len(definitions))
	unavailableItems := make(map[string]mediaModelUnavailableTombstone)
	for index, definition := range normalizedDefinitions {
		if !definition.Enabled {
			return fmt.Errorf("validate media model definition at index %d: disabled model returned by enabled model repository", index)
		}
	}
	for index, definition := range normalizedDefinitions {
		resolution := r.resolver.ResolveDefinition(definition)
		switch resolution.Status {
		case MediaAdapterResolutionReady:
			if !resolution.IsReady() {
				return fmt.Errorf("resolve media model definition at index %d (%q): ready resolution has no capabilities", index, definition.ModelID)
			}
			definition.AdapterResolution = cloneMediaAdapterResolution(resolution)
			definition.DefaultAdapter = resolution.ResolvedAdapter
			definition.DefaultAsyncMode = resolution.CompatibilityAsyncMode()
			items[definition.ModelID] = definition
			continue
		case MediaAdapterResolutionImplementationMissing:
			r.observeUnavailableResolution(definition, resolution)
			return fmt.Errorf(
				"resolve media model definition at index %d (%q): implementation_missing for adapter %q",
				index,
				definition.ModelID,
				resolution.ResolvedAdapter,
			)
		case MediaAdapterResolutionInvalidDefinition,
			MediaAdapterResolutionUnresolved,
			MediaAdapterResolutionAmbiguous,
			MediaAdapterResolutionCapabilityMismatch:
			r.observeUnavailableResolution(definition, resolution)
		default:
			return fmt.Errorf(
				"resolve media model definition at index %d (%q): unsupported resolution status %q",
				index,
				definition.ModelID,
				resolution.Status,
			)
		}
		unavailableItems[definition.ModelID] = mediaModelUnavailableTombstone{
			CanonicalModelID: definition.ModelID,
			Resolution:       cloneMediaAdapterResolution(resolution),
		}
	}

	aliasItems := make(map[string]string, len(aliases))
	unavailableAliasItems := make(map[string]string)
	allAliases := make(map[string]struct{}, len(aliases))
	for index, alias := range aliases {
		requestedModelID := normalizeMediaModelID(alias.RequestedModelID)
		if !isValidMediaModelIdentifier(requestedModelID) {
			return fmt.Errorf("media model alias at index %d has invalid requested model id", index)
		}
		if _, exists := allAliases[requestedModelID]; exists {
			return fmt.Errorf("duplicate media model alias %q", requestedModelID)
		}
		allAliases[requestedModelID] = struct{}{}
		if _, exists := allCanonicalIDs[requestedModelID]; exists {
			return fmt.Errorf("media model alias %q conflicts with a canonical model id", requestedModelID)
		}
		canonicalModelID, exists := ids[alias.ModelDefinitionID]
		if !exists {
			return fmt.Errorf("media model alias %q references unavailable model definition %d", requestedModelID, alias.ModelDefinitionID)
		}
		if _, ready := items[canonicalModelID]; ready {
			aliasItems[requestedModelID] = canonicalModelID
			continue
		}
		if _, unavailable := unavailableItems[canonicalModelID]; unavailable {
			unavailableAliasItems[requestedModelID] = canonicalModelID
			continue
		}
		return fmt.Errorf("media model alias %q references model definition %d without a routing state", requestedModelID, alias.ModelDefinitionID)
	}
	r.snapshot.Store(mediaModelRegistrySnapshot{
		models:             items,
		aliases:            aliasItems,
		unavailableModels:  unavailableItems,
		unavailableAliases: unavailableAliasItems,
	})
	return nil
}

func (r *MediaModelRegistry) SetRoutingMetrics(metrics MediaRoutingMetrics) {
	if r == nil {
		return
	}
	r.observerMu.Lock()
	r.routingMetrics = metrics
	r.observerMu.Unlock()
}

func (r *MediaModelRegistry) SetLogger(logger *slog.Logger) {
	if r == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	r.observerMu.Lock()
	r.logger = logger
	r.observerMu.Unlock()
}

func (r *MediaModelRegistry) observeUnavailableResolution(
	definition MediaModelDefinition,
	resolution MediaAdapterResolution,
) {
	r.observerMu.RLock()
	routingMetrics := r.routingMetrics
	logger := r.logger
	r.observerMu.RUnlock()
	if routingMetrics != nil {
		routingMetrics.IncrementAdapterResolutionFailure(resolution.Status)
	}
	if logger == nil {
		logger = slog.Default()
	}
	attributes := []any{
		"canonical_model_id", definition.ModelID,
		"vendor", definition.Vendor,
		"adapter_resolution_status", string(resolution.Status),
		"adapter_key", resolution.ResolvedAdapter,
		"matched_by", string(resolution.MatchedBy),
		"matched_family", resolution.MatchedFamily,
		"reason_code", resolution.ReasonCode,
	}
	if resolution.Status == MediaAdapterResolutionImplementationMissing {
		logger.Error("media_model_adapter_resolution_unavailable", attributes...)
		return
	}
	logger.Warn("media_model_adapter_resolution_unavailable", attributes...)
}

// StartPeriodicRefresh keeps every instance eventually consistent after an
// administrator changes the database-backed registry. Direct writes still
// refresh the serving instance synchronously; this loop is the multi-instance
// and transient-failure safety net.
func (r *MediaModelRegistry) StartPeriodicRefresh(ctx context.Context, interval time.Duration) {
	if r == nil || interval <= 0 {
		return
	}
	r.startOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), interval)
					err := r.Refresh(refreshCtx)
					cancel()
					if err != nil {
						slog.Warn("media_model_registry_periodic_refresh_failed", "error", err)
					}
				}
			}
		}()
	})
}

func (r *MediaModelRegistry) Resolve(model string, operation MediaOperation) (*MediaModelDefinition, error) {
	snapshot := r.snapshot.Load().(mediaModelRegistrySnapshot)
	modelID := normalizeMediaModelID(model)
	if alias, ok := snapshot.aliases[modelID]; ok {
		modelID = alias
	} else if alias, ok := snapshot.unavailableAliases[modelID]; ok {
		return nil, mediaModelUnavailableError(snapshot.unavailableModels[alias])
	}
	definition, ok := snapshot.models[modelID]
	if !ok {
		if tombstone, unavailable := snapshot.unavailableModels[modelID]; unavailable {
			return nil, mediaModelUnavailableError(tombstone)
		}
		return nil, ErrMediaModelNotFound
	}
	if !definition.Enabled {
		return nil, ErrMediaModelNotFound
	}
	if !definition.Supports(operation) {
		return nil, ErrMediaOperationUnsupported
	}
	copy := cloneMediaModelDefinition(definition)
	return &copy, nil
}

// CanonicalModelID resolves a public model or global alias without requiring
// an operation. Schedulers use this to apply group model scopes before the
// operation-specific capability check performed during selection.
func (r *MediaModelRegistry) CanonicalModelID(model string) (string, error) {
	snapshot := r.snapshot.Load().(mediaModelRegistrySnapshot)
	modelID := normalizeMediaModelID(model)
	if alias, ok := snapshot.aliases[modelID]; ok {
		modelID = alias
	} else if alias, ok := snapshot.unavailableAliases[modelID]; ok {
		return "", mediaModelUnavailableError(snapshot.unavailableModels[alias])
	}
	definition, ok := snapshot.models[modelID]
	if !ok {
		if tombstone, unavailable := snapshot.unavailableModels[modelID]; unavailable {
			return "", mediaModelUnavailableError(tombstone)
		}
		return "", ErrMediaModelNotFound
	}
	if !definition.Enabled {
		return "", ErrMediaModelNotFound
	}
	return definition.ModelID, nil
}

func (r *MediaModelRegistry) definitionByID(modelID string) (*MediaModelDefinition, error) {
	snapshot := r.snapshot.Load().(mediaModelRegistrySnapshot)
	canonicalModelID := normalizeMediaModelID(modelID)
	definition, ok := snapshot.models[canonicalModelID]
	if !ok {
		if tombstone, unavailable := snapshot.unavailableModels[canonicalModelID]; unavailable {
			return nil, mediaModelUnavailableError(tombstone)
		}
		return nil, ErrMediaModelNotFound
	}
	if !definition.Enabled {
		return nil, ErrMediaModelNotFound
	}
	copy := cloneMediaModelDefinition(definition)
	return &copy, nil
}

func mediaModelUnavailableError(tombstone mediaModelUnavailableTombstone) error {
	return ErrMediaModelAdapterUnavailable.WithMetadata(map[string]string{
		"model_id":          tombstone.CanonicalModelID,
		"resolution_status": string(tombstone.Resolution.Status),
		"reason_code":       tombstone.Resolution.ReasonCode,
	})
}

func (r *MediaModelRegistry) ResolveRouteRequest(request MediaRouteRequest) (*MediaModelDefinition, error) {
	definition, err := r.Resolve(request.RequestedModel, request.Operation)
	if err != nil {
		return nil, err
	}
	if request.Capability != definition.MediaType {
		return nil, ErrMediaCapabilityUnsupported
	}
	return definition, nil
}

func (r *MediaModelRegistry) listAliases(ctx context.Context) ([]MediaModelAlias, error) {
	if r.aliasRepo == nil {
		return nil, nil
	}
	aliases, err := r.aliasRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list media model aliases: %w", err)
	}
	return aliases, nil
}

func (r *MediaModelRegistry) ValidateSpec(model string, operation MediaOperation, spec MediaSpec) error {
	definition, err := r.Resolve(model, operation)
	if err != nil {
		return err
	}
	return validateMediaSpecAgainstDefinition(*definition, spec)
}

func validateMediaSpecAgainstDefinition(definition MediaModelDefinition, spec MediaSpec) error {
	constraints, err := decodeMediaModelConstraints(definition.Constraints)
	if err != nil {
		return err
	}
	switch definition.MediaType {
	case MediaTypeImage:
		if spec.Image == nil || spec.Video != nil {
			return ErrMediaSpecOutsideModelConstraints
		}
		if spec.Image.Size != "" && len(constraints.ImageSizes) > 0 && !containsString(constraints.ImageSizes, spec.Image.Size) {
			return ErrMediaSpecOutsideModelConstraints
		}
		if constraints.MaxImageCount > 0 && spec.Image.Count > constraints.MaxImageCount {
			return ErrMediaSpecOutsideModelConstraints
		}
		if constraints.MaxReferenceImages > 0 && len(spec.Image.InputArtifactIDs) > constraints.MaxReferenceImages {
			return ErrMediaSpecOutsideModelConstraints
		}
	case MediaTypeVideo:
		if spec.Video == nil || spec.Image != nil {
			return ErrMediaSpecOutsideModelConstraints
		}
		if spec.Video.DurationSeconds != 0 && len(constraints.VideoDurations) > 0 && !containsInt(constraints.VideoDurations, spec.Video.DurationSeconds) {
			return ErrMediaSpecOutsideModelConstraints
		}
		if spec.Video.Resolution != "" && len(constraints.VideoResolutions) > 0 && !containsString(constraints.VideoResolutions, spec.Video.Resolution) {
			return ErrMediaSpecOutsideModelConstraints
		}
		if spec.Video.FPS != 0 && constraints.MinFPS > 0 && spec.Video.FPS < constraints.MinFPS {
			return ErrMediaSpecOutsideModelConstraints
		}
		if spec.Video.FPS != 0 && constraints.MaxFPS > 0 && spec.Video.FPS > constraints.MaxFPS {
			return ErrMediaSpecOutsideModelConstraints
		}
		if constraints.MaxReferenceImages > 0 && len(spec.Video.ReferenceArtifactIDs) > constraints.MaxReferenceImages {
			return ErrMediaSpecOutsideModelConstraints
		}
	default:
		return ErrMediaSpecOutsideModelConstraints
	}
	return nil
}

func validateMediaModelDefinitionBase(definition MediaModelDefinition) error {
	if !isValidMediaModelIdentifier(definition.ModelID) {
		return errors.New("media model id has invalid format")
	}
	if !isValidMediaSimpleIdentifier(definition.Vendor, 64) {
		return errors.New("media model vendor has invalid format")
	}
	if !isValidMediaSimpleIdentifier(definition.BillingUnit, 32) {
		return errors.New("media model billing unit has invalid format")
	}
	return validateMediaModelShapeAndConstraints(definition)
}

func validateMediaModelShapeAndConstraints(definition MediaModelDefinition) error {
	if definition.MediaType != MediaTypeImage && definition.MediaType != MediaTypeVideo {
		return fmt.Errorf("unsupported media type %q", definition.MediaType)
	}
	if len(definition.Operations) == 0 {
		return errors.New("operations are empty")
	}
	seenOperations := make(map[MediaOperation]struct{}, len(definition.Operations))
	for _, operation := range definition.Operations {
		mediaType, ok := mediaTypeForOperation(operation)
		if !ok {
			return fmt.Errorf("unsupported media operation %q", operation)
		}
		if mediaType != definition.MediaType {
			return fmt.Errorf("media operation %q does not match media type %q", operation, definition.MediaType)
		}
		if _, exists := seenOperations[operation]; exists {
			return fmt.Errorf("duplicate media operation %q", operation)
		}
		seenOperations[operation] = struct{}{}
	}

	constraints, err := decodeMediaModelConstraints(definition.Constraints)
	if err != nil {
		return err
	}
	return validateMediaModelConstraints(definition.MediaType, constraints)
}

func validateEnabledMediaModelDefinition(definition MediaModelDefinition) error {
	if !definition.Enabled {
		return errors.New("disabled model returned by enabled model repository")
	}
	return validateMediaModelDefinitionBase(definition)
}

// validateMediaModelDefinition remains as a compatibility wrapper while the
// worker/admin call sites migrate to the base validator in later tasks.
func validateMediaModelDefinition(definition MediaModelDefinition) error {
	return validateEnabledMediaModelDefinition(definition)
}

// ResolveDefinition is the only model-definition resolution entrypoint. It
// keeps base shape validation consistent across serving and admin diagnosis,
// without requiring the definition to be enabled.
func (r *MediaAdapterResolver) ResolveDefinition(definition MediaModelDefinition) MediaAdapterResolution {
	if err := validateMediaModelDefinitionBase(definition); err != nil {
		return mediaAdapterResolutionFailure(
			MediaAdapterResolutionInvalidDefinition,
			"MEDIA_MODEL_DEFINITION_INVALID",
		)
	}
	return r.Resolve(definition.Vendor, definition.ModelID, definition.Operations)
}

func decodeMediaModelConstraints(raw json.RawMessage) (MediaModelConstraints, error) {
	var constraints MediaModelConstraints
	if len(raw) == 0 {
		return constraints, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&constraints); err != nil {
		return MediaModelConstraints{}, normalizeMediaModelConstraintsJSONError("", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return MediaModelConstraints{}, errors.New("decode media model constraints: multiple top-level JSON values")
		}
		return MediaModelConstraints{}, normalizeMediaModelConstraintsJSONError("trailing data", err)
	}
	return constraints, nil
}

func normalizeMediaModelConstraintsJSONError(category string, err error) error {
	prefix := "decode media model constraints"
	if category != "" {
		prefix += " " + category
	}

	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		field := typeError.Field
		if field == "" {
			field = "<root>"
		}
		expectedType := "valid JSON type"
		if typeError.Type != nil {
			expectedType = typeError.Type.String()
		}
		return fmt.Errorf("%s: type mismatch for field %q (expected %s, offset %d)", prefix, field, expectedType, typeError.Offset)
	}

	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return fmt.Errorf("%s: invalid JSON syntax at offset %d", prefix, syntaxError.Offset)
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New(prefix + ": unexpected end of JSON input")
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return errors.New(prefix + ": unknown field")
	}
	return errors.New(prefix + ": invalid JSON")
}

func validateMediaModelConstraints(mediaType MediaType, constraints MediaModelConstraints) error {
	if constraints.MaxImageCount < 0 || constraints.MinFPS < 0 || constraints.MaxFPS < 0 || constraints.MaxReferenceImages < 0 {
		return errors.New("media model constraints cannot be negative")
	}
	if constraints.MinFPS > 0 && constraints.MaxFPS > 0 && constraints.MinFPS > constraints.MaxFPS {
		return errors.New("minimum fps exceeds maximum fps")
	}
	for _, size := range constraints.ImageSizes {
		if strings.TrimSpace(size) == "" {
			return errors.New("image size constraint is empty")
		}
	}
	for _, duration := range constraints.VideoDurations {
		if duration <= 0 {
			return errors.New("video duration constraint must be positive")
		}
	}
	for _, resolution := range constraints.VideoResolutions {
		if strings.TrimSpace(resolution) == "" {
			return errors.New("video resolution constraint is empty")
		}
	}
	if mediaType == MediaTypeImage && (len(constraints.VideoDurations) > 0 || len(constraints.VideoResolutions) > 0 || constraints.MinFPS > 0 || constraints.MaxFPS > 0) {
		return errors.New("video constraints cannot be used by an image model")
	}
	if mediaType == MediaTypeVideo && (len(constraints.ImageSizes) > 0 || constraints.MaxImageCount > 0) {
		return errors.New("image constraints cannot be used by a video model")
	}
	return nil
}

func mediaTypeForOperation(operation MediaOperation) (MediaType, bool) {
	switch operation {
	case MediaOperationTextToImage, MediaOperationImageToImage, MediaOperationImageEdit:
		return MediaTypeImage, true
	case MediaOperationTextToVideo, MediaOperationImageToVideo, MediaOperationReferenceVideo, MediaOperationVideoExtend, MediaOperationVideoRemix:
		return MediaTypeVideo, true
	default:
		return "", false
	}
}

func cloneMediaModelDefinition(definition MediaModelDefinition) MediaModelDefinition {
	definition.Operations = append([]MediaOperation(nil), definition.Operations...)
	definition.Constraints = append(json.RawMessage(nil), definition.Constraints...)
	definition.AdapterResolution = cloneMediaAdapterResolution(definition.AdapterResolution)
	return definition
}

func cloneMediaAdapterResolution(input MediaAdapterResolution) MediaAdapterResolution {
	copy := input
	if input.Capabilities != nil {
		capabilities := *input.Capabilities
		capabilities.Operations = append([]MediaOperation(nil), input.Capabilities.Operations...)
		copy.Capabilities = &capabilities
	}
	return copy
}

func normalizeMediaModelID(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

func isValidMediaAdapterName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsInt(items []int, target int) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
