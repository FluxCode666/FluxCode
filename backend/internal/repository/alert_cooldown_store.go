package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const alertCooldownKeyPrefix = "alert:cooldown"

type alertCooldownStore struct {
	rdb *redis.Client
}

func NewAlertCooldownStore(rdb *redis.Client) service.AlertCooldownStore {
	return &alertCooldownStore{rdb: rdb}
}

func (s *alertCooldownStore) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return true, nil
	}
	if s == nil || s.rdb == nil {
		return true, nil
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return true, nil
	}

	redisKey := fmt.Sprintf("%s:%s", alertCooldownKeyPrefix, key)
	return s.rdb.SetNX(ctx, redisKey, "1", ttl).Result()
}
