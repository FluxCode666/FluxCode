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
