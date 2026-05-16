package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============ Account 开关测试 ============

func TestIsAnthropicSubUsageAccountingEnabled(t *testing.T) {
	tests := []struct {
		name     string
		account  *Account
		expected bool
	}{
		{
			name:     "nil account",
			account:  nil,
			expected: false,
		},
		{
			name: "non-Anthropic platform",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": true},
			},
			expected: false,
		},
		{
			name: "non-APIKey type",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": true},
			},
			expected: false,
		},
		{
			name: "nil extra",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra:    nil,
			},
			expected: false,
		},
		{
			name: "field missing",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{},
			},
			expected: false,
		},
		{
			name: "field false",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": false},
			},
			expected: false,
		},
		{
			name: "field true",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": true},
			},
			expected: true,
		},
		{
			name: "field wrong type",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": "true"},
			},
			expected: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.account.IsAnthropicSubUsageAccountingEnabled())
		})
	}
}

// ============ Token 估算测试 ============

func TestSubEstimateTokensForText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty", "", 0},
		{"whitespace", "   ", 0},
		{"short english", "hello", 2},    // (5+3)/4 = 2
		{"long english", "This is a test sentence for token estimation.", 12}, // 46 chars -> (46+3)/4=12
		{"CJK text", "你好世界", 4},    // 4 runes, CJK-heavy
		{"mixed text 80%+ ascii", "Hello World 你好", 4}, // 14 runes, 12 ascii -> 85.7% -> (14+3)/4=4
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := subEstimateTokensForText(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestEstimateLastUserMessageTokens(t *testing.T) {
	tests := []struct {
		name     string
		parsed   *ParsedRequest
		minToken int
		maxToken int
	}{
		{
			name:     "nil parsed",
			parsed:   nil,
			minToken: 0,
			maxToken: 0,
		},
		{
			name:     "empty messages",
			parsed:   &ParsedRequest{Messages: []any{}},
			minToken: 0,
			maxToken: 0,
		},
		{
			name: "single user message string content",
			parsed: &ParsedRequest{
				Messages: []any{
					map[string]any{"role": "user", "content": "Hello, how are you?"},
				},
			},
			minToken: 1,
			maxToken: 20,
		},
		{
			name: "multi-turn, last is user",
			parsed: &ParsedRequest{
				Messages: []any{
					map[string]any{"role": "user", "content": "First message with lots of text"},
					map[string]any{"role": "assistant", "content": "Response here"},
					map[string]any{"role": "user", "content": "Hi"},
				},
			},
			minToken: 1,
			maxToken: 5,
		},
		{
			name: "last message is assistant - should find previous user",
			parsed: &ParsedRequest{
				Messages: []any{
					map[string]any{"role": "user", "content": "A user message"},
					map[string]any{"role": "assistant", "content": "An assistant response"},
				},
			},
			minToken: 1,
			maxToken: 20,
		},
		{
			name: "content is array of blocks",
			parsed: &ParsedRequest{
				Messages: []any{
					map[string]any{
						"role": "user",
						"content": []any{
							map[string]any{"type": "text", "text": "Hello world"},
						},
					},
				},
			},
			minToken: 1,
			maxToken: 10,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := estimateLastUserMessageTokens(tc.parsed)
			if tc.minToken == 0 && tc.maxToken == 0 {
				assert.Equal(t, 0, got)
			} else {
				assert.GreaterOrEqual(t, got, tc.minToken)
				assert.LessOrEqual(t, got, tc.maxToken)
			}
		})
	}
}

func TestEstimateTotalPromptTokens(t *testing.T) {
	body := []byte(`{
		"system": "You are a helpful assistant.",
		"messages": [
			{"role": "user", "content": "Hello world"},
			{"role": "assistant", "content": "Hi there"},
			{"role": "user", "content": "Tell me a joke"}
		],
		"tools": [{"name": "get_weather", "description": "Get weather info", "input_schema": {"type": "object"}}]
	}`)
	parsed := &ParsedRequest{
		System:    "You are a helpful assistant.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "Hello world"},
			map[string]any{"role": "assistant", "content": "Hi there"},
			map[string]any{"role": "user", "content": "Tell me a joke"},
		},
	}

	total := estimateTotalPromptTokens(body, parsed)
	// Should be > 0 and reasonable
	assert.Greater(t, total, 0)
	assert.Less(t, total, 500, "sanity check: total prompt tokens should be reasonable for small request")
}

// ============ Cache Pool 拆分测试 ============

func TestSplitCachePoolTokens_Fallback(t *testing.T) {
	// 确保无历史时使用 85/15
	key := "test_fallback_" + time.Now().Format("150405.000")
	read, creation := splitCachePoolTokens(key, 39920)
	assert.Equal(t, 33932, read, "85% of 39920")
	assert.Equal(t, 5988, creation, "remainder")
}

