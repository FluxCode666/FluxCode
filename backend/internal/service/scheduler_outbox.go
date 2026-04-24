package service

import (
	"context"
	"time"
)

type SchedulerOutboxEvent struct {
	StreamID  string // Redis Stream message ID (e.g. "1619000000000-0")
	EventType string
	AccountID *int64
	GroupID   *int64
	Payload   map[string]any
	CreatedAt time.Time
}

// SchedulerOutboxQueue 基于 Redis Streams 的调度事件队列。
// 替代原有的 PostgreSQL outbox 表，提供更高吞吐量和更低延迟。
type SchedulerOutboxQueue interface {
	// Publish 发布事件到 outbox stream，支持自动去重。
	Publish(ctx context.Context, eventType string, accountID *int64, groupID *int64, payload map[string]any) error
	// Read 从 consumer group 读取最多 count 条事件。
	// 优先返回未 ack 的 pending 消息（崩溃恢复），再返回新消息。
	// blockTimeout > 0 时，在无新消息时阻塞等待直到超时，实现事件驱动消费。
	Read(ctx context.Context, count int64, blockTimeout time.Duration) ([]SchedulerOutboxEvent, error)
	// Ack 确认事件已处理完毕。
	Ack(ctx context.Context, ids ...string) error
	// Pending 返回 stream 中未消费的消息数量。
	Pending(ctx context.Context) (int64, error)
}
