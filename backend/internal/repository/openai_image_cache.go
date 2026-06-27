package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const openAIImageCacheKeyPrefix = "openai:image:proxy:"

type openAIImageCache struct {
	rdb *redis.Client
}

type openAIImageCacheValue struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
}

func NewOpenAIImageCache(rdb *redis.Client) service.OpenAIImageCache {
	return &openAIImageCache{rdb: rdb}
}

func openAIImageCacheKey(id string) string {
	return openAIImageCacheKeyPrefix + strings.TrimSpace(id)
}

func (c *openAIImageCache) SetImage(ctx context.Context, id string, data []byte, contentType string, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("redis client is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("image id is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("image ttl must be positive")
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	payload, err := json.Marshal(openAIImageCacheValue{
		ContentType: contentType,
		Data:        base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, openAIImageCacheKey(id), payload, ttl).Err()
}

func (c *openAIImageCache) GetImage(ctx context.Context, id string) ([]byte, string, error) {
	if c == nil || c.rdb == nil {
		return nil, "", fmt.Errorf("redis client is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, "", service.ErrOpenAIImageCacheNotFound
	}
	raw, err := c.rdb.Get(ctx, openAIImageCacheKey(id)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, "", service.ErrOpenAIImageCacheNotFound
		}
		return nil, "", err
	}
	var payload openAIImageCacheValue
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, "", err
	}
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(payload.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}