func TestSplitCachePoolTokens_EmptyKey(t *testing.T) {
	read, creation := splitCachePoolTokens("", 10000)
	assert.Equal(t, 8500, read)
	assert.Equal(t, 1500, creation)
}

func TestSplitCachePoolTokens_Zero(t *testing.T) {
	read, creation := splitCachePoolTokens("anykey", 0)
	assert.Equal(t, 0, read)
	assert.Equal(t, 0, creation)
}

func TestSplitCachePoolTokens_WithHistory(t *testing.T) {
	key := "test_history_" + time.Now().Format("150405.000")

	// 设置历史状态：上次 cache_pool = 39000
	updateSubUsageHistory(key, 39000)

	// 本次 cache_pool = 39920
	read, creation := splitCachePoolTokens(key, 39920)
	assert.Equal(t, 39000, read, "min(39920, 39000) = 39000")
	assert.Equal(t, 920, creation, "39920 - 39000 = 920")
}

func TestSplitCachePoolTokens_HistoryShrink(t *testing.T) {
	key := "test_shrink_" + time.Now().Format("150405.000")

	// 历史状态大于当前（上下文回退场景）
	updateSubUsageHistory(key, 50000)

	read, creation := splitCachePoolTokens(key, 30000)
	assert.Equal(t, 30000, read, "min(30000, 50000) = 30000")
	assert.Equal(t, 0, creation, "30000 - 30000 = 0")
}

func TestSplitCachePoolTokens_HistoryExpired(t *testing.T) {
	key := "test_expired_" + time.Now().Format("150405.000")

	// 手动插入过期条目
	subUsageAccountingStore.mu.Lock()
	subUsageAccountingStore.items[key] = &subUsageHistoryEntry{
		CachePoolTokens: 39000,
		UpdatedAt:       time.Now().Add(-2 * time.Hour), // 2h ago, expired
	}
	subUsageAccountingStore.mu.Unlock()

	// 应该回退到 85/15
	read, creation := splitCachePoolTokens(key, 10000)
	assert.Equal(t, 8500, read)
	assert.Equal(t, 1500, creation)
}

// ============ Cache TTL 归类测试 ============

func TestClassifyCacheCreationTTL_NoTTL(t *testing.T) {
	body := []byte(`{"system": "hi", "messages": []}`)
	c5m, c1h := classifyCacheCreationTTL(body, 1000)
	assert.Equal(t, 1000, c5m, "default 5m")
	assert.Equal(t, 0, c1h)
}

func TestClassifyCacheCreationTTL_WithTTL1h(t *testing.T) {
	body := []byte(`{
		"system": [{"type": "text", "text": "hello", "cache_control": {"type": "ephemeral", "ttl": "1h"}}],
		"messages": []
	}`)
	c5m, c1h := classifyCacheCreationTTL(body, 1000)
	assert.Equal(t, 0, c5m)
	assert.Equal(t, 1000, c1h, "should be 1h when ttl=1h present")
}

func TestClassifyCacheCreationTTL_ZeroCreation(t *testing.T) {
	body := []byte(`{"system": [{"type": "text", "text": "hello", "cache_control": {"ttl": "1h"}}]}`)
	c5m, c1h := classifyCacheCreationTTL(body, 0)
	assert.Equal(t, 0, c5m)
	assert.Equal(t, 0, c1h)
}

// ============ Session Key 测试 ============

func TestBuildSubUsageSessionKey(t *testing.T) {
	parsed := &ParsedRequest{
		MetadataUserID: "user_abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890_account__session_12345678-1234-1234-1234-123456789012",
	}
	key := buildSubUsageSessionKey(100, 200, "claude-sonnet-4-20250514", parsed)
	assert.Contains(t, key, "100:200:claude-sonnet-4-20250514:")
	assert.Contains(t, key, "sid:")
}

func TestBuildSubUsageSessionKey_NoMetadata(t *testing.T) {
	parsed := &ParsedRequest{
		System:    "You are a helper.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	key := buildSubUsageSessionKey(1, 2, "claude-sonnet-4-20250514", parsed)
	assert.Contains(t, key, "1:2:claude-sonnet-4-20250514:")
	assert.Contains(t, key, "ch:")
}

func TestBuildSubUsageSessionKey_UnrecognizedMetadata(t *testing.T) {
	parsed := &ParsedRequest{
		MetadataUserID: "some-random-user-id",
	}
	key := buildSubUsageSessionKey(1, 2, "claude-sonnet-4-20250514", parsed)
	assert.Contains(t, key, "muid:")
}

// ============ 集成测试: applySubUsageAccounting ============

func TestApplySubUsageAccounting_Disabled(t *testing.T) {
	svc := &GatewayService{}
	result := &ForwardResult{
		Usage: ClaudeUsage{InputTokens: 100, OutputTokens: 50},
	}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{}, // not enabled
	}
	applied := svc.applySubUsageAccounting(result, account, &APIKey{ID: 1}, &ParsedRequest{
		Body: []byte(`{"messages": [{"role": "user", "content": "hi"}]}`),
	})
	assert.False(t, applied)
	assert.Equal(t, 100, result.Usage.InputTokens, "should not be changed")
}

