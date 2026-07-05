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

func TestValidateChannelMonitorJitter(t *testing.T) {
	t.Run("allows zero jitter", func(t *testing.T) {
		require.NoError(t, validateJitter(0, 60))
	})

	t.Run("allows jitter while minimum delay stays at floor", func(t *testing.T) {
		require.NoError(t, validateJitter(45, 60))
	})

	t.Run("rejects negative jitter", func(t *testing.T) {
		require.ErrorIs(t, validateJitter(-1, 60), ErrChannelMonitorInvalidJitter)
	})

	t.Run("rejects jitter that drops effective interval below floor", func(t *testing.T) {
		require.ErrorIs(t, validateJitter(46, 60), ErrChannelMonitorInvalidJitter)
	})

	t.Run("rejects jitter equal to interval", func(t *testing.T) {
		require.ErrorIs(t, validateJitter(15, 15), ErrChannelMonitorInvalidJitter)
	})
}
