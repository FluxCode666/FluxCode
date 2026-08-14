//go:build unit

package repository

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSchedulerMetadataAccount_KeepsOpenAIWSFlags(t *testing.T) {
	account := service.Account{
		ID:       42,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
			"openai_ws_force_http":                         true,
			"mixed_scheduling":                             true,
			"unused_large_field":                           "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, true, got.Extra["openai_oauth_responses_websockets_v2_enabled"])
	require.Equal(t, service.OpenAIWSIngressModePassthrough, got.Extra["openai_oauth_responses_websockets_v2_mode"])
	require.Equal(t, true, got.Extra["openai_ws_force_http"])
	require.Equal(t, true, got.Extra["mixed_scheduling"])
	require.Nil(t, got.Extra["unused_large_field"])
}

func TestBuildSchedulerMetadataAccount_KeepsEmbeddingEligibilityCredentials(t *testing.T) {
	account := service.Account{
		ID:       43,
		Platform: service.PlatformEmbedding,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url":        "https://embedding.example.test/v1",
			"api_key":         "upstream-key",
			"model_mapping":   map[string]any{"embed-public": "embed-upstream"},
			"model_whitelist": []any{"legacy-embed"},
			"pool_mode":       true,
			"unused_secret":   "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, "https://embedding.example.test/v1", got.GetCredential("base_url"))
	require.Equal(t, "upstream-key", got.GetCredential("api_key"))
	require.Equal(t, map[string]any{"embed-public": "embed-upstream"}, got.Credentials["model_mapping"])
	require.Equal(t, []any{"legacy-embed"}, got.Credentials["model_whitelist"])
	require.Equal(t, true, got.Credentials["pool_mode"])
	require.Nil(t, got.Credentials["unused_secret"])
}

func TestBuildSchedulerMetadataAccount_KeepsPoolRetryAndCodexFingerprintSettings(t *testing.T) {
	account := service.Account{
		ID:       44,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"pool_mode":                    true,
			"pool_mode_retry_count":        float64(5),
			"pool_mode_retry_status_codes": []any{float64(401), float64(429), float64(502)},
			"refresh_token":                "drop-me",
		},
		Extra: map[string]any{
			"codex_fingerprint_mode": "full",
			"openai_device_id":       "device-44",
			"unused_large_field":     "drop-me",
		},
	}

	got := buildSchedulerMetadataAccount(account)

	require.Equal(t, true, got.Credentials["pool_mode"])
	require.Equal(t, float64(5), got.Credentials["pool_mode_retry_count"])
	require.Equal(t, []any{float64(401), float64(429), float64(502)}, got.Credentials["pool_mode_retry_status_codes"])
	require.Equal(t, "full", got.Extra["codex_fingerprint_mode"])
	require.Equal(t, "device-44", got.Extra["openai_device_id"])
	require.Nil(t, got.Credentials["refresh_token"])
	require.Nil(t, got.Extra["unused_large_field"])
}

func TestMetadataRoundTrip_PreservesPoolRetryAndCodexFingerprintSettings(t *testing.T) {
	poolAccount := service.Account{
		ID:       45,
		Platform: service.PlatformEmbedding,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode":                    true,
			"pool_mode_retry_count":        float64(4),
			"pool_mode_retry_status_codes": []any{float64(403), float64(500)},
		},
	}
	fingerprintAccount := service.Account{
		ID:       46,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			"codex_fingerprint_mode": "session",
			"openai_device_id":       "device-46",
		},
	}

	cached := simulateSnapshotRoundTrip(t, []service.Account{poolAccount, fingerprintAccount})
	require.Len(t, cached, 2)

	require.True(t, cached[0].IsPoolMode())
	require.Equal(t, 4, cached[0].GetPoolModeRetryCount())
	require.Equal(t, []int{403, 500}, cached[0].GetPoolModeRetryStatusCodes())
	require.Equal(t, "session", string(cached[1].GetCodexFingerprintMode()))
	require.Equal(t, "device-46", cached[1].GetOpenAIDeviceID())
}

// ---------------------------------------------------------------------------
// 一、元数据序列化层 — buildSchedulerMetadataAccount 代理字段保留
// ---------------------------------------------------------------------------

func TestBuildSchedulerMetadataAccount_NoProxy(t *testing.T) {
	account := service.Account{
		ID:       1,
		Platform: service.PlatformOpenAI,
	}

	got := buildSchedulerMetadataAccount(account)

	assert.Nil(t, got.ProxyID, "无代理账号 ProxyID 应为 nil")
	assert.Nil(t, got.Proxy, "无代理账号 Proxy 应为 nil")
}

