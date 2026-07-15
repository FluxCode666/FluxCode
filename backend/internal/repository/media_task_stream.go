package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	mediaTaskSyncStreamKey  = "media:tasks:sync"
	mediaTaskAsyncStreamKey = "media:tasks:async"
	mediaTaskConsumerGroup  = "media-workers"
	mediaTaskStreamMaxLen   = int64(100000)
	mediaTaskReadBatchSize  = int64(32)
	mediaTaskSyncBurst      = 8
	mediaTaskMaxBlock       = 250 * time.Millisecond
	defaultMediaTaskLease   = time.Minute
)

var mediaTaskConsumerSequence atomic.Uint64

var _ service.MediaTaskQueue = (*MediaTaskStream)(nil)

// MediaTaskStream 使用两个 Redis Streams 保存同步与异步媒体任务。
// Receive 在同一实例内串行化，确保批量读取的内存 backlog 和 8:1 公平计数一致。
type MediaTaskStream struct {
	rdb          *redis.Client
	consumerName string
	lease        time.Duration
	receiveGate  chan struct{}

	stateMu      sync.Mutex
	syncBacklog  []redis.XMessage
	asyncBacklog []redis.XMessage
	bufferedIDs  map[string]struct{}
	inflightIDs  map[string]struct{}
	syncStreak   int
}

// NewMediaTaskStream 创建媒体任务队列。consumerName 在 consumer group 内应唯一。
func NewMediaTaskStream(rdb *redis.Client, consumerName string, lease time.Duration) *MediaTaskStream {
	if consumerName == "" {
		consumerName = newMediaTaskConsumerName()
	}
	if lease <= 0 {
		lease = defaultMediaTaskLease
	}
	return &MediaTaskStream{
		rdb:          rdb,
		consumerName: consumerName,
		lease:        lease,
		receiveGate:  make(chan struct{}, 1),
		bufferedIDs:  make(map[string]struct{}),
		inflightIDs:  make(map[string]struct{}),
	}
}

// ProvideMediaTaskQueue 提供默认一分钟租约的媒体任务队列。
func ProvideMediaTaskQueue(rdb *redis.Client) service.MediaTaskQueue {
	return NewMediaTaskStream(rdb, newMediaTaskConsumerName(), defaultMediaTaskLease)
}

// EnsureGroups 幂等创建两条 Stream 的 consumer group。
// 从 0-0 创建可确保 group 创建前已经入队的消息不会丢失。
func (s *MediaTaskStream) EnsureGroups(ctx context.Context) error {
	for _, stream := range []string{mediaTaskSyncStreamKey, mediaTaskAsyncStreamKey} {
		err := s.rdb.XGroupCreateMkStream(ctx, stream, mediaTaskConsumerGroup, "0-0").Err()
		if err == nil || isRedisBusyGroup(err) {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ensure media task group for %s: %w", stream, err)
	}
	return nil
}

// Enqueue 将任务写入严格对应的优先级 Stream。
func (s *MediaTaskStream) Enqueue(ctx context.Context, taskID int64, priority service.MediaQueuePriority) error {
	if taskID <= 0 {
		return fmt.Errorf("%w: task ID must be positive", service.ErrInvalidMediaQueueMessage)
	}
	stream, err := mediaTaskStreamKey(priority)
	if err != nil {
		return err
	}
	if err := s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: mediaTaskStreamMaxLen,
		Approx: true,
		Values: map[string]any{"task_id": taskID},
	}).Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("enqueue media task: %w", err)
	}
	return nil
}

