package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestProviderResponseReportsCredentialWithoutReturningSecret(t *testing.T) {
	aggregate := &service.ProviderAggregate{
		Profile: service.NewProviderProfile(42, "NewAPI"),
		Account: &service.Account{
			ID: 42, Credentials: map[string]any{"api_key": "upstream-secret"},
		},
		LogicalModels: map[int64]service.LogicalModel{},
	}

	payload, err := json.Marshal(ProviderFromService(aggregate))

	require.NoError(t, err)
	require.Contains(t, string(payload), `"credential_configured":true`)
	require.NotContains(t, string(payload), "upstream-secret")
	require.NotContains(t, string(payload), `"api_key"`)
}

func TestLegacyAccountResponseDoesNotExposeProviderCredentials(t *testing.T) {
	account := &service.Account{
		ID: 42, Platform: service.PlatformProvider,
		Credentials: map[string]any{
			"api_key":              "upstream-secret",
			"api_key_encrypted_v1": "ciphertext",
		},
	}

	dtoAccount := AccountFromServiceShallow(account)
	payload, err := json.Marshal(dtoAccount)

	require.NoError(t, err)
	require.NotContains(t, string(payload), "upstream-secret")
	require.NotContains(t, string(payload), "ciphertext")
	require.Empty(t, dtoAccount.Credentials)
}

func TestProviderCapabilityTestResponseUsesSnakeCaseAndMilliseconds(t *testing.T) {
	payload, err := json.Marshal(ProviderCapabilityTestFromService(&service.ProviderCapabilityTestResult{
		ProviderID: 42, CapabilityID: 99, Protocol: service.ProtocolChatCompletions,
		LogicalModel: "model-a", UpstreamModel: "vendor-model", StatusCode: 200,
		Duration: 1500 * time.Millisecond, UpstreamRequestID: "req-upstream",
	}))

	require.NoError(t, err)
	require.JSONEq(t, `{
		"provider_id":42,"capability_id":99,"protocol":"chat_completions",
		"logical_model":"model-a","upstream_model":"vendor-model","status_code":200,
		"duration_ms":1500,"upstream_request_id":"req-upstream"
	}`, string(payload))
}
