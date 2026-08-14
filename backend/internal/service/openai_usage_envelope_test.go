//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIUsageReadsNestedDataEnvelope(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{
		"data":{"usage":{"prompt_tokens":8,"completion_tokens":27,"prompt_tokens_details":{"cached_tokens":4}}},
		"success":true
	}`))

	require.True(t, ok)
	require.Equal(t, 8, usage.InputTokens)
	require.Equal(t, 27, usage.OutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)
}

func TestExtractOpenAIUsagePreservesPathPrecedence(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{
		"usage":{"input_tokens":1,"output_tokens":2},
		"data":{"usage":{"input_tokens":30,"output_tokens":40}},
		"response":{"usage":{"input_tokens":50,"output_tokens":60}}
	}`))

	require.True(t, ok)
	require.Equal(t, 1, usage.InputTokens)
	require.Equal(t, 2, usage.OutputTokens)
}

func TestExtractOpenAIUsagePrefersResponseBeforeData(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{
		"data":{"usage":{"input_tokens":30,"output_tokens":40}},
		"response":{"usage":{"input_tokens":50,"output_tokens":60}}
	}`))

	require.True(t, ok)
	require.Equal(t, 50, usage.InputTokens)
	require.Equal(t, 60, usage.OutputTokens)
}
