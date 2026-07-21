package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
		BillingStatus:      service.MediaBillingStatusPrecharged,
	}
}

func TestMediaTaskRepositoryClaimAndCompleteWithCAS(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	task, err := repo.Create(context.Background(), newRepositoryMediaTask("task_repo_claim"))
	require.NoError(t, err)

	claimed, err := repo.Claim(context.Background(), task.ID, "worker-a", mediaRepositoryClaimToken("worker-a"), time.Now().Add(time.Minute), task.Version)
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
	input.SettlementRecovery = json.RawMessage(`{"type":"success","usage":{"ImageCount":1}}`)
	input.BillingStatus = "precharged"
	input.PrechargedAmount = 1.5
	input.AdditionalChargedAmount = 0.75
	input.RetryCount = 2
	input.ErrorCode = "retryable"
	input.ErrorMessage = "try again"
	input.WorkerID = "worker-round-trip"
	input.ClaimToken = "claim-round-trip"
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
	require.Equal(t, input.AdditionalChargedAmount, created.AdditionalChargedAmount)
	require.Equal(t, input.ClaimToken, created.ClaimToken)
	require.JSONEq(t, string(input.PollMetadata), string(created.PollMetadata))
	require.JSONEq(t, string(input.SettlementRecovery), string(created.SettlementRecovery))

	byID, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, byID.ID)
	byPublicID, err := repo.GetByPublicIDForUser(ctx, created.PublicID, created.UserID, created.APIKeyID)
	require.NoError(t, err)
	require.Equal(t, created.ID, byPublicID.ID)
	byIdempotencyKey, err := repo.GetByIdempotencyKey(ctx, created.UserID, created.APIKeyID, created.IdempotencyKey)
	require.NoError(t, err)
	require.Equal(t, created.ID, byIdempotencyKey.ID)

	_, err = repo.GetByPublicIDForUser(ctx, created.PublicID, created.UserID+1, created.APIKeyID)
	require.ErrorIs(t, err, service.ErrMediaTaskNotFound)
	require.True(t, dbent.IsNotFound(err))
	_, err = repo.GetByPublicIDForUser(ctx, created.PublicID, created.UserID, created.APIKeyID+1)
	require.ErrorIs(t, err, service.ErrMediaTaskNotFound)
	require.True(t, dbent.IsNotFound(err))
	_, err = repo.GetByIdempotencyKey(ctx, created.UserID, created.APIKeyID, "")
	require.ErrorIs(t, err, service.ErrMediaTaskNotFound)
	require.True(t, dbent.IsNotFound(err))
}

func TestMediaTaskRepositoryUpdateQueuedUsesStatusAndVersionCAS(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_update_queued"))
	require.NoError(t, err)

	updated, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{
		"adapter":           "adapter-a",
		"native_async_mode": service.NativeAsyncOptional,
		"stage":             service.MediaTaskStageScheduling,
		"progress":          9,
		"channel_id":        int64(81),
	})
	require.NoError(t, err)
	require.True(t, updated)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), stored.Version)
	require.Equal(t, "adapter-a", stored.Adapter)
	require.Equal(t, service.NativeAsyncOptional, stored.NativeAsyncMode)
	require.Equal(t, service.MediaTaskStageScheduling, stored.Stage)
	require.Equal(t, 9, stored.Progress)
	require.Equal(t, int64(81), *stored.ChannelID)

	stale, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"progress": 10})
	require.NoError(t, err)
	require.False(t, stale)

	claimed, err := repo.Claim(ctx, task.ID, "worker-a", mediaRepositoryClaimToken("worker-a"), time.Now().Add(time.Minute), stored.Version)
	require.NoError(t, err)
	require.True(t, claimed)
	notQueued, err := repo.UpdateQueued(ctx, task.ID, stored.Version+1, map[string]any{"progress": 11})
	require.NoError(t, err)
	require.False(t, notQueued)
}

func TestMediaTaskRepositoryUpdateQueuedPersistsRequestSpecWithCAS(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_update_queued_request_spec"))
	require.NoError(t, err)
	requestSpec := json.RawMessage(`{"image":{"prompt":"cat","n":1,"input_artifact_ids":[17]}}`)

	updated, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"request_spec": requestSpec})
	require.NoError(t, err)
	require.True(t, updated)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.JSONEq(t, string(requestSpec), string(stored.RequestSpec))

	stale, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"request_spec": json.RawMessage(`{}`)})
	require.NoError(t, err)
	require.False(t, stale)
}

func TestMediaTaskRepositoryReadyCASClearsInitializationLease(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task := newRepositoryMediaTask("task_ready_clears_initialization_lease")
	task.BillingStatus = service.MediaBillingStatusPending
	initializationLease := time.Now().UTC().Add(time.Minute)
	task.LeaseUntil = &initializationLease
	created, err := repo.Create(ctx, task)
	require.NoError(t, err)

	updated, err := repo.UpdateQueued(ctx, created.ID, created.Version, map[string]any{
		"billing_status":    service.MediaBillingStatusPrecharged,
		"precharged_amount": float64(2),
		"lease_until":       nil,
	})
	require.NoError(t, err)
	require.True(t, updated)
	ready, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaBillingStatusPrecharged, ready.BillingStatus)
	require.Nil(t, ready.LeaseUntil)
}

