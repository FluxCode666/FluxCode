package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newMediaTaskRepositoryTestHarness(t *testing.T) (service.MediaTaskRepository, *dbent.Client) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return NewMediaTaskRepository(client), client
}

func newMediaArtifactRepositoryTestHarness(t *testing.T) (service.MediaArtifactRepository, *service.MediaTask) {
	t.Helper()
	taskRepo, client := newMediaTaskRepositoryTestHarness(t)
	task, err := taskRepo.Create(context.Background(), newRepositoryMediaTask("task_artifact"))
	require.NoError(t, err)
	return NewMediaArtifactRepository(client), task
}

func newRepositoryMediaTask(publicID string) *service.MediaTask {
	return &service.MediaTask{
		PublicID:           publicID,
		UserID:             1,
		APIKeyID:           2,
		GroupID:            3,
		MediaType:          service.MediaTypeImage,
		Operation:          service.MediaOperationTextToImage,
		RequestedModel:     "fake-image",
		RequestSpec:        json.RawMessage(`{"prompt":"cat"}`),
		CandidateSnapshot:  json.RawMessage(`[]`),
		RequestFingerprint: "fp",
		Status:             service.MediaTaskStatusQueued,
	}
}

func TestMediaTaskRepositoryClaimAndCompleteWithCAS(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	task, err := repo.Create(context.Background(), newRepositoryMediaTask("task_repo_claim"))
	require.NoError(t, err)

	claimed, err := repo.Claim(context.Background(), task.ID, "worker-a", time.Now().Add(time.Minute), task.Version)
	require.NoError(t, err)
	require.True(t, claimed)

	completed, err := repo.Transition(context.Background(), task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, map[string]any{"progress": 100})
	require.NoError(t, err)
	require.True(t, completed)

	stale, err := repo.Transition(context.Background(), task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusFailed, nil)
	require.NoError(t, err)
	require.False(t, stale)

	stored, err := client.MediaTask.Get(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", stored.Status)
	require.Equal(t, 100, stored.Progress)
}

func TestMediaTaskRepositoryCreateAndLookupRoundTrip(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	channelID, accountID := int64(41), int64(42)
	widthTime := time.Now().UTC().Truncate(time.Millisecond)
	input := newRepositoryMediaTask("task_round_trip")
	input.ChannelID = &channelID
	input.AccountID = &accountID
	input.UpstreamModel = "upstream-image"
	input.Adapter = "fake-adapter"
	input.NativeAsyncMode = service.NativeAsyncOptional
	input.ClientAsync = true
	input.Stage = service.MediaTaskStageSubmitting
	input.Progress = 17
	input.IdempotencyKey = "idem-round-trip"
	input.UpstreamTaskID = "upstream-task"
	input.PollMetadata = json.RawMessage(`{"attempt":1}`)
	input.BillingSnapshot = json.RawMessage(`{"price":"1.5"}`)
	input.SettlementPlan = json.RawMessage(`{"mode":"actual"}`)
	input.BillingStatus = "precharged"
	input.PrechargedAmount = 1.5
	input.RetryCount = 2
	input.ErrorCode = "retryable"
	input.ErrorMessage = "try again"
	input.WorkerID = "worker-round-trip"
	input.LeaseUntil = &widthTime
	input.SubmittedAt = &widthTime
	input.StartedAt = &widthTime

	created, err := repo.Create(ctx, input)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, int64(1), created.Version)
	require.Equal(t, input.PublicID, created.PublicID)
	require.Equal(t, input.ChannelID, created.ChannelID)
	require.Equal(t, input.AccountID, created.AccountID)
	require.Equal(t, input.NativeAsyncMode, created.NativeAsyncMode)
	require.JSONEq(t, string(input.PollMetadata), string(created.PollMetadata))

	byID, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, byID.ID)
	byPublicID, err := repo.GetByPublicIDForUser(ctx, created.PublicID, created.UserID)
	require.NoError(t, err)
	require.Equal(t, created.ID, byPublicID.ID)
	byIdempotencyKey, err := repo.GetByIdempotencyKey(ctx, created.UserID, created.APIKeyID, created.IdempotencyKey)
	require.NoError(t, err)
	require.Equal(t, created.ID, byIdempotencyKey.ID)

	_, err = repo.GetByPublicIDForUser(ctx, created.PublicID, created.UserID+1)
	require.True(t, dbent.IsNotFound(err))
	_, err = repo.GetByIdempotencyKey(ctx, created.UserID, created.APIKeyID, "")
	require.True(t, dbent.IsNotFound(err))
}

