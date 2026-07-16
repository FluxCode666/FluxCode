//go:build integration

package repository

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestMediaTaskStreamPrefersSyncAndPublishesTerminal(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-test", time.Minute)

	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 101, service.MediaQueuePriorityAsync))
	require.NoError(t, stream.Enqueue(ctx, 202, service.MediaQueuePrioritySync))

	message, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(202), message.TaskID)
	require.Equal(t, service.MediaQueuePrioritySync, message.Priority)
	require.NoError(t, stream.Ack(ctx, message))

	waitCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	done, unsubscribe, err := stream.SubscribeTerminal(waitCtx, 202)
	require.NoError(t, err)
	defer unsubscribe()
	require.NoError(t, stream.PublishTerminal(ctx, 202, service.MediaTaskStatusCompleted))
	status, ok := <-done
	require.True(t, ok)
	require.Equal(t, service.MediaTaskStatusCompleted, status)
	_, ok = <-done
	require.False(t, ok)
}

func TestMediaTaskStreamEnsureGroupsIsIdempotentAndKeepsExistingMessages(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-groups", time.Minute)

	require.NoError(t, stream.Enqueue(ctx, 301, service.MediaQueuePriorityAsync))
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.EnsureGroups(ctx))

	message, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(301), message.TaskID)
	require.Equal(t, service.MediaQueuePriorityAsync, message.Priority)
	require.NoError(t, stream.Ack(ctx, message))
}

func TestMediaTaskStreamAckTracksTheMessagePriority(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-ack", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 401, service.MediaQueuePriorityAsync))
	require.NoError(t, stream.Enqueue(ctx, 402, service.MediaQueuePrioritySync))

	syncMessage, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, service.MediaQueuePrioritySync, syncMessage.Priority)
	asyncMessage, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, service.MediaQueuePriorityAsync, asyncMessage.Priority)

	syncPending, err := stream.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Equal(t, int64(1), syncPending)
	asyncPending, err := stream.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, err)
	require.Equal(t, int64(1), asyncPending)

	require.NoError(t, stream.Ack(ctx, asyncMessage))
	wrongStreamCopy := *syncMessage
	wrongStreamCopy.Priority = service.MediaQueuePriorityAsync
	require.ErrorIs(t, stream.Ack(ctx, &wrongStreamCopy), service.ErrMediaQueueMessageNotPending)
	syncPending, err = stream.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Equal(t, int64(1), syncPending, "an ACK must never be sent to the other priority stream")

	require.NoError(t, stream.Ack(ctx, syncMessage))
	syncPending, err = stream.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Zero(t, syncPending)
	asyncPending, err = stream.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, err)
	require.Zero(t, asyncPending)
}

func TestMediaTaskStreamLeavesUnackedMessageRecoverableByAnotherConsumer(t *testing.T) {
	ctx := context.Background()
	resetMediaTaskStreamKeys(t)
	lease := 30 * time.Millisecond
	workerA := NewMediaTaskStream(integrationRedis, "worker-a", lease)
	workerB := NewMediaTaskStream(integrationRedis, "worker-b", lease)

	require.NoError(t, workerA.EnsureGroups(ctx))
	require.NoError(t, workerA.Enqueue(ctx, 501, service.MediaQueuePriorityAsync))
	first, err := workerA.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(501), first.TaskID)

	pending, err := workerA.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, err)
	require.Equal(t, int64(1), pending)

	require.Eventually(t, func() bool {
		recovered, receiveErr := workerB.Receive(ctx, 100*time.Millisecond)
		if receiveErr != nil {
			return false
		}
		if recovered.ID != first.ID || recovered.TaskID != first.TaskID {
			return false
		}
		return workerB.Ack(ctx, recovered) == nil
	}, 2*time.Second, 40*time.Millisecond)

	pending, err = workerB.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, err)
	require.Zero(t, pending)
}

func TestMediaTaskStreamDoesNotStarveAsyncQueue(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-fair", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	for id := int64(1); id <= 20; id++ {
		require.NoError(t, stream.Enqueue(ctx, id, service.MediaQueuePrioritySync))
	}
	require.NoError(t, stream.Enqueue(ctx, 999, service.MediaQueuePriorityAsync))

	seenAsyncAt := 0
	for i := 1; i <= 9; i++ {
		message, err := stream.Receive(ctx, time.Second)
		require.NoError(t, err)
		if message.TaskID == 999 {
			seenAsyncAt = i
		}
		require.NoError(t, stream.Ack(ctx, message))
	}
	require.NotZero(t, seenAsyncAt)
	require.LessOrEqual(t, seenAsyncAt, 9)
}

