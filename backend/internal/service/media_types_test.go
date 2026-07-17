package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaTaskStatusCanTransitionTo(t *testing.T) {
	tests := []struct {
		from MediaTaskStatus
		to   MediaTaskStatus
		want bool
	}{
		{MediaTaskStatusQueued, MediaTaskStatusInProgress, true},
		{MediaTaskStatusInProgress, MediaTaskStatusCompleted, true},
		{MediaTaskStatusInProgress, MediaTaskStatusFailed, true},
		{MediaTaskStatusCompleted, MediaTaskStatusInProgress, false},
		{MediaTaskStatusFailed, MediaTaskStatusQueued, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, tt.from.CanTransitionTo(tt.to))
	}
}

func TestMediaSpecValidateEnforcesGlobalHardLimitsWithoutModelConstraints(t *testing.T) {
	tests := []struct {
		name      string
		mediaType MediaType
		spec      MediaSpec
		wantErr   bool
	}{
		{name: "image boundary", mediaType: MediaTypeImage, spec: MediaSpec{Image: &ImageSpec{Prompt: strings.Repeat("图", 32000), Count: 16}}},
		{name: "image count too high", mediaType: MediaTypeImage, spec: MediaSpec{Image: &ImageSpec{Prompt: "cat", Count: 17}}, wantErr: true},
		{name: "image prompt too long", mediaType: MediaTypeImage, spec: MediaSpec{Image: &ImageSpec{Prompt: strings.Repeat("p", 32001), Count: 1}}, wantErr: true},
		{name: "video boundary", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: strings.Repeat("片", 32000), DurationSeconds: 600, FPS: 120}}},
		{name: "video zero duration", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: "cat", FPS: 24}}, wantErr: true},
		{name: "video negative duration", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: "cat", DurationSeconds: -1, FPS: 24}}, wantErr: true},
		{name: "video duration too high", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: "cat", DurationSeconds: 601, FPS: 24}}, wantErr: true},
		{name: "video zero fps", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: "cat", DurationSeconds: 5}}, wantErr: true},
		{name: "video negative fps", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: "cat", DurationSeconds: 5, FPS: -1}}, wantErr: true},
		{name: "video fps too high", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: "cat", DurationSeconds: 5, FPS: 121}}, wantErr: true},
		{name: "video prompt too long", mediaType: MediaTypeVideo, spec: MediaSpec{Video: &VideoSpec{Prompt: strings.Repeat("p", 32001), DurationSeconds: 5, FPS: 24}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate(tt.mediaType)
			require.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestMediaTaskStatusIsTerminal(t *testing.T) {
	require.False(t, MediaTaskStatusQueued.IsTerminal())
	require.False(t, MediaTaskStatusInProgress.IsTerminal())
	require.True(t, MediaTaskStatusCompleted.IsTerminal())
	require.True(t, MediaTaskStatusFailed.IsTerminal())
}

func TestNormalizeNativeAsyncMode(t *testing.T) {
	require.Equal(t, NativeAsyncUnsupported, NormalizeNativeAsyncMode(""))
	require.Equal(t, NativeAsyncOptional, NormalizeNativeAsyncMode("OPTIONAL"))
	require.Equal(t, NativeAsyncRequired, NormalizeNativeAsyncMode(" required "))
	require.Equal(t, NativeAsyncUnsupported, NormalizeNativeAsyncMode("invalid"))
}

func TestMediaSpecValidateRequiresMatchingExclusiveSpec(t *testing.T) {
	validImage := &ImageSpec{Prompt: "cat", Count: 1}
	validVideo := &VideoSpec{Prompt: "sunset", DurationSeconds: 5, FPS: 24}
	tests := []struct {
		name      string
		mediaType MediaType
		spec      MediaSpec
		wantErr   bool
	}{
		{"image", MediaTypeImage, MediaSpec{Image: validImage}, false},
		{"video", MediaTypeVideo, MediaSpec{Video: validVideo}, false},
		{"both", MediaTypeImage, MediaSpec{Image: validImage, Video: validVideo}, true},
		{"wrong_type", MediaTypeVideo, MediaSpec{Image: validImage}, true},
		{"empty_prompt", MediaTypeImage, MediaSpec{Image: &ImageSpec{Count: 1}}, true},
		{"zero_count", MediaTypeImage, MediaSpec{Image: &ImageSpec{Prompt: "cat"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate(tt.mediaType)
			require.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestMediaTaskStageCanTransitionTo(t *testing.T) {
	require.True(t, MediaTaskStageQueued.CanTransitionTo(MediaTaskStageScheduling))
	require.True(t, MediaTaskStageScheduling.CanTransitionTo(MediaTaskStageSubmitting))
	require.True(t, MediaTaskStagePolling.CanTransitionTo(MediaTaskStageStoring))
	require.True(t, MediaTaskStageSettling.CanTransitionTo(MediaTaskStageCompleted))
	require.False(t, MediaTaskStageCompleted.CanTransitionTo(MediaTaskStagePolling))
}
