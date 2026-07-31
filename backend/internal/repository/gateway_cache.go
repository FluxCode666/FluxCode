package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"

const (
	providerStickyRoutePrefix  = "provider_route_sticky:"
	providerRouteBindingPrefix = "provider_route_binding:"
)

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

func NewProviderRouteStateStore(rdb *redis.Client) service.ProviderRouteStateStore {
	return &gatewayCache{rdb: rdb}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Get(ctx, key).Int64()
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

func buildProviderStickyRouteKey(
	groupID int64,
	logicalModel string,
	protocol service.ProtocolFamily,
	tier service.RouteTier,
	sessionHash string,
) string {
	digest := service.HashUsageRequestPayload([]byte(strings.Join([]string{
		strings.TrimSpace(logicalModel), string(protocol), string(tier), strings.TrimSpace(sessionHash),
	}, "\x00")))
	return fmt.Sprintf("%s%d:%s", providerStickyRoutePrefix, groupID, digest)
}

func buildProviderRouteBindingKey(responseID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(responseID)))
	return fmt.Sprintf("%s%x", providerRouteBindingPrefix, digest)
}

func (c *gatewayCache) GetProviderStickyRoute(
	ctx context.Context,
	groupID int64,
	logicalModel string,
	protocol service.ProtocolFamily,
	tier service.RouteTier,
	sessionHash string,
) (*service.RouteIdentity, error) {
	encoded, err := c.rdb.Get(ctx, buildProviderStickyRouteKey(groupID, logicalModel, protocol, tier, sessionHash)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var route service.RouteIdentity
	if err := json.Unmarshal(encoded, &route); err != nil {
		return nil, err
	}
	return &route, nil
}

func (c *gatewayCache) SetProviderStickyRoute(
	ctx context.Context,
	groupID int64,
	logicalModel string,
	protocol service.ProtocolFamily,
	tier service.RouteTier,
	sessionHash string,
	route service.RouteIdentity,
	ttl time.Duration,
) error {
	encoded, err := json.Marshal(route)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, buildProviderStickyRouteKey(groupID, logicalModel, protocol, tier, sessionHash), encoded, ttl).Err()
}

func (c *gatewayCache) GetProviderRouteBinding(ctx context.Context, responseID string) (*service.ProviderRouteBinding, error) {
	if strings.TrimSpace(responseID) == "" {
		return nil, nil
	}
	encoded, err := c.rdb.Get(ctx, buildProviderRouteBindingKey(responseID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var binding service.ProviderRouteBinding
	if err := json.Unmarshal(encoded, &binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

func (c *gatewayCache) SetProviderRouteBinding(
	ctx context.Context,
	responseID string,
	binding service.ProviderRouteBinding,
	ttl time.Duration,
) error {
	if strings.TrimSpace(responseID) == "" {
		return nil
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, buildProviderRouteBindingKey(responseID), encoded, ttl).Err()
}

var _ service.ProviderRouteStateStore = (*gatewayCache)(nil)