func TestMediaTaskRepositoryUpdateQueuedUsesStatusAndVersionCAS(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_update_queued"))
	require.NoError(t, err)

	updated, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{
		"adapter":    "adapter-a",
		"stage":      service.MediaTaskStageScheduling,
		"progress":   9,
		"channel_id": int64(81),
	})
	require.NoError(t, err)
	require.True(t, updated)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), stored.Version)
	require.Equal(t, "adapter-a", stored.Adapter)
	require.Equal(t, service.MediaTaskStageScheduling, stored.Stage)
	require.Equal(t, 9, stored.Progress)
	require.Equal(t, int64(81), *stored.ChannelID)

	stale, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"progress": 10})
	require.NoError(t, err)
	require.False(t, stale)

	claimed, err := repo.Claim(ctx, task.ID, "worker-a", time.Now().Add(time.Minute), stored.Version)
	require.NoError(t, err)
	require.True(t, claimed)
	notQueued, err := repo.UpdateQueued(ctx, task.ID, stored.Version+1, map[string]any{"progress": 11})
	require.NoError(t, err)
	require.False(t, notQueued)
}

func TestMediaTaskRepositoryClaimSupportsLeaseRecoveryAndRejectsActiveOrStaleClaims(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	leaseLessInput := newRepositoryMediaTask("task_claim_without_lease")
	leaseLessInput.Status = service.MediaTaskStatusInProgress
	leaseLess, err := repo.Create(ctx, leaseLessInput)
	require.NoError(t, err)
	recoveredWithoutLease, err := repo.Claim(ctx, leaseLess.ID, "worker-recovery", time.Now().Add(time.Minute), leaseLess.Version)
	require.NoError(t, err)
	require.True(t, recoveredWithoutLease)

	task, err := repo.Create(ctx, newRepositoryMediaTask("task_claim_recovery"))
	require.NoError(t, err)

	stale, err := repo.Claim(ctx, task.ID, "worker-stale", time.Now().Add(time.Minute), task.Version+1)
	require.NoError(t, err)
	require.False(t, stale)

	claimed, err := repo.Claim(ctx, task.ID, "worker-a", time.Now().Add(time.Minute), task.Version)
	require.NoError(t, err)
	require.True(t, claimed)
	active, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusInProgress, active.Status)
	require.Equal(t, "worker-a", active.WorkerID)
	require.Equal(t, int64(2), active.Version)

	notExpired, err := repo.Claim(ctx, task.ID, "worker-b", time.Now().Add(2*time.Minute), active.Version)
	require.NoError(t, err)
	require.False(t, notExpired)

	forcedExpired := time.Now().Add(-time.Minute)
	_, err = repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{"lease_until": forcedExpired})
	require.NoError(t, err)
	expired, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), expired.Version)

	recovered, err := repo.Claim(ctx, task.ID, "worker-b", time.Now().Add(time.Minute), expired.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	afterRecovery, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "worker-b", afterRecovery.WorkerID)
	require.Equal(t, int64(4), afterRecovery.Version)

	completed, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.True(t, completed)
	terminal, err := repo.Claim(ctx, task.ID, "worker-c", time.Now().Add(time.Minute), afterRecovery.Version)
	require.NoError(t, err)
	require.False(t, terminal)
}

