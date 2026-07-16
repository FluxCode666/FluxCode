package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaBillingCoordinatorRetriesPersistedSettlementIdempotently(t *testing.T) {
	repo := newWorkerTaskRepository(workerCompletedTask(101, "billing-retry"))
	port := &recordingMediaBillingPort{failFirstSettlement: true}
	coordinator := NewMediaBillingCoordinator(repo, port)
	task := repo.mustGet(101)

	err := coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	})
	require.Error(t, err)
	require.Equal(t, MediaBillingStatusRetry, repo.mustGet(task.ID).BillingStatus)
	require.NoError(t, coordinator.RetryPending(context.Background(), task.ID))
	require.NoError(t, coordinator.RetryPending(context.Background(), task.ID))
	require.Equal(t, MediaBillingStatusSettled, repo.mustGet(task.ID).BillingStatus)
	require.Equal(t, 1, port.successfulSettlementCalls())
	require.Equal(t, task.PrechargedAmount, repo.mustGet(task.ID).RefundedAmount)
}

func TestDisabledMediaBillingSucceedsWithoutSideEffects(t *testing.T) {
	task := &MediaTask{PublicID: "task_disabled"}
	port := DisabledMediaBilling{}
	require.NoError(t, port.Precharge(context.Background(), task, MediaBillingSnapshot{EstimatedAmount: 2}))
	require.NoError(t, port.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 1}))
	require.NoError(t, port.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1,
	}))
}

func TestMediaBillingIdempotencyKeyUsesPublicIDAndSettlementType(t *testing.T) {
	task := &MediaTask{PublicID: "task_idempotent"}
	precharge, err := MediaBillingIdempotencyKey(task, MediaBillingOperationPrecharge)
	require.NoError(t, err)
	success, err := MediaBillingIdempotencyKey(task, MediaBillingOperationSuccess)
	require.NoError(t, err)
	failure, err := MediaBillingIdempotencyKey(task, MediaBillingOperationFailure)
	require.NoError(t, err)
	require.Equal(t, "task_idempotent:precharge", precharge)
	require.Equal(t, "task_idempotent:success", success)
	require.Equal(t, "task_idempotent:failure", failure)
	require.NotEqual(t, success, failure)
}

func TestRecordingMediaBillingDeduplicatesSamePublicIDAndSettlementType(t *testing.T) {
	recorder := newIdempotentRecordingMediaBilling()
	task := &MediaTask{PublicID: "task_recorded"}
	for range 2 {
		require.NoError(t, recorder.Precharge(context.Background(), task, MediaBillingSnapshot{}))
		require.NoError(t, recorder.SettleSuccess(context.Background(), task, MediaUsage{}))
		require.NoError(t, recorder.SettleFailure(context.Background(), task, MediaFailureSettlement{
			Kind: MediaFailureKindSystem, RefundRatio: 1,
		}))
	}
	require.Equal(t, 1, recorder.calls(MediaBillingOperationPrecharge))
	require.Equal(t, 1, recorder.calls(MediaBillingOperationSuccess))
	require.Equal(t, 1, recorder.calls(MediaBillingOperationFailure))
}

func TestMediaBillingCoordinatorRejectsInvalidFailureRatiosBeforePersistence(t *testing.T) {
	task := workerCompletedTask(100, "billing-invalid-ratios")
	repo := newWorkerTaskRepository(task)
	coordinator := NewMediaBillingCoordinator(repo, &recordingMediaBillingPort{})

	err := coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSyncTimeout, RefundRatio: 0.5, PenaltyRatio: 0.6,
	})
	require.ErrorIs(t, err, ErrInvalidMediaFailureSettlement)
	require.Empty(t, repo.mustGet(task.ID).SettlementPlan)
}

func TestMediaBillingCoordinatorRetryRejectsInvalidPersistedFailureRatios(t *testing.T) {
	task := workerCompletedTask(99, "billing-invalid-persisted-ratios")
	plan, err := json.Marshal(MediaSettlementPlan{
		Type: MediaSettlementTypeFailure,
		Failure: &MediaFailureSettlement{
			Kind: MediaFailureKindSyncTimeout, RefundRatio: 0.4, PenaltyRatio: 0.4,
		},
	})
	require.NoError(t, err)
	task.BillingStatus = MediaBillingStatusRetry
	task.SettlementPlan = plan
	repo := newWorkerTaskRepository(task)
	port := &recordingMediaBillingPort{}
	coordinator := NewMediaBillingCoordinator(repo, port)

	err = coordinator.RetryPending(context.Background(), task.ID)
	require.ErrorIs(t, err, ErrInvalidMediaFailureSettlement)
	require.Equal(t, MediaBillingStatusRetry, repo.mustGet(task.ID).BillingStatus)
	require.Zero(t, port.settlementAttemptCalls())
}

