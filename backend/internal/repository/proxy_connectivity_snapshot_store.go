package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyConnectivitySnapshotKeyPrefix = "proxy_connectivity:snapshot"

var proxyConnectivitySnapshotUpsertScript = redis.NewScript(`
	local key = KEYS[1]
	local field = ARGV[1]
	local new_checked = tonumber(ARGV[2])
	local payload = ARGV[3]

	local existing = redis.call('HGET', key, field)
	if existing then
		local ok, existing_data = pcall(cjson.decode, existing)
		if ok and existing_data and existing_data.checked_at_unix_nano then
			local old_checked = tonumber(existing_data.checked_at_unix_nano)
			if old_checked and new_checked <= old_checked then
				return 0
			end
		end
	end

	redis.call('HSET', key, field, payload)
	return 1
`)

type proxyConnectivitySnapshotStore struct {
	rdb *redis.Client
}

type storedProxyConnectivitySnapshot struct {
	Reachable         bool   `json:"reachable"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	LatencyMs         int64  `json:"latency_ms,omitempty"`
	Message           string `json:"message,omitempty"`
	CheckedAt         string `json:"checked_at"`
	CheckedAtUnixNano int64  `json:"checked_at_unix_nano"`
}

func NewProxyConnectivitySnapshotStore(rdb *redis.Client) service.ProxyConnectivitySnapshotStore {
	return &proxyConnectivitySnapshotStore{rdb: rdb}
}

func (s *proxyConnectivitySnapshotStore) GetByProxyIDs(ctx context.Context, platform string, proxyIDs []int64) (map[int64]service.PlatformConnectivityStatus, error) {
	if s == nil || s.rdb == nil {
		return map[int64]service.PlatformConnectivityStatus{}, nil
	}
	platform = normalizeProxyConnectivityPlatform(platform)
	if platform == "" {
		return map[int64]service.PlatformConnectivityStatus{}, nil
	}

	uniqueIDs := uniquePositiveProxyIDs(proxyIDs)
	if len(uniqueIDs) == 0 {
		return map[int64]service.PlatformConnectivityStatus{}, nil
	}

	fields := make([]string, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		fields = append(fields, strconv.FormatInt(id, 10))
	}
	rawItems, err := s.rdb.HMGet(ctx, proxyConnectivitySnapshotHashKey(platform), fields...).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[int64]service.PlatformConnectivityStatus, len(rawItems))
	for i, raw := range rawItems {
		if raw == nil {
			continue
		}
		payload, ok := raw.(string)
		if !ok || strings.TrimSpace(payload) == "" {
			continue
		}
		var stored storedProxyConnectivitySnapshot
		if err := json.Unmarshal([]byte(payload), &stored); err != nil {
			continue
		}

		status := service.PlatformConnectivityStatus{
			Reachable:  stored.Reachable,
			HTTPStatus: stored.HTTPStatus,
			LatencyMs:  stored.LatencyMs,
			Message:    strings.TrimSpace(stored.Message),
		}

		if ts := strings.TrimSpace(stored.CheckedAt); ts != "" {
			if checkedAt, parseErr := time.Parse(time.RFC3339Nano, ts); parseErr == nil {
				status.CheckedAt = checkedAt
			}
		}
		if status.CheckedAt.IsZero() && stored.CheckedAtUnixNano > 0 {
			status.CheckedAt = time.Unix(0, stored.CheckedAtUnixNano)
		}

		result[uniqueIDs[i]] = status
	}
	return result, nil
}

func (s *proxyConnectivitySnapshotStore) UpsertIfNewer(ctx context.Context, platform string, proxyID int64, snapshot service.PlatformConnectivityStatus) error {
	if s == nil || s.rdb == nil || proxyID <= 0 {
		return nil
	}
	platform = normalizeProxyConnectivityPlatform(platform)
	if platform == "" {
		return nil
	}

	checkedAt := snapshot.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	stored := storedProxyConnectivitySnapshot{
		Reachable:         snapshot.Reachable,
		HTTPStatus:        snapshot.HTTPStatus,
		LatencyMs:         snapshot.LatencyMs,
		Message:           strings.TrimSpace(snapshot.Message),
		CheckedAt:         checkedAt.UTC().Format(time.RFC3339Nano),
		CheckedAtUnixNano: checkedAt.UnixNano(),
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("marshal proxy connectivity snapshot: %w", err)
	}

	_, err = proxyConnectivitySnapshotUpsertScript.Run(
		ctx,
		s.rdb,
		[]string{proxyConnectivitySnapshotHashKey(platform)},
		strconv.FormatInt(proxyID, 10),
		stored.CheckedAtUnixNano,
		string(payload),
	).Result()
	if err != nil {
		return err
	}
	return nil
}

func proxyConnectivitySnapshotHashKey(platform string) string {
	return fmt.Sprintf("%s:%s", proxyConnectivitySnapshotKeyPrefix, normalizeProxyConnectivityPlatform(platform))
}

func normalizeProxyConnectivityPlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}

func uniquePositiveProxyIDs(items []int64) []int64 {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(items))
	out := make([]int64, 0, len(items))
	for _, id := range items {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
