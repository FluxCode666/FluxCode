package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
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
	mediaTaskMaxBlock       = 50 * time.Millisecond
	mediaTaskRedisIOTimeout = 100 * time.Millisecond
	defaultMediaTaskLease   = time.Minute
)

var mediaTaskConsumerSequence atomic.Uint64

var _ service.MediaTaskQueue = (*MediaTaskStream)(nil)

type mediaReceiveDeadlineSource uint8

const (
	mediaReceiveDeadlineInternal mediaReceiveDeadlineSource = iota
	mediaReceiveDeadlineParent
)

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
	claimCursor  map[service.MediaQueuePriority]string
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
		claimCursor: map[service.MediaQueuePriority]string{
			service.MediaQueuePrioritySync:  "0-0",
			service.MediaQueuePriorityAsync: "0-0",
		},
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
func (s *MediaTaskStream) Receive(parent context.Context, block time.Duration) (message *service.MediaQueueMessage, err error) {
	if block < time.Millisecond {
		return nil, fmt.Errorf("%w: block must be at least one millisecond", service.ErrInvalidMediaQueueTimeout)
	}

	internalDeadline := time.Now().Add(block)
	effectiveDeadline := internalDeadline
	deadlineSource := mediaReceiveDeadlineInternal
	if parentDeadline, ok := parent.Deadline(); ok && !parentDeadline.After(internalDeadline) {
		effectiveDeadline = parentDeadline
		deadlineSource = mediaReceiveDeadlineParent
	}
	receiveCtx, cancel := context.WithDeadline(parent, effectiveDeadline)
	defer cancel()
	defer func() {
		if err != nil {
			err = normalizeMediaReceiveError(parent, receiveCtx, deadlineSource, effectiveDeadline, err)
		}
	}()

	if err := s.acquireReceive(receiveCtx); err != nil {
		return nil, err
	}
	defer s.releaseReceive()

	for {
		if err := receiveCtx.Err(); err != nil {
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
			if err := s.recoverPriority(receiveCtx, service.MediaQueuePriorityAsync); err != nil {
				return nil, err
			}
			if _, asyncAvailable, _ := s.backlogState(); asyncAvailable {
				return s.popBuffered(service.MediaQueuePriorityAsync)
			}
			if _, err := s.readNewPriority(receiveCtx, service.MediaQueuePriorityAsync); err != nil {
				return nil, err
			}
			if _, asyncAvailable, _ := s.backlogState(); asyncAvailable {
				return s.popBuffered(service.MediaQueuePriorityAsync)
			}
			return s.popBuffered(service.MediaQueuePrioritySync)
		}

		// backlog 为空时，任何新消息读取前都先领取超过租约的 pending 消息。
		if err := s.recoverPending(receiveCtx); err != nil {
			return nil, err
		}
		if hasSync, hasAsync, _ := s.backlogState(); hasSync || hasAsync {
			continue
		}

		for _, priority := range s.priorityOrder() {
			found, err := s.readNewPriority(receiveCtx, priority)
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

		deadline, _ := receiveCtx.Deadline()
		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			return nil, context.DeadlineExceeded
		}
		chunk := min(remaining, mediaTaskMaxBlock)
		started := time.Now()
		found, err := s.readNewBoth(receiveCtx, chunk)
		if err != nil {
			return nil, err
		}
		if found {
			continue
		}
		if wait := chunk - time.Since(started); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-receiveCtx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil, receiveCtx.Err()
			}
		}
		if time.Until(deadline) < time.Millisecond {
			return nil, context.DeadlineExceeded
		}
	}
}

