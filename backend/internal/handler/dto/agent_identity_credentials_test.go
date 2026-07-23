package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactAgentIdentityPrivateKey(t *testing.T) {
	credentials := map[string]any{
		"auth_mode":          "agentIdentity",
		"agent_runtime_id":   "runtime-a",
		"agent_private_key":  "private-key",
		"chatgpt_account_id": "team-a",
	}
	redacted := redactAgentIdentityPrivateKey(credentials)
	require.NotContains(t, redacted, "agent_private_key")
	require.Equal(t, "runtime-a", redacted["agent_runtime_id"])
	require.Contains(t, credentials, "agent_private_key")
}
