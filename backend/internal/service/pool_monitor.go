package service

import (
	"context"
	"strings"
	"time"
)

const (
	// DisabledProxyScheduleModeDirectWithoutProxy means accounts with a disabled
	// proxy are still schedulable but route traffic directly (no proxy).
	DisabledProxyScheduleModeDirectWithoutProxy = "direct_without_proxy"
	// DisabledProxyScheduleModeExcludeAccount means accounts with a disabled
	// proxy are excluded from the available pool entirely.
	DisabledProxyScheduleModeExcludeAccount = "exclude_account"

	defaultPoolMonitorAvailableCountThreshold = 0
	defaultPoolMonitorAvailableRatioThreshold = 20
	defaultPoolMonitorCheckIntervalMinutes    = 5
	defaultProxyProbeIntervalMinutes          = 5
	defaultProxyFailureWindowMinutes          = 10
	defaultProxyFailureThreshold              = 5
	defaultProxyActiveProbeEnabled            = true
	defaultDisabledProxyScheduleMode          = DisabledProxyScheduleModeDirectWithoutProxy
)

// AccountPoolAlertConfig stores platform-level pool monitor config.
type AccountPoolAlertConfig struct {
	Platform                  string
	PoolThresholdEnabled      bool
	ProxyFailureEnabled       bool
	ProxyActiveProbeEnabled   bool
	DisabledProxyScheduleMode string
	AvailableCountThreshold   int
	AvailableRatioThreshold   int
	CheckIntervalMinutes      int
	ProxyProbeIntervalMinutes int
	ProxyFailureWindowMinutes int
	ProxyFailureThreshold     int
	AlertEmails               []string
	AlertCooldownMinutes      int
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// PoolThresholdAlertConfig is runtime config for pool threshold worker.
type PoolThresholdAlertConfig struct {
	Enabled                 bool
	AvailableCountThreshold int
	AvailableRatioThreshold int
	CheckIntervalMinutes    int
}

// ProxyFailureAlertConfig is runtime config for proxy failure monitoring.
type ProxyFailureAlertConfig struct {
	Enabled                   bool
	ProxyFailureWindowMinutes int
	ProxyFailureThreshold     int
}

// PoolThresholdEvaluation is the result of one account pool threshold check.
type PoolThresholdEvaluation struct {
	Triggered         bool
	CountTriggered    bool
	RatioTriggered    bool
	AvailableCount    int
	BaseAccountCount  int
	AvailableRatioPct float64
}

// ProxyFailureCheckResult is the result of one proxy transport failure event.
type ProxyFailureCheckResult struct {
	Enabled       bool
	Triggered     bool
	Count         int64
	WindowMinutes int
	Threshold     int
}

// PoolAlertConfigRepository persists account pool monitoring configs.
type PoolAlertConfigRepository interface {
	GetByPlatform(ctx context.Context, platform string) (*AccountPoolAlertConfig, error)
	Upsert(ctx context.Context, cfg *AccountPoolAlertConfig) error
}

// PoolAlertConfigCache stores account pool config for fast reads.
type PoolAlertConfigCache interface {
	GetByPlatform(ctx context.Context, platform string) (*AccountPoolAlertConfig, error)
	Set(ctx context.Context, cfg *AccountPoolAlertConfig) error
	Delete(ctx context.Context, platform string) error
}

// ProxyTransportFailureCounter stores sliding-window proxy transport failures.
type ProxyTransportFailureCounter interface {
	IncrementAndCount(ctx context.Context, platform string, proxyID int64, window time.Duration, now time.Time) (int64, error)
}

// normalizeDisabledProxyScheduleMode normalizes and validates the disabled-proxy
// scheduling mode string, falling back to the default if empty or unrecognized.
func normalizeDisabledProxyScheduleMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case DisabledProxyScheduleModeDirectWithoutProxy, DisabledProxyScheduleModeExcludeAccount:
		return mode
	default:
		return defaultDisabledProxyScheduleMode
	}
}

// filterAccountsByDisabledProxyScheduleMode filters out accounts whose assigned
// proxy is disabled, according to the configured scheduling mode.
//
// - "direct_without_proxy": keep all accounts (they will route without proxy).
// - "exclude_account": remove accounts that reference a non-nil ProxyID
//
//	(these are assumed to need a proxy that is currently disabled).
//
// If the mode is unrecognized the accounts are returned unmodified (fail-open).
func filterAccountsByDisabledProxyScheduleMode(accounts []Account, mode string) []Account {
	mode = normalizeDisabledProxyScheduleMode(mode)
	if mode != DisabledProxyScheduleModeExcludeAccount {
		return accounts
	}
	// In exclude_account mode, drop accounts that have a ProxyID assigned
	// (those accounts depend on a proxy; if the proxy is disabled they
	// should not be counted as available).
	filtered := make([]Account, 0, len(accounts))
	for _, a := range accounts {
		if a.ProxyID == nil {
			filtered = append(filtered, a)
		}
	}
	return filtered
}
