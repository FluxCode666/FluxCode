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
		{
			name:            "media account binds only media group",
			accountPlatform: PlatformMedia,
			groupPlatform:   PlatformMedia,
			want:            true,
		},
		{
			name:            "media account does not bind text group",
			accountPlatform: PlatformMedia,
			groupPlatform:   PlatformOpenAI,
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
