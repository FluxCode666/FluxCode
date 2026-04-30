//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSchedulerOutboxStream_RecoversFromMissingStream 验证当 stream/group 被外部清理
// （如 Redis 重启未持久化、运维 FLUSHDB/DEL）之后，Read 不再返回 NOGROUP 错误，
// 而是自动重建 consumer group 并继续工作，避免 outbox worker 陷入 hot loop 报错。
func TestSchedulerOutboxStream_RecoversFromMissingStream(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)

	q := NewSchedulerOutboxQueue(rdb, "test-consumer-recovery")

	// 模拟 stream 被外部清理。
	require.NoError(t, rdb.Del(ctx, schedulerStreamKey).Err())

	// 删除后立即 Read 应自动重建 group 并返回空结果，而不是 NOGROUP。
	events, err := q.Read(ctx, 16, 50*time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, events)

	// 重建后能正常 Publish + Read + Ack 新消息。
	require.NoError(t, q.Publish(ctx, "test_recovery_event", nil, nil, map[string]any{"k": "v"}))

	require.Eventually(t, func() bool {
		got, err := q.Read(ctx, 16, 200*time.Millisecond)
		if err != nil || len(got) == 0 {
			return false
		}
		ids := make([]string, len(got))
		for i, ev := range got {
			ids[i] = ev.StreamID
		}
		return q.Ack(ctx, ids...) == nil
	}, 3*time.Second, 100*time.Millisecond)
}

// TestSchedulerOutboxStream_AckTolerantToMissingGroup 验证当 stream 在 Read 之后
// 被外部删除时，Ack 不返回 NOGROUP 错误（消息已不存在，无需重试）。
func TestSchedulerOutboxStream_AckTolerantToMissingGroup(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)

	q := NewSchedulerOutboxQueue(rdb, "test-consumer-ack")

	require.NoError(t, q.Publish(ctx, "ack_event", nil, nil, nil))

	var received []string
	require.Eventually(t, func() bool {
		got, err := q.Read(ctx, 16, 200*time.Millisecond)
		if err != nil || len(got) == 0 {
			return false
		}
		received = make([]string, len(got))
		for i, ev := range got {
			received[i] = ev.StreamID
		}
		return true
	}, 3*time.Second, 100*time.Millisecond)
	require.NotEmpty(t, received)

	// 模拟 stream 在 Ack 之前被外部清理。
	require.NoError(t, rdb.Del(ctx, schedulerStreamKey).Err())

	require.NoError(t, q.Ack(ctx, received...))
}
