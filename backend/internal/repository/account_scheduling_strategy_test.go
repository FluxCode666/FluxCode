package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestResolveAccountSchedulingStrategy(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "default_openai", platform: service.PlatformOpenAI, want: defaultSchedulingStrategyPlatform},
		{name: "default_gemini", platform: service.PlatformGemini, want: defaultSchedulingStrategyPlatform},
		{name: "default_anthropic", platform: service.PlatformAnthropic, want: defaultSchedulingStrategyPlatform},
		{name: "antigravity", platform: service.PlatformAntigravity, want: service.PlatformAntigravity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, resolveAccountSchedulingStrategy(tt.platform).Platform())
		})
	}
}

func TestBuildAccountSchedulingBucketCaseSQLIncludesPlatformBranches(t *testing.T) {
	sql := buildAccountSchedulingBucketCaseSQL()
	require.Contains(t, sql, "platform = 'antigravity'")
	require.Contains(t, sql, "platform NOT IN ('antigravity')")
	require.Contains(t, sql, string(service.AccountSchedulingStateAvailable))
	require.Contains(t, sql, string(service.AccountSchedulingStateManualUnschedulable))
}
