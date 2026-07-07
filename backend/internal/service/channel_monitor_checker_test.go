package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyChannelMonitorHTTPStatus(t *testing.T) {
	require.Equal(t, ChannelMonitorStatusOperational, classifyChannelMonitorHTTPStatus(http.StatusOK, nil))
	require.Equal(t, ChannelMonitorStatusDegraded, classifyChannelMonitorHTTPStatus(http.StatusTooManyRequests, nil))
	require.Equal(t, ChannelMonitorStatusFailed, classifyChannelMonitorHTTPStatus(http.StatusUnauthorized, nil))
	require.Equal(t, ChannelMonitorStatusError, classifyChannelMonitorHTTPStatus(http.StatusInternalServerError, nil))
}

func TestExtractOpenAIResponsesTextFromSSEDelta(t *testing.T) {
	body := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"14\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[]}}\n\n")

	require.Equal(t, "14", extractOpenAIResponsesText(body))
}