func TestBuildSchedulerMetadataAccount_ActiveProxy(t *testing.T) {
	proxyID := int64(99)
	account := service.Account{
		ID:       2,
		Platform: service.PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    &service.Proxy{ID: 99, Status: service.StatusActive},
	}

	got := buildSchedulerMetadataAccount(account)

	require.NotNil(t, got.ProxyID)
	assert.Equal(t, int64(99), *got.ProxyID)
	require.NotNil(t, got.Proxy)
	assert.Equal(t, int64(99), got.Proxy.ID)
	assert.True(t, got.Proxy.IsActive(), "活跃代理 IsActive 应为 true")
}

func TestBuildSchedulerMetadataAccount_DisabledProxy(t *testing.T) {
	proxyID := int64(88)
	account := service.Account{
		ID:       3,
		Platform: service.PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    &service.Proxy{ID: 88, Status: service.StatusDisabled},
	}

	got := buildSchedulerMetadataAccount(account)

	require.NotNil(t, got.ProxyID)
	assert.Equal(t, int64(88), *got.ProxyID)
	require.NotNil(t, got.Proxy)
	assert.Equal(t, service.StatusDisabled, got.Proxy.Status)
	assert.False(t, got.Proxy.IsActive(), "禁用代理 IsActive 应为 false")
}

func TestBuildSchedulerMetadataAccount_DanglingProxyID(t *testing.T) {
	proxyID := int64(77)
	account := service.Account{
		ID:       4,
		Platform: service.PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    nil, // 代理已删除，悬空引用
	}

	got := buildSchedulerMetadataAccount(account)

	require.NotNil(t, got.ProxyID, "悬空 ProxyID 必须保留")
	assert.Equal(t, int64(77), *got.ProxyID)
	assert.Nil(t, got.Proxy, "Proxy 为 nil 时元数据也应为 nil")
}

// ---------------------------------------------------------------------------
// 二、JSON 序列化/反序列化 round-trip — 模拟 Redis 缓存路径
// ---------------------------------------------------------------------------

func TestMetadataRoundTrip_WithProxy(t *testing.T) {
	proxyID := int64(50)
	original := service.Account{
		ID:       10,
		Platform: service.PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    &service.Proxy{ID: 50, Status: service.StatusActive, Host: "1.2.3.4", Port: 8080},
	}

	meta := buildSchedulerMetadataAccount(original)
	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var restored service.Account
	require.NoError(t, json.Unmarshal(data, &restored))

	require.NotNil(t, restored.ProxyID)
	assert.Equal(t, int64(50), *restored.ProxyID)
	require.NotNil(t, restored.Proxy)
	assert.Equal(t, int64(50), restored.Proxy.ID)
	assert.True(t, restored.Proxy.IsActive())
}

func TestMetadataRoundTrip_WithDisabledProxy(t *testing.T) {
	proxyID := int64(60)
	original := service.Account{
		ID:       11,
		Platform: service.PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    &service.Proxy{ID: 60, Status: service.StatusDisabled},
	}

	meta := buildSchedulerMetadataAccount(original)
	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var restored service.Account
	require.NoError(t, json.Unmarshal(data, &restored))

	require.NotNil(t, restored.ProxyID)
	assert.Equal(t, int64(60), *restored.ProxyID)
	require.NotNil(t, restored.Proxy)
	assert.Equal(t, service.StatusDisabled, restored.Proxy.Status)
	assert.False(t, restored.Proxy.IsActive())
}

func TestMetadataRoundTrip_NoProxy(t *testing.T) {
	original := service.Account{
		ID:       12,
		Platform: service.PlatformOpenAI,
	}

	meta := buildSchedulerMetadataAccount(original)
	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var restored service.Account
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Nil(t, restored.ProxyID)
	assert.Nil(t, restored.Proxy)
}

func TestMetadataRoundTrip_DanglingProxyID(t *testing.T) {
	proxyID := int64(70)
	original := service.Account{
		ID:       13,
		Platform: service.PlatformOpenAI,
		ProxyID:  &proxyID,
		Proxy:    nil,
	}

	meta := buildSchedulerMetadataAccount(original)
	data, err := json.Marshal(meta)
	require.NoError(t, err)

	var restored service.Account
	require.NoError(t, json.Unmarshal(data, &restored))

	require.NotNil(t, restored.ProxyID)
	assert.Equal(t, int64(70), *restored.ProxyID)
	assert.Nil(t, restored.Proxy, "悬空 ProxyID 反序列化后 Proxy 仍为 nil")
}