func TestApplySubUsageAccounting_Enabled(t *testing.T) {
	svc := &GatewayService{}
	result := &ForwardResult{
		Model: "claude-sonnet-4-20250514",
		Usage: ClaudeUsage{InputTokens: 40000, OutputTokens: 300},
	}
	account := &Account{
		ID:       49,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": true},
	}
	parsed := &ParsedRequest{
		Body: []byte(`{
			"system": "You are a helpful coding assistant. You help users write code.",
			"messages": [
				{"role": "user", "content": "Write a hello world program"},
				{"role": "assistant", "content": "Sure, here is a hello world program in Python:\n\nprint('Hello, World!')"},
				{"role": "user", "content": "Thanks!"}
			]
		}`),
		System:    "You are a helpful coding assistant. You help users write code.",
		HasSystem: true,
		Messages: []any{
			map[string]any{"role": "user", "content": "Write a hello world program"},
			map[string]any{"role": "assistant", "content": "Sure, here is a hello world program in Python:\n\nprint('Hello, World!')"},
			map[string]any{"role": "user", "content": "Thanks!"},
		},
	}

	applied := svc.applySubUsageAccounting(result, account, &APIKey{ID: 1}, parsed)
	require.True(t, applied)

	// input_tokens 应该是 "Thanks!" 的 token 估算（很小）
	assert.Greater(t, result.Usage.InputTokens, 0)
	assert.Less(t, result.Usage.InputTokens, 20, "short message should have few input tokens")

	// output_tokens 应该保持原值
	assert.Equal(t, 300, result.Usage.OutputTokens)

	// cache_read + cache_creation 应该 > 0
	assert.Greater(t, result.Usage.CacheReadInputTokens+result.Usage.CacheCreationInputTokens, 0)

	// total should be reasonable
	total := result.Usage.InputTokens + result.Usage.CacheReadInputTokens + result.Usage.CacheCreationInputTokens
	assert.Greater(t, total, 0)
}

func TestApplySubUsageAccounting_NilParsedRequest(t *testing.T) {
	svc := &GatewayService{}
	result := &ForwardResult{Usage: ClaudeUsage{InputTokens: 100}}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"anthropic_sub_usage_accounting_enabled": true},
	}
	applied := svc.applySubUsageAccounting(result, account, nil, nil)
	assert.False(t, applied)
	assert.Equal(t, 100, result.Usage.InputTokens, "should not change without parsed request")
}

// ============ 历史状态缓存清理 ============

func TestCleanupSubUsageHistory(t *testing.T) {
	// 插入新旧条目
	subUsageAccountingStore.mu.Lock()
	subUsageAccountingStore.items["fresh_key"] = &subUsageHistoryEntry{
		CachePoolTokens: 1000,
		UpdatedAt:       time.Now(),
	}
	subUsageAccountingStore.items["stale_key"] = &subUsageHistoryEntry{
		CachePoolTokens: 2000,
		UpdatedAt:       time.Now().Add(-2 * time.Hour),
	}
	subUsageAccountingStore.mu.Unlock()

	cleanupSubUsageHistory()

	subUsageAccountingStore.mu.Lock()
	_, freshExists := subUsageAccountingStore.items["fresh_key"]
	_, staleExists := subUsageAccountingStore.items["stale_key"]
	subUsageAccountingStore.mu.Unlock()

	assert.True(t, freshExists, "fresh entry should survive cleanup")
	assert.False(t, staleExists, "stale entry should be cleaned up")
}

// ============ estimateContentTokens 测试 ============

func TestEstimateContentTokens_StringContent(t *testing.T) {
	tokens := estimateContentTokens("Hello, how are you today?")
	assert.Greater(t, tokens, 0)
}

func TestEstimateContentTokens_ArrayContent(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "First paragraph of text"},
		map[string]any{"type": "text", "text": "Second paragraph"},
	}
	tokens := estimateContentTokens(content)
	assert.Greater(t, tokens, 0)
}

func TestEstimateContentTokens_ImageBlock(t *testing.T) {
	content := []any{
		map[string]any{"type": "image", "source": map[string]any{"type": "base64"}},
	}
	tokens := estimateContentTokens(content)
	assert.Equal(t, 1600, tokens, "image should estimate 1600 tokens")
}

func TestEstimateContentTokens_ToolUseBlock(t *testing.T) {
	content := []any{
		map[string]any{
			"type":  "tool_use",
			"name":  "get_weather",
			"input": map[string]any{"location": "San Francisco"},
		},
	}
	tokens := estimateContentTokens(content)
	assert.Greater(t, tokens, 0)
}
