package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestMediaBillingNumeric20Scale8Normalization(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "round down beyond scale", value: 1.234567894, want: "1.23456789"},
		{name: "round up beyond scale", value: 1.234567895, want: "1.23456790"},
		{name: "smallest unit", value: 0.00000001, want: "0.00000001"},
		{name: "largest representable float below numeric max", value: math.Nextafter(1_000_000_000_000, 0), want: "999999999999.99990000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, err := normalizeMediaAmount(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.want, amount.StringFixed(8))
		})
	}

	maximum, err := normalizeMediaDecimalAmount(decimal.RequireFromString("999999999999.99999999"))
	require.NoError(t, err)
	require.Equal(t, "999999999999.99999999", maximum.StringFixed(8))

	for _, invalid := range []float64{-0.00000001, math.NaN(), math.Inf(1), math.Inf(-1), 1_000_000_000_000} {
		_, err := normalizeMediaAmount(invalid)
		require.ErrorIs(t, err, ErrInvalidMediaBillingResult)
	}
	_, err = normalizeMediaDecimalAmount(decimal.RequireFromString("1000000000000.00000000"))
	require.ErrorIs(t, err, ErrInvalidMediaBillingResult)
}

func TestMediaBillingRejectsOneNumericUnitAccountingMismatch(t *testing.T) {
	err := validateMediaSettlementResult(0.00000001, MediaSettlementResult{})
	require.ErrorIs(t, err, ErrInvalidMediaBillingResult)
}

func TestMediaBillingCoordinatorNormalizesPortAmountsBeforePersistence(t *testing.T) {
	task := workerCompletedTask(114, "billing-normalized-port-amounts")
	task.PrechargedAmount = 3.000000004
	repo := newWorkerTaskRepository(task)
	port := &recordingMediaBillingPort{
		inspect: func(portTask *MediaTask) {
			require.Equal(t, 3.0, portTask.PrechargedAmount)
		},
		settlementResult: MediaSettlementResult{
			FinalAmount:    1.234567894,
			RefundedAmount: 1.765432106,
		},
	}

	require.NoError(t, NewMediaBillingCoordinator(repo, port).SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 1}))
	stored := repo.mustGet(task.ID)
	require.Equal(t, 1.23456789, stored.FinalAmount)
	require.Equal(t, 1.76543211, stored.RefundedAmount)
}

func TestMediaBillingCoordinatorRetriesPersistedSettlementIdempotently(t *testing.T) {
	repo := newWorkerTaskRepository(workerCompletedTask(101, "billing-retry"))
	port := &recordingMediaBillingPort{
		failFirstSettlement: true,
		settlementResult:    MediaSettlementResult{FinalAmount: 0, RefundedAmount: 3},
	}
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

func TestMediaBillingCoordinatorPersistsActualSettlementResults(t *testing.T) {
	tests := []struct {
		name       string
		result     MediaSettlementResult
		failure    *MediaFailureSettlement
		wantFinal  float64
		wantRefund float64
		wantAdd    float64
	}{
		{name: "success charges less", result: MediaSettlementResult{FinalAmount: 2, RefundedAmount: 1}, wantFinal: 2, wantRefund: 1},
		{name: "success charges estimate", result: MediaSettlementResult{FinalAmount: 3}, wantFinal: 3},
		{name: "success charges additional", result: MediaSettlementResult{FinalAmount: 5, AdditionalChargedAmount: 2}, wantFinal: 5, wantAdd: 2},
		{name: "failure refunds all", result: MediaSettlementResult{RefundedAmount: 3}, failure: &MediaFailureSettlement{Kind: MediaFailureKindSystem, RefundRatio: 1}, wantRefund: 3},
		{name: "sync timeout keeps 80 percent", result: MediaSettlementResult{FinalAmount: 2.4, RefundedAmount: 0.6}, failure: &MediaFailureSettlement{Kind: MediaFailureKindSyncTimeout, RefundRatio: 0.2, PenaltyRatio: 0.8}, wantFinal: 2.4, wantRefund: 0.6},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := workerCompletedTask(int64(200+index), "billing-actual-"+tt.name)
			repo := newWorkerTaskRepository(task)
			port := &recordingMediaBillingPort{settlementResult: tt.result}
			coordinator := NewMediaBillingCoordinator(repo, port)

			var err error
			if tt.failure == nil {
				err = coordinator.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 1})
			} else {
				err = coordinator.SettleFailure(context.Background(), task, *tt.failure)
			}
			require.NoError(t, err)
			stored := repo.mustGet(task.ID)
			require.Equal(t, tt.wantFinal, stored.FinalAmount)
			require.Equal(t, tt.wantRefund, stored.RefundedAmount)
			require.Equal(t, tt.wantAdd, stored.AdditionalChargedAmount)
		})
	}
}

