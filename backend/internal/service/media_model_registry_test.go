package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaModelRepoStub struct {
	items []MediaModelDefinition
	err   error
}

func (s *mediaModelRepoStub) ListEnabled(context.Context) ([]MediaModelDefinition, error) {
	return s.items, s.err
}

func validImageModelDefinition() MediaModelDefinition {
	return MediaModelDefinition{
		ModelID:     "fake-image",
		MediaType:   MediaTypeImage,
		Operations:  []MediaOperation{MediaOperationTextToImage, MediaOperationImageToImage},
		Constraints: json.RawMessage(`{}`),
		Enabled:     true,
	}
}

func validVideoModelDefinition() MediaModelDefinition {
	return MediaModelDefinition{
		ModelID:     "fake-video",
		MediaType:   MediaTypeVideo,
		Operations:  []MediaOperation{MediaOperationTextToVideo, MediaOperationImageToVideo},
		Constraints: json.RawMessage(`{}`),
		Enabled:     true,
	}
}

func TestMediaModelRegistryValidateOperation(t *testing.T) {
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{{
		ModelID:     "fake-image",
		MediaType:   MediaTypeImage,
		Operations:  []MediaOperation{MediaOperationTextToImage},
		Constraints: json.RawMessage(`{"image_sizes":["1024x1024"],"max_image_count":2}`),
		Enabled:     true,
	}}}
	registry := NewMediaModelRegistry(repo)
	require.NoError(t, registry.Refresh(context.Background()))

	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	_, err = registry.Resolve("fake-image", MediaOperationTextToVideo)
	require.ErrorIs(t, err, ErrMediaOperationUnsupported)
	_, err = registry.Resolve("missing", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
	err = registry.ValidateSpec("fake-image", MediaOperationTextToImage, MediaSpec{Image: &ImageSpec{
		Prompt: "cat", Size: "2048x2048", Count: 1,
	}})
	require.ErrorIs(t, err, ErrMediaSpecOutsideModelConstraints)
}

func TestMediaModelDefinitionSupportsExactOperation(t *testing.T) {
	definition := validImageModelDefinition()
	require.True(t, definition.Supports(MediaOperationTextToImage))
	require.False(t, definition.Supports(MediaOperationTextToVideo))
}

func TestMediaModelRegistryRefreshNormalizesAndReplacesSnapshot(t *testing.T) {
	definition := validImageModelDefinition()
	definition.ModelID = "  Fake-Image  "
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := NewMediaModelRegistry(repo)

	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
	require.NoError(t, registry.Refresh(context.Background()))
	resolved, err := registry.Resolve("  FAKE-IMAGE ", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, "fake-image", resolved.ModelID)

	repo.items = nil
	require.NoError(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve("fake-image", MediaOperationTextToImage)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
}

func TestMediaModelRegistryRefreshPreservesSnapshotOnFailure(t *testing.T) {
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{validImageModelDefinition()}}
	registry := NewMediaModelRegistry(repo)
	require.NoError(t, registry.Refresh(context.Background()))

	repo.err = errors.New("database unavailable")
	require.Error(t, registry.Refresh(context.Background()))
	_, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)

	repo.err = nil
	invalid := validVideoModelDefinition()
	invalid.Constraints = json.RawMessage(`{"min_fps":30,"max_fps":24}`)
	repo.items = []MediaModelDefinition{invalid}
	require.Error(t, registry.Refresh(context.Background()))
	_, err = registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	_, err = registry.Resolve("fake-video", MediaOperationTextToVideo)
	require.ErrorIs(t, err, ErrMediaModelNotFound)
}

func TestMediaModelRegistryRefreshRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name       string
		definition MediaModelDefinition
	}{
		{name: "empty model id", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.ModelID = "  "
			return definition
		}()},
		{name: "disabled model", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Enabled = false
			return definition
		}()},
		{name: "unknown media type", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.MediaType = MediaType("audio")
			return definition
		}()},
		{name: "no operations", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Operations = nil
			return definition
		}()},
		{name: "unknown operation", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Operations = []MediaOperation{"unknown"}
			return definition
		}()},
		{name: "operation mismatches media type", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Operations = []MediaOperation{MediaOperationTextToVideo}
			return definition
		}()},
		{name: "malformed constraints", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Constraints = json.RawMessage(`{`)
			return definition
		}()},
		{name: "negative constraint", definition: func() MediaModelDefinition {
			definition := validImageModelDefinition()
			definition.Constraints = json.RawMessage(`{"max_image_count":-1}`)
			return definition
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{tt.definition}})
			require.Error(t, registry.Refresh(context.Background()))
		})
	}

	first := validImageModelDefinition()
	first.ModelID = "fake-image"
	second := validImageModelDefinition()
	second.ModelID = "  FAKE-IMAGE "
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{first, second}})
	require.Error(t, registry.Refresh(context.Background()))
}

