package service

import "testing"

func TestAccountCanBelongToGroupPlatform(t *testing.T) {
	tests := []struct {
		name            string
		accountPlatform string
		groupPlatform   string
		want            bool
	}{
		{
			name:            "codex2api account belongs to openai group",
			accountPlatform: PlatformCodex2API,
			groupPlatform:   PlatformOpenAI,
			want:            true,
		},
		{
			name:            "codex2api account does not belong to codex2api group",
			accountPlatform: PlatformCodex2API,
			groupPlatform:   PlatformCodex2API,
			want:            false,
		},
		{
			name:            "antigravity mixed scheduling can bind anthropic group",
			accountPlatform: PlatformAntigravity,
			groupPlatform:   PlatformAnthropic,
			want:            true,
		},
		{
			name:            "antigravity mixed scheduling can bind gemini group",
			accountPlatform: PlatformAntigravity,
			groupPlatform:   PlatformGemini,
			want:            true,
		},
		{
			name:            "openai account does not bind anthropic group",
			accountPlatform: PlatformOpenAI,
			groupPlatform:   PlatformAnthropic,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccountCanBelongToGroupPlatform(tt.accountPlatform, tt.groupPlatform)
			if got != tt.want {
				t.Fatalf("AccountCanBelongToGroupPlatform(%q, %q) = %v, want %v", tt.accountPlatform, tt.groupPlatform, got, tt.want)
			}
		})
	}
}

func TestEmbeddingPlatformHasExactGroupBinding(t *testing.T) {
	if !AccountCanBelongToGroupPlatform(PlatformEmbedding, PlatformEmbedding) {
		t.Fatal("embedding account must bind an embedding group")
	}
	if IsOpenAICompatiblePlatform(PlatformEmbedding) {
		t.Fatal("embedding must not enter the OpenAI-compatible platform set")
	}
}

func TestValidateEmbeddingAccount(t *testing.T) {
	valid := &Account{Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://embeddings.example.test",
		"api_key":  "sk-test",
		"model_mapping": map[string]any{
			"text-embedding-3-small": "vendor-embedding-small",
		},
	}}
	if err := validateEmbeddingAccount(valid); err != nil {
		t.Fatalf("valid embedding account rejected: %v", err)
	}

	for name, account := range map[string]*Account{
		"oauth":            {Platform: PlatformEmbedding, Type: AccountTypeOAuth, Credentials: valid.Credentials},
		"missing base url": {Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk-test", "model_mapping": valid.Credentials["model_mapping"]}},
		"missing api key":  {Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://embeddings.example.test", "model_mapping": valid.Credentials["model_mapping"]}},
		"missing mapping":  {Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://embeddings.example.test", "api_key": "sk-test"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateEmbeddingAccount(account); err == nil {
				t.Fatal("invalid embedding account accepted")
			}
		})
	}

	whitelist := &Account{Platform: PlatformEmbedding, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url":        "https://embeddings.example.test",
		"api_key":         "sk-test",
		"model_whitelist": []string{"text-embedding-3-small"},
	}}
	if err := validateEmbeddingAccount(whitelist); err != nil {
		t.Fatalf("embedding whitelist account rejected: %v", err)
	}
}
