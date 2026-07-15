package repository

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestMediaTaskStreamSubscribeTerminalCancelBeforeConfirmation(t *testing.T) {
	client, server := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "subscribe" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "cancel-before-confirm", time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := stream.SubscribeTerminal(ctx, 1)
		result <- err
	}()

	server.waitForCommand(t, "subscribe")
	started := time.Now()
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
		require.Less(t, time.Since(started), 250*time.Millisecond)
	case <-time.After(350 * time.Millisecond):
		_ = client.Close()
		t.Fatal("SubscribeTerminal did not stop while waiting for Redis confirmation")
	}
}

func TestMediaTaskStreamSubscribeTerminalCancelDuringRedisInitialization(t *testing.T) {
	client, server := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "hello" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "cancel-during-hello", time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := stream.SubscribeTerminal(ctx, 7)
		result <- err
	}()

	server.waitForCommand(t, "hello")
	started := time.Now()
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
		require.Less(t, time.Since(started), 250*time.Millisecond)
	case <-time.After(350 * time.Millisecond):
		_ = client.Close()
		t.Fatal("SubscribeTerminal did not stop while Redis HELLO was stalled")
	}
}

func TestMediaTaskStreamSubscribeTerminalDeadlineDuringRedisInitialization(t *testing.T) {
	client, server := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "hello" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "deadline-during-hello", time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := make(chan error, 1)
	go func() {
		_, _, err := stream.SubscribeTerminal(ctx, 8)
		result <- err
	}()
	server.waitForCommand(t, "hello")
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(started), 250*time.Millisecond)
	case <-time.After(350 * time.Millisecond):
		_ = client.Close()
		t.Fatal("SubscribeTerminal ignored parent deadline while Redis HELLO was stalled")
	}
}

