//go:build integration

package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type OpenAIImageCacheSuite struct {
	IntegrationRedisSuite
	cache service.OpenAIImageCache
}

func (s *OpenAIImageCacheSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewOpenAIImageCache(s.rdb)
}

func (s *OpenAIImageCacheSuite) TestSetAndGetImageWithTTL() {
	err := s.cache.SetImage(s.ctx, "img_1", []byte("png-bytes"), "image/png", 3*time.Hour)
	require.NoError(s.T(), err)

	data, contentType, err := s.cache.GetImage(s.ctx, "img_1")
	require.NoError(s.T(), err)
	require.Equal(s.T(), []byte("png-bytes"), data)
	require.Equal(s.T(), "image/png", contentType)

	s.AssertTTLWithin(s.rdb.TTL(s.ctx, openAIImageCacheKey("img_1")).Val(), 2*time.Hour, 3*time.Hour)
}

func (s *OpenAIImageCacheSuite) TestGetImageMissingReturnsTypedError() {
	_, _, err := s.cache.GetImage(s.ctx, "missing")
	require.True(s.T(), errors.Is(err, service.ErrOpenAIImageCacheNotFound))
	require.False(s.T(), errors.Is(err, redis.Nil))
}

func TestOpenAIImageCacheSuite(t *testing.T) {
	suite.Run(t, new(OpenAIImageCacheSuite))
}