// Receive 恢复超过租约的 pending 消息后再读取新消息。
// block 小于一毫秒无有效 Redis BLOCK 语义，因此作为无效超时拒绝。
func (s *MediaTaskStream) Receive(ctx context.Context, block time.Duration) (*service.MediaQueueMessage, error) {
	if block < time.Millisecond {
		return nil, fmt.Errorf("%w: block must be at least one millisecond", service.ErrInvalidMediaQueueTimeout)
	}

	deadline := time.Now().Add(block)
	if err := s.acquireReceive(ctx, block); err != nil {
		return nil, err
	}
	defer s.releaseReceive()

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		hasSync, hasAsync, asyncDue := s.backlogState()
		switch {
		case asyncDue && hasAsync:
			return s.popBuffered(service.MediaQueuePriorityAsync)
		case hasSync && !asyncDue:
			return s.popBuffered(service.MediaQueuePrioritySync)
		case !hasSync && hasAsync:
			return s.popBuffered(service.MediaQueuePriorityAsync)
		case hasSync && asyncDue:
			// 已连续返回八条同步任务。先恢复异步 pending，再探测异步新任务；
			// 只有两者都没有时才继续返回同步 backlog。
			if err := s.recoverPending(ctx); err != nil {
				return nil, err
			}
			if _, asyncAvailable, _ := s.backlogState(); asyncAvailable {
				return s.popBuffered(service.MediaQueuePriorityAsync)
			}
			if _, err := s.readNewPriority(ctx, service.MediaQueuePriorityAsync); err != nil {
				return nil, err
			}
			if _, asyncAvailable, _ := s.backlogState(); asyncAvailable {
				return s.popBuffered(service.MediaQueuePriorityAsync)
			}
			return s.popBuffered(service.MediaQueuePrioritySync)
		}

		// backlog 为空时，任何新消息读取前都先领取超过租约的 pending 消息。
		if err := s.recoverPending(ctx); err != nil {
			return nil, err
		}
		if hasSync, hasAsync, _ := s.backlogState(); hasSync || hasAsync {
			continue
		}

		for _, priority := range s.priorityOrder() {
			found, err := s.readNewPriority(ctx, priority)
			if err != nil {
				return nil, err
			}
			if found {
				break
			}
		}
		if hasSync, hasAsync, _ := s.backlogState(); hasSync || hasAsync {
			continue
		}

		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			return nil, service.ErrMediaQueueReceiveTimeout
		}
		chunk := min(remaining, mediaTaskMaxBlock)
		found, err := s.readNewBoth(ctx, chunk)
		if err != nil {
			return nil, err
		}
		if found {
			continue
		}
		if time.Until(deadline) < time.Millisecond {
			return nil, service.ErrMediaQueueReceiveTimeout
		}
	}
}

// Ack 确认一条消息。调用方必须在数据库状态推进成功后显式调用。
// 若 Stream/group 已被清理，PEL 也已不存在；方法重建 group 并按幂等 ACK 成功处理。
func (s *MediaTaskStream) Ack(ctx context.Context, message *service.MediaQueueMessage) error {
	if message == nil || !isValidMediaStreamID(message.ID) || message.TaskID <= 0 {
		return fmt.Errorf("%w: ACK requires message ID and positive task ID", service.ErrInvalidMediaQueueMessage)
	}
	stream, err := mediaTaskStreamKey(message.Priority)
	if err != nil {
		return err
	}
	if err := s.rdb.XAck(ctx, stream, mediaTaskConsumerGroup, message.ID).Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRedisNoGroup(err) {
			return fmt.Errorf("ack media task: %w", err)
		}
		if ensureErr := s.EnsureGroups(ctx); ensureErr != nil {
			return ensureErr
		}
	}
	s.stateMu.Lock()
	delete(s.inflightIDs, mediaDeliveryKey(message.Priority, message.ID))
	s.stateMu.Unlock()
	return nil
}