func TestMediaTaskRepositoryQueuedClaimRequiresReadyBillingAndExpiredLease(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()

	initializing := newRepositoryMediaTask("task_claim_initializing")
	initializing.BillingStatus = service.MediaBillingStatusPending
	initializing.LeaseUntil = mediaRepositoryTimePointer(now.Add(-time.Minute))
	initializingTask, err := repo.Create(ctx, initializing)
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, initializingTask.ID, "worker", mediaRepositoryClaimToken("worker"), now.Add(time.Minute), initializingTask.Version)
	require.NoError(t, err)
	require.False(t, claimed)

	notPublished := newRepositoryMediaTask("task_claim_not_published")
	notPublished.LeaseUntil = mediaRepositoryTimePointer(now.Add(time.Minute))
	notPublishedTask, err := repo.Create(ctx, notPublished)
	require.NoError(t, err)
	claimed, err = repo.Claim(ctx, notPublishedTask.ID, "worker", mediaRepositoryClaimToken("worker"), now.Add(2*time.Minute), notPublishedTask.Version)
	require.NoError(t, err)
	require.False(t, claimed)

	ready := newRepositoryMediaTask("task_claim_ready")
	ready.LeaseUntil = mediaRepositoryTimePointer(now.Add(-time.Minute))
	readyTask, err := repo.Create(ctx, ready)
	require.NoError(t, err)
	claimed, err = repo.Claim(ctx, readyTask.ID, "worker", mediaRepositoryClaimToken("worker"), now.Add(time.Minute), readyTask.Version)
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestMediaTaskRepositoryRequestSpecUpdateRequiresRawMessageAndQueuedPath(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_request_spec_strict_type"))
	require.NoError(t, err)

	_, err = repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"request_spec": []byte(`{}`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "want json.RawMessage")
	_, err = repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"request_spec": json.RawMessage(`{"broken"`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON")

	_, err = repo.UpdateClaimed(ctx, task.ID, "claim-a", task.Version, task.Stage, map[string]any{"request_spec": json.RawMessage(`{}`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not allowed")
}

func TestMediaTaskRepositoryMediaNotFoundMappingPreservesContextErrors(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.GetByPublicIDForUser(canceled, "task_missing", 1, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, errors.Is(err, service.ErrMediaTaskNotFound))
}

func TestMediaTaskRepositoryClaimSupportsLeaseRecoveryAndRejectsActiveOrStaleClaims(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	leaseLessInput := newRepositoryMediaTask("task_claim_without_lease")
	leaseLessInput.Status = service.MediaTaskStatusInProgress
	leaseLess, err := repo.Create(ctx, leaseLessInput)
	require.NoError(t, err)
	recoveredWithoutLease, err := repo.Claim(ctx, leaseLess.ID, "worker-recovery", mediaRepositoryClaimToken("worker-recovery"), time.Now().Add(time.Minute), leaseLess.Version)
	require.NoError(t, err)
	require.True(t, recoveredWithoutLease)

	task, err := repo.Create(ctx, newRepositoryMediaTask("task_claim_recovery"))
	require.NoError(t, err)

	stale, err := repo.Claim(ctx, task.ID, "worker-stale", mediaRepositoryClaimToken("worker-stale"), time.Now().Add(time.Minute), task.Version+1)
	require.NoError(t, err)
	require.False(t, stale)

	claimed, err := repo.Claim(ctx, task.ID, "worker-a", mediaRepositoryClaimToken("worker-a"), time.Now().Add(time.Minute), task.Version)
	require.NoError(t, err)
	require.True(t, claimed)
	active, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusInProgress, active.Status)
	require.Equal(t, "worker-a", active.WorkerID)
	require.Equal(t, int64(2), active.Version)

	notExpired, err := repo.Claim(ctx, task.ID, "worker-b", mediaRepositoryClaimToken("worker-b"), time.Now().Add(2*time.Minute), active.Version)
	require.NoError(t, err)
	require.False(t, notExpired)

	forcedExpired := time.Now().Add(-time.Minute)
	forceMediaTaskLease(t, ctx, client, task.ID, forcedExpired)
	expired, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), expired.Version)

	recovered, err := repo.Claim(ctx, task.ID, "worker-b", mediaRepositoryClaimToken("worker-b"), time.Now().Add(time.Minute), expired.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	afterRecovery, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "worker-b", afterRecovery.WorkerID)
	require.Equal(t, int64(3), afterRecovery.Version)

	completed, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.True(t, completed)
	terminal, err := repo.Claim(ctx, task.ID, "worker-c", mediaRepositoryClaimToken("worker-c"), time.Now().Add(time.Minute), afterRecovery.Version)
	require.NoError(t, err)
	require.False(t, terminal)
}

func TestMediaTaskRepositoryRenewLeaseRequiresCurrentLiveClaim(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_renew_lease"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))

	renewedUntil := time.Now().Add(2 * time.Minute)
	renewed, err := repo.RenewLease(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), renewedUntil)
	require.NoError(t, err)
	require.True(t, renewed)
	afterRenew, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), afterRenew.Version)
	wrongWorker, err := repo.RenewLease(ctx, task.ID, mediaRepositoryClaimToken("worker-b"), time.Now().Add(3*time.Minute))
	require.NoError(t, err)
	require.False(t, wrongWorker)

	forceMediaTaskLease(t, ctx, client, task.ID, time.Now().Add(-time.Minute))
	expired, err := repo.RenewLease(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.False(t, expired)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	recovered, err := repo.Claim(ctx, task.ID, "worker-c", mediaRepositoryClaimToken("worker-c"), time.Now().Add(time.Minute), stored.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	completed, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.True(t, completed)
	terminal, err := repo.RenewLease(ctx, task.ID, mediaRepositoryClaimToken("worker-c"), time.Now().Add(2*time.Minute))
	require.NoError(t, err)
	require.False(t, terminal)
}

func TestMediaTaskRepositoryOldExecutionCannotRenewAfterSameWorkerIDReclaims(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_same_worker_reclaim"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "shared-worker", time.Now().Add(time.Minute))

	forceMediaTaskLease(t, ctx, client, task.ID, time.Now().Add(-time.Minute))
	expired, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	reclaimed, err := repo.Claim(ctx, task.ID, "shared-worker", "new-claim-token", time.Now().Add(time.Minute), expired.Version)
	require.NoError(t, err)
	require.True(t, reclaimed)

	oldExecutionRenewed, err := repo.RenewLease(ctx, task.ID, mediaRepositoryClaimToken("shared-worker"), time.Now().Add(2*time.Minute))
	require.NoError(t, err)
	require.False(t, oldExecutionRenewed, "a worker process ID must not identify a claim generation")
}

func TestMediaTaskRepositoryUpdateClaimedRequiresWorkerAndUnexpiredLease(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_update_claimed"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))
	claimed, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)

	updated, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, map[string]any{
		"progress":          47,
		"poll_metadata":     json.RawMessage(`{"poll":2}`),
		"upstream_task_id":  "up-47",
		"native_async_mode": service.NativeAsyncRequired,
	})
	require.NoError(t, err)
	require.True(t, updated)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), stored.Version)
	require.Equal(t, 47, stored.Progress)
	require.Equal(t, "up-47", stored.UpstreamTaskID)
	require.Equal(t, service.NativeAsyncRequired, stored.NativeAsyncMode)

	wrongWorker, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-b"), stored.Version, stored.Stage, map[string]any{"progress": 48})
	require.NoError(t, err)
	require.False(t, wrongWorker)

	expiredLease := time.Now().Add(-time.Minute)
	changed, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), stored.Version, stored.Stage, map[string]any{"lease_until": expiredLease})
	require.ErrorContains(t, err, "not allowed")
	require.False(t, changed)
	forceMediaTaskLease(t, ctx, client, task.ID, expiredLease)
	expired, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), stored.Version, stored.Stage, map[string]any{"progress": 49})
	require.NoError(t, err)
	require.False(t, expired)
}