func TestMediaTaskRepositoryRenewLeaseRequiresCurrentLiveClaim(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_renew_lease"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))

	renewedUntil := time.Now().Add(2 * time.Minute)
	renewed, err := repo.RenewLease(ctx, task.ID, "worker-a", renewedUntil)
	require.NoError(t, err)
	require.True(t, renewed)
	afterRenew, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), afterRenew.Version)
	wrongWorker, err := repo.RenewLease(ctx, task.ID, "worker-b", time.Now().Add(3*time.Minute))
	require.NoError(t, err)
	require.False(t, wrongWorker)

	updated, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{"lease_until": time.Now().Add(-time.Minute)})
	require.NoError(t, err)
	require.True(t, updated)
	expired, err := repo.RenewLease(ctx, task.ID, "worker-a", time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.False(t, expired)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	recovered, err := repo.Claim(ctx, task.ID, "worker-c", time.Now().Add(time.Minute), stored.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	completed, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.True(t, completed)
	terminal, err := repo.RenewLease(ctx, task.ID, "worker-c", time.Now().Add(2*time.Minute))
	require.NoError(t, err)
	require.False(t, terminal)
}

func TestMediaTaskRepositoryUpdateClaimedRequiresWorkerAndUnexpiredLease(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_update_claimed"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))

	updated, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{
		"progress":         47,
		"poll_metadata":    json.RawMessage(`{"poll":2}`),
		"upstream_task_id": "up-47",
	})
	require.NoError(t, err)
	require.True(t, updated)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), stored.Version)
	require.Equal(t, 47, stored.Progress)
	require.Equal(t, "up-47", stored.UpstreamTaskID)

	wrongWorker, err := repo.UpdateClaimed(ctx, task.ID, "worker-b", map[string]any{"progress": 48})
	require.NoError(t, err)
	require.False(t, wrongWorker)

	expiredLease := time.Now().Add(-time.Minute)
	changed, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{"lease_until": expiredLease})
	require.NoError(t, err)
	require.True(t, changed)
	expired, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{"progress": 49})
	require.NoError(t, err)
	require.False(t, expired)
}

func TestMediaTaskRepositoryMarkSyncFallbackOnlyOnceAndNeverOnTerminalTask(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_sync_fallback"))
	require.NoError(t, err)
	at := time.Now().UTC().Truncate(time.Millisecond)

	marked, err := repo.MarkSyncFallback(ctx, task.ID, at)
	require.NoError(t, err)
	require.True(t, marked)
	again, err := repo.MarkSyncFallback(ctx, task.ID, at.Add(time.Second))
	require.NoError(t, err)
	require.False(t, again)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, stored.SyncFallback)
	require.WithinDuration(t, at, *stored.SyncFallbackAt, time.Millisecond)

	terminalTask, err := repo.Create(ctx, newRepositoryMediaTask("task_terminal_fallback"))
	require.NoError(t, err)
	failed, err := repo.Transition(ctx, terminalTask.ID, service.MediaTaskStatusQueued, service.MediaTaskStatusFailed, nil)
	require.NoError(t, err)
	require.True(t, failed)
	terminal, err := repo.MarkSyncFallback(ctx, terminalTask.ID, at)
	require.NoError(t, err)
	require.False(t, terminal)
}

func TestMediaTaskRepositoryListsRecoverableAndSettlementPending(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queued, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_queued"))
	require.NoError(t, err)
	expired, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_expired"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, expired, "worker-expired", now.Add(time.Minute))
	changed, err := repo.UpdateClaimed(ctx, expired.ID, "worker-expired", map[string]any{"lease_until": now.Add(-time.Minute)})
	require.NoError(t, err)
	require.True(t, changed)
	active, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_active"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, active, "worker-active", now.Add(time.Minute))
	terminal, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_terminal"))
	require.NoError(t, err)
	transitioned, err := repo.Transition(ctx, terminal.ID, service.MediaTaskStatusQueued, service.MediaTaskStatusFailed, nil)
	require.NoError(t, err)
	require.True(t, transitioned)

	recoverable, err := repo.ListRecoverable(ctx, now, 10)
	require.NoError(t, err)
	require.Equal(t, []int64{queued.ID, expired.ID}, mediaTaskIDs(recoverable))
	limited, err := repo.ListRecoverable(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)

	pending := newRepositoryMediaTask("task_settlement_pending")
	pending.SettlementPlan = json.RawMessage(`{"amount":1}`)
	pending.BillingStatus = "precharged"
	pendingTask, err := repo.Create(ctx, pending)
	require.NoError(t, err)
	retrying := newRepositoryMediaTask("task_settlement_retry")
	retrying.SettlementPlan = json.RawMessage(`{"amount":2}`)
	retrying.BillingStatus = "retry"
	retryingTask, err := repo.Create(ctx, retrying)
	require.NoError(t, err)
	noPlan := newRepositoryMediaTask("task_settlement_no_plan")
	noPlan.BillingStatus = "settling"
	_, err = repo.Create(ctx, noPlan)
	require.NoError(t, err)
	settled := newRepositoryMediaTask("task_settlement_done")
	settled.SettlementPlan = json.RawMessage(`{"amount":3}`)
	settled.BillingStatus = "settled"
	_, err = repo.Create(ctx, settled)
	require.NoError(t, err)

	settlementPending, err := repo.ListSettlementPending(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []int64{pendingTask.ID, retryingTask.ID}, mediaTaskIDs(settlementPending))
}