// Ack 确认一条消息。调用方必须在数据库状态推进成功后显式调用。
// Stream 不存在时重建 group 并按幂等成功处理；Stream 仍在但 group/PEL 丢失时
// 返回 ErrMediaQueueDeliveryStateLost，XACK 返回 0 时返回 ErrMediaQueueMessageNotPending。
func (s *MediaTaskStream) Ack(ctx context.Context, message *service.MediaQueueMessage) error {
	if message == nil || !isValidMediaStreamID(message.ID) || message.TaskID <= 0 {
		return fmt.Errorf("%w: ACK requires message ID and positive task ID", service.ErrInvalidMediaQueueMessage)
	}
	stream, err := mediaTaskStreamKey(message.Priority)
	if err != nil {
		return err
	}
	acked, err := s.rdb.XAck(ctx, stream, mediaTaskConsumerGroup, message.ID).Result()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRedisNoGroup(err) {
			return fmt.Errorf("ack media task: %w", err)
		}
		exists, existsErr := s.rdb.Exists(ctx, stream).Result()
		if existsErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("inspect media task stream after NOGROUP: %w", existsErr)
		}
		if ensureErr := s.EnsureGroups(ctx); ensureErr != nil {
			return ensureErr
		}
		if exists > 0 {
			return fmt.Errorf("%w: consumer group disappeared before ACK", service.ErrMediaQueueDeliveryStateLost)
		}
		return nil
	}
	if acked == 0 {
		return fmt.Errorf("%w: message %s is not pending in %s", service.ErrMediaQueueMessageNotPending, message.ID, message.Priority)
	}
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
	subscriptionCtx, cancel := context.WithCancel(ctx)
	subscriptionClient := newMediaTerminalSubscriptionClient(s.rdb, subscriptionCtx)
	var closeClientOnce sync.Once
	closeSubscriptionClient := func() {
		closeClientOnce.Do(func() {
			_ = subscriptionClient.Close()
		})
	}
	watcherDone := make(chan struct{})
	stopAfterFunc := context.AfterFunc(subscriptionCtx, func() {
		defer close(watcherDone)
		closeSubscriptionClient()
	})
	var stopWatcherOnce sync.Once
	stopCloseWatcher := func() {
		stopWatcherOnce.Do(func() {
			if stopAfterFunc() {
				close(watcherDone)
			}
		})
		<-watcherDone
	}
	pubsub := subscriptionClient.Subscribe(subscriptionCtx, channel)
	cleanupConfirmation := func() {
		cancel()
		closeSubscriptionClient()
		_ = pubsub.Close()
		stopCloseWatcher()
	}
	confirmation, err := pubsub.Receive(subscriptionCtx)
	if err != nil {
		cleanupConfirmation()
		if contextErr := originalContextError(ctx, err); contextErr != nil {
			return nil, nil, contextErr
		}
		return nil, nil, fmt.Errorf("confirm media terminal subscription: %w", err)
	}
	subscription, ok := confirmation.(*redis.Subscription)
	if !ok || subscription.Kind != "subscribe" || subscription.Channel != channel {
		cleanupConfirmation()
		return nil, nil, fmt.Errorf("%w: Redis did not confirm terminal subscription", service.ErrInvalidMediaTerminalPayload)
	}

	statuses := make(chan service.MediaTaskStatus, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		defer close(statuses)
		defer cancel()
		defer stopCloseWatcher()
		defer closeSubscriptionClient()
		defer func() { _ = pubsub.Close() }()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[MediaTaskStream] terminal subscription stopped after panic")
			}
		}()

		for {
			message, receiveErr := pubsub.ReceiveMessage(subscriptionCtx)
			if receiveErr != nil {
				return
			}
			status := service.MediaTaskStatus(message.Payload)
			if !status.IsTerminal() {
				continue
			}
			select {
			case statuses <- status:
			case <-subscriptionCtx.Done():
			}
			return
		}
	}()

	var stopOnce sync.Once
	unsubscribe := func() {
		stopOnce.Do(func() {
			cancel()
			closeSubscriptionClient()
			_ = pubsub.Close()
		})
		<-stopped
	}
	return statuses, unsubscribe, nil
}

func (s *MediaTaskStream) acquireReceive(ctx context.Context) error {
	select {
	case s.receiveGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
		if err := s.recoverPriority(ctx, priority); err != nil {
			return err
		}
	}
	return nil
}