func TestMediaTaskRepositoryUpdateClaimedRejectsIllegalStageJump(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_illegal_stage_jump"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))
	claimed, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)

	updated, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, map[string]any{
		"stage": service.MediaTaskStageGenerating,
	})
	require.NoError(t, err)
	require.False(t, updated)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStageQueued, stored.Stage)
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
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queued, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_queued"))
	require.NoError(t, err)
	initializing := newRepositoryMediaTask("task_recover_initializing")
	initializing.BillingStatus = service.MediaBillingStatusPending
	initializing.LeaseUntil = mediaRepositoryTimePointer(now.Add(-time.Minute))
	initializingTask, err := repo.Create(ctx, initializing)
	require.NoError(t, err)
	notPublished := newRepositoryMediaTask("task_recover_not_published")
	notPublished.LeaseUntil = mediaRepositoryTimePointer(now.Add(time.Minute))
	_, err = repo.Create(ctx, notPublished)
	require.NoError(t, err)
	expired, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_expired"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, expired, "worker-expired", now.Add(time.Minute))
	forceMediaTaskLease(t, ctx, client, expired.ID, now.Add(-time.Minute))
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
	require.Equal(t, []int64{queued.ID, initializingTask.ID, expired.ID}, mediaTaskIDs(recoverable))
	limited, err := repo.ListRecoverable(ctx, now, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)

	pending := newRepositoryMediaTask("task_settlement_pending")
	pending.SettlementPlan = json.RawMessage(`{"amount":1}`)
	pending.BillingStatus = "precharged"
	pendingTask, err := repo.Create(ctx, pending)
	require.NoError(t, err)
	recoveryOnly := newRepositoryMediaTask("task_settlement_recovery_only")
	recoveryOnly.Status = service.MediaTaskStatusCompleted
	recoveryOnly.Stage = service.MediaTaskStageCompleted
	recoveryOnly.SettlementRecovery = json.RawMessage(`{"type":"success","usage":{"ImageCount":1}}`)
	recoveryOnly.BillingStatus = "precharged"
	recoveryOnlyTask, err := repo.Create(ctx, recoveryOnly)
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

	settlementPending, err := repo.ListSettlementPending(ctx, time.Now().Add(time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, []int64{pendingTask.ID, recoveryOnlyTask.ID, retryingTask.ID}, mediaTaskIDs(settlementPending))
}