func TestMediaBillingCoordinatorPersistsImmutablePlanBeforeCallingPort(t *testing.T) {
	repo := newWorkerTaskRepository(workerCompletedTask(102, "billing-plan"))
	port := &recordingMediaBillingPort{inspect: func(task *MediaTask) {
		stored := repo.mustGet(task.ID)
		require.Equal(t, MediaBillingStatusSettling, stored.BillingStatus)
		require.NotEmpty(t, stored.SettlementPlan)
	}}
	coordinator := NewMediaBillingCoordinator(repo, port)
	task := repo.mustGet(102)

	require.NoError(t, coordinator.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 1}))
	stored := repo.mustGet(task.ID)
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
	require.Equal(t, task.PrechargedAmount, stored.FinalAmount)
	var plan MediaSettlementPlan
	require.NoError(t, json.Unmarshal(stored.SettlementPlan, &plan))
	require.Equal(t, MediaSettlementTypeSuccess, plan.Type)
	require.NotNil(t, plan.Usage)
}

func TestMediaBillingCoordinatorRejectsConflictingPersistedPlan(t *testing.T) {
	task := workerCompletedTask(103, "billing-conflict")
	plan, err := json.Marshal(MediaSettlementPlan{
		Type:  MediaSettlementTypeSuccess,
		Usage: &MediaUsage{ImageCount: 1},
	})
	require.NoError(t, err)
	task.SettlementPlan = plan
	task.BillingStatus = MediaBillingStatusRetry
	repo := newWorkerTaskRepository(task)
	coordinator := NewMediaBillingCoordinator(repo, &recordingMediaBillingPort{})

	err = coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	})
	require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
}

func TestMediaBillingCoordinatorMarksInitialPlanPersistenceFailureUnsafeToAck(t *testing.T) {
	task := workerCompletedTask(104, "billing-plan-write-failure")
	usage := MediaUsage{ImageCount: 1, ImageSize: "1024x1024"}
	recovery, err := json.Marshal(MediaSettlementPlan{Type: MediaSettlementTypeSuccess, Usage: &usage})
	require.NoError(t, err)
	task.SettlementRecovery = recovery
	repo := newWorkerTaskRepository(task)
	repo.failSettlementPlanWrites = 1
	coordinator := NewMediaBillingCoordinator(repo, &recordingMediaBillingPort{})

	err = coordinator.SettleSuccess(context.Background(), task, usage)
	require.ErrorIs(t, err, ErrMediaSettlementPlanNotPersisted)
	require.Empty(t, repo.mustGet(task.ID).SettlementPlan)
}

func TestMediaBillingCoordinatorRejectsRecoveryAndFormalPlanDivergence(t *testing.T) {
	task := workerCompletedTask(105, "billing-recovery-conflict")
	recovery, err := json.Marshal(MediaSettlementPlan{
		Type:  MediaSettlementTypeSuccess,
		Usage: &MediaUsage{ImageCount: 1},
	})
	require.NoError(t, err)
	task.SettlementRecovery = recovery
	repo := newWorkerTaskRepository(task)
	coordinator := NewMediaBillingCoordinator(repo, &recordingMediaBillingPort{})

	err = coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	})
	require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
	require.Empty(t, repo.mustGet(task.ID).SettlementPlan)
}

func TestMediaBillingCoordinatorRejectsSettledRecoveryAndFormalPlanDivergence(t *testing.T) {
	usage := MediaUsage{ImageCount: 1}
	formal, err := json.Marshal(MediaSettlementPlan{
		Type:  MediaSettlementTypeSuccess,
		Usage: &usage,
	})
	require.NoError(t, err)
	recovery, err := json.Marshal(MediaSettlementPlan{
		Type: MediaSettlementTypeFailure,
		Failure: &MediaFailureSettlement{
			Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
		},
	})
	require.NoError(t, err)

	t.Run("settle", func(t *testing.T) {
		task := workerCompletedTask(106, "billing-settled-conflict")
		task.BillingStatus = MediaBillingStatusSettled
		task.SettlementPlan = formal
		task.SettlementRecovery = recovery
		coordinator := NewMediaBillingCoordinator(newWorkerTaskRepository(task), &recordingMediaBillingPort{})

		err := coordinator.SettleSuccess(context.Background(), task, usage)
		require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
	})

	t.Run("retry", func(t *testing.T) {
		task := workerCompletedTask(107, "billing-settled-retry-conflict")
		task.BillingStatus = MediaBillingStatusSettled
		task.SettlementPlan = formal
		task.SettlementRecovery = recovery
		coordinator := NewMediaBillingCoordinator(newWorkerTaskRepository(task), &recordingMediaBillingPort{})

		err := coordinator.RetryPending(context.Background(), task.ID)
		require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
	})
}

