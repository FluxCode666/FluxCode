//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountPoolModeRetryStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want []int
	}{
		{name: "missing", want: nil},
		{name: "empty disables defaults", raw: []any{}, want: []int{}},
		{name: "normalizes values", raw: []any{float64(503), json.Number("502"), "529", float64(503), float64(502.9), float64(99)}, want: []int{502, 503, 529}},
		{name: "invalid shape uses defaults", raw: "429", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := map[string]any{}
			if tt.raw != nil {
				credentials["pool_mode_retry_status_codes"] = tt.raw
			}
			account := &Account{Credentials: credentials}
			require.Equal(t, tt.want, account.GetPoolModeRetryStatusCodes())
		})
	}
}

func TestAccountIsPoolModeRetryableStatus(t *testing.T) {
	require.True(t, (*Account)(nil).IsPoolModeRetryableStatus(401))
	require.False(t, (*Account)(nil).IsPoolModeRetryableStatus(502))

	account := &Account{Credentials: map[string]any{
		"pool_mode_retry_status_codes": []any{float64(502)},
	}}
	require.True(t, account.IsPoolModeRetryableStatus(502))
	require.False(t, account.IsPoolModeRetryableStatus(401))

	account.Credentials["pool_mode_retry_status_codes"] = []any{}
	require.False(t, account.IsPoolModeRetryableStatus(429))
}

func TestGatewayShouldFailoverAccountSpecificPoolRetryStatus(t *testing.T) {
	service := &GatewayService{}
	account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{
		"pool_mode":                    true,
		"pool_mode_retry_status_codes": []any{float64(404)},
	}}

	require.True(t, service.shouldFailoverUpstreamError(account, 404))
	require.False(t, service.shouldFailoverUpstreamError(account, 400))
}

func TestOpenAIShouldFailoverAccountSpecificPoolRetryStatus(t *testing.T) {
	service := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"pool_mode":                    true,
		"pool_mode_retry_status_codes": []any{float64(404)},
	}}

	require.True(t, service.shouldFailoverOpenAIUpstreamResponse(account, 404, "missing", nil))
	require.False(t, service.shouldFailoverOpenAIUpstreamResponse(account, 400, "invalid request", nil))
}
