package service

import (
	"context"
	"strings"
	"time"
)

const (
	PlatformConnectivityOpenAI = PlatformOpenAI
)

// PlatformConnectivityStatus describes one platform connectivity probe result.
type PlatformConnectivityStatus struct {
	Reachable  bool
	HTTPStatus int
	LatencyMs  int64
	Message    string
	CheckedAt  time.Time
}

// PlatformConnectivity stores connectivity probe results grouped by platform.
type PlatformConnectivity map[string]PlatformConnectivityStatus

func normalizeConnectivityPlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func (c PlatformConnectivity) Clone() PlatformConnectivity {
	if len(c) == 0 {
		return nil
	}
	out := make(PlatformConnectivity, len(c))
	for k, v := range c {
		key := normalizeConnectivityPlatform(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProxyConnectivitySnapshotStore persists latest platform connectivity snapshots for proxies.
type ProxyConnectivitySnapshotStore interface {
	GetByProxyIDs(ctx context.Context, platform string, proxyIDs []int64) (map[int64]PlatformConnectivityStatus, error)
	UpsertIfNewer(ctx context.Context, platform string, proxyID int64, snapshot PlatformConnectivityStatus) error
}