func TestMediaBillingCoordinatorRejectsSettledRecoveryWithoutFormalPlan(t *testing.T) {
	usage := MediaUsage{ImageCount: 1}
	recovery, err := json.Marshal(MediaSettlementPlan{
		Type:  MediaSettlementTypeSuccess,
		Usage: &usage,
	})
	require.NoError(t, err)

	t.Run("settle", func(t *testing.T) {
		task := workerCompletedTask(108, "billing-settled-plan-missing")
		task.BillingStatus = MediaBillingStatusSettled
		task.SettlementRecovery = recovery
		coordinator := NewMediaBillingCoordinator(newWorkerTaskRepository(task), &recordingMediaBillingPort{})

		err := coordinator.SettleSuccess(context.Background(), task, usage)
		require.ErrorIs(t, err, ErrMediaSettlementPlanNotPersisted)
	})

	t.Run("retry", func(t *testing.T) {
		task := workerCompletedTask(109, "billing-settled-retry-plan-missing")
		task.BillingStatus = MediaBillingStatusSettled
		task.SettlementRecovery = recovery
		coordinator := NewMediaBillingCoordinator(newWorkerTaskRepository(task), &recordingMediaBillingPort{})

		err := coordinator.RetryPending(context.Background(), task.ID)
		require.ErrorIs(t, err, ErrMediaSettlementPlanNotPersisted)
	})
}

func TestMediaBillingCoordinatorRejectsSettledSuccessToFailureConflict(t *testing.T) {
	usage := MediaUsage{ImageCount: 2, ImageSize: "1024x1024"}
	task := settledMediaBillingTask(t, 110, "billing-settled-success-conflict", MediaSettlementPlan{
		Type: MediaSettlementTypeSuccess, Usage: &usage,
	})
	task.FinalAmount = task.PrechargedAmount
	task.RefundedAmount = 0
	repo := newWorkerTaskRepository(task)
	port := &recordingMediaBillingPort{}
	coordinator := NewMediaBillingCoordinator(repo, port)
	before := repo.mustGet(task.ID)

	err := coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSyncTimeout, RefundRatio: 1, ErrorCode: "system_timeout",
	})
	require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
	assertSettledMediaBillingTaskUnchanged(t, before, repo.mustGet(task.ID))
	require.Zero(t, port.settlementAttemptCalls())
}

func TestMediaBillingCoordinatorRejectsSettledFailureToSuccessConflict(t *testing.T) {
	failure := MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	}
	task := settledMediaBillingTask(t, 111, "billing-settled-failure-conflict", MediaSettlementPlan{
		Type: MediaSettlementTypeFailure, Failure: &failure,
	})
	task.FinalAmount = 0
	task.RefundedAmount = task.PrechargedAmount
	repo := newWorkerTaskRepository(task)
	port := &recordingMediaBillingPort{}
	coordinator := NewMediaBillingCoordinator(repo, port)
	before := repo.mustGet(task.ID)

	err := coordinator.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 1})
	require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
	assertSettledMediaBillingTaskUnchanged(t, before, repo.mustGet(task.ID))
	require.Zero(t, port.settlementAttemptCalls())
}

func TestMediaBillingCoordinatorTreatsMatchingSettledPlanAsIdempotent(t *testing.T) {
	usage := MediaUsage{ImageCount: 1, ImageSize: "1024x1024"}
	task := settledMediaBillingTask(t, 112, "billing-settled-same-plan", MediaSettlementPlan{
		Type: MediaSettlementTypeSuccess, Usage: &usage,
	})
	task.FinalAmount = task.PrechargedAmount
	repo := newWorkerTaskRepository(task)
	port := &recordingMediaBillingPort{}
	coordinator := NewMediaBillingCoordinator(repo, port)
	before := repo.mustGet(task.ID)

	require.NoError(t, coordinator.SettleSuccess(context.Background(), task, usage))
	require.NoError(t, coordinator.RetryPending(context.Background(), task.ID))
	assertSettledMediaBillingTaskUnchanged(t, before, repo.mustGet(task.ID))
	require.Zero(t, port.settlementAttemptCalls())
}