func TestMediaTaskRepositorySettlementRecoveryBackoffAdvancesPastFailedBatch(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	oldest := now.Add(-time.Hour)
	ids := make([]int64, 0, 3)
	for index := 0; index < 3; index++ {
		task := newRepositoryMediaTask(fmt.Sprintf("task_settlement_fair_%d", index))
		task.Status = service.MediaTaskStatusCompleted
		task.Stage = service.MediaTaskStageCompleted
		task.BillingStatus = service.MediaBillingStatusRetry
		task.SettlementPlan = json.RawMessage(`{"type":"success","usage":{"image_count":1}}`)
		task.CreatedAt = oldest.Add(time.Duration(index) * time.Second)
		task.UpdatedAt = task.CreatedAt
		created, err := repo.Create(ctx, task)
		require.NoError(t, err)
		ids = append(ids, created.ID)
	}

	firstBatch, err := repo.ListSettlementPending(ctx, now, 2)
	require.NoError(t, err)
	require.Equal(t, ids[:2], mediaTaskIDs(firstBatch))

	// Model two failed attempts through the production billing CAS. Ent's
	// UpdateDefault advances updated_at, which is the durable retry cursor.
	for _, id := range ids[:2] {
		updated, err := repo.UpdateBilling(ctx, id, service.MediaBillingStatusRetry, map[string]any{
			"billing_status": service.MediaBillingStatusSettling,
		})
		require.NoError(t, err)
		require.True(t, updated)
		updated, err = repo.UpdateBilling(ctx, id, service.MediaBillingStatusSettling, map[string]any{
			"billing_status": service.MediaBillingStatusRetry,
		})
		require.NoError(t, err)
		require.True(t, updated)
		stored, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		require.True(t, stored.UpdatedAt.After(oldest))
	}
	// Pin exact timestamps only to make the tie-break assertion deterministic.
	_, err = client.MediaTask.UpdateOneID(ids[0]).SetUpdatedAt(now).Save(ctx)
	require.NoError(t, err)
	_, err = client.MediaTask.UpdateOneID(ids[1]).SetUpdatedAt(now.Add(time.Second)).Save(ctx)
	require.NoError(t, err)

	backoffReady, err := repo.ListSettlementPending(ctx, now.Add(-time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, []int64{ids[2]}, mediaTaskIDs(backoffReady))

	nextBatch, err := repo.ListSettlementPending(ctx, now.Add(time.Minute), 2)
	require.NoError(t, err)
	require.Equal(t, []int64{ids[2], ids[0]}, mediaTaskIDs(nextBatch))
}

func TestMediaTaskRepositoryTransitionPersistsSettlementRecoveryForPendingScan(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_timeout_recovery")
	input.BillingStatus = service.MediaBillingStatusPrecharged
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	plan := service.MediaSettlementPlan{
		Type: service.MediaSettlementTypeFailure,
		Failure: &service.MediaFailureSettlement{
			Kind:        service.MediaFailureKindSyncTimeout,
			RefundRatio: 1,
			ErrorCode:   "sync_timeout",
		},
	}
	recovery, err := json.Marshal(plan)
	require.NoError(t, err)
	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	transitioned, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusQueued, service.MediaTaskStatusFailed, map[string]any{
		"stage":               service.MediaTaskStageFailed,
		"error_code":          "sync_timeout",
		"error_message":       "synchronous media wait timed out",
		"finished_at":         finishedAt,
		"settlement_recovery": json.RawMessage(recovery),
	})
	require.NoError(t, err)
	require.True(t, transitioned)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.JSONEq(t, string(recovery), string(stored.SettlementRecovery))
	pending, err := repo.ListSettlementPending(ctx, time.Now().Add(time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, []int64{task.ID}, mediaTaskIDs(pending))
}

func TestMediaTaskRepositoryTransitionSyncTimeoutFencesFreshSystemState(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_versioned_system_transition")
	input.Status = service.MediaTaskStatusInProgress
	input.Stage = service.MediaTaskStageGenerating
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	stale, err := repo.TransitionSyncTimeout(
		ctx,
		task.ID,
		task.Version+1,
		task.Stage,
		service.MediaTaskStatusInProgress,
		map[string]any{"stage": service.MediaTaskStageFailed, "error_code": "sync_timeout"},
	)
	require.NoError(t, err)
	require.False(t, stale)

	transitioned, err := repo.TransitionSyncTimeout(
		ctx,
		task.ID,
		task.Version,
		task.Stage,
		service.MediaTaskStatusInProgress,
		map[string]any{"stage": service.MediaTaskStageFailed, "error_code": "sync_timeout"},
	)
	require.NoError(t, err)
	require.True(t, transitioned)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusFailed, stored.Status)
	require.Equal(t, service.MediaTaskStageFailed, stored.Stage)
	require.Equal(t, "sync_timeout", stored.ErrorCode)
	require.Equal(t, task.Version+1, stored.Version)
}