// PendingCount 返回某优先级已投递但未 ACK 的消息数，供监控和恢复测试使用。
func (s *MediaTaskStream) PendingCount(ctx context.Context, priority service.MediaQueuePriority) (int64, error) {
	stream, err := mediaTaskStreamKey(priority)
	if err != nil {
		return 0, err
	}
	pending, err := s.rdb.XPending(ctx, stream, mediaTaskConsumerGroup).Result()
	if err == nil {
		return pending.Count, nil
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if !isRedisNoGroup(err) && !strings.Contains(err.Error(), "no such key") {
		return 0, fmt.Errorf("count pending media tasks: %w", err)
	}
	if ensureErr := s.EnsureGroups(ctx); ensureErr != nil {
		return 0, ensureErr
	}
	return 0, nil
}

// PublishTerminal 发布 completed/failed 唤醒提示；通知本身不是任务状态事实源。
func (s *MediaTaskStream) PublishTerminal(ctx context.Context, taskID int64, status service.MediaTaskStatus) error {
	if taskID <= 0 || !status.IsTerminal() {
		return fmt.Errorf("%w: positive task ID and terminal status required", service.ErrInvalidMediaTerminalPayload)
	}
	if err := s.rdb.Publish(ctx, terminalChannel(taskID), string(status)).Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("publish media terminal hint: %w", err)
	}
	return nil
}

// SubscribeTerminal 在 Redis 确认订阅建立后返回，避免 subscribe/publish 竞态。
// 输出最多包含一个合法终态，随后由唯一 goroutine 关闭。unsubscribe 可重复调用。
func (s *MediaTaskStream) SubscribeTerminal(ctx context.Context, taskID int64) (<-chan service.MediaTaskStatus, func(), error) {
	if taskID <= 0 {
		return nil, nil, fmt.Errorf("%w: task ID must be positive", service.ErrInvalidMediaTerminalPayload)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	channel := terminalChannel(taskID)
	pubsub := s.rdb.Subscribe(ctx, channel)
	confirmation, err := pubsub.Receive(ctx)
	if err != nil {
		_ = pubsub.Close()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, fmt.Errorf("confirm media terminal subscription: %w", err)
	}
	subscription, ok := confirmation.(*redis.Subscription)
	if !ok || subscription.Kind != "subscribe" || subscription.Channel != channel {
		_ = pubsub.Close()
		return nil, nil, fmt.Errorf("%w: Redis did not confirm terminal subscription", service.ErrInvalidMediaTerminalPayload)
	}

	runCtx, cancel := context.WithCancel(ctx)
	statuses := make(chan service.MediaTaskStatus, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		defer close(statuses)
		defer cancel()
		defer func() { _ = pubsub.Close() }()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[MediaTaskStream] terminal subscription stopped after panic")
			}
		}()

		for {
			message, receiveErr := pubsub.ReceiveMessage(runCtx)
			if receiveErr != nil {
				return
			}
			status := service.MediaTaskStatus(message.Payload)
			if !status.IsTerminal() {
				continue
			}
			select {
			case statuses <- status:
			case <-runCtx.Done():
			}
			return
		}
	}()

	var stopOnce sync.Once
	unsubscribe := func() {
		stopOnce.Do(func() {
			cancel()
			_ = pubsub.Close()
		})
		<-stopped
	}
	return statuses, unsubscribe, nil
}

func (s *MediaTaskStream) acquireReceive(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()
	select {
	case s.receiveGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return service.ErrMediaQueueReceiveTimeout
	}
}

func (s *MediaTaskStream) releaseReceive() {
	<-s.receiveGate
}

func (s *MediaTaskStream) backlogState() (hasSync, hasAsync, asyncDue bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return len(s.syncBacklog) > 0, len(s.asyncBacklog) > 0, s.syncStreak >= mediaTaskSyncBurst
}