func TestMediaTaskStreamSubscribeTerminalCancelAfterConfirmation(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "subscribe" {
			return fakeRedisAction{response: respSubscription("subscribe", command[1], 1)}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "cancel-after-confirm", time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	statuses, unsubscribe, err := stream.SubscribeTerminal(ctx, 2)
	require.NoError(t, err)
	defer unsubscribe()

	started := time.Now()
	cancel()
	requireStatusChannelClosedWithin(t, statuses, 300*time.Millisecond)
	require.Less(t, time.Since(started), 250*time.Millisecond)
}

func TestMediaTaskStreamSubscribeTerminalDeadlineBeforeConfirmation(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "subscribe" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "deadline-before-confirm", time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, _, err := stream.SubscribeTerminal(ctx, 3)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMediaTaskStreamSubscribeTerminalNaturalTerminalClosesOnce(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "subscribe" {
			return fakeRedisAction{response: respSubscription("subscribe", command[1], 1) + respPubSubMessage(command[1], "completed")}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "natural-terminal", time.Minute)
	statuses, unsubscribe, err := stream.SubscribeTerminal(context.Background(), 4)
	require.NoError(t, err)
	defer unsubscribe()
	status, ok := <-statuses
	require.True(t, ok)
	require.Equal(t, service.MediaTaskStatusCompleted, status)
	requireStatusChannelClosedWithin(t, statuses, 300*time.Millisecond)
}

func TestMediaTaskStreamSubscribeTerminalExplicitUnsubscribeIsIdempotent(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "subscribe" {
			return fakeRedisAction{response: respSubscription("subscribe", command[1], 1)}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "explicit-unsubscribe", time.Minute)
	statuses, unsubscribe, err := stream.SubscribeTerminal(context.Background(), 5)
	require.NoError(t, err)
	unsubscribe()
	unsubscribe()
	requireStatusChannelClosedWithin(t, statuses, 300*time.Millisecond)
}

func TestMediaTaskStreamSubscribeTerminalDisconnectClosesOutput(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "subscribe" {
			return fakeRedisAction{response: respSubscription("subscribe", command[1], 1), closeAfter: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "redis-disconnect", time.Minute)
	statuses, unsubscribe, err := stream.SubscribeTerminal(context.Background(), 6)
	require.NoError(t, err)
	defer unsubscribe()
	requireStatusChannelClosedWithin(t, statuses, 300*time.Millisecond)
}

func TestMediaTaskStreamTerminalSubscriptionUsesOnlyOneDedicatedConnection(t *testing.T) {
	client, server := newFakeRedisClientWithOptions(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "hello":
			return fakeRedisAction{response: respError("unknown command 'HELLO'")}
		case "subscribe":
			return fakeRedisAction{response: respSubscription("subscribe", command[1], 1)}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	}, func(options *redis.Options) {
		options.PoolSize = 32
		options.MinIdleConns = 16
		options.MaxIdleConns = 16
		options.MaxActiveConns = 32
		options.MaxConcurrentDials = 16
	})
	require.Eventually(t, func() bool {
		return server.connectionCount() == 16
	}, 2*time.Second, 10*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	baselineConnections := server.connectionCount()
	baselineAccepted := server.acceptedCount()

	stream := NewMediaTaskStream(client, "bounded-subscription-pool", time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	statuses, unsubscribe, err := stream.SubscribeTerminal(ctx, 9)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, baselineConnections+1, server.connectionCount())
	require.Equal(t, int64(1), server.acceptedCount()-baselineAccepted)
	require.Equal(t, 32, client.Options().PoolSize)
	require.Equal(t, 16, client.Options().MinIdleConns)

	cancel()
	unsubscribe()
	requireStatusChannelClosedWithin(t, statuses, 300*time.Millisecond)
	require.Eventually(t, func() bool {
		return server.connectionCount() == baselineConnections
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, client.Close())
	require.Eventually(t, func() bool {
		return server.connectionCount() == 0
	}, time.Second, 10*time.Millisecond)
}

func TestMediaTaskStreamReceiveHasHardTimeoutOnStalledRedis(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "xautoclaim" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "receive-timeout", time.Minute)
	started := time.Now()
	result := make(chan error, 1)
	go func() {
		_, err := stream.Receive(context.Background(), 50*time.Millisecond)
		result <- err
	}()

	select {
	case err := <-result:
		require.ErrorIs(t, err, service.ErrMediaQueueReceiveTimeout)
		require.Less(t, time.Since(started), 300*time.Millisecond)
	case <-time.After(400 * time.Millisecond):
		_ = client.Close()
		t.Fatal("Receive exceeded its hard timeout while Redis was stalled")
	}
}

func TestMediaTaskStreamReceiveCancelInterruptsStalledRedis(t *testing.T) {
	client, server := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "xautoclaim" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "receive-cancel", time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := stream.Receive(ctx, time.Second)
		result <- err
	}()

	server.waitForCommand(t, "xautoclaim")
	started := time.Now()
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
		require.Less(t, time.Since(started), 250*time.Millisecond)
	case <-time.After(350 * time.Millisecond):
		_ = client.Close()
		t.Fatal("Receive did not return promptly after caller cancellation")
	}
}

func TestMediaTaskStreamReceivePreservesParentDeadline(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "xautoclaim" {
			return fakeRedisAction{stall: true}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "receive-parent-deadline", time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := stream.Receive(ctx, time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 250*time.Millisecond)
}

func TestMediaTaskStreamReceiveWrapsRedisErrors(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "xautoclaim" {
			return fakeRedisAction{response: respError("forced receive failure")}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "receive-error", time.Minute)
	_, err := stream.Receive(context.Background(), time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "forced receive failure")
	require.NotErrorIs(t, err, service.ErrMediaQueueReceiveTimeout)
}

func TestMediaTaskStreamRedeliversOwnUnackedMessageAfterLease(t *testing.T) {
	var syncClaims atomic.Int64
	var syncReads atomic.Int64
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "xautoclaim":
			if command[1] == mediaTaskSyncStreamKey {
				if syncClaims.Add(1) == 1 {
					return fakeRedisAction{response: respXAutoClaim("0-0", nil)}
				}
				return fakeRedisAction{response: respXAutoClaim("0-0", []fakeStreamMessage{{id: "10-0", taskID: 10}})}
			}
			return fakeRedisAction{response: respXAutoClaim("0-0", nil)}
		case "xreadgroup":
			if commandContains(command, mediaTaskSyncStreamKey) && syncReads.Add(1) == 1 {
				return fakeRedisAction{response: respXRead(mediaTaskSyncStreamKey, []fakeStreamMessage{{id: "10-0", taskID: 10}})}
			}
			return fakeRedisAction{response: respNil()}
		case "xack":
			return fakeRedisAction{response: respInteger(1)}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	})
	stream := NewMediaTaskStream(client, "same-consumer", 5*time.Millisecond)
	first, err := stream.Receive(context.Background(), time.Second)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	redelivered, err := stream.Receive(context.Background(), 80*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, first, redelivered)
	require.NoError(t, stream.Ack(context.Background(), redelivered))
}

