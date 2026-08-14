package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOpenAIWSTokenEvent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		eventType string
		want      bool
	}{
		{eventType: "response.output_text.delta", want: true},
		{eventType: "response.output_audio.delta", want: true},
		{eventType: "response.function_call_arguments.delta", want: true},
		{eventType: "response.output_text.done", want: true},
		{eventType: "response.function_call_arguments.done", want: true},
		{eventType: "response.completed", want: false},
		{eventType: "response.done", want: false},
		{eventType: "response.failed", want: false},
		{eventType: "response.incomplete", want: false},
		{eventType: "response.cancelled", want: false},
		{eventType: "response.canceled", want: false},
		{eventType: "response.output_item.done", want: false},
		{eventType: "response.output_audio.done", want: false},
		{eventType: "response.output_text.annotation.added", want: false},
		{eventType: "response.created", want: false},
		{eventType: "", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			require.Equal(t, tc.want, isOpenAIWSTokenEvent(tc.eventType))
		})
	}
}

func TestOpenAIWSTerminalAndTokenEventSetsAreDisjoint(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		"response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
	} {
		require.True(t, isOpenAIWSTerminalEvent(eventType), eventType)
		require.False(t, isOpenAIWSTokenEvent(eventType), eventType)
	}
}
