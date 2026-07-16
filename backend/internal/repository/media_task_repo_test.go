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
	input.SettlementRecovery = json.RawMessage(`{"type":"success","usage":{"ImageCount":1}}`)
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
	require.JSONEq(t, string(input.SettlementRecovery), string(created.SettlementRecovery))

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

	claimed, err := repo.Claim(ctx, task.ID, "worker-a", time.Now().Add(time.Minute), stored.Version)
	require.NoError(t, err)
	require.True(t, claimed)
	notQueued, err := repo.UpdateQueued(ctx, task.ID, stored.Version+1, map[string]any{"progress": 11})
	require.NoError(t, err)
	require.False(t, notQueued)
}

func TestMediaTaskRepositoryClaimSupportsLeaseRecoveryAndRejectsActiveOrStaleClaims(t *testing.T) {
	repo, client := newMediaTaskRepositoryTestHarness(t)
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
	forceMediaTaskLease(t, ctx, client, task.ID, forcedExpired)
	expired, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), expired.Version)

	recovered, err := repo.Claim(ctx, task.ID, "worker-b", time.Now().Add(time.Minute), expired.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	afterRecovery, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "worker-b", afterRecovery.WorkerID)
	require.Equal(t, int64(3), afterRecovery.Version)

	completed, err := repo.Transition(ctx, task.ID, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.True(t, completed)
	terminal, err := repo.Claim(ctx, task.ID, "worker-c", time.Now().Add(time.Minute), afterRecovery.Version)
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
	renewed, err := repo.RenewLease(ctx, task.ID, "worker-a", renewedUntil)
	require.NoError(t, err)
	require.True(t, renewed)
	afterRenew, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), afterRenew.Version)
	wrongWorker, err := repo.RenewLease(ctx, task.ID, "worker-b", time.Now().Add(3*time.Minute))
	require.NoError(t, err)
	require.False(t, wrongWorker)

	forceMediaTaskLease(t, ctx, client, task.ID, time.Now().Add(-time.Minute))
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
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_update_claimed"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))

	updated, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{
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

	wrongWorker, err := repo.UpdateClaimed(ctx, task.ID, "worker-b", map[string]any{"progress": 48})
	require.NoError(t, err)
	require.False(t, wrongWorker)

	expiredLease := time.Now().Add(-time.Minute)
	changed, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{"lease_until": expiredLease})
	require.ErrorContains(t, err, "not allowed")
	require.False(t, changed)
	forceMediaTaskLease(t, ctx, client, task.ID, expiredLease)
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
	repo, client := newMediaTaskRepositoryTestHarness(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queued, err := repo.Create(ctx, newRepositoryMediaTask("task_recover_queued"))
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
	require.Equal(t, []int64{queued.ID, expired.ID}, mediaTaskIDs(recoverable))
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

	settlementPending, err := repo.ListSettlementPending(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, []int64{pendingTask.ID, recoveryOnlyTask.ID, retryingTask.ID}, mediaTaskIDs(settlementPending))
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

		updated, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{
			"progress":       12,
			"billing_status": "settled",
		})
		require.ErrorContains(t, err, "not allowed")
		require.False(t, updated)
		stored, err := repo.GetByID(ctx, task.ID)
		require.NoError(t, err)
		require.Equal(t, 0, stored.Progress)
		require.Equal(t, "pending", stored.BillingStatus)
		require.Equal(t, int64(2), stored.Version)
	})

	t.Run("UpdateClaimed rejects settlement recovery", func(t *testing.T) {
		repo, _ := newMediaTaskRepositoryTestHarness(t)
		ctx := context.Background()
		task, err := repo.Create(ctx, newRepositoryMediaTask("task_whitelist_claimed_recovery"))
		require.NoError(t, err)
		requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))

		updated, err := repo.UpdateClaimed(ctx, task.ID, "worker-a", map[string]any{
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
		require.Equal(t, "pending", stored.BillingStatus)
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
	task, err := repo.Create(ctx, newRepositoryMediaTask("task_transition_claimed"))
	require.NoError(t, err)
	requireClaimed(t, ctx, repo, task, "worker-a", time.Now().Add(time.Minute))
	claimed, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), claimed.Version)

	wrongWorker, err := repo.TransitionClaimed(ctx, task.ID, "worker-b", claimed.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.False(t, wrongWorker)
	staleVersion, err := repo.TransitionClaimed(ctx, task.ID, "worker-a", task.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.False(t, staleVersion)
	illegalSelfTransition, err := repo.TransitionClaimed(ctx, task.ID, "worker-a", claimed.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusInProgress, nil)
	require.NoError(t, err)
	require.False(t, illegalSelfTransition)

	crossDomain, err := repo.TransitionClaimed(ctx, task.ID, "worker-a", claimed.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, map[string]any{
		"progress":       100,
		"billing_status": "settled",
	})
	require.ErrorContains(t, err, "not allowed")
	require.False(t, crossDomain)
	afterRejectedPatch, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusInProgress, afterRejectedPatch.Status)
	require.Equal(t, 0, afterRejectedPatch.Progress)
	require.Equal(t, "pending", afterRejectedPatch.BillingStatus)
	require.Equal(t, claimed.Version, afterRejectedPatch.Version)

	forceMediaTaskLease(t, ctx, client, task.ID, time.Now().Add(-time.Minute))
	expiredLease, err := repo.TransitionClaimed(ctx, task.ID, "worker-a", claimed.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.False(t, expiredLease)
	recovered, err := repo.Claim(ctx, task.ID, "worker-b", time.Now().Add(time.Minute), claimed.Version)
	require.NoError(t, err)
	require.True(t, recovered)
	recoveredTask, err := repo.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), recoveredTask.Version)

	oldWorker, err := repo.TransitionClaimed(ctx, task.ID, "worker-a", recoveredTask.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, nil)
	require.NoError(t, err)
	require.False(t, oldWorker)
	finishedAt := time.Now().UTC().Truncate(time.Millisecond)
	completed, err := repo.TransitionClaimed(ctx, task.ID, "worker-b", recoveredTask.Version, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, map[string]any{
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

	reopened, err := repo.TransitionClaimed(ctx, task.ID, "worker-b", stored.Version, service.MediaTaskStatusCompleted, service.MediaTaskStatusInProgress, nil)
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