func TestMediaTaskRepositoryTransitionSyncTimeoutRejectsPersistedFallbackWithoutChangingMarkVersion(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_timeout_after_fallback")
	input.Status = service.MediaTaskStatusInProgress
	input.Stage = service.MediaTaskStageGenerating
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	marked, err := repo.MarkSyncFallback(ctx, task.ID, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, marked)
	afterMark, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, afterMark.SyncFallback)
	require.Equal(t, task.Version, afterMark.Version)

	transitioned, err := repo.TransitionSyncTimeout(
		ctx,
		task.ID,
		task.Version,
		task.Stage,
		service.MediaTaskStatusInProgress,
		map[string]any{"stage": service.MediaTaskStageFailed, "error_code": "sync_timeout"},
	)
	require.NoError(t, err)
	require.False(t, transitioned)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusInProgress, stored.Status)
	require.Equal(t, service.MediaTaskStageGenerating, stored.Stage)
	require.True(t, stored.SyncFallback)
	require.Equal(t, task.Version, stored.Version)
}

func TestMediaTaskRepositoryTransitionQueuedRejectsNonQueuedStage(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_queued_status_wrong_stage")
	input.Stage = service.MediaTaskStageScheduling
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	transitioned, err := repo.TransitionQueued(ctx, task.ID, task.Version, service.MediaTaskStatusFailed, map[string]any{
		"stage": service.MediaTaskStageFailed,
	})
	require.NoError(t, err)
	require.False(t, transitioned)
}

func TestMediaTaskRepositoryTransitionQueuedFencesInitializationOwnerAndPersistsRefundIntent(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_initialization_owner_fence")
	input.BillingStatus = service.MediaBillingStatusPending
	input.LeaseUntil = mediaRepositoryTimePointer(time.Now().Add(time.Minute))
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	recovery := json.RawMessage(`{"type":"failure","failure":{"Kind":"system","RefundRatio":1,"PenaltyRatio":0,"ErrorCode":"system_queue"}}`)
	stale, err := repo.TransitionQueued(ctx, task.ID, task.Version+1, service.MediaTaskStatusFailed, map[string]any{
		"stage":               service.MediaTaskStageFailed,
		"billing_status":      service.MediaBillingStatusPrecharged,
		"precharged_amount":   3.25,
		"settlement_recovery": recovery,
	})
	require.NoError(t, err)
	require.False(t, stale)
	illegal, err := repo.TransitionQueued(ctx, task.ID, task.Version, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.False(t, illegal)

	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	transitioned, err := repo.TransitionQueued(ctx, task.ID, task.Version, service.MediaTaskStatusFailed, map[string]any{
		"stage":               service.MediaTaskStageFailed,
		"error_code":          "system_queue",
		"error_message":       "system_queue",
		"finished_at":         finishedAt,
		"billing_status":      service.MediaBillingStatusPrecharged,
		"precharged_amount":   3.25,
		"settlement_recovery": recovery,
		"lease_until":         nil,
	})
	require.NoError(t, err)
	require.True(t, transitioned)

	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusFailed, stored.Status)
	require.Equal(t, service.MediaTaskStageFailed, stored.Stage)
	require.Equal(t, "system_queue", stored.ErrorCode)
	require.Equal(t, service.MediaBillingStatusPrecharged, stored.BillingStatus)
	require.Equal(t, 3.25, stored.PrechargedAmount)
	require.JSONEq(t, string(recovery), string(stored.SettlementRecovery))
	require.Nil(t, stored.LeaseUntil)
	require.Equal(t, task.Version+1, stored.Version)

	staleAfterSuccess, err := repo.TransitionQueued(ctx, task.ID, task.Version, service.MediaTaskStatusFailed, nil)
	require.NoError(t, err)
	require.False(t, staleAfterSuccess)
	pending, err := repo.ListSettlementPending(ctx, time.Now().Add(time.Minute), 10)
	require.NoError(t, err)
	require.Equal(t, []int64{task.ID}, mediaTaskIDs(pending))
}

func mediaRepositoryTimePointer(value time.Time) *time.Time { return &value }