func TestMediaTaskStreamRedeliversMalformedPayloadWithoutAckAfterLease(t *testing.T) {
	var syncClaims atomic.Int64
	var syncReads atomic.Int64
	var ackCalls atomic.Int64
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "xautoclaim":
			if command[1] == mediaTaskSyncStreamKey {
				if syncClaims.Add(1) == 1 {
					return fakeRedisAction{response: respXAutoClaim("0-0", nil)}
				}
				return fakeRedisAction{response: respXAutoClaim("0-0", []fakeStreamMessage{{id: "11-0", taskID: "broken"}})}
			}
			return fakeRedisAction{response: respXAutoClaim("0-0", nil)}
		case "xreadgroup":
			if commandContains(command, mediaTaskSyncStreamKey) && syncReads.Add(1) == 1 {
				return fakeRedisAction{response: respXRead(mediaTaskSyncStreamKey, []fakeStreamMessage{{id: "11-0", taskID: "broken"}})}
			}
			return fakeRedisAction{response: respNil()}
		case "xack":
			ackCalls.Add(1)
			return fakeRedisAction{response: respInteger(1)}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	})
	stream := NewMediaTaskStream(client, "malformed-redelivery", 5*time.Millisecond)
	message, err := stream.Receive(context.Background(), time.Second)
	require.Nil(t, message)
	require.ErrorIs(t, err, service.ErrInvalidMediaQueuePayload)
	time.Sleep(10 * time.Millisecond)
	message, err = stream.Receive(context.Background(), 80*time.Millisecond)
	require.Nil(t, message)
	require.ErrorIs(t, err, service.ErrInvalidMediaQueuePayload)
	require.Zero(t, ackCalls.Load())
}

func TestMediaTaskStreamAdvancesAutoClaimCursorPastFreshWindow(t *testing.T) {
	var mu sync.Mutex
	var starts []string
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "xautoclaim":
			if command[1] != mediaTaskAsyncStreamKey {
				return fakeRedisAction{response: respXAutoClaim("0-0", nil)}
			}
			start := command[5]
			mu.Lock()
			starts = append(starts, start)
			mu.Unlock()
			if start == "0-0" {
				return fakeRedisAction{response: respXAutoClaim("999-0", nil)}
			}
			return fakeRedisAction{response: respXAutoClaim("0-0", []fakeStreamMessage{{id: "1000-0", taskID: 1000}})}
		case "xreadgroup":
			return fakeRedisAction{response: respNil()}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	})
	stream := NewMediaTaskStream(client, "cursor-consumer", time.Minute)
	message, err := stream.Receive(context.Background(), 100*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, int64(1000), message.TaskID)
	mu.Lock()
	require.Equal(t, []string{"0-0", "999-0"}, starts[:2])
	mu.Unlock()
}

func TestMediaTaskStreamAckRejectsMessageThatIsNotPending(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		if commandName(command) == "xack" {
			return fakeRedisAction{response: respInteger(0)}
		}
		return fakeRedisAction{response: respError("unexpected command")}
	})
	stream := NewMediaTaskStream(client, "ack-zero", time.Minute)
	err := stream.Ack(context.Background(), &service.MediaQueueMessage{
		ID:       "20-0",
		TaskID:   20,
		Priority: service.MediaQueuePrioritySync,
	})
	require.ErrorIs(t, err, service.ErrMediaQueueMessageNotPending)
}

func TestMediaTaskStreamAckReportsLostGroupDeliveryState(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "xack":
			return fakeRedisAction{response: respError("NOGROUP No such key or consumer group")}
		case "exists":
			return fakeRedisAction{response: respInteger(1)}
		case "xgroup":
			return fakeRedisAction{response: respSimpleString("OK")}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	})
	stream := NewMediaTaskStream(client, "ack-lost-group", time.Minute)
	err := stream.Ack(context.Background(), &service.MediaQueueMessage{
		ID:       "21-0",
		TaskID:   21,
		Priority: service.MediaQueuePriorityAsync,
	})
	require.ErrorIs(t, err, service.ErrMediaQueueDeliveryStateLost)
}

