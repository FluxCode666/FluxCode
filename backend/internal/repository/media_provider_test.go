package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestMediaTaskQueueProviderUsesDeploymentLease(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{MediaTasks: config.MediaTaskConfig{LeaseTTLSeconds: 37}}

	queue := ProvideMediaTaskQueue(rdb, cfg)
	stream, ok := queue.(*MediaTaskStream)
	require.True(t, ok)
	require.Equal(t, 37*time.Second, stream.lease)
	require.NotEmpty(t, stream.consumerName)
}

func TestMediaHTTPContentReaderProviderUsesDeploymentLimits(t *testing.T) {
	cfg := &config.Config{MediaTasks: config.MediaTaskConfig{
		ContentProxyTimeoutSeconds: 41,
		MaxContentBytes:            123456,
	}}

	reader := ProvideMediaHTTPContentReader(&mediaHTTPUpstreamStub{}, cfg)
	concrete, ok := reader.(*mediaHTTPContentReader)
	require.True(t, ok)
	require.Equal(t, 41*time.Second, concrete.timeout)
	require.Equal(t, int64(123456), concrete.maxBytes)
}
