package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	schedulerStreamKey   = "sched:outbox:stream"
	schedulerGroupName   = "sched:outbox:workers"
	schedulerDedupPrefix = "sched:outbox:dd:"
	schedulerDedupTTL    = 10 * time.Second
	// schedulerStreamMaxLen 限制 stream 长度，XADD 时自动裁剪。
	// 使用近似裁剪（~）避免每次 XADD 都精确计数。
	schedulerStreamMaxLen int64 = 100000
)

type schedulerOutboxStream struct {
	rdb          *redis.Client
	consumerName string
}

// NewSchedulerOutboxQueue 创建基于 Redis Streams 的调度事件队列。
// consumerName 应在同一 consumer group 内唯一（建议使用 hostname）。
func NewSchedulerOutboxQueue(rdb *redis.Client, consumerName string) service.SchedulerOutboxQueue {
	if consumerName == "" {
		consumerName = "default"
	}
	q := &schedulerOutboxStream{rdb: rdb, consumerName: consumerName}
	q.ensureGroup()
	return q
}

// ensureGroup 确保 consumer group 存在。
// 使用 $ 作为起始 ID，表示只消费 group 创建之后的新消息。
func (q *schedulerOutboxStream) ensureGroup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := q.rdb.XGroupCreateMkStream(ctx, schedulerStreamKey, schedulerGroupName, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		log.Printf("[SchedulerOutboxQueue] create consumer group: %v", err)
	}
}

func (q *schedulerOutboxStream) Publish(ctx context.Context, eventType string, accountID *int64, groupID *int64, payload map[string]any) error {
	values := map[string]any{
		"t":  eventType,
		"ts": time.Now().UnixMilli(),
	}
	if accountID != nil {
		values["aid"] = *accountID
	}
	if groupID != nil {
		values["gid"] = *groupID
	}
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		values["p"] = string(encoded)
	}

	xaddArgs := &redis.XAddArgs{
		Stream: schedulerStreamKey,
		MaxLen: schedulerStreamMaxLen,
		Approx: true,
		Values: values,
	}

	// 去重：支持去重的事件类型通过 SET NX EX 实现幂等
	if schedulerOutboxEventSupportsDedup(eventType) {
		dedupKey := schedulerDedupKey(eventType, accountID, groupID)
		ok, err := q.rdb.SetNX(ctx, dedupKey, 1, schedulerDedupTTL).Result()
		if err != nil {
			// 去重检查失败不阻塞发布，降级为不去重
		} else if !ok {
			return nil // 去重命中，跳过
		}
	}

	return q.rdb.XAdd(ctx, xaddArgs).Err()
}

func (q *schedulerOutboxStream) Read(ctx context.Context, count int64, blockTimeout time.Duration) ([]service.SchedulerOutboxEvent, error) {
	// Phase 1: 优先处理 pending 消息（崩溃恢复）
	pending, err := q.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    schedulerGroupName,
		Consumer: q.consumerName,
		Streams:  []string{schedulerStreamKey, "0"},
		Count:    count,
	}).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	if len(pending) > 0 && len(pending[0].Messages) > 0 {
		return parseStreamMessages(pending[0].Messages), nil
	}

	// Phase 2: 读取新消息
	// blockTimeout > 0 时在 Redis 侧阻塞等待，有新消息立即返回，
	// 避免空轮询的 CPU 和网络开销。
	args := &redis.XReadGroupArgs{
		Group:    schedulerGroupName,
		Consumer: q.consumerName,
		Streams:  []string{schedulerStreamKey, ">"},
		Count:    count,
	}
	if blockTimeout > 0 {
		args.Block = blockTimeout
	}
	streams, err := q.rdb.XReadGroup(ctx, args).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, nil
	}
	return parseStreamMessages(streams[0].Messages), nil
}

func (q *schedulerOutboxStream) Ack(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return q.rdb.XAck(ctx, schedulerStreamKey, schedulerGroupName, ids...).Err()
}

func (q *schedulerOutboxStream) Pending(ctx context.Context) (int64, error) {
	info, err := q.rdb.XInfoGroups(ctx, schedulerStreamKey).Result()
	if err != nil {
		// stream 不存在时返回 0
		if strings.Contains(err.Error(), "no such key") {
			return 0, nil
		}
		return 0, err
	}
	for _, g := range info {
		if g.Name == schedulerGroupName {
			// Lag = 未投递给 consumer group 的新消息数（-1 表示无法确定）
			// Pending = 已投递但未 ACK 的消息数
			lag := g.Lag
			if lag < 0 {
				lag = 0
			}
			return lag + int64(g.Pending), nil
		}
	}
	return 0, nil
}

// --- helpers ---

func parseStreamMessages(msgs []redis.XMessage) []service.SchedulerOutboxEvent {
	events := make([]service.SchedulerOutboxEvent, 0, len(msgs))
	for _, msg := range msgs {
		events = append(events, parseStreamMessage(msg))
	}
	return events
}

func parseStreamMessage(msg redis.XMessage) service.SchedulerOutboxEvent {
	event := service.SchedulerOutboxEvent{
		StreamID: msg.ID,
	}

	if v, ok := msg.Values["t"]; ok {
		event.EventType, _ = v.(string)
	}

	if v, ok := msg.Values["ts"]; ok {
		if s, ok := v.(string); ok {
			if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
				event.CreatedAt = time.UnixMilli(ms)
			}
		}
	}
	if event.CreatedAt.IsZero() {
		// 从 stream ID 解析时间戳（格式 "1619000000000-0"）
		if parts := strings.SplitN(msg.ID, "-", 2); len(parts) == 2 {
			if ms, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				event.CreatedAt = time.UnixMilli(ms)
			}
		}
	}

	if v, ok := msg.Values["aid"]; ok {
		if s, ok := v.(string); ok {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				event.AccountID = &id
			}
		}
	}

	if v, ok := msg.Values["gid"]; ok {
		if s, ok := v.(string); ok {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				event.GroupID = &id
			}
		}
	}

	if v, ok := msg.Values["p"]; ok {
		if s, ok := v.(string); ok && s != "" {
			var payload map[string]any
			if err := json.Unmarshal([]byte(s), &payload); err == nil {
				event.Payload = payload
			}
		}
	}

	return event
}

func schedulerDedupKey(eventType string, accountID *int64, groupID *int64) string {
	var aid, gid string
	if accountID != nil {
		aid = strconv.FormatInt(*accountID, 10)
	}
	if groupID != nil {
		gid = strconv.FormatInt(*groupID, 10)
	}
	return fmt.Sprintf("%s%s:%s:%s", schedulerDedupPrefix, eventType, aid, gid)
}
