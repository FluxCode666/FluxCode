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