func TestMediaTaskStreamBuffersEveryDeliveredExtra(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-buffer", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	for id := int64(601); id <= 604; id++ {
		require.NoError(t, stream.Enqueue(ctx, id, service.MediaQueuePrioritySync))
	}

	first, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	pending, err := stream.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Equal(t, int64(4), pending, "one batched XREADGROUP should leave every delivered message in the PEL")

	seen := map[int64]bool{first.TaskID: true}
	require.NoError(t, stream.Ack(ctx, first))
	for range 3 {
		message, receiveErr := stream.Receive(ctx, time.Second)
		require.NoError(t, receiveErr)
		require.False(t, seen[message.TaskID], "a buffered message must only be returned once")
		seen[message.TaskID] = true
		require.NoError(t, stream.Ack(ctx, message))
	}
	require.Len(t, seen, 4)
	pending, err = stream.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Zero(t, pending)
}

func TestMediaTaskStreamRecoversUnreturnedBufferedExtrasAfterLease(t *testing.T) {
	ctx := context.Background()
	resetMediaTaskStreamKeys(t)
	lease := 30 * time.Millisecond
	workerA := NewMediaTaskStream(integrationRedis, "worker-buffer-a", lease)
	workerB := NewMediaTaskStream(integrationRedis, "worker-buffer-b", lease)
	require.NoError(t, workerA.EnsureGroups(ctx))
	for id := int64(650); id <= 653; id++ {
		require.NoError(t, workerA.Enqueue(ctx, id, service.MediaQueuePrioritySync))
	}

	_, err := workerA.Receive(ctx, time.Second)
	require.NoError(t, err)
	pending, err := workerA.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Equal(t, int64(4), pending)

	time.Sleep(40 * time.Millisecond)
	seen := make(map[int64]bool, 4)
	for range 4 {
		message, receiveErr := workerB.Receive(ctx, time.Second)
		require.NoError(t, receiveErr)
		require.False(t, seen[message.TaskID])
		seen[message.TaskID] = true
		require.NoError(t, workerB.Ack(ctx, message))
	}
	require.Len(t, seen, 4)
	pending, err = workerB.PendingCount(ctx, service.MediaQueuePrioritySync)
	require.NoError(t, err)
	require.Zero(t, pending)
}

func TestMediaTaskStreamSerializesConcurrentReceive(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-concurrent", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 701, service.MediaQueuePrioritySync))
	require.NoError(t, stream.Enqueue(ctx, 702, service.MediaQueuePrioritySync))

	results := make(chan *service.MediaQueueMessage, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			message, err := stream.Receive(ctx, time.Second)
			results <- message
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	seen := make(map[string]bool, 2)
	for message := range results {
		require.NotNil(t, message)
		require.False(t, seen[message.ID])
		seen[message.ID] = true
		require.NoError(t, stream.Ack(ctx, message))
	}
	require.Len(t, seen, 2)
}

func TestMediaTaskStreamRedeliversItsOwnUnackedMessageAfterLease(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-inflight", 20*time.Millisecond)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 750, service.MediaQueuePrioritySync))

	first, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	time.Sleep(30 * time.Millisecond)
	redelivered, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, first, redelivered)
	require.NoError(t, stream.Ack(ctx, redelivered))
}

func TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshPendingWindow(t *testing.T) {
	ctx := context.Background()
	resetMediaTaskStreamKeys(t)
	lease := 500 * time.Millisecond
	seed := NewMediaTaskStream(integrationRedis, "cursor-seed", lease)
	require.NoError(t, seed.EnsureGroups(ctx))
	for id := int64(1); id <= mediaTaskReadBatchSize*10+1; id++ {
		require.NoError(t, seed.Enqueue(ctx, id, service.MediaQueuePriorityAsync))
	}
	streams, err := integrationRedis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    mediaTaskConsumerGroup,
		Consumer: "cursor-seed",
		Streams:  []string{mediaTaskAsyncStreamKey, ">"},
		Count:    mediaTaskReadBatchSize*10 + 1,
		Block:    -1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, int(mediaTaskReadBatchSize*10+1))
	ids := make([]string, 0, len(streams[0].Messages)-1)
	for _, message := range streams[0].Messages[:len(streams[0].Messages)-1] {
		ids = append(ids, message.ID)
	}
	time.Sleep(lease + 50*time.Millisecond)
	_, err = integrationRedis.XClaim(ctx, &redis.XClaimArgs{
		Stream:   mediaTaskAsyncStreamKey,
		Group:    mediaTaskConsumerGroup,
		Consumer: "cursor-fresh",
		MinIdle:  0,
		Messages: ids,
	}).Result()
	require.NoError(t, err)

	worker := NewMediaTaskStream(integrationRedis, "cursor-worker", lease)
	message, err := worker.Receive(ctx, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, mediaTaskReadBatchSize*10+1, message.TaskID)
	require.NoError(t, worker.Ack(ctx, message))
}

