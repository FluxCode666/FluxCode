package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateChannelMonitorEndpointRejectsPrivateTargets(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1:8080/v1/chat/completions",
		"http://localhost:8080/v1/chat/completions",
		"http://10.0.0.1/v1/chat/completions",
		"http://172.16.1.1/v1/chat/completions",
		"http://192.168.1.1/v1/chat/completions",
		"file:///etc/passwd",
	}
	for _, endpoint := range blocked {
		require.Error(t, ValidateChannelMonitorEndpoint(endpoint), endpoint)
	}
}

func TestValidateChannelMonitorEndpointAllowsPublicHTTPS(t *testing.T) {
	require.NoError(t, ValidateChannelMonitorEndpoint("https://api.openai.com"))
	require.NoError(t, ValidateChannelMonitorEndpoint("https://api.anthropic.com"))
}