func TestMediaTaskStreamAckTreatsMissingStreamAsIdempotent(t *testing.T) {
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "xack":
			return fakeRedisAction{response: respError("NOGROUP No such key or consumer group")}
		case "exists":
			return fakeRedisAction{response: respInteger(0)}
		case "xgroup":
			return fakeRedisAction{response: respSimpleString("OK")}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	})
	stream := NewMediaTaskStream(client, "ack-missing-stream", time.Minute)
	require.NoError(t, stream.Ack(context.Background(), &service.MediaQueueMessage{
		ID:       "22-0",
		TaskID:   22,
		Priority: service.MediaQueuePrioritySync,
	}))
}

func TestMediaTaskStreamAsyncFairnessProbeDoesNotGrowSyncBacklog(t *testing.T) {
	var syncClaims atomic.Int64
	client, _ := newFakeRedisClient(t, func(command []string) fakeRedisAction {
		switch commandName(command) {
		case "xautoclaim":
			if command[1] == mediaTaskSyncStreamKey {
				syncClaims.Add(1)
				messages := make([]fakeStreamMessage, mediaTaskReadBatchSize)
				for i := range messages {
					messages[i] = fakeStreamMessage{id: fmt.Sprintf("%d-0", 100+i), taskID: int64(100 + i)}
				}
				return fakeRedisAction{response: respXAutoClaim("0-0", messages)}
			}
			return fakeRedisAction{response: respXAutoClaim("0-0", nil)}
		case "xreadgroup":
			return fakeRedisAction{response: respNil()}
		default:
			return fakeRedisAction{response: respError("unexpected command")}
		}
	})
	stream := NewMediaTaskStream(client, "bounded-backlog", time.Minute)
	stream.syncBacklog = []redis.XMessage{{ID: "30-0", Values: map[string]any{"task_id": "30"}}}
	stream.bufferedIDs[mediaDeliveryKey(service.MediaQueuePrioritySync, "30-0")] = struct{}{}
	stream.syncStreak = mediaTaskSyncBurst

	message, err := stream.Receive(context.Background(), 80*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, int64(30), message.TaskID)
	require.Zero(t, syncClaims.Load())
	stream.stateMu.Lock()
	require.Empty(t, stream.syncBacklog)
	require.LessOrEqual(t, len(stream.bufferedIDs), int(mediaTaskReadBatchSize*2))
	stream.stateMu.Unlock()
}

type fakeRedisAction struct {
	response   string
	stall      bool
	closeAfter bool
}

type fakeRESPServer struct {
	listener net.Listener
	handler  func([]string) fakeRedisAction
	commands chan []string
	stop     chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	accepted atomic.Int64
	wg       sync.WaitGroup
}

func newFakeRedisClient(t *testing.T, handler func([]string) fakeRedisAction) (*redis.Client, *fakeRESPServer) {
	return newFakeRedisClientWithOptions(t, handler, nil)
}

func newFakeRedisClientWithOptions(
	t *testing.T,
	handler func([]string) fakeRedisAction,
	configure func(*redis.Options),
) (*redis.Client, *fakeRESPServer) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &fakeRESPServer{
		listener: listener,
		handler:  handler,
		commands: make(chan []string, 256),
		stop:     make(chan struct{}),
		conns:    make(map[net.Conn]struct{}),
	}
	server.wg.Add(1)
	go server.serve()
	options := &redis.Options{
		Addr:                  listener.Addr().String(),
		Protocol:              2,
		DisableIdentity:       true,
		MaxRetries:            -1,
		DialTimeout:           time.Second,
		ReadTimeout:           -1,
		WriteTimeout:          time.Second,
		ContextTimeoutEnabled: false,
		PoolSize:              1,
	}
	if configure != nil {
		configure(options)
	}
	client := redis.NewClient(options)
	t.Cleanup(server.Close)
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}

func (s *fakeRESPServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stop:
				return
			default:
				return
			}
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.accepted.Add(1)
		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *fakeRESPServer) serveConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		_ = conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()
	reader := bufio.NewReader(conn)
	for {
		command, err := readRESPCommand(reader)
		if err != nil {
			return
		}
		select {
		case s.commands <- command:
		default:
		}
		action := s.handler(command)
		if action.response != "" {
			if _, err := io.WriteString(conn, action.response); err != nil {
				return
			}
		}
		if action.closeAfter {
			return
		}
		if action.stall {
			_, _ = reader.ReadByte()
			return
		}
	}
}

