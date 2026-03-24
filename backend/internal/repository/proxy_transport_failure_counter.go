package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const proxyTransportFailureCounterKeyPrefix = "pool_alert:proxy_transport_failure"

type proxyTransportFailureCounter struct {
	rdb *redis.Client
	seq atomic.Uint64
}

func NewProxyTransportFailureCounter(rdb *redis.Client) service.ProxyTransportFailureCounter {
	return &proxyTransportFailureCounter{rdb: rdb}
}

func (c *proxyTransportFailureCounter) IncrementAndCount(ctx context.Context, platform string, proxyID int64, window time.Duration, now time.Time) (int64, error) {
	if c == nil || c.rdb == nil || proxyID <= 0 || window <= 0 {
		return 0, nil
	}
	platform = strings.TrimSpace(strings.ToLower(platform))
	if platform == "" {
		return 0, nil
	}

	key := fmt.Sprintf("%s:%s:%d", proxyTransportFailureCounterKeyPrefix, platform, proxyID)
	nowMs := now.UnixMilli()
	windowStart := now.Add(-window).UnixMilli()
	member := fmt.Sprintf("%d-%d", now.UnixNano(), c.seq.Add(1))

	pipe := c.rdb.TxPipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member})
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("(%d", windowStart))
	countCmd := pipe.ZCount(ctx, key, strconv.FormatInt(windowStart, 10), "+inf")
	pipe.Expire(ctx, key, window+2*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}

	count, err := countCmd.Result()
	if err != nil {
		return 0, err
	}
	return count, nil
}