func TestMediaTaskRepositoryUpdateBillingUsesBillingStatusCASWithoutChangingTaskStatus(t *testing.T) {
	repo, _ := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_update_billing")
	input.BillingStatus = "precharged"
	input.PrechargedAmount = 2
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)

	updated, err := repo.UpdateBilling(ctx, task.ID, "precharged", map[string]any{
		"billing_status":            "settled",
		"final_amount":              1.75,
		"refunded_amount":           0.75,
		"additional_charged_amount": 0.5,
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
	require.Equal(t, 1.75, stored.FinalAmount)
	require.Equal(t, 0.75, stored.RefundedAmount)
	require.Equal(t, 0.5, stored.AdditionalChargedAmount)
}

func TestMediaTaskRepositoryTransitionEnforcesDomainStateMachine(t *testing.T) {
	tests := []struct {
		name    string
		current service.MediaTaskStatus
		from    service.MediaTaskStatus
		to      service.MediaTaskStatus
	}{
		{name: "rejects queued self transition", current: service.MediaTaskStatusQueued, from: service.MediaTaskStatusQueued, to: service.MediaTaskStatusQueued},
		{name: "does not reopen completed task", current: service.MediaTaskStatusCompleted, from: service.MediaTaskStatusCompleted, to: service.MediaTaskStatusInProgress},
		{name: "does not reopen failed task", current: service.MediaTaskStatusFailed, from: service.MediaTaskStatusFailed, to: service.MediaTaskStatusQueued},
		{name: "rejects unknown target status", current: service.MediaTaskStatusQueued, from: service.MediaTaskStatusQueued, to: service.MediaTaskStatus("unknown")},
		{name: "rejects unknown current status", current: service.MediaTaskStatus("unknown"), from: service.MediaTaskStatus("unknown"), to: service.MediaTaskStatusQueued},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newMediaTaskRepositoryTestHarness(t)
			ctx := context.Background()
			input := newRepositoryMediaTask("task_transition_" + strings.ReplaceAll(tt.name, " ", "_"))
			input.Status = tt.current
			task, err := repo.Create(ctx, input)
			require.NoError(t, err)

			transitioned, err := repo.Transition(ctx, task.ID, tt.from, tt.to, nil)
			require.NoError(t, err)
			require.False(t, transitioned)
			stored, err := repo.GetByID(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, tt.current, stored.Status)
		})
	}
}

func TestMediaTaskRepositoryOperationFieldWhitelistsRejectCrossDomainUpdates(t *testing.T) {
	t.Run("UpdateQueued rejects sync fallback and does not partially update", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		task, err := repo.Create(ctx, newRepositoryMediaTask("task_whitelist_queued"))
		require.NoError(t, err)

		updated, err := repo.UpdateQueued(ctx, task.ID, task.Version, map[string]any{
			"progress":      12,
			"sync_fallback": true,
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, updated)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Equal(t, 0, stored.Progress)
		require.False(t, stored.SyncFallback)
		require.Equal(t, task.Version, stored.Version)
	})

	t.Run("UpdateClaimed rejects billing fields and does not partially update", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		task, err := repo.Create(ctx, newRepositoryMediaTask("task_whitelist_claimed"))
		require.NoError(t, err)
		requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))
		claimed, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)

		updated, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, map[string]any{
			"progress":       12,
			"billing_status": "settled",
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, updated)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Equal(t, 0, stored.Progress)
		require.Equal(t, service.MediaBillingStatusPrecharged, stored.BillingStatus)
		require.Equal(t, int64(2), stored.Version)
	})

	t.Run("UpdateClaimed rejects settlement recovery", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		task, err := repo.Create(ctx, newRepositoryMediaTask("task_whitelist_claimed_recovery"))
		require.NoError(t, err)
		requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))
		claimed, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)

		updated, err := repo.UpdateClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, map[string]any{
			"settlement_recovery": json.RawMessage(`{"type":"success"}`),
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, updated)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Empty(t, stored.SettlementRecovery)
		require.Equal(t, int64(2), stored.Version)
	})

	t.Run("Transition rejects billing fields and leaves status unchanged", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		task, err := repo.Create(ctx, newRepositoryMediaTask("task_whitelist_transition"))
		require.NoError(t, err)

		transitioned, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusQueued, service.MediaTaskStatusFailed, map[string]any{
			"error_code":     "rejected",
			"billing_status": "settled",
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, transitioned)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Equal(t, service.MediaTaskStatusQueued, stored.Status)
		require.Empty(t, stored.ErrorCode)
		require.Equal(t, service.MediaBillingStatusPrecharged, stored.BillingStatus)
	})

	t.Run("UpdateBilling rejects execution fields and does not partially update", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		input := newRepositoryMediaTask("task_whitelist_billing")
		input.BillingStatus = "precharged"
		task, err := repo.Create(ctx, input)
		require.NoError(t, err)

		updated, err := repo.UpdateBilling(ctx, task.ID, "precharged", map[string]any{
			"final_amount": 2.5,
			"progress":     99,
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, updated)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Equal(t, float64(0), stored.FinalAmount)
		require.Equal(t, 0, stored.Progress)
		require.Equal(t, "precharged", stored.BillingStatus)
	})

	t.Run("UpdateBilling rejects settlement recovery", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		input := newRepositoryMediaTask("task_whitelist_billing_recovery")
		input.BillingStatus = "precharged"
		task, err := repo.Create(ctx, input)
		require.NoError(t, err)

		updated, err := repo.UpdateBilling(ctx, task.ID, "precharged", map[string]any{
			"settlement_recovery": json.RawMessage(`{"type":"success"}`),
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, updated)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Empty(t, stored.SettlementRecovery)
		require.Equal(t, "precharged", stored.BillingStatus)
	})
}

