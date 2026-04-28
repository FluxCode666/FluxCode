//go:build unit

package service

import (
	"testing"
)

func TestGetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected string
	}{
		{
			name: "non-apikey type returns empty",
			account: Account{
				Type:     AccountTypeOAuth,
				Platform: PlatformAnthropic,
			},
			expected: "",
		},
		{
			name: "apikey without base_url returns default anthropic",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAnthropic,
				Credentials: map[string]any{},
			},
			expected: "https://api.anthropic.com",
		},
		{
			name: "apikey with custom base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAnthropic,
				Credentials: map[string]any{"base_url": "https://custom.example.com"},
			},
			expected: "https://custom.example.com",
		},
		{
			name: "antigravity apikey auto-appends /antigravity",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity apikey trims trailing slash before appending",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com/"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity non-apikey returns empty",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetBaseURL()
			if result != tt.expected {
				t.Errorf("GetBaseURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetGeminiBaseURL(t *testing.T) {
	const defaultGeminiURL = "https://generativelanguage.googleapis.com"

	tests := []struct {
		name     string
		account  Account
		expected string
	}{
		{
			name: "apikey without base_url returns default",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{},
			},
			expected: defaultGeminiURL,
		},
		{
			name: "apikey with custom base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformGemini,
				Credentials: map[string]any{"base_url": "https://custom-gemini.example.com"},
			},
			expected: "https://custom-gemini.example.com",
		},
		{
			name: "antigravity apikey auto-appends /antigravity",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity apikey trims trailing slash",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com/"},
			},
			expected: "https://upstream.example.com/antigravity",
		},
		{
			name: "antigravity oauth does NOT append /antigravity",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{"base_url": "https://upstream.example.com"},
			},
			expected: "https://upstream.example.com",
		},
		{
			name: "oauth without base_url returns default",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformAntigravity,
				Credentials: map[string]any{},
			},
			expected: defaultGeminiURL,
		},
		{
			name: "nil credentials returns default",
			account: Account{
				Type:     AccountTypeAPIKey,
				Platform: PlatformGemini,
			},
			expected: defaultGeminiURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.account.GetGeminiBaseURL(defaultGeminiURL)
			if result != tt.expected {
				t.Errorf("GetGeminiBaseURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetOpenAIBaseURL_Codex2API(t *testing.T) {
	tests := []struct {
		name             string
		account          Account
		wantBaseURL      string
		wantOpenAIApiKey bool
		wantOpenAICompat bool
	}{
		{
			name: "apikey with explicit base_url",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformCodex2API,
				Credentials: map[string]any{"base_url": "https://codex2api.example.com"},
			},
			wantBaseURL:      "https://codex2api.example.com",
			wantOpenAIApiKey: true,
			wantOpenAICompat: true,
		},
		{
			name: "apikey without base_url does not fallback to official openai",
			account: Account{
				Type:        AccountTypeAPIKey,
				Platform:    PlatformCodex2API,
				Credentials: map[string]any{},
			},
			wantBaseURL:      "",
			wantOpenAIApiKey: true,
			wantOpenAICompat: true,
		},
		{
			name: "oauth is not a valid codex2api openai apikey account",
			account: Account{
				Type:        AccountTypeOAuth,
				Platform:    PlatformCodex2API,
				Credentials: map[string]any{"base_url": "https://codex2api.example.com"},
			},
			wantBaseURL:      "",
			wantOpenAIApiKey: false,
			wantOpenAICompat: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.account.GetOpenAIBaseURL(); got != tt.wantBaseURL {
				t.Errorf("GetOpenAIBaseURL() = %q, want %q", got, tt.wantBaseURL)
			}
			if got := tt.account.IsOpenAIApiKey(); got != tt.wantOpenAIApiKey {
				t.Errorf("IsOpenAIApiKey() = %v, want %v", got, tt.wantOpenAIApiKey)
			}
			if got := tt.account.IsOpenAICompatible(); got != tt.wantOpenAICompat {
				t.Errorf("IsOpenAICompatible() = %v, want %v", got, tt.wantOpenAICompat)
			}
		})
	}
}
