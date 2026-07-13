package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeGPT56ModelAlias_UpstreamSpellings(t *testing.T) {
	tests := map[string]string{
		"gpt-5.6": "gpt-5.6-sol", "gpt5.6": "gpt-5.6-sol",
		"openai/gpt-5.6": "gpt-5.6-sol", "gpt-5.6-high": "gpt-5.6-sol",
		"gpt-5.6-max": "gpt-5.6-sol", "gpt-5.6-2026-07-09": "gpt-5.6-sol",
		"gpt-5.6-sol-2026-07-09": "gpt-5.6-sol",
		"gpt-5.6-terra-high":     "gpt-5.6-terra", "gpt-5.6-luna-preview": "gpt-5.6-luna",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, ok := normalizeGPT56ModelAlias(input)
			require.True(t, ok)
			require.Equal(t, want, got)
		})
	}
}

func TestNormalizeGPT56ModelAlias_RejectsUnknownSuffixes(t *testing.T) {
	for _, input := range []string{"gpt-5.6-extra-high", "gpt-5.6-foo", "gpt-5.6-terra-foo", "openai/gpt-5.6-unknown"} {
		t.Run(input, func(t *testing.T) {
			_, ok := normalizeGPT56ModelAlias(input)
			require.False(t, ok)
		})
	}
}

func TestUsageBillingModelCandidates_AppendsCanonicalGPT56(t *testing.T) {
	require.Equal(t,
		[]string{"billing-explicit", "channel-model", "openai/gpt5.6", "upstream-model", "gpt-5.6-sol"},
		usageBillingModelCandidates("billing-explicit", "channel-model", "openai/gpt5.6", "upstream-model"),
	)
	require.Equal(t,
		[]string{"custom", "gpt-5.6-terra-high", "gpt-5.6-terra"},
		usageBillingModelCandidates("custom", "gpt-5.6-terra-high"),
	)
}

func TestResolveOpenAIForwardModel(t *testing.T) {
	tests := []struct {
		name               string
		account            *Account
		requestedModel     string
		defaultMappedModel string
		expectedModel      string
	}{
		{
			name: "falls back to group default when account has no mapping",
			account: &Account{
				Credentials: map[string]any{},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-4o-mini",
		},
		{
			name: "preserves exact passthrough mapping instead of group default",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "preserves wildcard passthrough mapping instead of group default",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-*": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5.4",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
		{
			name: "uses account remap when explicit target differs",
			account: &Account{
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"gpt-5": "gpt-5.4",
					},
				},
			},
			requestedModel:     "gpt-5",
			defaultMappedModel: "gpt-4o-mini",
			expectedModel:      "gpt-5.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAIForwardModel(tt.account, tt.requestedModel, tt.defaultMappedModel); got != tt.expectedModel {
				t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", got, tt.expectedModel)
			}
		})
	}
}

func TestResolveOpenAIForwardModel_PreventsClaudeModelFromFallingBackToGpt51(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{},
	}

	withoutDefault := normalizeCodexModel(resolveOpenAIForwardModel(account, "claude-opus-4-6", ""))
	if withoutDefault != "gpt-5.1" {
		t.Fatalf("normalizeCodexModel(...) = %q, want %q", withoutDefault, "gpt-5.1")
	}

	withDefault := normalizeCodexModel(resolveOpenAIForwardModel(account, "claude-opus-4-6", "gpt-5.4"))
	if withDefault != "gpt-5.4" {
		t.Fatalf("normalizeCodexModel(...) = %q, want %q", withDefault, "gpt-5.4")
	}
}

func TestNormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex-spark":       "gpt-5.3-codex",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex",
		"gpt-5.3":                   "gpt-5.3-codex",
	}

	for input, expected := range cases {
		if got := normalizeCodexModel(input); got != expected {
			t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeOpenAIModelForUpstream(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    string
	}{
		{
			name:    "oauth keeps codex normalization behavior",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gemini-3-flash-preview",
			want:    "gpt-5.1",
		},
		{
			name:    "apikey preserves custom compatible model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
		},
		{
			name:    "apikey preserves official non codex model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gpt-4.1",
			want:    "gpt-4.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOpenAIModelForUpstream(tt.account, tt.model); got != tt.want {
				t.Fatalf("normalizeOpenAIModelForUpstream(...) = %q, want %q", got, tt.want)
			}
		})
	}
}
