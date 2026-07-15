package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrMediaModelNotFound               = errors.New("media model not found")
	ErrMediaOperationUnsupported        = errors.New("media operation unsupported")
	ErrMediaSpecOutsideModelConstraints = errors.New("media spec outside model constraints")
)

type MediaModelDefinition struct {
	ID          int64
	ModelID     string
	MediaType   MediaType
	Operations  []MediaOperation
	Constraints json.RawMessage
	BillingUnit string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

type MediaModelRegistry struct {
	repo     MediaModelDefinitionRepository
	snapshot atomic.Value // map[string]MediaModelDefinition
}

func NewMediaModelRegistry(repo MediaModelDefinitionRepository) *MediaModelRegistry {
	registry := &MediaModelRegistry{repo: repo}
	registry.snapshot.Store(map[string]MediaModelDefinition{})
	return registry
}

func (r *MediaModelRegistry) Refresh(ctx context.Context) error {
	if r.repo == nil {
		return errors.New("media model repository is nil")
	}
	definitions, err := r.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	items := make(map[string]MediaModelDefinition, len(definitions))
	for index, definition := range definitions {
		definition.ModelID = normalizeMediaModelID(definition.ModelID)
		definition.Operations = append([]MediaOperation(nil), definition.Operations...)
		definition.Constraints = append(json.RawMessage(nil), definition.Constraints...)
		if err := validateMediaModelDefinition(definition); err != nil {
			return fmt.Errorf("validate media model definition at index %d: %w", index, err)
		}
		if _, exists := items[definition.ModelID]; exists {
			return fmt.Errorf("duplicate media model id %q", definition.ModelID)
		}
		items[definition.ModelID] = definition
	}
	r.snapshot.Store(items)
	return nil
}

func (r *MediaModelRegistry) Resolve(model string, operation MediaOperation) (*MediaModelDefinition, error) {
	items := r.snapshot.Load().(map[string]MediaModelDefinition)
	definition, ok := items[normalizeMediaModelID(model)]
	if !ok || !definition.Enabled {
		return nil, ErrMediaModelNotFound
	}
	if !definition.Supports(operation) {
		return nil, ErrMediaOperationUnsupported
	}
	copy := cloneMediaModelDefinition(definition)
	return &copy, nil
}

func (r *MediaModelRegistry) ValidateSpec(model string, operation MediaOperation, spec MediaSpec) error {
	definition, err := r.Resolve(model, operation)
	if err != nil {
		return err
	}
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

func validateMediaModelDefinition(definition MediaModelDefinition) error {
	if definition.ModelID == "" {
		return errors.New("model id is empty")
	}
	if !definition.Enabled {
		return errors.New("disabled model returned by enabled model repository")
	}
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

func decodeMediaModelConstraints(raw json.RawMessage) (MediaModelConstraints, error) {
	var constraints MediaModelConstraints
	if len(raw) == 0 {
		return constraints, nil
	}
	if err := json.Unmarshal(raw, &constraints); err != nil {
		return MediaModelConstraints{}, fmt.Errorf("decode media model constraints: %w", err)
	}
	return constraints, nil
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
	return definition
}

func normalizeMediaModelID(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
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