func TestMediaTaskRepositoryUpdateBillingUsesBillingStatusCASWithoutChangingTaskStatus(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_update_billing")
	input.BillingStatus = "precharged"
	input.PrechargedAmount = 2
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	updated, err := repo.UpdateBilling(ctx, task.ID, "precharged", map[string]any{
		"billing_status":  "settled",
		"final_amount":    1.25,
		"refunded_amount": 0.75,
	})
	require.NoError(t, err)
	require.True(t, updated)
	stale, err := repo.UpdateBilling(ctx, task.ID, "precharged", map[string]any{"billing_status": "retry"})
	require.NoError(t, err)
	require.False(t, stale)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusQueued, stored.Status)
	require.Equal(t, "settled", stored.BillingStatus)
	require.Equal(t, 1.25, stored.FinalAmount)
	require.Equal(t, 0.75, stored.RefundedAmount)
}

func TestMediaArtifactRepositoryCreateIsIdempotentByPosition(t *testing.T) {
	repo, task := newMediaArtifactRepositoryTestHarness(t)
	input := &service.MediaArtifact{
		TaskID: task.ID, Direction: "output", Position: 0,
		MediaType: service.MediaTypeImage, ContentType: "image/png",
	}
	first, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	second, err := repo.Create(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
}

func TestMediaArtifactRepositoryCreateAndListRoundTrip(t *testing.T) {
	repo, task := newMediaArtifactRepositoryTestHarness(t)
	ctx := context.Background()
	width, height := 1024, 768
	duration, fps := 2.5, 24.0
	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	second, err := repo.Create(ctx, &service.MediaArtifact{
		TaskID: task.ID, Direction: "output", Position: 1,
		MediaType: service.MediaTypeVideo, ContentType: "video/mp4",
		SizeBytes: 4096, ChecksumSHA256: "checksum", Width: &width, Height: &height,
		DurationSeconds: &duration, Resolution: "1024x768", FPS: &fps,
		StorageStatus: "stored", ObjectKey: "objects/video", PublicURL: "https://example.test/video",
		UpstreamReference: "upstream-secret", ExpiresAt: &expiresAt,
	})
	require.NoError(t, err)
	first, err := repo.Create(ctx, &service.MediaArtifact{
		TaskID: task.ID, Direction: "input", Position: 0,
		MediaType: service.MediaTypeImage, ContentType: "image/png",
	})
	require.NoError(t, err)

	items, err := repo.ListByTaskID(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, first.ID, items[0].ID)
	require.Equal(t, second.ID, items[1].ID)
	require.Equal(t, int64(4096), items[1].SizeBytes)
	require.Equal(t, "checksum", items[1].ChecksumSHA256)
	require.Equal(t, &width, items[1].Width)
	require.Equal(t, &height, items[1].Height)
	require.Equal(t, &duration, items[1].DurationSeconds)
	require.Equal(t, &fps, items[1].FPS)
	require.Equal(t, "objects/video", items[1].ObjectKey)
	require.Equal(t, "https://example.test/video", items[1].PublicURL)
	require.Equal(t, "upstream-secret", items[1].UpstreamReference)
}

func requireClaimed(t *testing.T, ctx context.Context, repo service.MediaTaskRepository, task *service.MediaTask, workerID string, leaseUntil time.Time) {
	t.Helper()
	claimed, err := repo.Claim(ctx, task.ID, workerID, leaseUntil, task.Version)
	require.NoError(t, err)
	require.True(t, claimed)
}

func mediaTaskIDs(tasks []service.MediaTask) []int64 {
	ids := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
