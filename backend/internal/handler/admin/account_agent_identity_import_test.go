package admin

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func agentIdentityImportPrivateKey(t *testing.T) string {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(der)
}

func TestNormalizeAgentIdentityImportValue(t *testing.T) {
	privateKey := agentIdentityImportPrivateKey(t)
	item, err := normalizeAgentIdentityImportValue(map[string]any{
		"auth_mode": "agentIdentity",
		"agent_identity": map[string]any{
			"agent_runtime_id":  "runtime-import",
			"agent_private_key": privateKey,
			"account_id":        "team-a",
			"chatgpt_user_id":   "user-a",
			"email":             "agent@example.invalid",
		},
	})
	require.NoError(t, err)
	credentials := buildAgentIdentityImportCredentials(item)
	require.Equal(t, service.OpenAIAuthModeAgentIdentity, credentials["auth_mode"])
	require.Equal(t, "team-a", credentials["chatgpt_account_id"])
	require.Equal(t, "user-a", credentials["chatgpt_user_id"])
	require.NotContains(t, credentials, "access_token")
	require.NotContains(t, credentials, "refresh_token")
}

func TestFindAgentIdentityImportAccountSeparatesTeamsAndUsers(t *testing.T) {
	accounts := []service.Account{{
		ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode": service.OpenAIAuthModeAgentIdentity, "chatgpt_account_id": "team-a", "chatgpt_user_id": "user-a",
		},
	}}
	require.NotNil(t, findAgentIdentityImportAccount(accounts, "team-a", "user-a"))
	require.Nil(t, findAgentIdentityImportAccount(accounts, "team-b", "user-a"))
	require.Nil(t, findAgentIdentityImportAccount(accounts, "team-a", "user-b"))
}

func TestMergeAgentIdentityImportCredentialsDropsOAuthTokensAndStaleTask(t *testing.T) {
	merged := mergeAgentIdentityImportCredentials(map[string]any{
		"access_token": "access", "refresh_token": "refresh", "id_token": "id", "task_id": "old-task", "model_mapping": map[string]any{"a": "b"},
	}, map[string]any{
		"auth_mode": service.OpenAIAuthModeAgentIdentity, "agent_runtime_id": "runtime-new",
	}, false)
	require.NotContains(t, merged, "access_token")
	require.NotContains(t, merged, "refresh_token")
	require.NotContains(t, merged, "id_token")
	require.NotContains(t, merged, "task_id")
	require.Contains(t, merged, "model_mapping")
}