func (s *MediaTaskStream) popBuffered(priority service.MediaQueuePriority) (*service.MediaQueueMessage, error) {
	s.stateMu.Lock()
	var raw redis.XMessage
	switch priority {
	case service.MediaQueuePrioritySync:
		raw = s.syncBacklog[0]
		s.syncBacklog = s.syncBacklog[1:]
	case service.MediaQueuePriorityAsync:
		raw = s.asyncBacklog[0]
		s.asyncBacklog = s.asyncBacklog[1:]
	}
	delete(s.bufferedIDs, mediaDeliveryKey(priority, raw.ID))
	s.stateMu.Unlock()

	message, err := parseMediaQueueMessage(raw, priority)
	if err != nil {
		return nil, err
	}
	s.stateMu.Lock()
	s.inflightIDs[mediaDeliveryKey(priority, raw.ID)] = struct{}{}
	if priority == service.MediaQueuePrioritySync {
		s.syncStreak++
	} else {
		s.syncStreak = 0
	}
	s.stateMu.Unlock()
	return message, nil
}

func (s *MediaTaskStream) priorityOrder() []service.MediaQueuePriority {
	_, _, asyncDue := s.backlogState()
	if asyncDue {
		return []service.MediaQueuePriority{service.MediaQueuePriorityAsync, service.MediaQueuePrioritySync}
	}
	return []service.MediaQueuePriority{service.MediaQueuePrioritySync, service.MediaQueuePriorityAsync}
}

func (s *MediaTaskStream) recoverPending(ctx context.Context) error {
	for _, priority := range s.priorityOrder() {
		stream, _ := mediaTaskStreamKey(priority)
		messages, _, err := s.xAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   stream,
			Group:    mediaTaskConsumerGroup,
			Consumer: s.consumerName,
			MinIdle:  s.lease,
			Start:    "0-0",
			Count:    mediaTaskReadBatchSize,
		})
		if err != nil {
			return err
		}
		s.bufferMessages(priority, messages)
	}
	return nil
}

func (s *MediaTaskStream) xAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
	messages, next, err := s.rdb.XAutoClaim(ctx, args).Result()
	if err == nil || err == redis.Nil {
		return messages, next, nil
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	if !isRedisNoGroup(err) {
		return nil, "", fmt.Errorf("recover pending media tasks: %w", err)
	}
	if ensureErr := s.EnsureGroups(ctx); ensureErr != nil {
		return nil, "", ensureErr
	}
	messages, next, err = s.rdb.XAutoClaim(ctx, args).Result()
	if err == nil || err == redis.Nil {
		return messages, next, nil
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	return nil, "", fmt.Errorf("recover pending media tasks after group recreation: %w", err)
}

func (s *MediaTaskStream) readNewPriority(ctx context.Context, priority service.MediaQueuePriority) (bool, error) {
	stream, _ := mediaTaskStreamKey(priority)
	streams, err := s.xReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    mediaTaskConsumerGroup,
		Consumer: s.consumerName,
		Streams:  []string{stream, ">"},
		Count:    mediaTaskReadBatchSize,
		Block:    -1,
	})
	if err != nil {
		return false, err
	}
	return s.bufferStreams(streams), nil
}

func (s *MediaTaskStream) readNewBoth(ctx context.Context, block time.Duration) (bool, error) {
	priorities := s.priorityOrder()
	first, _ := mediaTaskStreamKey(priorities[0])
	second, _ := mediaTaskStreamKey(priorities[1])
	streams, err := s.xReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    mediaTaskConsumerGroup,
		Consumer: s.consumerName,
		Streams:  []string{first, second, ">", ">"},
		Count:    mediaTaskReadBatchSize,
		Block:    block,
	})
	if err != nil {
		return false, err
	}
	return s.bufferStreams(streams), nil
}