func TestMediaTaskStreamRejectsMalformedPayloadWithoutAcknowledging(t *testing.T) {
	ctx := context.Background()
	lease := 20 * time.Millisecond
	stream := newTestMediaTaskStream(t, "worker-malformed", lease)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, integrationRedis.XAdd(ctx, &redis.XAddArgs{
		Stream: mediaTaskAsyncStreamKey,
		Values: map[string]any{"task_id": "not-an-integer"},
	}).Err())

	message, err := stream.Receive(ctx, time.Second)
	require.Nil(t, message)
	require.ErrorIs(t, err, service.ErrInvalidMediaQueuePayload)
	pending, pendingErr := stream.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, pendingErr)
	require.Equal(t, int64(1), pending)
	time.Sleep(lease + 10*time.Millisecond)
	message, err = stream.Receive(ctx, time.Second)
	require.Nil(t, message)
	require.ErrorIs(t, err, service.ErrInvalidMediaQueuePayload)
	pending, pendingErr = stream.PendingCount(ctx, service.MediaQueuePriorityAsync)
	require.NoError(t, pendingErr)
	require.Equal(t, int64(1), pending)
}

func TestMediaTaskStreamParsesRedisValueRepresentations(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		taskID int64
	}{
		{name: "string", value: "801", taskID: 801},
		{name: "bytes", value: []byte("802"), taskID: 802},
		{name: "int64", value: int64(803), taskID: 803},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := parseMediaQueueMessage(redis.XMessage{
				ID:     "1-0",
				Values: map[string]any{"task_id": tt.value},
			}, service.MediaQueuePriorityAsync)
			require.NoError(t, err)
			require.Equal(t, tt.taskID, message.TaskID)
		})
	}
}

func TestMediaTaskStreamValidatesInputsAndTimeouts(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-validation", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))

	require.ErrorIs(t, stream.Enqueue(ctx, 0, service.MediaQueuePrioritySync), service.ErrInvalidMediaQueueMessage)
	require.ErrorIs(t, stream.Enqueue(ctx, 1, service.MediaQueuePriority("urgent")), service.ErrInvalidMediaQueuePriority)
	_, err := stream.Receive(ctx, 0)
	require.ErrorIs(t, err, service.ErrInvalidMediaQueueTimeout)
	require.ErrorIs(t, stream.Ack(ctx, nil), service.ErrInvalidMediaQueueMessage)
	require.ErrorIs(t, stream.Ack(ctx, &service.MediaQueueMessage{ID: "", TaskID: 1, Priority: service.MediaQueuePrioritySync}), service.ErrInvalidMediaQueueMessage)
	require.ErrorIs(t, stream.Ack(ctx, &service.MediaQueueMessage{ID: "invalid", TaskID: 1, Priority: service.MediaQueuePrioritySync}), service.ErrInvalidMediaQueueMessage)
	require.ErrorIs(t, stream.Ack(ctx, &service.MediaQueueMessage{ID: "1-0", TaskID: 1, Priority: service.MediaQueuePriority("urgent")}), service.ErrInvalidMediaQueuePriority)
	require.ErrorIs(t, stream.PublishTerminal(ctx, 0, service.MediaTaskStatusCompleted), service.ErrInvalidMediaTerminalPayload)
	require.ErrorIs(t, stream.PublishTerminal(ctx, 1, service.MediaTaskStatusQueued), service.ErrInvalidMediaTerminalPayload)
	_, _, err = stream.SubscribeTerminal(ctx, 0)
	require.ErrorIs(t, err, service.ErrInvalidMediaTerminalPayload)

	started := time.Now()
	message, err := stream.Receive(ctx, 50*time.Millisecond)
	require.Nil(t, message)
	require.ErrorIs(t, err, service.ErrMediaQueueReceiveTimeout)
	require.GreaterOrEqual(t, time.Since(started), 40*time.Millisecond)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	message, err = stream.Receive(canceledCtx, time.Second)
	require.Nil(t, message)
	require.ErrorIs(t, err, context.Canceled)
}

func TestMediaTaskStreamRecoversMissingGroup(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-nogroup", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, integrationRedis.Del(ctx, mediaTaskSyncStreamKey).Err())
	require.NoError(t, stream.Enqueue(ctx, 901, service.MediaQueuePrioritySync))

	message, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, int64(901), message.TaskID)
	require.NoError(t, stream.Ack(ctx, message))

	require.NoError(t, stream.Enqueue(ctx, 902, service.MediaQueuePriorityAsync))
	message, err = stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	require.NoError(t, integrationRedis.Del(ctx, mediaTaskAsyncStreamKey).Err())
	require.NoError(t, stream.Ack(ctx, message), "ACK after NOGROUP is idempotent because the delivery state no longer exists")
	require.NoError(t, stream.EnsureGroups(ctx))
}