func (s *fakeRESPServer) waitForCommand(t *testing.T, name string) []string {
	t.Helper()
	for {
		select {
		case command := <-s.commands:
			if commandName(command) == name {
				return command
			}
		case <-time.After(time.Second):
			t.Fatalf("fake Redis did not receive %s", name)
		}
	}
}

func (s *fakeRESPServer) Close() {
	s.stopOnce.Do(func() {
		close(s.stop)
		_ = s.listener.Close()
		s.mu.Lock()
		for conn := range s.conns {
			_ = conn.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	})
}

func (s *fakeRESPServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func (s *fakeRESPServer) acceptedCount() int64 {
	return s.accepted.Load()
}

func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(header) < 4 || header[0] != '*' {
		return nil, fmt.Errorf("invalid RESP array header %q", header)
	}
	count, err := strconv.Atoi(strings.TrimSpace(header[1:]))
	if err != nil {
		return nil, err
	}
	command := make([]string, count)
	for i := range command {
		bulkHeader, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		if len(bulkHeader) < 4 || bulkHeader[0] != '$' {
			return nil, fmt.Errorf("invalid RESP bulk header %q", bulkHeader)
		}
		length, parseErr := strconv.Atoi(strings.TrimSpace(bulkHeader[1:]))
		if parseErr != nil {
			return nil, parseErr
		}
		payload := make([]byte, length+2)
		if _, readErr = io.ReadFull(reader, payload); readErr != nil {
			return nil, readErr
		}
		command[i] = string(payload[:length])
	}
	return command, nil
}

func commandName(command []string) string {
	if len(command) == 0 {
		return ""
	}
	return strings.ToLower(command[0])
}

func commandContains(command []string, value string) bool {
	for _, item := range command {
		if item == value {
			return true
		}
	}
	return false
}

type fakeStreamMessage struct {
	id     string
	taskID any
}

func respError(message string) string {
	return "-ERR " + message + "\r\n"
}

func respSimpleString(value string) string {
	return "+" + value + "\r\n"
}

func respInteger(value int64) string {
	return ":" + strconv.FormatInt(value, 10) + "\r\n"
}

func respNil() string {
	return "$-1\r\n"
}

func respBulk(value string) string {
	return "$" + strconv.Itoa(len(value)) + "\r\n" + value + "\r\n"
}

func respSubscription(kind, channel string, count int64) string {
	return "*3\r\n" + respBulk(kind) + respBulk(channel) + respInteger(count)
}

func respPubSubMessage(channel, payload string) string {
	return "*3\r\n" + respBulk("message") + respBulk(channel) + respBulk(payload)
}

func respXAutoClaim(next string, messages []fakeStreamMessage) string {
	var response strings.Builder
	response.WriteString("*3\r\n")
	response.WriteString(respBulk(next))
	response.WriteString("*")
	response.WriteString(strconv.Itoa(len(messages)))
	response.WriteString("\r\n")
	writeRESPStreamMessages(&response, messages)
	response.WriteString("*0\r\n")
	return response.String()
}

func respXRead(stream string, messages []fakeStreamMessage) string {
	var response strings.Builder
	response.WriteString("*1\r\n*2\r\n")
	response.WriteString(respBulk(stream))
	response.WriteString("*")
	response.WriteString(strconv.Itoa(len(messages)))
	response.WriteString("\r\n")
	writeRESPStreamMessages(&response, messages)
	return response.String()
}

func writeRESPStreamMessages(response *strings.Builder, messages []fakeStreamMessage) {
	for _, message := range messages {
		response.WriteString("*2\r\n")
		response.WriteString(respBulk(message.id))
		response.WriteString("*2\r\n")
		response.WriteString(respBulk("task_id"))
		response.WriteString(respBulk(fmt.Sprint(message.taskID)))
	}
}

func requireStatusChannelClosedWithin(t *testing.T, statuses <-chan service.MediaTaskStatus, timeout time.Duration) {
	t.Helper()
	select {
	case _, ok := <-statuses:
		require.False(t, ok)
	case <-time.After(timeout):
		t.Fatal("terminal status channel did not close")
	}
}