func TestMediaBillingCoordinatorAcceptsLegacySettledTaskWithoutSettlementPlans(t *testing.T) {
	task := workerCompletedTask(113, "billing-legacy-settled")
	task.BillingStatus = MediaBillingStatusSettled
	task.FinalAmount = 1.25
	task.RefundedAmount = 1.75
	repo := newWorkerTaskRepository(task)
	port := &recordingMediaBillingPort{}
	coordinator := NewMediaBillingCoordinator(repo, port)
	before := repo.mustGet(task.ID)

	require.NoError(t, coordinator.RetryPending(context.Background(), task.ID))
	require.NoError(t, coordinator.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	}))
	assertSettledMediaBillingTaskUnchanged(t, before, repo.mustGet(task.ID))
	require.Empty(t, repo.mustGet(task.ID).SettlementPlan)
	require.Empty(t, repo.mustGet(task.ID).SettlementRecovery)
	require.Zero(t, port.settlementAttemptCalls())
}

func settledMediaBillingTask(t *testing.T, id int64, publicID string, plan MediaSettlementPlan) *MediaTask {
	t.Helper()
	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	task := workerCompletedTask(id, publicID)
	task.BillingStatus = MediaBillingStatusSettled
	task.SettlementPlan = append(json.RawMessage(nil), encoded...)
	task.SettlementRecovery = append(json.RawMessage(nil), encoded...)
	return task
}

func assertSettledMediaBillingTaskUnchanged(t *testing.T, before, after *MediaTask) {
	t.Helper()
	require.Equal(t, before.Status, after.Status)
	require.Equal(t, before.BillingStatus, after.BillingStatus)
	require.Equal(t, before.PrechargedAmount, after.PrechargedAmount)
	require.Equal(t, before.FinalAmount, after.FinalAmount)
	require.Equal(t, before.RefundedAmount, after.RefundedAmount)
	require.Equal(t, before.SettlementPlan, after.SettlementPlan)
	require.Equal(t, before.SettlementRecovery, after.SettlementRecovery)
}

type recordingMediaBillingPort struct {
	mu                    sync.Mutex
	failFirstSettlement   bool
	settlementAttempts    int
	successfulSettlements int
	inspect               func(*MediaTask)
}

func (p *recordingMediaBillingPort) Precharge(context.Context, *MediaTask, MediaBillingSnapshot) error {
	return nil
}

func (p *recordingMediaBillingPort) SettleSuccess(_ context.Context, task *MediaTask, _ MediaUsage) error {
	return p.settle(task)
}

func (p *recordingMediaBillingPort) SettleFailure(_ context.Context, task *MediaTask, _ MediaFailureSettlement) error {
	return p.settle(task)
}

func (p *recordingMediaBillingPort) settle(task *MediaTask) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settlementAttempts++
	if p.inspect != nil {
		p.inspect(task)
	}
	if p.failFirstSettlement && p.settlementAttempts == 1 {
		return errors.New("billing unavailable")
	}
	p.successfulSettlements++
	return nil
}

func (p *recordingMediaBillingPort) successfulSettlementCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.successfulSettlements
}

func (p *recordingMediaBillingPort) settlementAttemptCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.settlementAttempts
}

type idempotentRecordingMediaBilling struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	count map[MediaBillingOperation]int
}

func newIdempotentRecordingMediaBilling() *idempotentRecordingMediaBilling {
	return &idempotentRecordingMediaBilling{
		seen: make(map[string]struct{}), count: make(map[MediaBillingOperation]int),
	}
}

func (b *idempotentRecordingMediaBilling) Precharge(_ context.Context, task *MediaTask, _ MediaBillingSnapshot) error {
	return b.record(task, MediaBillingOperationPrecharge)
}

func (b *idempotentRecordingMediaBilling) SettleSuccess(_ context.Context, task *MediaTask, _ MediaUsage) error {
	return b.record(task, MediaBillingOperationSuccess)
}

func (b *idempotentRecordingMediaBilling) SettleFailure(_ context.Context, task *MediaTask, settlement MediaFailureSettlement) error {
	if err := validateMediaFailureSettlement(settlement); err != nil {
		return err
	}
	return b.record(task, MediaBillingOperationFailure)
}

func (b *idempotentRecordingMediaBilling) record(task *MediaTask, operation MediaBillingOperation) error {
	key, err := MediaBillingIdempotencyKey(task, operation)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.seen[key]; exists {
		return nil
	}
	b.seen[key] = struct{}{}
	b.count[operation]++
	return nil
}

func (b *idempotentRecordingMediaBilling) calls(operation MediaBillingOperation) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count[operation]
}