func TestMediaModelRegistryResolveReturnsDetachedDefinition(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Constraints = json.RawMessage(`{"image_sizes":["1024x1024"]}`)
	repo := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := NewMediaModelRegistry(repo)
	require.NoError(t, registry.Refresh(context.Background()))

	repo.items[0].Operations[0] = MediaOperationTextToVideo
	repo.items[0].Constraints[0] = '['
	resolved, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	resolved.Operations[0] = MediaOperationTextToVideo
	resolved.Constraints[0] = '['

	resolvedAgain, err := registry.Resolve("fake-image", MediaOperationTextToImage)
	require.NoError(t, err)
	require.Equal(t, MediaOperationTextToImage, resolvedAgain.Operations[0])
	require.JSONEq(t, `{"image_sizes":["1024x1024"]}`, string(resolvedAgain.Constraints))
}

func TestMediaModelRegistryValidateImageConstraints(t *testing.T) {
	definition := validImageModelDefinition()
	definition.Constraints = json.RawMessage(`{
		"image_sizes":["1024x1024"],
		"max_image_count":2,
		"max_reference_images":1
	}`)
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{definition}})
	require.NoError(t, registry.Refresh(context.Background()))

	tests := []struct {
		name string
		spec MediaSpec
		err  error
	}{
		{name: "allowed", spec: MediaSpec{Image: &ImageSpec{Size: "1024x1024", Count: 2, InputArtifactIDs: []int64{1}}}},
		{name: "unspecified optional size", spec: MediaSpec{Image: &ImageSpec{Count: 1}}},
		{name: "size", spec: MediaSpec{Image: &ImageSpec{Size: "2048x2048", Count: 1}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "count", spec: MediaSpec{Image: &ImageSpec{Size: "1024x1024", Count: 3}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "references", spec: MediaSpec{Image: &ImageSpec{Size: "1024x1024", Count: 1, InputArtifactIDs: []int64{1, 2}}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "wrong spec kind", spec: MediaSpec{Video: &VideoSpec{}}, err: ErrMediaSpecOutsideModelConstraints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSpec("fake-image", MediaOperationTextToImage, tt.spec)
			if tt.err == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestMediaModelRegistryValidateVideoConstraints(t *testing.T) {
	definition := validVideoModelDefinition()
	definition.Constraints = json.RawMessage(`{
		"video_durations":[5,10],
		"video_resolutions":["720p"],
		"min_fps":24,
		"max_fps":30,
		"max_reference_images":2
	}`)
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{definition}})
	require.NoError(t, registry.Refresh(context.Background()))

	tests := []struct {
		name string
		spec MediaSpec
		err  error
	}{
		{name: "allowed", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "720p", FPS: 24, ReferenceArtifactIDs: []int64{1, 2}}}},
		{name: "unspecified optional fields", spec: MediaSpec{Video: &VideoSpec{}}},
		{name: "duration", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 6, Resolution: "720p", FPS: 24}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "resolution", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "1080p", FPS: 24}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "fps below minimum", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "720p", FPS: 23}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "fps above maximum", spec: MediaSpec{Video: &VideoSpec{DurationSeconds: 5, Resolution: "720p", FPS: 31}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "references", spec: MediaSpec{Video: &VideoSpec{ReferenceArtifactIDs: []int64{1, 2, 3}}}, err: ErrMediaSpecOutsideModelConstraints},
		{name: "wrong spec kind", spec: MediaSpec{Image: &ImageSpec{}}, err: ErrMediaSpecOutsideModelConstraints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSpec("fake-video", MediaOperationTextToVideo, tt.spec)
			if tt.err == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestMediaModelRegistryConstraintFieldsAreOptional(t *testing.T) {
	image := validImageModelDefinition()
	video := validVideoModelDefinition()
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{image, video}})
	require.NoError(t, registry.Refresh(context.Background()))

	require.NoError(t, registry.ValidateSpec("fake-image", MediaOperationTextToImage, MediaSpec{Image: &ImageSpec{
		Size: "unrestricted", Count: 100, InputArtifactIDs: []int64{1, 2, 3},
	}}))
	require.NoError(t, registry.ValidateSpec("fake-video", MediaOperationTextToVideo, MediaSpec{Video: &VideoSpec{
		DurationSeconds: 999, Resolution: "unrestricted", FPS: 999, ReferenceArtifactIDs: []int64{1, 2, 3},
	}}))
}