func TestMediaTaskStreamAckReportsDestroyedGroupDeliveryState(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-ack-destroyed", time.Minute)
	require.NoError(t, stream.EnsureGroups(ctx))
	require.NoError(t, stream.Enqueue(ctx, 950, service.MediaQueuePriorityAsync))
	message, err := stream.Receive(ctx, time.Second)
	require.NoError(t, err)
	destroyed, err := integrationRedis.XGroupDestroy(ctx, mediaTaskAsyncStreamKey, mediaTaskConsumerGroup).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), destroyed)
	require.ErrorIs(t, stream.Ack(ctx, message), service.ErrMediaQueueDeliveryStateLost)
	require.NoError(t, stream.EnsureGroups(ctx))
}

func TestMediaTaskStreamAckPreservesAtomicRedisErrors(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-ack-wrongtype", time.Minute)
	require.NoError(t, integrationRedis.Set(ctx, mediaTaskAsyncStreamKey, "not-a-stream", 0).Err())

	err := stream.Ack(ctx, &service.MediaQueueMessage{
		ID:       "1-0",
		TaskID:   1,
		Priority: service.MediaQueuePriorityAsync,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "WRONGTYPE")
	require.NotErrorIs(t, err, service.ErrMediaQueueDeliveryStateLost)
	require.NotErrorIs(t, err, service.ErrMediaQueueMessageNotPending)
}

func TestMediaTaskStreamTerminalSubscriptionFiltersAndStops(t *testing.T) {
	ctx := context.Background()
	stream := newTestMediaTaskStream(t, "worker-terminal", time.Minute)

	done, unsubscribe, err := stream.SubscribeTerminal(ctx, 1001)
	require.NoError(t, err)
	require.NoError(t, integrationRedis.Publish(ctx, terminalChannel(1001), "queued").Err())
	select {
	case status := <-done:
		t.Fatalf("non-terminal payload unexpectedly delivered: %q", status)
	case <-time.After(40 * time.Millisecond):
	}
	require.NoError(t, stream.PublishTerminal(ctx, 1001, service.MediaTaskStatusFailed))
	status, ok := <-done
	require.True(t, ok)
	require.Equal(t, service.MediaTaskStatusFailed, status)
	_, ok = <-done
	require.False(t, ok)
	unsubscribe()
	unsubscribe()

	unsubscribed, stop, err := stream.SubscribeTerminal(ctx, 1002)
	require.NoError(t, err)
	stop()
	stop()
	requireChannelClosed(t, unsubscribed)

	waitCtx, cancel := context.WithCancel(ctx)
	canceled, cancelSubscription, err := stream.SubscribeTerminal(waitCtx, 1003)
	require.NoError(t, err)
	cancel()
	requireChannelClosed(t, canceled)
	cancelSubscription()
}

func TestMediaTaskStreamProviderUsesUniqueConsumerNames(t *testing.T) {
	resetMediaTaskStreamKeys(t)
	cfg := &config.Config{MediaTasks: config.MediaTaskConfig{LeaseTTLSeconds: int(defaultMediaTaskLease / time.Second)}}
	first := ProvideMediaTaskQueue(integrationRedis, cfg)
	second := ProvideMediaTaskQueue(integrationRedis, cfg)
	firstStream, ok := first.(*MediaTaskStream)
	require.True(t, ok)
	secondStream, ok := second.(*MediaTaskStream)
	require.True(t, ok)
	require.NotEmpty(t, firstStream.consumerName)
	require.NotEqual(t, firstStream.consumerName, secondStream.consumerName)
	hostname, _ := os.Hostname()
	if hostname != "" {
		require.Contains(t, firstStream.consumerName, hostname)
	}
	require.Equal(t, defaultMediaTaskLease, firstStream.lease)
}

func newTestMediaTaskStream(t *testing.T, consumer string, lease time.Duration) *MediaTaskStream {
	t.Helper()
	resetMediaTaskStreamKeys(t)
	return NewMediaTaskStream(integrationRedis, consumer, lease)
}

func resetMediaTaskStreamKeys(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, integrationRedis.Del(ctx, mediaTaskSyncStreamKey, mediaTaskAsyncStreamKey).Err())
	t.Cleanup(func() {
		require.NoError(t, integrationRedis.Del(context.Background(), mediaTaskSyncStreamKey, mediaTaskAsyncStreamKey).Err())
	})
}

func requireChannelClosed(t *testing.T, statuses <-chan service.MediaTaskStatus) {
	t.Helper()
	select {
	case _, ok := <-statuses:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("terminal status channel was not closed")
	}
}