// ---------------------------------------------------------------------------
// 三、快照路径 + 过滤集成 — 验证 round-trip 后字段完整性
//
// filterAccountsByDisabledProxyScheduleMode 函数已在 service 包中充分测试。
// 此处验证元数据经 JSON round-trip 后，ProxyID/Proxy 字段在各场景下正确保留，
// 确保过滤函数收到的数据具备正确的判断依据。
// ---------------------------------------------------------------------------

// simulateSnapshotRoundTrip 模拟 SetSnapshot → GetSnapshot 的 JSON 序列化路径。
func simulateSnapshotRoundTrip(t *testing.T, accounts []service.Account) []service.Account {
	t.Helper()
	result := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		meta := buildSchedulerMetadataAccount(acc)
		data, err := json.Marshal(meta)
		require.NoError(t, err)
		var restored service.Account
		require.NoError(t, json.Unmarshal(data, &restored))
		result = append(result, restored)
	}
	return result
}

func ptrInt64(v int64) *int64 { return &v }

func TestSnapshotRoundTrip_MixedProxyScenarios(t *testing.T) {
	accounts := []service.Account{
		{ID: 1, Platform: service.PlatformOpenAI}, // 无代理
		{ID: 2, Platform: service.PlatformOpenAI, ProxyID: ptrInt64(10),
			Proxy: &service.Proxy{ID: 10, Status: service.StatusActive}}, // 活跃代理
		{ID: 3, Platform: service.PlatformOpenAI, ProxyID: ptrInt64(20),
			Proxy: &service.Proxy{ID: 20, Status: service.StatusDisabled}}, // 禁用代理
		{ID: 4, Platform: service.PlatformOpenAI, ProxyID: ptrInt64(30), Proxy: nil}, // 悬空引用
	}

	cached := simulateSnapshotRoundTrip(t, accounts)
	require.Len(t, cached, 4)

	// 场景 1: 无代理 → ProxyID nil, Proxy nil（过滤函数会保留）
	assert.Nil(t, cached[0].ProxyID)
	assert.Nil(t, cached[0].Proxy)

	// 场景 2: 活跃代理 → ProxyID 和 Proxy.IsActive() 均正确（过滤函数会保留）
	require.NotNil(t, cached[1].ProxyID)
	assert.Equal(t, int64(10), *cached[1].ProxyID)
	require.NotNil(t, cached[1].Proxy)
	assert.True(t, cached[1].Proxy.IsActive())

	// 场景 3: 禁用代理 → ProxyID 正确，Proxy 非 active（过滤函数会排除）
	require.NotNil(t, cached[2].ProxyID)
	assert.Equal(t, int64(20), *cached[2].ProxyID)
	require.NotNil(t, cached[2].Proxy)
	assert.False(t, cached[2].Proxy.IsActive())

	// 场景 4: 悬空引用 → ProxyID 正确，Proxy 为 nil（过滤函数会排除）
	require.NotNil(t, cached[3].ProxyID)
	assert.Equal(t, int64(30), *cached[3].ProxyID)
	assert.Nil(t, cached[3].Proxy)
}

func TestSnapshotRoundTrip_ErrorAndBannedProxy(t *testing.T) {
	accounts := []service.Account{
		{ID: 1, Platform: service.PlatformOpenAI, ProxyID: ptrInt64(10),
			Proxy: &service.Proxy{ID: 10, Status: service.StatusError}},
		{ID: 2, Platform: service.PlatformOpenAI, ProxyID: ptrInt64(20),
			Proxy: &service.Proxy{ID: 20, Status: service.StatusBanned}},
	}

	cached := simulateSnapshotRoundTrip(t, accounts)
	require.Len(t, cached, 2)

	// error 状态代理 → 非 active（过滤函数会排除）
	require.NotNil(t, cached[0].Proxy)
	assert.Equal(t, service.StatusError, cached[0].Proxy.Status)
	assert.False(t, cached[0].Proxy.IsActive())

	// banned 状态代理 → 非 active（过滤函数会排除）
	require.NotNil(t, cached[1].Proxy)
	assert.Equal(t, service.StatusBanned, cached[1].Proxy.Status)
	assert.False(t, cached[1].Proxy.IsActive())
}