func TestDisabledMediaBillingSucceedsWithoutSideEffects(t *testing.T) {
	task := &MediaTask{PublicID: "task_disabled"}
	port := DisabledMediaBilling{}
	precharge, err := port.Precharge(context.Background(), task, MediaBillingSnapshot{EstimatedAmount: 2})
	require.NoError(t, err)
	require.Zero(t, precharge.PrechargedAmount)
	success, err := port.SettleSuccess(context.Background(), task, MediaUsage{ImageCount: 1})
	require.NoError(t, err)
	require.Equal(t, MediaSettlementResult{}, success)
	failure, err := port.SettleFailure(context.Background(), task, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1,
	})
	require.NoError(t, err)
	require.Equal(t, MediaSettlementResult{}, failure)
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
		_, err := recorder.Precharge(context.Background(), task, MediaBillingSnapshot{})
		require.NoError(t, err)
		_, err = recorder.SettleSuccess(context.Background(), task, MediaUsage{})
		require.NoError(t, err)
		_, err = recorder.SettleFailure(context.Background(), task, MediaFailureSettlement{
			Kind: MediaFailureKindSystem, RefundRatio: 1,
		})
		require.NoError(t, err)
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
	settlementResult      MediaSettlementResult
}

func (p *recordingMediaBillingPort) Precharge(_ context.Context, _ *MediaTask, snapshot MediaBillingSnapshot) (MediaPrechargeResult, error) {
	return MediaPrechargeResult{PrechargedAmount: snapshot.EstimatedAmount}, nil
}

func (p *recordingMediaBillingPort) SettleSuccess(_ context.Context, task *MediaTask, _ MediaUsage) (MediaSettlementResult, error) {
	result := p.settlementResult
	if result == (MediaSettlementResult{}) {
		result.FinalAmount = task.PrechargedAmount
	}
	return p.settle(task, result)
}

func (p *recordingMediaBillingPort) SettleFailure(_ context.Context, task *MediaTask, settlement MediaFailureSettlement) (MediaSettlementResult, error) {
	result := p.settlementResult
	if result == (MediaSettlementResult{}) {
		result.FinalAmount = task.PrechargedAmount * (1 - settlement.RefundRatio)
		result.RefundedAmount = task.PrechargedAmount * settlement.RefundRatio
	}
	return p.settle(task, result)
}

func (p *recordingMediaBillingPort) settle(task *MediaTask, result MediaSettlementResult) (MediaSettlementResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settlementAttempts++
	if p.inspect != nil {
		p.inspect(task)
	}
	if p.failFirstSettlement && p.settlementAttempts == 1 {
		return MediaSettlementResult{}, errors.New("billing unavailable")
	}
	p.successfulSettlements++
	return result, nil
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

func (b *idempotentRecordingMediaBilling) Precharge(_ context.Context, task *MediaTask, _ MediaBillingSnapshot) (MediaPrechargeResult, error) {
	return MediaPrechargeResult{}, b.record(task, MediaBillingOperationPrecharge)
}

func (b *idempotentRecordingMediaBilling) SettleSuccess(_ context.Context, task *MediaTask, _ MediaUsage) (MediaSettlementResult, error) {
	return MediaSettlementResult{}, b.record(task, MediaBillingOperationSuccess)
}

func (b *idempotentRecordingMediaBilling) SettleFailure(_ context.Context, task *MediaTask, settlement MediaFailureSettlement) (MediaSettlementResult, error) {
	if err := validateMediaFailureSettlement(settlement); err != nil {
		return MediaSettlementResult{}, err
	}
	return MediaSettlementResult{}, b.record(task, MediaBillingOperationFailure)
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