func TestMediaTaskRepositoryTransitionClaimedProtectsWorkerVersionLeaseAndState(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	input := newRepositoryMediaTask("task_transition_claimed")
	input.Status = service.MediaTaskStatusInProgress
	input.Stage = service.MediaTaskStageSettling
	task, err := repo.Create(ctx, input)
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))
	claimed, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), claimed.Version)

	terminalUpdates := map[string]any{"stage": service.MediaTaskStageCompleted}
	wrongWorker, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-b"), claimed.Version, claimed.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, terminalUpdates)
	require.NoError(t, err)
	require.False(t, wrongWorker)
	staleVersion, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), task.Version, claimed.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, terminalUpdates)
	require.NoError(t, err)
	require.False(t, staleVersion)
	illegalSelfTransition, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusInProgress, nil)
	require.NoError(t, err)
	require.False(t, illegalSelfTransition)

	crossDomain, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, map[string]any{
		"progress":       100,
		"billing_status": "settled",
	})
	require.ErrorContains(t, err, "not allowed")
	require.False(t, crossDomain)
	afterRejectedPatch, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusInProgress, afterRejectedPatch.Status)
	require.Equal(t, 0, afterRejectedPatch.Progress)
	require.Equal(t, service.MediaBillingStatusPrecharged, afterRejectedPatch.BillingStatus)
	require.Equal(t, claimed.Version, afterRejectedPatch.Version)

	forceMediaTaskLease(t, ctx, client, task.ID, time.Now().Add(-time.Minute))
	expiredLease, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), claimed.Version, claimed.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, terminalUpdates)
	require.NoError(t, err)
	require.False(t, expiredLease)
	recovered, err := repo.Claim(ctx, task.ID, "worker-b", mediaRepositoryClaimToken("worker-b"), time.Now().Add(time.Minute), claimed.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	recoveredTask, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), recoveredTask.Version)

	oldWorker, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-a"), recoveredTask.Version, recoveredTask.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, terminalUpdates)
	require.NoError(t, err)
	require.False(t, oldWorker)
	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	completed, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-b"), recoveredTask.Version, recoveredTask.Stage, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, map[string]any{
		"stage":               service.MediaTaskStageCompleted,
		"progress":            100,
		"finished_at":         finishedAt,
		"settlement_recovery": json.RawMessage(`{"type":"success","usage":{"ImageCount":1}}`),
	})
	require.NoError(t, err)
	require.True(t, completed)
	stored, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusCompleted, stored.Status)
	require.Equal(t, service.MediaTaskStageCompleted, stored.Stage)
	require.Equal(t, 100, stored.Progress)
	require.WithinDuration(t, finishedAt, *stored.FinishedAt, time.Millisecond)
	require.JSONEq(t, `{"type":"success","usage":{"ImageCount":1}}`, string(stored.SettlementRecovery))
	require.Equal(t, int64(4), stored.Version)

	reopened, err := repo.TransitionClaimed(ctx, task.ID, mediaRepositoryClaimToken("worker-b"), stored.Version, stored.Stage, service.MediaTaskStatusCompleted, service.MediaTaskStatusInProgress, nil)
	require.NoError(t, err)
	require.False(t, reopened)
}