func (s *MediaTaskStream) recoverPriority(ctx context.Context, priority service.MediaQueuePriority) error {
	stream, _ := mediaTaskStreamKey(priority)
	start := s.autoClaimCursor(priority)
	messages, next, err := s.xAutoClaim(ctx, priority, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    mediaTaskConsumerGroup,
		Consumer: s.consumerName,
		MinIdle:  s.lease,
		Start:    start,
		Count:    mediaTaskReadBatchSize,
	})
	if err != nil {
		return err
	}
	s.setAutoClaimCursor(priority, next)
	s.bufferMessages(priority, messages)
	return nil
}

func (s *MediaTaskStream) xAutoClaim(ctx context.Context, priority service.MediaQueuePriority, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
	client, commandCtx, cancel, err := s.receiveRedisCommand(ctx)
	if err != nil {
		return nil, "", err
	}
	messages, next, err := client.XAutoClaim(commandCtx, args).Result()
	cancel()
	if err == nil || err == redis.Nil {
		return messages, next, nil
	}
	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	if !isRedisNoGroup(err) {
		return nil, "", fmt.Errorf("recover pending media tasks: %w", err)
	}
	s.setAutoClaimCursor(priority, "0-0")
	if ensureErr := s.ensureGroupsForReceive(ctx); ensureErr != nil {
		return nil, "", ensureErr
	}
	retryArgs := *args
	retryArgs.Start = "0-0"
	client, commandCtx, cancel, err = s.receiveRedisCommand(ctx)
	if err != nil {
		return nil, "", err
	}
	messages, next, err = client.XAutoClaim(commandCtx, &retryArgs).Result()
	cancel()
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
	client, commandCtx, cancel, err := s.receiveRedisCommand(ctx)
	if err != nil {
		return nil, err
	}
	streams, err := client.XReadGroup(commandCtx, args).Result()
	cancel()
	if err == nil || err == redis.Nil {
		return streams, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !isRedisNoGroup(err) {
		return nil, fmt.Errorf("receive media tasks: %w", err)
	}
	for _, stream := range args.Streams[:len(args.Streams)/2] {
		if priority, ok := mediaTaskPriorityForStream(stream); ok {
			s.setAutoClaimCursor(priority, "0-0")
		}
	}
	if ensureErr := s.ensureGroupsForReceive(ctx); ensureErr != nil {
		return nil, ensureErr
	}
	client, commandCtx, cancel, err = s.receiveRedisCommand(ctx)
	if err != nil {
		return nil, err
	}
	streams, err = client.XReadGroup(commandCtx, args).Result()
	cancel()
	if err == nil || err == redis.Nil {
		return streams, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, fmt.Errorf("receive media tasks after group recreation: %w", err)
}

func (s *MediaTaskStream) ensureGroupsForReceive(ctx context.Context) error {
	for _, stream := range []string{mediaTaskSyncStreamKey, mediaTaskAsyncStreamKey} {
		client, commandCtx, cancel, err := s.receiveRedisCommand(ctx)
		if err != nil {
			return err
		}
		err = client.XGroupCreateMkStream(commandCtx, stream, mediaTaskConsumerGroup, "0-0").Err()
		cancel()
		if err == nil || isRedisBusyGroup(err) {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ensure media task group for %s while receiving: %w", stream, err)
	}
	return nil
}

func (s *MediaTaskStream) receiveRedisCommand(ctx context.Context) (*redis.Client, context.Context, context.CancelFunc, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, nil, fmt.Errorf("receive media task command requires deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		return nil, nil, nil, context.DeadlineExceeded
	}
	timeout := min(remaining, mediaTaskRedisIOTimeout)
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	client := s.rdb.WithTimeout(timeout)
	// WithTimeout clones Options before sharing the underlying pool, so this does not
	// mutate the application client while allowing command deadlines to beat the
	// XREADGROUP BLOCK read-timeout grace used by go-redis.
	client.Options().ContextTimeoutEnabled = true
	return client, commandCtx, cancel, nil
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
		s.bufferedIDs[key] = struct{}{}
		filtered = append(filtered, message)
	}
	if priority == service.MediaQueuePrioritySync {
		s.syncBacklog = append(s.syncBacklog, filtered...)
		return
	}
	s.asyncBacklog = append(s.asyncBacklog, filtered...)
}

func (s *MediaTaskStream) autoClaimCursor(priority service.MediaQueuePriority) string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	start := s.claimCursor[priority]
	if start == "" {
		return "0-0"
	}
	return start
}

func (s *MediaTaskStream) setAutoClaimCursor(priority service.MediaQueuePriority, next string) {
	if next == "" {
		next = "0-0"
	}
	s.stateMu.Lock()
	s.claimCursor[priority] = next
	s.stateMu.Unlock()
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

func mediaTaskPriorityForStream(stream string) (service.MediaQueuePriority, bool) {
	switch stream {
	case mediaTaskSyncStreamKey:
		return service.MediaQueuePrioritySync, true
	case mediaTaskAsyncStreamKey:
		return service.MediaQueuePriorityAsync, true
	default:
		return "", false
	}
}

func normalizeMediaReceiveError(
	parent context.Context,
	receiveCtx context.Context,
	deadlineSource mediaReceiveDeadlineSource,
	effectiveDeadline time.Time,
	err error,
) error {
	parentErr := parent.Err()
	if errors.Is(parentErr, context.Canceled) {
		return context.Canceled
	}
	if deadlineSource == mediaReceiveDeadlineParent {
		if parentErr != nil {
			return parentErr
		}
		if receiveErr := receiveCtx.Err(); receiveErr != nil {
			return receiveErr
		}
	} else if receiveCtx.Err() != nil {
		return service.ErrMediaQueueReceiveTimeout
	}

	var networkErr net.Error
	remaining := time.Until(effectiveDeadline)
	if remaining <= mediaTaskMaxBlock && errors.As(err, &networkErr) && networkErr.Timeout() {
		if remaining > 0 {
			timer := time.NewTimer(remaining)
			select {
			case <-receiveCtx.Done():
			case <-timer.C:
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		if parentErr := parent.Err(); parentErr != nil {
			if errors.Is(parentErr, context.Canceled) || deadlineSource == mediaReceiveDeadlineParent {
				return parentErr
			}
		}
		if deadlineSource == mediaReceiveDeadlineParent {
			if receiveErr := receiveCtx.Err(); receiveErr != nil {
				return receiveErr
			}
			return context.DeadlineExceeded
		}
		return service.ErrMediaQueueReceiveTimeout
	}
	return err
}

func originalContextError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	var networkErr net.Error
	deadline, hasDeadline := ctx.Deadline()
	remaining := time.Until(deadline)
	if !hasDeadline || remaining > mediaTaskMaxBlock || !errors.As(err, &networkErr) || !networkErr.Timeout() {
		return nil
	}
	if remaining > 0 {
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if time.Until(deadline) <= 0 {
		return context.DeadlineExceeded
	}
	return nil
}

func terminalChannel(taskID int64) string {
	return "media:task:" + strconv.FormatInt(taskID, 10) + ":terminal"
}

func newMediaTerminalSubscriptionClient(parent *redis.Client, lifecycleCtx context.Context) *redis.Client {
	options := *parent.Options()
	options.ContextTimeoutEnabled = true
	baseDialer := options.Dialer
	if baseDialer == nil {
		baseDialer = redis.NewDialer(&options)
	}
	options.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := baseDialer(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		wrapped := &mediaSubscriptionConn{Conn: conn}
		wrapped.stopContextClose = context.AfterFunc(lifecycleCtx, wrapped.closeUnderlying)
		return wrapped, nil
	}
	return redis.NewClient(&options)
}

type mediaSubscriptionConn struct {
	net.Conn
	closeOnce        sync.Once
	closeErr         error
	stopContextClose func() bool
}

func (c *mediaSubscriptionConn) Close() error {
	if c.stopContextClose != nil {
		c.stopContextClose()
	}
	c.closeUnderlying()
	return c.closeErr
}

func (c *mediaSubscriptionConn) closeUnderlying() {
	c.closeOnce.Do(func() {
		c.closeErr = c.Conn.Close()
	})
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
