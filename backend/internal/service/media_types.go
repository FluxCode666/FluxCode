package service

import (
	"errors"
	"slices"
	"strings"
)

var ErrInvalidMediaSpec = errors.New("invalid media spec")

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

type MediaTaskStage string

const (
	MediaTaskStageQueued     MediaTaskStage = "queued"
	MediaTaskStageScheduling MediaTaskStage = "scheduling"
	MediaTaskStageSubmitting MediaTaskStage = "submitting"
	MediaTaskStageGenerating MediaTaskStage = "generating"
	MediaTaskStagePolling    MediaTaskStage = "polling"
	MediaTaskStageStoring    MediaTaskStage = "storing"
	MediaTaskStageSettling   MediaTaskStage = "settling"
	MediaTaskStageCompleted  MediaTaskStage = "completed"
	MediaTaskStageFailed     MediaTaskStage = "failed"
)

func (s MediaTaskStage) CanTransitionTo(next MediaTaskStage) bool {
	allowed := map[MediaTaskStage][]MediaTaskStage{
		MediaTaskStageQueued:     {MediaTaskStageScheduling, MediaTaskStageFailed},
		MediaTaskStageScheduling: {MediaTaskStageSubmitting, MediaTaskStageFailed},
		MediaTaskStageSubmitting: {MediaTaskStageGenerating, MediaTaskStagePolling, MediaTaskStageFailed},
		MediaTaskStageGenerating: {MediaTaskStageStoring, MediaTaskStageFailed},
		MediaTaskStagePolling:    {MediaTaskStageStoring, MediaTaskStageFailed},
		MediaTaskStageStoring:    {MediaTaskStageSettling, MediaTaskStageFailed},
		MediaTaskStageSettling:   {MediaTaskStageCompleted, MediaTaskStageFailed},
	}
	return slices.Contains(allowed[s], next)
}

type MediaOperation string

const (
	MediaOperationTextToImage    MediaOperation = "text_to_image"
	MediaOperationImageToImage   MediaOperation = "image_to_image"
	MediaOperationImageEdit      MediaOperation = "image_edit"
	MediaOperationTextToVideo    MediaOperation = "text_to_video"
	MediaOperationImageToVideo   MediaOperation = "image_to_video"
	MediaOperationReferenceVideo MediaOperation = "reference_to_video"
	MediaOperationVideoExtend    MediaOperation = "video_extend"
	MediaOperationVideoRemix     MediaOperation = "video_remix"
)

type MediaTaskStatus string

const (
	MediaTaskStatusQueued     MediaTaskStatus = "queued"
	MediaTaskStatusInProgress MediaTaskStatus = "in_progress"
	MediaTaskStatusCompleted  MediaTaskStatus = "completed"
	MediaTaskStatusFailed     MediaTaskStatus = "failed"
)

func (s MediaTaskStatus) CanTransitionTo(next MediaTaskStatus) bool {
	switch s {
	case MediaTaskStatusQueued:
		return next == MediaTaskStatusInProgress || next == MediaTaskStatusFailed
	case MediaTaskStatusInProgress:
		return next == MediaTaskStatusCompleted || next == MediaTaskStatusFailed
	default:
		return false
	}
}

func (s MediaTaskStatus) IsTerminal() bool {
	return s == MediaTaskStatusCompleted || s == MediaTaskStatusFailed
}

type NativeAsyncMode string

const (
	NativeAsyncUnsupported NativeAsyncMode = "unsupported"
	NativeAsyncOptional    NativeAsyncMode = "optional"
	NativeAsyncRequired    NativeAsyncMode = "required"
)

func NormalizeNativeAsyncMode(raw string) NativeAsyncMode {
	switch NativeAsyncMode(strings.ToLower(strings.TrimSpace(raw))) {
	case NativeAsyncOptional:
		return NativeAsyncOptional
	case NativeAsyncRequired:
		return NativeAsyncRequired
	default:
		return NativeAsyncUnsupported
	}
}

type ImageSpec struct {
	Prompt           string  `json:"prompt"`
	Size             string  `json:"size,omitempty"`
	Quality          string  `json:"quality,omitempty"`
	OutputFormat     string  `json:"output_format,omitempty"`
	ResponseFormat   string  `json:"response_format,omitempty"`
	Count            int     `json:"n"`
	InputArtifactIDs []int64 `json:"input_artifact_ids,omitempty"`
}

type VideoSpec struct {
	Prompt               string  `json:"prompt"`
	DurationSeconds      int     `json:"duration_seconds,omitempty"`
	Resolution           string  `json:"resolution,omitempty"`
	FPS                  int     `json:"fps,omitempty"`
	ReferenceArtifactIDs []int64 `json:"reference_artifact_ids,omitempty"`
	SourceArtifactID     *int64  `json:"source_artifact_id,omitempty"`
}

type MediaSpec struct {
	Image *ImageSpec `json:"image,omitempty"`
	Video *VideoSpec `json:"video,omitempty"`
}

func (s MediaSpec) Validate(mediaType MediaType) error {
	if mediaType != MediaTypeImage && mediaType != MediaTypeVideo {
		return ErrInvalidMediaSpec
	}
	if (s.Image == nil) == (s.Video == nil) {
		return ErrInvalidMediaSpec
	}
	if mediaType == MediaTypeImage && s.Image == nil {
		return ErrInvalidMediaSpec
	}
	if mediaType == MediaTypeVideo && s.Video == nil {
		return ErrInvalidMediaSpec
	}
	if s.Image != nil && (strings.TrimSpace(s.Image.Prompt) == "" || s.Image.Count < 1) {
		return ErrInvalidMediaSpec
	}
	if s.Video != nil && strings.TrimSpace(s.Video.Prompt) == "" {
		return ErrInvalidMediaSpec
	}
	return nil
}