func (s *MediaTaskStream) xReadGroup(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error) {
	streams, err := s.rdb.XReadGroup(ctx, args).Result()
	if err == nil || err == redis.Nil {
		return streams, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !isRedisNoGroup(err) {
		return nil, fmt.Errorf("receive media tasks: %w", err)
	}
	if ensureErr := s.EnsureGroups(ctx); ensureErr != nil {
		return nil, ensureErr
	}
	streams, err = s.rdb.XReadGroup(ctx, args).Result()
	if err == nil || err == redis.Nil {
		return streams, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("receive media tasks after group recreation: %w", err)
}

func (s *MediaTaskStream) bufferStreams(streams []redis.XStream) bool {
	found := false
	for _, stream := range streams {
		var priority service.MediaQueuePriority
		switch stream.Stream {
		case mediaTaskSyncStreamKey:
			priority = service.MediaQueuePrioritySync
		case mediaTaskAsyncStreamKey:
			priority = service.MediaQueuePriorityAsync
		default:
			continue
		}
		if len(stream.Messages) > 0 {
			found = true
			s.bufferMessages(priority, stream.Messages)
		}
	}
	return found
}

func (s *MediaTaskStream) bufferMessages(priority service.MediaQueuePriority, messages []redis.XMessage) {
	if len(messages) == 0 {
		return
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	filtered := make([]redis.XMessage, 0, len(messages))
	for _, message := range messages {
		key := mediaDeliveryKey(priority, message.ID)
		if _, exists := s.bufferedIDs[key]; exists {
			continue
		}
		if _, exists := s.inflightIDs[key]; exists {
			continue
		}
		s.bufferedIDs[key] = struct{}{}
		filtered = append(filtered, message)
	}
	if priority == service.MediaQueuePrioritySync {
		s.syncBacklog = append(s.syncBacklog, filtered...)
		return
	}
	s.asyncBacklog = append(s.asyncBacklog, filtered...)
}

func parseMediaQueueMessage(raw redis.XMessage, priority service.MediaQueuePriority) (*service.MediaQueueMessage, error) {
	if raw.ID == "" || !priority.IsValid() {
		return nil, fmt.Errorf("%w: missing stream ID or priority", service.ErrInvalidMediaQueuePayload)
	}
	taskID, err := parseMediaTaskID(raw.Values["task_id"])
	if err != nil || taskID <= 0 {
		return nil, fmt.Errorf("%w: task_id must be a positive integer", service.ErrInvalidMediaQueuePayload)
	}
	return &service.MediaQueueMessage{ID: raw.ID, TaskID: taskID, Priority: priority}, nil
}

func parseMediaTaskID(value any) (int64, error) {
	switch typed := value.(type) {
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	default:
		return 0, service.ErrInvalidMediaQueuePayload
	}
}

func mediaTaskStreamKey(priority service.MediaQueuePriority) (string, error) {
	switch priority {
	case service.MediaQueuePrioritySync:
		return mediaTaskSyncStreamKey, nil
	case service.MediaQueuePriorityAsync:
		return mediaTaskAsyncStreamKey, nil
	default:
		return "", fmt.Errorf("%w: %q", service.ErrInvalidMediaQueuePriority, priority)
	}
}

func terminalChannel(taskID int64) string {
	return "media:task:" + strconv.FormatInt(taskID, 10) + ":terminal"
}

func mediaDeliveryKey(priority service.MediaQueuePriority, messageID string) string {
	return string(priority) + ":" + messageID
}

func isValidMediaStreamID(messageID string) bool {
	first, second, ok := strings.Cut(messageID, "-")
	if !ok || first == "" || second == "" {
		return false
	}
	if _, err := strconv.ParseUint(first, 10, 64); err != nil {
		return false
	}
	if _, err := strconv.ParseUint(second, 10, 64); err != nil {
		return false
	}
	return true
}

func isRedisBusyGroup(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

func isRedisNoGroup(err error) bool {
	return err != nil && strings.Contains(err.Error(), "NOGROUP")
}

func newMediaTaskConsumerName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "media-worker"
	}
	sequence := mediaTaskConsumerSequence.Add(1)
	var randomBytes [6]byte
	suffix := ""
	if _, err := rand.Read(randomBytes[:]); err == nil {
		suffix = hex.EncodeToString(randomBytes[:])
	} else {
		// 时间戳与进程内单调序列共同提供安全 fallback，不输出随机源错误或环境数据。
		suffix = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return fmt.Sprintf("%s-%d-%s-%d", hostname, os.Getpid(), suffix, sequence)
}
