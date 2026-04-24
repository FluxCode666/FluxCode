//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryBatchInsertErrorLogs(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE ops_error_logs RESTART IDENTITY")

	repo := NewOpsRepository(integrationDB).(*opsRepository)
	now := time.Now().UTC()
	inserted, err := repo.BatchInsertErrorLogs(ctx, []*service.OpsInsertErrorLogInput{
		{
			RequestID:    "batch-ops-1",
			ErrorPhase:   "upstream",
			ErrorType:    "upstream_error",
			Severity:     "error",
			StatusCode:   429,
			ErrorMessage: "rate limited",
			CreatedAt:    now,
		},
		{
			RequestID:    "batch-ops-2",
			ErrorPhase:   "internal",
			ErrorType:    "api_error",
			Severity:     "error",
			StatusCode:   500,
			ErrorMessage: "internal error",
			CreatedAt:    now.Add(time.Millisecond),
		},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, inserted)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_error_logs WHERE request_id IN ('batch-ops-1', 'batch-ops-2')").Scan(&count))
	require.Equal(t, 2, count)
}

func TestEnqueueSchedulerOutbox_DeduplicatesIdempotentEvents(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)

	queue := NewSchedulerOutboxQueue(rdb, "dedup-test")
	SetSchedulerOutboxPublisher(queue)
	t.Cleanup(func() { SetSchedulerOutboxPublisher(nil) })

	accountID := int64(12345)
	require.NoError(t, enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil))
	require.NoError(t, enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil))

	streamLen, err := queue.Pending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), streamLen, "second publish should be deduped")

	// 等待去重窗口过期后再次发布
	time.Sleep(schedulerDedupTTL + 150*time.Millisecond)
	require.NoError(t, enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil))
	streamLen, err = queue.Pending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), streamLen, "publish after dedup window should succeed")
}

func TestEnqueueSchedulerOutbox_DoesNotDeduplicateLastUsed(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)

	queue := NewSchedulerOutboxQueue(rdb, "nodedup-test")
	SetSchedulerOutboxPublisher(queue)
	t.Cleanup(func() { SetSchedulerOutboxPublisher(nil) })

	accountID := int64(67890)
	payload1 := map[string]any{"last_used": map[string]int64{"67890": 100}}
	payload2 := map[string]any{"last_used": map[string]int64{"67890": 200}}
	require.NoError(t, enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventAccountLastUsed, &accountID, nil, payload1))
	require.NoError(t, enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventAccountLastUsed, &accountID, nil, payload2))

	streamLen, err := queue.Pending(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), streamLen, "last_used events should not be deduped")
}
