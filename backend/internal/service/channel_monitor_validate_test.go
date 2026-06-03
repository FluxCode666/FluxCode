package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorConstants(t *testing.T) {
	require.Equal(t, "openai", ChannelMonitorProviderOpenAI)
	require.Equal(t, "anthropic", ChannelMonitorProviderAnthropic)
	require.Equal(t, "gemini", ChannelMonitorProviderGemini)
	require.Equal(t, "chat_completions", ChannelMonitorAPIModeChatCompletions)
	require.Equal(t, "responses", ChannelMonitorAPIModeResponses)
	require.Equal(t, "operational", ChannelMonitorStatusOperational)
	require.Equal(t, "degraded", ChannelMonitorStatusDegraded)
	require.Equal(t, "failed", ChannelMonitorStatusFailed)
	require.Equal(t, "error", ChannelMonitorStatusError)
}

func TestNormalizeChannelMonitorInterval(t *testing.T) {
	require.Equal(t, 60, NormalizeChannelMonitorInterval(0, 60))
	require.Equal(t, 15, NormalizeChannelMonitorInterval(1, 60))
	require.Equal(t, 3600, NormalizeChannelMonitorInterval(7200, 60))
	require.Equal(t, 120, NormalizeChannelMonitorInterval(120, 60))
}
