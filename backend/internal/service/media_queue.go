package service

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrInvalidMediaQueuePriority 表示调用方传入了未知队列优先级。
	ErrInvalidMediaQueuePriority = errors.New("invalid media queue priority")
	// ErrInvalidMediaQueueMessage 表示待入队或待确认的消息缺少有效标识。
	ErrInvalidMediaQueueMessage = errors.New("invalid media queue message")
	// ErrInvalidMediaQueuePayload 表示队列中的任务载荷无法安全解析。
	ErrInvalidMediaQueuePayload = errors.New("invalid media queue payload")
	// ErrInvalidMediaQueueTimeout 表示 Receive 的等待时长不足一毫秒。
	ErrInvalidMediaQueueTimeout = errors.New("invalid media queue timeout")
	// ErrMediaQueueReceiveTimeout 表示 Receive 在指定时长内没有取得消息。
	ErrMediaQueueReceiveTimeout = errors.New("media queue receive timeout")
	// ErrInvalidMediaTerminalPayload 表示终态通知的任务 ID 或状态无效。
	ErrInvalidMediaTerminalPayload = errors.New("invalid media terminal payload")
)

// MediaQueuePriority 区分同步等待请求与显式异步请求的调度优先级。
type MediaQueuePriority string

const (
	MediaQueuePrioritySync  MediaQueuePriority = "sync"
	MediaQueuePriorityAsync MediaQueuePriority = "async"
)

// IsValid 报告优先级是否属于队列支持的严格枚举。
func (p MediaQueuePriority) IsValid() bool {
	return p == MediaQueuePrioritySync || p == MediaQueuePriorityAsync
}

// MediaQueueMessage 是一次 Redis Stream 投递；调用方只应在完成数据库推进后 ACK。
type MediaQueueMessage struct {
	ID       string
	TaskID   int64
	Priority MediaQueuePriority
}

// MediaTaskQueue 提供媒体任务的双优先级投递和终态唤醒通知。
//
// SubscribeTerminal 的消息只是低延迟 wake-up hint，并非事实源。调用方必须先订阅，
// 再读取数据库；无论收到终态、订阅关闭或上下文取消，都必须重新读取数据库确认状态。
// Receive 在 block 内没有消息时返回 ErrMediaQueueReceiveTimeout；上下文取消错误保持原样。
type MediaTaskQueue interface {
	EnsureGroups(ctx context.Context) error
	Enqueue(ctx context.Context, taskID int64, priority MediaQueuePriority) error
	Receive(ctx context.Context, block time.Duration) (*MediaQueueMessage, error)
	Ack(ctx context.Context, message *MediaQueueMessage) error
	PublishTerminal(ctx context.Context, taskID int64, status MediaTaskStatus) error
	SubscribeTerminal(ctx context.Context, taskID int64) (<-chan MediaTaskStatus, func(), error)
}
