package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewProviderProfileDefaults(t *testing.T) {
	profile := NewProviderProfile(42, "NewAPI")

	require.Equal(t, int64(42), profile.ID)
	require.Equal(t, ProviderStatusDraft, profile.Status)
	require.False(t, profile.AllowProtocolConversion)
	require.Equal(t, int64(1), profile.Version)
}

func TestProviderCapabilityValidationRejectsInvalidCombinations(t *testing.T) {
	tests := []struct {
		name       string
		capability ProviderModelCapability
	}{
		{
			name: "embedding cannot use conversational protocol",
			capability: ProviderModelCapability{
				ProviderID:     1,
				LogicalModelID: 2,
				Protocol:       ProtocolChatCompletions,
				UpstreamModel:  "BAAI/bge-m3",
				FeatureProfile: FeatureProfileEmbeddings,
			},
		},
		{
			name: "unknown protocol",
			capability: ProviderModelCapability{
				ProviderID:     1,
				LogicalModelID: 2,
				Protocol:       ProtocolFamily("vendor_magic"),
				UpstreamModel:  "model",
				FeatureProfile: FeatureProfileText,
			},
		},
		{
			name: "missing upstream model",
			capability: ProviderModelCapability{
				ProviderID:     1,
				LogicalModelID: 2,
				Protocol:       ProtocolResponses,
				FeatureProfile: FeatureProfileText,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.capability.Validate())
		})
	}
}

func TestProviderCapabilityValidationAcceptsFourNativeProtocols(t *testing.T) {
	protocols := []ProtocolFamily{
		ProtocolChatCompletions,
		ProtocolResponses,
		ProtocolAnthropicMessages,
		ProtocolEmbeddings,
	}
	for _, protocol := range protocols {
		profile := FeatureProfileText
		if protocol == ProtocolEmbeddings {
			profile = FeatureProfileEmbeddings
		}
		capability := ProviderModelCapability{
			ProviderID:     1,
			LogicalModelID: 2,
			Protocol:       protocol,
			UpstreamModel:  "upstream-model",
			WireProfile:    WireProfileCanonical,
			FeatureProfile: profile,
			Enabled:        true,
			Version:        1,
		}
		require.NoError(t, capability.Validate(), protocol)
	}
}

func TestProviderEndpointEffectiveConfigDoesNotLeakSensitiveHeaders(t *testing.T) {
	base := ProviderConnectionConfig{
		BaseURL: "https://api.example.com",
		Headers: map[string]string{"X-Tenant": "one"},
	}
	endpoint := ProviderProtocolEndpoint{
		Protocol: ProtocolChatCompletions,
		Path:     "/v1/chat/completions",
		Headers: map[string]string{
			"X-Feature":     "on",
			"Authorization": "client-token",
			"Host":          "evil.example.com",
		},
	}

	config, err := endpoint.EffectiveConfig(base)
	require.NoError(t, err)
	require.Equal(t, "https://api.example.com/v1/chat/completions", config.URL)
	require.Equal(t, "one", config.Headers["X-Tenant"])
	require.Equal(t, "on", config.Headers["X-Feature"])
	require.NotContains(t, config.Headers, "Authorization")
	require.NotContains(t, config.Headers, "Host")
}