func TestMediaTaskRepositoryOperationFieldWhitelistsValidateDomainEnums(t *testing.T) {
	tests := []struct {
		name    string
		updates map[string]any
	}{
		{name: "rejects raw stage string", updates: map[string]any{"stage": "scheduling"}},
		{name: "rejects unknown stage enum", updates: map[string]any{"stage": service.MediaTaskStage("unknown")}},
		{name: "rejects raw native async string", updates: map[string]any{"native_async_mode": "optional"}},
		{name: "rejects unknown native async enum", updates: map[string]any{"native_async_mode": service.NativeAsyncMode("unknown")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newMediaTaskRepositoryTestHarness(t)
			ctx := context.Background()
			task, err := repo.Create(ctx, newRepositoryMediaTask("task_enum_"+strings.ReplaceAll(tt.name, " ", "_")))
			require.NoError(t, err)

			updated, err := repo.UpdateQueued(ctx, task.ID, task.Version, tt.updates)
			require.Error(t, err)
			require.False(t, updated)
			stored, err := repo.GetByID(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, task.Stage, stored.Stage)
			require.Equal(t, task.NativeAsyncMode, stored.NativeAsyncMode)
			require.Equal(t, task.Version, stored.Version)
		})
	}
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

func TestMediaArtifactRepositoryCreateRejectsConflictingContentIdentity(t *testing.T) {
	mutations := map[string]func(*service.MediaArtifact){
		"media type":         func(a *service.MediaArtifact) { a.MediaType = service.MediaTypeVideo },
		"content type":       func(a *service.MediaArtifact) { a.ContentType = "image/jpeg" },
		"size":               func(a *service.MediaArtifact) { a.SizeBytes++ },
		"checksum":           func(a *service.MediaArtifact) { a.ChecksumSHA256 = "different" },
		"width":              func(a *service.MediaArtifact) { a.Width = mediaRepositoryIntPointer(2048) },
		"height":             func(a *service.MediaArtifact) { a.Height = mediaRepositoryIntPointer(2048) },
		"duration":           func(a *service.MediaArtifact) { a.DurationSeconds = mediaRepositoryFloatPointer(2) },
		"resolution":         func(a *service.MediaArtifact) { a.Resolution = "2048x2048" },
		"fps":                func(a *service.MediaArtifact) { a.FPS = mediaRepositoryFloatPointer(30) },
		"storage status":     func(a *service.MediaArtifact) { a.StorageStatus = "proxy" },
		"object key":         func(a *service.MediaArtifact) { a.ObjectKey = "objects/b" },
		"public url":         func(a *service.MediaArtifact) { a.PublicURL = "https://cdn.example/b" },
		"upstream reference": func(a *service.MediaArtifact) { a.UpstreamReference = "internal-b" },
		"expiry": func(a *service.MediaArtifact) {
			a.ExpiresAt = mediaRepositoryTimePointer(time.Now().UTC().Add(2 * time.Hour))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			repo, task := newMediaArtifactRepositoryTestHarness(t)
			base := completeRepositoryMediaArtifact(task.ID)
			_, err := repo.Create(context.Background(), base)
			require.NoError(t, err)
			conflict := completeRepositoryMediaArtifact(task.ID)
			mutate(conflict)

			created, err := repo.Create(context.Background(), conflict)
			require.Nil(t, created)
			require.ErrorIs(t, err, service.ErrMediaArtifactConflict)
		})
	}
}

func TestMediaArtifactRepositoryDeleteExactRequiresImmutableIdentity(t *testing.T) {
	repo, task := newMediaArtifactRepositoryTestHarness(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, completeRepositoryMediaArtifact(task.ID))
	require.NoError(t, err)

	wrong := *created
	wrong.ObjectKey = "objects/not-the-created-object"
	deleted, err := repo.DeleteExact(ctx, &wrong)
	require.NoError(t, err)
	require.False(t, deleted)
	items, err := repo.ListByTaskID(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	deleted, err = repo.DeleteExact(ctx, created)
	require.NoError(t, err)
	require.True(t, deleted)
	items, err = repo.ListByTaskID(ctx, task.ID)
	require.NoError(t, err)
	require.Empty(t, items)
}

func completeRepositoryMediaArtifact(taskID int64) *service.MediaArtifact {
	expiresAt := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	return &service.MediaArtifact{
		TaskID: taskID, Direction: "output", Position: 0,
		MediaType: service.MediaTypeImage, ContentType: "image/png",
		SizeBytes: 100, ChecksumSHA256: "checksum-a",
		Width: mediaRepositoryIntPointer(1024), Height: mediaRepositoryIntPointer(1024),
		DurationSeconds: mediaRepositoryFloatPointer(1), Resolution: "1024x1024", FPS: mediaRepositoryFloatPointer(24),
		StorageStatus: "stored", ObjectKey: "objects/a", PublicURL: "https://cdn.example/a",
		UpstreamReference: "internal-a", ExpiresAt: &expiresAt,
	}
}

func mediaRepositoryIntPointer(value int) *int { return &value }

func mediaRepositoryFloatPointer(value float64) *float64 { return &value }

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
	claimed, err := repo.Claim(ctx, task.ID, workerID, mediaRepositoryClaimToken(workerID), leaseUntil, task.Version)
	require.NoError(t, err)
	require.True(t, claimed)
}

func mediaRepositoryClaimToken(workerID string) string {
	return "claim-" + workerID
}

func forceMediaTaskLease(t *testing.T, ctx context.Context, client *dbent.Client, taskID int64, leaseUntil time.Time) {
	t.Helper()
	_, err := client.MediaTask.UpdateOneID(taskID).SetLeaseUntil(leaseUntil.UTC()).Save(ctx)
	require.NoError(t, err)
}

func mediaTaskIDs(tasks []service.MediaTask) []int64 {
	ids := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
