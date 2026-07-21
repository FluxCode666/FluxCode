package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaRequestMappingApplyOperationsAndNestedPaths(t *testing.T) {
	mapping := MediaRequestMapping{Rules: []MediaMappingRule{
		{Operation: "rename", Source: "input.prompt", Target: "payload.text"},
		{Operation: "copy", Source: "input.style", Target: "payload.style"},
		{Operation: "default", Target: "payload.count", Value: 1},
		{Operation: "enum", Source: "input.quality", Target: "input.quality", Values: map[string]string{"hd": "high"}},
		{Operation: "cast", Source: "input.seed", Target: "payload.seed", Cast: "integer"},
	}}
	request := map[string]any{"input": map[string]any{"prompt": "hello", "style": "cinematic", "quality": "hd", "seed": "42"}}

	actual, err := mapping.Apply(request)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"input": map[string]any{"style": "cinematic", "quality": "high", "seed": "42"}, "payload": map[string]any{"text": "hello", "style": "cinematic", "count": 1, "seed": int64(42)}}, actual)
	require.Contains(t, request["input"].(map[string]any), "prompt")
}

func TestMediaRequestMappingSkipsMissingOptionalSourcesAndReportsPresentValueFailures(t *testing.T) {
	result, err := (MediaRequestMapping{Rules: []MediaMappingRule{
		{Operation: "rename", Source: "size", Target: "chicun"},
		{Operation: "copy", Source: "quality", Target: "pinzhi"},
	}}).Apply(map[string]any{"prompt": "cat"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"prompt": "cat"}, result)

	tests := []struct {
		name    string
		mapping MediaRequestMapping
		request map[string]any
		want    error
	}{
		{"enum", MediaRequestMapping{Rules: []MediaMappingRule{{Operation: "enum", Source: "kind", Target: "out", Values: map[string]string{"a": "b"}}}}, map[string]any{"kind": "x"}, ErrMediaMappingEnumMiss},
		{"cast", MediaRequestMapping{Rules: []MediaMappingRule{{Operation: "cast", Source: "count", Target: "out", Cast: "integer"}}}, map[string]any{"count": "not-a-number"}, ErrMediaMappingCastFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.mapping.Apply(tt.request)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestMediaRequestMappingRejectsUnsafeAndConflictingTargets(t *testing.T) {
	tests := []MediaRequestMapping{
		{Rules: []MediaMappingRule{{Operation: "script", Target: "out"}}},
		{Rules: []MediaMappingRule{{Operation: "copy", Source: "a[0]", Target: "out"}}},
		{Rules: []MediaMappingRule{{Operation: "rename", Source: "a", Target: "a"}}},
		{Rules: []MediaMappingRule{{Operation: "copy", Source: "a", Target: "a"}}},
		{Rules: []MediaMappingRule{{Operation: "copy", Source: "a", Target: "a.child"}}},
		{Rules: []MediaMappingRule{{Operation: "cast", Source: "a.child", Target: "a", Cast: "string"}}},
		{Rules: []MediaMappingRule{{Operation: "copy", Source: "a", Target: "out"}, {Operation: "default", Target: "out", Value: true}}},
		{Rules: []MediaMappingRule{{Operation: "copy", Source: "a", Target: "out"}, {Operation: "default", Target: "out.child", Value: true}}},
		{Rules: []MediaMappingRule{{Operation: "copy", Source: "source.child", Target: "out"}, {Operation: "default", Target: "source", Value: map[string]any{}}}},
		{Rules: []MediaMappingRule{{Operation: "rename", Source: "image.size", Target: "image.chicun"}, {Operation: "copy", Source: "quality", Target: "pinzhi"}}},
		{Rules: []MediaMappingRule{{Operation: "default", Target: "image", Value: map[string]any{}}}},
	}
	for _, mapping := range tests {
		require.Error(t, mapping.Validate())
	}
	_, err := (MediaRequestMapping{Rules: []MediaMappingRule{{Operation: "copy", Source: "a", Target: "out"}}}).Apply(map[string]any{"a": 1, "out": 2})
	require.True(t, errors.Is(err, ErrMediaMappingTargetConflict), err)
}
