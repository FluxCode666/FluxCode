package service

import (
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
	validVideo := &VideoSpec{Prompt: "sunset"}
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
