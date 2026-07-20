package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMediaWorkerExecutionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		clientAsync bool
		mode        NativeAsyncMode
		wantSync    int64
		wantSubmit  int64
	}{
		{"sync_unsupported", false, NativeAsyncUnsupported, 1, 0},
		{"sync_optional", false, NativeAsyncOptional, 1, 0},
		{"sync_required", false, NativeAsyncRequired, 0, 1},
		{"async_unsupported", true, NativeAsyncUnsupported, 1, 0},
		{"async_optional", true, NativeAsyncOptional, 0, 1},
		{"async_required", true, NativeAsyncRequired, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, tt.clientAsync, tt.mode)
			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Equal(t, tt.wantSync, fixture.adapter.syncCalls.Load())
			require.Equal(t, tt.wantSubmit, fixture.adapter.submitCalls.Load())
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
		})
	}
}

func TestMediaWorkerPersistsSubmittedAtBeforeSynchronousGenerate(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	var requestTask *MediaTask
	var storedAtGenerate *MediaTask
	fixture.adapter.generateHook = func(req MediaExecutionRequest) {
		requestTask = cloneWorkerTask(req.Task)
		storedAtGenerate = fixture.repo.mustGet(fixture.task.ID)
	}

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.NotNil(t, requestTask)
	require.NotNil(t, requestTask.SubmittedAt)
	require.Equal(t, MediaTaskStageGenerating, requestTask.Stage)
	require.NotNil(t, storedAtGenerate)
	require.NotNil(t, storedAtGenerate.SubmittedAt)
	require.Equal(t, MediaTaskStageGenerating, storedAtGenerate.Stage)
}

func TestMediaWorkerPassesMappedResolvedRequestToAdapterWithoutMediaSpecDecode(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	var resolved json.RawMessage
	fixture.repo.mu.Lock()
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(fixture.repo.tasks[fixture.task.ID].CandidateSnapshot, &candidates))
	candidates[0].ResolvedRequest = json.RawMessage(`{"image":{"prompt":"cat","chicun":"1024x1024"}}`)
	fixture.repo.tasks[fixture.task.ID].CandidateSnapshot, _ = json.Marshal(candidates)
	fixture.repo.mu.Unlock()
	fixture.adapter.generateHook = func(req MediaExecutionRequest) {
		resolved = append(json.RawMessage(nil), req.ResolvedRequest...)
	}

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.JSONEq(t, `{"image":{"prompt":"cat","chicun":"1024x1024"}}`, string(resolved))
}

func TestMediaWorkerDoesNotExecuteQueuedTaskBeforeReady(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].BillingStatus = MediaBillingStatusPending
	fixture.repo.tasks[fixture.task.ID].LeaseUntil = mediaTimePointer(time.Now().Add(time.Minute))
	fixture.repo.mu.Unlock()

	err := fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaTaskNotClaimed)
	require.Zero(t, fixture.adapter.syncCalls.Load())
	require.Zero(t, fixture.adapter.submitCalls.Load())
}

func TestMediaWorkerAcksEarlyInitializationMessageAndRecoveryRequeuesAfterReady(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.BillingStatus = MediaBillingStatusPending
	stored.LeaseUntil = mediaTimePointer(time.Now().Add(time.Minute))
	fixture.repo.mu.Unlock()

	err := fixture.worker.processMessage(context.Background(), &MediaQueueMessage{ID: "early", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync})
	require.NoError(t, err)
	require.Equal(t, int64(1), fixture.queue.ackCalls.Load())
	require.Zero(t, fixture.adapter.syncCalls.Load())
	require.Zero(t, fixture.adapter.submitCalls.Load())

	fixture.repo.mu.Lock()
	stored.BillingStatus = MediaBillingStatusPrecharged
	stored.LeaseUntil = nil
	stored.Version++
	fixture.repo.mu.Unlock()
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
}

func TestMediaWorkerInitialExecutionUsesStableTaskSlotID(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, MediaTaskSlotID(fixture.task.ID), fixture.selector.lastStableSlotID())
}

func TestMediaWorkerStableSlotIgnoresPublicIDWhitespaceAndSubmitPreservesPublicID(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	publicID := "  public task id  "
	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].PublicID = publicID
	fixture.repo.mu.Unlock()

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, MediaTaskSlotID(fixture.task.ID), fixture.selector.lastStableSlotID())
	require.Equal(t, publicID, fixture.adapter.lastRequest().IdempotencyKey)
}

func TestMediaWorkerInitialExecutionAllowsUnlimitedAccountConcurrency(t *testing.T) {
	for _, concurrency := range []int{0, -1} {
		t.Run(fmt.Sprintf("concurrency_%d", concurrency), func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			fixture.account.Concurrency = concurrency

			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
			require.Zero(t, fixture.selector.waitCalls.Load())
		})
	}
}

func TestMediaWorkerUnlimitedStableSlotsBypassOptionalStableAcquirer(t *testing.T) {
	for _, tt := range []struct {
		name        string
		concurrency int
	}{{name: "zero", concurrency: 0}, {name: "negative", concurrency: -1}} {
		t.Run("initial "+tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			fixture.account.Concurrency = tt.concurrency
			fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(
				&panicAccountCandidateConcurrency{}, nil, task12SchedulingConfig(),
			)

			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
		})

		t.Run("fixed recovery "+tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			fixture.account.Concurrency = tt.concurrency
			prepareRecoverableExistingUpstream(fixture, "existing-upstream")
			fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(
				&panicAccountCandidateConcurrency{}, nil, task12SchedulingConfig(),
			)

			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
			require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
		})
	}
}

func TestMediaWorkerFiniteStableSlotFailsClosedWithoutConcurrencyService(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(nil, nil, task12SchedulingConfig())

	err := fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID})
	require.ErrorIs(t, err, ErrStableAccountSlotUnsupported)
	require.Zero(t, fixture.adapter.syncCalls.Load())
	require.Zero(t, fixture.adapter.submitCalls.Load())
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
	require.Zero(t, fixture.queue.publishCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
}

func TestMediaWorkerSchedulingInfrastructureErrorsRemainRetryable(t *testing.T) {
	assertRetryable := func(t *testing.T, fixture *mediaWorkerFixture, want error) {
		t.Helper()
		err := fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID})
		require.ErrorIs(t, err, want)
		require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
		require.Zero(t, fixture.adapter.syncCalls.Load())
		require.Zero(t, fixture.adapter.submitCalls.Load())
		require.Zero(t, fixture.adapter.pollCalls.Load())
		require.Zero(t, fixture.billing.settlementCalls())
		require.Zero(t, fixture.queue.publishCalls.Load())
		require.Zero(t, fixture.queue.ackCalls.Load())
	}

	t.Run("account repository", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		infrastructureErr := errors.New("account database unavailable")
		fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository).listErr = infrastructureErr
		assertRetryable(t, fixture, infrastructureErr)
	})

	t.Run("immediate redis acquire", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		infrastructureErr := errors.New("redis acquire unavailable")
		concurrency := &accountCandidateConcurrencyStub{acquireErr: infrastructureErr}
		cfg := task12SchedulingConfig()
		cfg.LoadBatchEnabled = false
		fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(concurrency, nil, cfg)
		assertRetryable(t, fixture, infrastructureErr)
	})

	t.Run("redis wait", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		infrastructureErr := errors.New("redis wait unavailable")
		concurrency := &accountCandidateConcurrencyStub{
			acquireResults: map[int64][]bool{fixture.account.ID: {false}}, waitErr: infrastructureErr,
		}
		cfg := task12SchedulingConfig()
		cfg.LoadBatchEnabled = false
		fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(concurrency, nil, cfg)
		assertRetryable(t, fixture, infrastructureErr)
	})

	t.Run("stable capability missing", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		cfg := task12SchedulingConfig()
		cfg.LoadBatchEnabled = false
		fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(&atomicAccountCandidateConcurrency{}, nil, cfg)
		assertRetryable(t, fixture, ErrStableAccountSlotUnsupported)
	})
}

func TestMediaWorkerSchedulerExhaustionClassificationPreservesErrorKinds(t *testing.T) {
	require.True(t, isMediaSchedulerExhaustion(fmt.Errorf("wrapped: %w", ErrNoAvailableAccounts)))
	require.True(t, isMediaSchedulerExhaustion(fmt.Errorf("wrapped: %w", ErrAccountConcurrencySaturated)))
	for _, err := range []error{
		errors.New("database unavailable"),
		ErrStableAccountSlotUnsupported,
		context.Canceled,
		context.DeadlineExceeded,
	} {
		require.False(t, isMediaSchedulerExhaustion(fmt.Errorf("wrapped: %w", err)))
	}
}

func TestMediaWorkerFixedSchedulingInfrastructureErrorsRemainRetryable(t *testing.T) {
	assertRetryable := func(t *testing.T, fixture *mediaWorkerFixture, want error) {
		t.Helper()
		err := fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID})
		require.ErrorIs(t, err, want)
		require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
		require.Zero(t, fixture.adapter.submitCalls.Load())
		require.Zero(t, fixture.adapter.pollCalls.Load())
		require.Zero(t, fixture.billing.settlementCalls())
		require.Zero(t, fixture.queue.publishCalls.Load())
		require.Zero(t, fixture.queue.ackCalls.Load())
	}

	t.Run("account repository", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		prepareRecoverableExistingUpstream(fixture, "existing-upstream")
		infrastructureErr := errors.New("fixed account database unavailable")
		fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository).getErr = infrastructureErr
		assertRetryable(t, fixture, infrastructureErr)
	})

	for _, tt := range []struct {
		name        string
		concurrency AccountCandidateConcurrency
		want        error
	}{
		{name: "immediate redis acquire", concurrency: &accountCandidateConcurrencyStub{acquireErr: errors.New("fixed redis acquire unavailable")}},
		{name: "redis wait", concurrency: &accountCandidateConcurrencyStub{acquireResults: map[int64][]bool{7: {false}}, waitErr: errors.New("fixed redis wait unavailable")}},
		{name: "stable capability missing", concurrency: &atomicAccountCandidateConcurrency{}, want: ErrStableAccountSlotUnsupported},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			prepareRecoverableExistingUpstream(fixture, "existing-upstream")
			cfg := task12SchedulingConfig()
			cfg.LoadBatchEnabled = false
			fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(tt.concurrency, nil, cfg)
			want := tt.want
			if want == nil {
				switch concurrency := tt.concurrency.(type) {
				case *accountCandidateConcurrencyStub:
					if concurrency.acquireErr != nil {
						want = concurrency.acquireErr
					} else {
						want = concurrency.waitErr
					}
				}
			}
			assertRetryable(t, fixture, want)
		})
	}
}

func TestMediaWorkerSchedulingExhaustionStillFailsAndRefunds(t *testing.T) {
	assertFailed := func(t *testing.T, fixture *mediaWorkerFixture) {
		t.Helper()
		require.NoError(t, fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID}))
		stored := fixture.repo.mustGet(fixture.task.ID)
		require.Equal(t, MediaTaskStatusFailed, stored.Status)
		require.Equal(t, "system_scheduler", stored.ErrorCode)
		require.Equal(t, MediaFailureSettlement{
			Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_scheduler",
		}, fixture.billing.lastFailure())
		require.Zero(t, fixture.adapter.syncCalls.Load())
		require.Zero(t, fixture.adapter.submitCalls.Load())
		require.Zero(t, fixture.adapter.pollCalls.Load())
		require.Equal(t, int64(1), fixture.queue.publishCalls.Load())
		require.Equal(t, int64(1), fixture.queue.ackCalls.Load())
	}

	t.Run("initial no accounts", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		fixture.account.Schedulable = false
		assertFailed(t, fixture)
	})

	t.Run("initial saturated", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		concurrency := &accountCandidateConcurrencyStub{acquireResults: map[int64][]bool{fixture.account.ID: {false}}}
		cfg := task12SchedulingConfig()
		cfg.LoadBatchEnabled = false
		fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(concurrency, nil, cfg)
		assertFailed(t, fixture)
	})

	t.Run("fixed saturated", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		prepareRecoverableExistingUpstream(fixture, "existing-upstream")
		concurrency := &accountCandidateConcurrencyStub{acquireResults: map[int64][]bool{fixture.account.ID: {false}}}
		cfg := task12SchedulingConfig()
		cfg.LoadBatchEnabled = false
		fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(concurrency, nil, cfg)
		assertFailed(t, fixture)
	})
}

func TestMediaWorkerDeletedFixedAccountFailsAndRefunds(t *testing.T) {
	assertSchedulerFailure := func(t *testing.T, fixture *mediaWorkerFixture) {
		t.Helper()
		repo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)
		repo.getErr = fmt.Errorf("soft-deleted account: %w", ErrAccountNotFound)

		require.NoError(t, fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID}))
		stored := fixture.repo.mustGet(fixture.task.ID)
		require.Equal(t, MediaTaskStatusFailed, stored.Status)
		require.Equal(t, "system_scheduler", stored.ErrorCode)
		require.Equal(t, MediaFailureSettlement{
			Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_scheduler",
		}, fixture.billing.lastFailure())
		require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
		require.NotEmpty(t, stored.SettlementPlan)
		var plan MediaSettlementPlan
		require.NoError(t, json.Unmarshal(stored.SettlementPlan, &plan))
		require.Equal(t, MediaSettlementTypeFailure, plan.Type)
		require.NotNil(t, plan.Failure)
		require.Equal(t, fixture.billing.lastFailure(), *plan.Failure)
		require.Equal(t, int64(1), fixture.queue.publishCalls.Load())
		require.Equal(t, int64(1), fixture.queue.ackCalls.Load())
	}

	t.Run("existing upstream", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		prepareRecoverableExistingUpstream(fixture, "existing-upstream")

		assertSchedulerFailure(t, fixture)
		require.Zero(t, fixture.adapter.pollCalls.Load())
		require.Zero(t, fixture.adapter.abortCalls.Load())
	})

	t.Run("unknown submission", func(t *testing.T) {
		fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
		fixedAdapter, _ := prepareRecoverableSubmittingTask(t, fixture, true)

		assertSchedulerFailure(t, fixture)
		require.Zero(t, fixedAdapter.submitCalls.Load())
		require.Zero(t, fixture.selector.selectCalls.Load())
	})
}

func TestMediaWorkerFixedAccountLookupInfrastructureErrorsRemainPending(t *testing.T) {
	tests := []struct {
		name      string
		lookupErr error
		prepare   func(*testing.T, *mediaWorkerFixture)
	}{
		{
			name:      "existing upstream database error",
			lookupErr: errors.New("fixed account database unavailable"),
			prepare: func(_ *testing.T, fixture *mediaWorkerFixture) {
				prepareRecoverableExistingUpstream(fixture, "existing-upstream")
			},
		},
		{
			name:      "unknown submission context deadline",
			lookupErr: context.DeadlineExceeded,
			prepare: func(t *testing.T, fixture *mediaWorkerFixture) {
				prepareRecoverableSubmittingTask(t, fixture, true)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			tt.prepare(t, fixture)
			fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository).getErr = tt.lookupErr

			err := fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID})
			require.ErrorIs(t, err, tt.lookupErr)
			require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
			require.Zero(t, fixture.billing.settlementCalls())
			require.Zero(t, fixture.queue.publishCalls.Load())
			require.Zero(t, fixture.queue.ackCalls.Load())
		})
	}
}

func TestMediaWorkerIgnoresDuplicateTerminalMessage(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Equal(t, 1, fixture.billing.settlementCalls())
	require.Equal(t, int64(1), fixture.metrics.DuplicateMessages())
}

func TestMediaWorkerRecoverOnceRequeuesExpiredLease(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.setExpiredLease(fixture.task.ID, "dead-worker")
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
	require.Equal(t, []MediaQueuePriority{MediaQueuePriorityAsync}, fixture.queue.enqueuedPriorities())
	require.Equal(t, int64(1), fixture.metrics.Recoveries())
}

func TestMediaWorkerRecoverOnceCleansExpiredPendingInitializerWithoutIdempotencyKey(t *testing.T) {
	tests := []struct {
		name               string
		prechargeBeforeRun bool
	}{
		{name: "crash after create"},
		{name: "crash after external precharge", prechargeBeforeRun: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
			snapshot := MediaBillingSnapshot{RequestedModel: "fake-image", EstimatedAmount: 2}
			encoded, err := json.Marshal(snapshot)
			require.NoError(t, err)
			fixture.repo.mu.Lock()
			stored := fixture.repo.tasks[fixture.task.ID]
			stored.IdempotencyKey = ""
			stored.BillingStatus = MediaBillingStatusPending
			stored.PrechargedAmount = 0
			stored.BillingSnapshot = encoded
			stored.LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
			fixture.repo.mu.Unlock()

			port := newWorkerInitializationBillingPort()
			fixture.worker.deps.Precharger = port
			fixture.worker.deps.Billing = NewMediaBillingCoordinator(fixture.repo, port)
			if tt.prechargeBeforeRun {
				_, err := port.Precharge(context.Background(), fixture.repo.mustGet(fixture.task.ID), snapshot)
				require.NoError(t, err)
			}

			require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
			cleaned := fixture.repo.mustGet(fixture.task.ID)
			require.Equal(t, MediaTaskStatusFailed, cleaned.Status)
			require.Equal(t, "system_initialization_expired", cleaned.ErrorCode)
			require.Equal(t, MediaBillingStatusSettled, cleaned.BillingStatus)
			require.Equal(t, 2.0, cleaned.PrechargedAmount)
			require.Equal(t, 2.0, cleaned.RefundedAmount)
			require.Zero(t, cleaned.FinalAmount)
			require.NotEmpty(t, cleaned.SettlementRecovery)
			require.NotEmpty(t, cleaned.SettlementPlan)
			require.Equal(t, 1, port.prechargeMutationCalls())
			require.Zero(t, port.balanceAmount())
			require.Empty(t, fixture.queue.enqueuedTaskIDs())
		})
	}
}

func TestMediaWorkerRecoverOnceRetriesUnknownPrechargeResultBeforeCleanup(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
	snapshot := MediaBillingSnapshot{RequestedModel: "fake-image", EstimatedAmount: 2}
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.IdempotencyKey = ""
	stored.BillingStatus = MediaBillingStatusPending
	stored.PrechargedAmount = 0
	stored.BillingSnapshot = encoded
	stored.LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
	fixture.repo.mu.Unlock()

	port := newWorkerInitializationBillingPort()
	port.failAfterFirstPrechargeMutation = true
	fixture.worker.deps.Precharger = port
	fixture.worker.deps.Billing = NewMediaBillingCoordinator(fixture.repo, port)

	err = fixture.worker.RecoverOnce(context.Background())
	require.ErrorContains(t, err, "precharge result unavailable")
	unknown := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusQueued, unknown.Status)
	require.Equal(t, MediaBillingStatusPending, unknown.BillingStatus)
	require.NotNil(t, unknown.LeaseUntil)
	require.True(t, unknown.LeaseUntil.After(time.Now()))
	require.Equal(t, 1, port.prechargeMutationCalls())
	require.Equal(t, -2.0, port.balanceAmount())

	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
	fixture.repo.mu.Unlock()
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	cleaned := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, cleaned.Status)
	require.Equal(t, MediaBillingStatusSettled, cleaned.BillingStatus)
	require.Equal(t, 2.0, cleaned.RefundedAmount)
	require.Equal(t, 1, port.prechargeMutationCalls())
	require.Zero(t, port.balanceAmount())
}

func TestMediaWorkerRecoverOnceTerminatesDeterministicPrechargeRejectionWithoutRefund(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
	snapshot := MediaBillingSnapshot{RequestedModel: "fake-image", EstimatedAmount: 2}
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.BillingStatus = MediaBillingStatusPending
	stored.PrechargedAmount = 0
	stored.BillingSnapshot = encoded
	stored.LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
	fixture.repo.mu.Unlock()

	port := newWorkerInitializationBillingPort()
	port.prechargeErr = errors.New("insufficient balance")
	fixture.worker.deps.Precharger = port
	fixture.worker.deps.Billing = NewMediaBillingCoordinator(fixture.repo, port)

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	failed := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, failed.Status)
	require.Equal(t, MediaTaskStageFailed, failed.Stage)
	require.Equal(t, "billing_precharge", failed.ErrorCode)
	require.Equal(t, MediaBillingStatusSettled, failed.BillingStatus)
	require.Zero(t, failed.PrechargedAmount)
	require.Nil(t, failed.LeaseUntil)
	require.Empty(t, failed.SettlementRecovery)
	require.Empty(t, failed.SettlementPlan)
	require.Equal(t, 0, port.prechargeMutationCalls())
	require.Zero(t, port.balanceAmount())
	require.Empty(t, fixture.queue.enqueuedTaskIDs())
}

func TestMediaWorkerRecoverOnceTerminatesInvalidLegacyPricingAndContinuesBatch(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
	invalidSnapshot, err := json.Marshal(MediaBillingSnapshot{RequestedModel: "fake-image", EstimatedAmount: -1})
	require.NoError(t, err)
	settlementPlan := json.RawMessage(`{"type":"success","usage":{"image_count":1}}`)

	fixture.repo.mu.Lock()
	invalid := fixture.repo.tasks[fixture.task.ID]
	invalid.BillingStatus = MediaBillingStatusPending
	invalid.PrechargedAmount = 0
	invalid.BillingSnapshot = invalidSnapshot
	invalid.LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
	execution := cloneWorkerTask(fixture.task)
	execution.ID = 2
	execution.PublicID = "valid-execution-recovery"
	execution.Status = MediaTaskStatusInProgress
	execution.Stage = MediaTaskStagePolling
	execution.BillingStatus = MediaBillingStatusPrecharged
	execution.LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
	settlement := workerCompletedTask(3, "valid-settlement-recovery")
	settlement.BillingStatus = MediaBillingStatusRetry
	settlement.SettlementPlan = settlementPlan
	fixture.repo.tasks[execution.ID] = execution
	fixture.repo.tasks[settlement.ID] = settlement
	fixture.repo.mu.Unlock()

	port := newWorkerInitializationBillingPort()
	fixture.worker.deps.Precharger = port
	fixture.worker.deps.Billing = NewMediaBillingCoordinator(fixture.repo, port)

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	failed := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, failed.Status)
	require.Equal(t, MediaTaskStageFailed, failed.Stage)
	require.Equal(t, "billing_precharge", failed.ErrorCode)
	require.Equal(t, MediaBillingStatusSettled, failed.BillingStatus)
	require.Zero(t, failed.PrechargedAmount)
	require.Nil(t, failed.LeaseUntil)
	require.Empty(t, failed.SettlementRecovery)
	require.Empty(t, failed.SettlementPlan)
	require.Zero(t, port.prechargeCallsCount())
	require.Zero(t, port.prechargeMutationCalls())
	require.Zero(t, port.settleFailureCallsCount())
	require.Zero(t, port.balanceAmount())
	require.ElementsMatch(t, []int64{execution.ID, settlement.ID}, fixture.queue.enqueuedTaskIDs())
}

func TestMediaWorkerAcksResidualMessageForDeterministicPrechargeRejectionWithoutSettlement(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncOptional)
	snapshot := MediaBillingSnapshot{RequestedModel: "fake-image", EstimatedAmount: 2}
	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.BillingStatus = MediaBillingStatusPending
	stored.PrechargedAmount = 0
	stored.BillingSnapshot = encoded
	stored.LeaseUntil = mediaTimePointer(time.Now().Add(-time.Minute))
	fixture.repo.mu.Unlock()

	port := newWorkerInitializationBillingPort()
	port.prechargeErr = errors.New("insufficient balance")
	fixture.worker.deps.Precharger = port

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	err = fixture.worker.processMessage(context.Background(), &MediaQueueMessage{
		ID: "residual-precharge-rejection", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync,
	})
	require.NoError(t, err)
	failed := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, failed.Status)
	require.Equal(t, "billing_precharge", failed.ErrorCode)
	require.Equal(t, MediaBillingStatusSettled, failed.BillingStatus)
	require.Equal(t, int64(1), fixture.queue.ackCalls.Load())
	require.Equal(t, 0, fixture.billing.settlementCalls())
	require.Zero(t, fixture.adapter.syncCalls.Load())
	require.Zero(t, fixture.adapter.submitCalls.Load())
}

func TestMediaWorkerRecoveryPreservesSynchronousPriority(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.repo.setExpiredLease(fixture.task.ID, "dead-worker")
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []MediaQueuePriority{MediaQueuePrioritySync}, fixture.queue.enqueuedPriorities())

	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].SyncFallback = true
	fixture.repo.mu.Unlock()
	fixture.queue.resetEnqueued()
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []MediaQueuePriority{MediaQueuePriorityAsync}, fixture.queue.enqueuedPriorities())
}

func TestMediaWorkerRecoveryRequeuesSettlementPendingAtNormalPriority(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.mu.Lock()
	task := fixture.repo.tasks[fixture.task.ID]
	task.Status = MediaTaskStatusCompleted
	task.Stage = MediaTaskStageCompleted
	task.LeaseUntil = nil
	task.SettlementPlan = json.RawMessage(`{"type":"success","usage":{"image_count":1}}`)
	task.BillingStatus = MediaBillingStatusRetry
	fixture.repo.mu.Unlock()

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
	require.Equal(t, []MediaQueuePriority{MediaQueuePriorityAsync}, fixture.queue.enqueuedPriorities())
}

func TestMediaWorkerRecoveryStageMatrix(t *testing.T) {
	fixedAccountID := int64(7)
	tests := []struct {
		name string
		task MediaTask
		want mediaTaskRecoveryMode
	}{
		{name: "queued task", task: MediaTask{Status: MediaTaskStatusQueued, Stage: MediaTaskStageQueued}, want: mediaTaskRecoveryReschedule},
		{name: "claimed queued stage", task: MediaTask{Status: MediaTaskStatusInProgress, Stage: MediaTaskStageQueued}, want: mediaTaskRecoveryReschedule},
		{name: "scheduling", task: MediaTask{Status: MediaTaskStatusInProgress, Stage: MediaTaskStageScheduling}, want: mediaTaskRecoveryReschedule},
		{name: "submitting before fixed fields", task: MediaTask{Status: MediaTaskStatusInProgress, Stage: MediaTaskStageSubmitting}, want: mediaTaskRecoveryReschedule},
		{name: "unknown fixed submission", task: MediaTask{
			Status: MediaTaskStatusInProgress, Stage: MediaTaskStageSubmitting, AccountID: &fixedAccountID,
			Adapter: "fixed", UpstreamModel: "fixed-model",
		}, want: mediaTaskRecoveryUnknownSubmission},
		{name: "unknown generating result", task: MediaTask{Status: MediaTaskStatusInProgress, Stage: MediaTaskStageGenerating}, want: mediaTaskRecoveryUnknownResult},
		{name: "unknown storing result", task: MediaTask{Status: MediaTaskStatusInProgress, Stage: MediaTaskStageStoring}, want: mediaTaskRecoveryUnknownResult},
		{name: "unknown settling result", task: MediaTask{Status: MediaTaskStatusInProgress, Stage: MediaTaskStageSettling}, want: mediaTaskRecoveryUnknownResult},
		{name: "existing upstream wins", task: MediaTask{
			Status: MediaTaskStatusInProgress, Stage: MediaTaskStageGenerating, UpstreamTaskID: "upstream-1",
		}, want: mediaTaskRecoveryExistingUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, classifyMediaTaskRecovery(&tt.task))
		})
	}
}

func TestMediaWorkerFailsRecoveredGeneratingWithoutReexecution(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	prepareRecoverableSyncStage(fixture, MediaTaskStageGenerating)
	fixture.queue.deliver(&MediaQueueMessage{ID: "recover-generating", TaskID: fixture.task.ID, Priority: MediaQueuePrioritySync})

	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	require.Eventually(t, func() bool {
		return fixture.repo.mustGet(fixture.task.ID).Status.IsTerminal()
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fixture.queue.ackCalls.Load() == 1 }, time.Second, 10*time.Millisecond)

	require.Zero(t, fixture.selector.selectCalls.Load())
	require.Zero(t, fixture.adapter.syncCalls.Load())
	require.Zero(t, fixture.artifactWriter.calls.Load())
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "upstream_generate_failed", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: "upstream_generate_failed",
	}, fixture.billing.lastFailure())
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
	require.NotEmpty(t, stored.SettlementRecovery)
	require.NotEmpty(t, stored.SettlementPlan)
	require.True(t, mediaSettlementPlansEqual(stored.SettlementRecovery, stored.SettlementPlan))
}

func TestMediaWorkerFailsRecoveredStoringWithoutReexecution(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	prepareRecoverableSyncStage(fixture, MediaTaskStageStoring)
	existing := []MediaArtifact{{
		TaskID: fixture.task.ID, Direction: "output", Position: 0, MediaType: MediaTypeImage,
		ContentType: "image/png", SizeBytes: 123, StorageStatus: "stored",
	}}
	fixture.artifactWriter.outputs = map[int64][]MediaArtifact{fixture.task.ID: append([]MediaArtifact(nil), existing...)}
	fixture.queue.deliver(&MediaQueueMessage{ID: "recover-storing", TaskID: fixture.task.ID, Priority: MediaQueuePrioritySync})

	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	require.Eventually(t, func() bool {
		return fixture.repo.mustGet(fixture.task.ID).Status.IsTerminal()
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fixture.queue.ackCalls.Load() == 1 }, time.Second, 10*time.Millisecond)

	require.Zero(t, fixture.selector.selectCalls.Load())
	require.Zero(t, fixture.adapter.syncCalls.Load())
	require.Zero(t, fixture.artifactWriter.calls.Load())
	fixture.artifactWriter.mu.Lock()
	storedOutputs := append([]MediaArtifact(nil), fixture.artifactWriter.outputs[fixture.task.ID]...)
	fixture.artifactWriter.mu.Unlock()
	require.Equal(t, existing, storedOutputs)
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "upstream_generate_failed", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: "upstream_generate_failed",
	}, fixture.billing.lastFailure())
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
	require.NotEmpty(t, stored.SettlementRecovery)
	require.NotEmpty(t, stored.SettlementPlan)
	require.True(t, mediaSettlementPlansEqual(stored.SettlementRecovery, stored.SettlementPlan))
}

func TestMediaWorkerStorageFailureAlwaysRefunds(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.artifactWriter.err = errors.New("object storage unavailable")
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_storage", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	}, fixture.billing.lastFailure())
	require.Equal(t, int64(1), fixture.metrics.StorageFailures())
}

func TestMediaWorkerPrivateImageObjectWithoutDeliveryPathFailsAndRefunds(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	artifacts := &mediaContentArtifactRepoStub{}
	fixture.worker.deps.Artifacts = NewMediaContentService(
		nil, artifacts, mediaContentSettingsStub{settings: &SystemSettings{}}, nil, nil,
		mediaContentHTTPReaderStub{}, mediaContentObjectStoreStub{put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{ObjectKey: "private/images/output.png"}, nil
		}},
	)

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_storage", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
	}, fixture.billing.lastFailure())
	require.Empty(t, artifacts.items)
}

func TestMediaWorkerUpstreamCanceledAlwaysRefunds(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.setPollResult(MediaPollResult{State: MediaPollStateCanceled})
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: "upstream_canceled",
	}, fixture.billing.lastFailure())
}

func TestMediaWorkerRenewsLeaseWhilePolling(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 10 * time.Millisecond
	fixture.adapter.blockPollUntil(fixture.releasePoll)
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	require.Eventually(t, func() bool { return fixture.repo.renewLeaseCalls.Load() >= 1 }, time.Second, 10*time.Millisecond)
	close(fixture.releasePoll)
	require.NoError(t, <-done)
}

func TestMediaWorkerRefreshesStableAccountSlotWhilePollingAndStopsBeforeRelease(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	fixture.adapter.blockPollUntil(fixture.releasePoll)
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()

	require.Eventually(t, func() bool {
		return fixture.selector.refreshCalls.Load() >= 3
	}, time.Second, time.Millisecond)
	close(fixture.releasePoll)
	require.NoError(t, <-done)
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.False(t, fixture.selector.releaseWhileRefreshing.Load())
	refreshCalls := fixture.selector.refreshCalls.Load()
	time.Sleep(4 * fixture.worker.cfg.LeaseRenewInterval)
	require.Equal(t, refreshCalls, fixture.selector.refreshCalls.Load())
}

func TestMediaWorkerReleasesStableAccountSlotBeforeTerminalSettlement(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	persistRelease := make(chan struct{})
	fixture.artifactWriter.block = persistRelease
	blocking := &blockingSettlementCoordinator{
		inner: fixture.billing, entered: make(chan struct{}), release: make(chan struct{}),
	}
	fixture.worker.deps.Billing = blocking
	t.Cleanup(func() {
		select {
		case <-persistRelease:
		default:
			close(persistRelease)
		}
		select {
		case <-blocking.release:
		default:
			close(blocking.release)
		}
	})
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()

	require.Eventually(t, func() bool {
		return fixture.selector.refreshCalls.Load() >= 2
	}, time.Second, time.Millisecond)
	close(persistRelease)
	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("expected terminal settlement to start")
	}
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.False(t, fixture.selector.releaseWhileRefreshing.Load())
	refreshCalls := fixture.selector.refreshCalls.Load()
	time.Sleep(4 * fixture.worker.cfg.LeaseRenewInterval)
	require.Equal(t, refreshCalls, fixture.selector.refreshCalls.Load())
	close(blocking.release)
	require.NoError(t, <-done)
}

func TestMediaWorkerRefreshesStableAccountSlotAcquiredAfterWait(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	fixture.selector.configureWait(waitEntered, releaseWait)
	fixture.adapter.blockPollUntil(fixture.releasePoll)
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()

	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("expected stable media account slot wait")
	}
	close(releaseWait)
	require.Eventually(t, func() bool {
		return fixture.selector.refreshCalls.Load() >= 2
	}, time.Second, time.Millisecond)
	close(fixture.releasePoll)
	require.NoError(t, <-done)
	require.Equal(t, int64(1), fixture.selector.waitCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
}

func TestMediaWorkerSlotOwnershipLossCancelsPollingWithoutTerminalStateOrAck(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	fixture.selector.configureRefresh(false, nil, "")
	fixture.adapter.blockPollUntil(make(chan struct{}))

	err := fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID})
	require.ErrorIs(t, err, ErrMediaAccountSlotLost)
	require.GreaterOrEqual(t, fixture.selector.refreshCalls.Load(), int64(1))
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
	require.Zero(t, fixture.queue.publishCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
}

func TestMediaWorkerSlotRefreshErrorIsReportedWithoutLeakingSensitiveDetails(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	var output bytes.Buffer
	fixture.worker.logger = slog.New(slog.NewJSONHandler(&output, nil))
	sensitive := "redis refresh failed token=slot-secret"
	fixture.selector.configureRefresh(false, errors.New(sensitive), "")
	fixture.adapter.blockPollUntil(make(chan struct{}))
	fixture.queue.deliver(&MediaQueueMessage{ID: "slot-lost", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync})

	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	require.Eventually(t, func() bool {
		return fixture.selector.releaseCalls.Load() == 1
	}, time.Second, time.Millisecond)
	fixture.worker.Stop()
	require.Zero(t, fixture.queue.ackCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
	require.Contains(t, output.String(), `"error_code":"slot_lost"`)
	require.NotContains(t, output.String(), sensitive)

	var reported error
	require.Eventually(t, func() bool {
		select {
		case reported = <-fixture.worker.Errors():
			return errors.Is(reported, ErrMediaAccountSlotLost) && strings.Contains(reported.Error(), sensitive)
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}

func TestMediaWorkerSlotRefreshPanicCancelsExecutionAndReleasesAfterHeartbeatStops(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	fixture.selector.configureRefresh(false, nil, "refresh panic secret")
	fixture.adapter.blockPollUntil(make(chan struct{}))

	err := fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaAccountSlotLost)
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.False(t, fixture.selector.releaseWhileRefreshing.Load())
	refreshCalls := fixture.selector.refreshCalls.Load()
	time.Sleep(4 * fixture.worker.cfg.LeaseRenewInterval)
	require.Equal(t, refreshCalls, fixture.selector.refreshCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
}

func TestMediaWorkerCancelsExecutionWhenLeaseIsLostBeforeTerminalTransition(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 10 * time.Millisecond
	fixture.repo.rejectRenewLease.Store(true)
	fixture.adapter.blockPollUntil(make(chan struct{}))

	err := fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaWorkerLeaseLost)
	require.GreaterOrEqual(t, fixture.repo.renewLeaseCalls.Load(), int64(1))
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
	require.Zero(t, fixture.queue.publishCalls.Load())
}

func TestMediaWorkerStopsLeaseRenewalBeforeTerminalSettlement(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.worker.cfg.LeaseRenewInterval = 10 * time.Millisecond
	blocking := &blockingSettlementCoordinator{
		inner:   fixture.billing,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	fixture.worker.deps.Billing = blocking
	t.Cleanup(func() {
		select {
		case <-blocking.release:
		default:
			close(blocking.release)
		}
	})

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("expected terminal settlement to start")
	}
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
	renewCalls := fixture.repo.renewLeaseCalls.Load()
	require.Never(t, func() bool {
		return fixture.repo.renewLeaseCalls.Load() > renewCalls
	}, 5*fixture.worker.cfg.LeaseRenewInterval, time.Millisecond)
	require.False(t, blocking.ctxCanceled.Load())

	close(blocking.release)
	require.NoError(t, <-done)
	require.False(t, blocking.ctxCanceled.Load())
	require.Equal(t, int64(1), fixture.queue.publishCalls.Load())
	require.Equal(t, MediaBillingStatusSettled, fixture.repo.mustGet(fixture.task.ID).BillingStatus)
}

func TestMediaWorkerTerminalTransitionWinsRaceWithLeaseRenewer(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	applied := make(chan struct{})
	releaseTransition := make(chan struct{})
	fixture.repo.terminalTransitionApplied = applied
	fixture.repo.releaseTerminalTransition = releaseTransition
	observing := &contextObservingSettlementCoordinator{inner: fixture.billing}
	fixture.worker.deps.Billing = observing

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("expected terminal transition to be applied")
	}
	renewCalls := fixture.repo.renewLeaseCalls.Load()
	require.Eventually(t, func() bool {
		return fixture.repo.renewLeaseCalls.Load() > renewCalls
	}, time.Second, time.Millisecond)
	close(releaseTransition)

	require.NoError(t, <-done)
	require.False(t, observing.ctxCanceled.Load())
	require.Equal(t, MediaBillingStatusSettled, fixture.repo.mustGet(fixture.task.ID).BillingStatus)
}

func TestMediaWorkerResumesExistingUpstreamTaskWithoutSubmit(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.mu.Lock()
	task := fixture.repo.tasks[fixture.task.ID]
	task.Status = MediaTaskStatusInProgress
	task.Stage = MediaTaskStagePolling
	task.AccountID = workerInt64Ptr(fixture.account.ID)
	task.Adapter = fixture.adapter.Name()
	task.UpstreamModel = "upstream-image"
	task.NativeAsyncMode = NativeAsyncRequired
	task.UpstreamTaskID = "existing-upstream"
	task.PollMetadata = json.RawMessage(`{"cursor":1}`)
	task.WorkerID = "dead-worker"
	expired := time.Now().Add(-time.Minute)
	task.LeaseUntil = &expired
	task.Version++
	fixture.repo.mu.Unlock()
	fixture.adapter.allowUpstream("existing-upstream")

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(1), fixture.selector.selectCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Equal(t, fixture.task.PublicID, fixture.selector.lastSessionKey())
	require.Equal(t, MediaTaskSlotID(fixture.task.ID), fixture.selector.lastStableSlotID())
	require.Equal(t, []int64{fixture.account.ID}, fixture.selector.lastCandidateAccountIDs())
	require.Zero(t, fixture.adapter.submitCalls.Load())
	require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerFixedRecoveryIgnoresRealtimeEligibilityAndPollsOriginalAccount(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Account)
	}{
		{name: "unschedulable", mutate: func(account *Account) { account.Schedulable = false }},
		{name: "disabled", mutate: func(account *Account) { account.Status = StatusDisabled }},
		{name: "cooling", mutate: func(account *Account) {
			until := time.Now().Add(time.Minute)
			account.TempUnschedulableUntil = &until
		}},
		{name: "rate limited", mutate: func(account *Account) {
			until := time.Now().Add(time.Minute)
			account.RateLimitResetAt = &until
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			prepareRecoverableExistingUpstream(fixture, "existing-upstream")
			tt.mutate(fixture.account)
			concurrency := &accountCandidateConcurrencyStub{}
			fixture.worker.deps.Scheduler.selector = NewAccountCandidateSelector(
				concurrency, nil, task12SchedulingConfig(),
			)

			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			stored := fixture.repo.mustGet(fixture.task.ID)
			require.Equal(t, MediaTaskStatusCompleted, stored.Status)
			require.Equal(t, fixture.account.ID, *stored.AccountID)
			require.Zero(t, fixture.adapter.submitCalls.Load())
			require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
			acquired, _, _, _ := concurrency.snapshot()
			require.Equal(t, []int64{fixture.account.ID}, acquired)
		})
	}
}

func TestMediaWorkerSameWorkerIDReclaimFencesOldExecutionStageWrites(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	prepareRecoverableExistingUpstream(fixture, "existing-upstream")
	pollRelease := make(chan struct{})
	var releasePoll sync.Once
	t.Cleanup(func() { releasePoll.Do(func() { close(pollRelease) }) })
	fixture.adapter.blockPollUntil(pollRelease)
	fixture.worker.cfg.LeaseRenewInterval = time.Hour

	oldDone := make(chan error, 1)
	go func() { oldDone <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	require.Eventually(t, func() bool {
		return fixture.adapter.pollCalls.Load() >= 1
	}, time.Second, time.Millisecond)
	oldClaim := fixture.repo.mustGet(fixture.task.ID)
	require.NotEmpty(t, oldClaim.ClaimToken)

	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].LeaseUntil = workerTimePtr(time.Now().Add(-time.Minute))
	fixture.repo.mu.Unlock()

	replacement := NewMediaWorker(fixture.worker.cfg, fixture.worker.deps)
	replacement.workerID = fixture.worker.workerID
	replacement.cfg.LeaseRenewInterval = time.Hour
	newDone := make(chan error, 1)
	go func() { newDone <- replacement.ProcessOne(context.Background(), fixture.task.ID) }()
	require.Eventually(t, func() bool {
		return fixture.adapter.pollCalls.Load() >= 2
	}, time.Second, time.Millisecond)
	newClaim := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, oldClaim.WorkerID, newClaim.WorkerID)
	require.NotEqual(t, oldClaim.ClaimToken, newClaim.ClaimToken)

	staleWrite, err := fixture.repo.UpdateClaimed(
		context.Background(), fixture.task.ID, oldClaim.ClaimToken,
		newClaim.Version, newClaim.Stage, map[string]any{"stage": MediaTaskStageStoring},
	)
	require.NoError(t, err)
	require.False(t, staleWrite)
	require.Equal(t, MediaTaskStagePolling, fixture.repo.mustGet(fixture.task.ID).Stage)

	releasePoll.Do(func() { close(pollRelease) })
	require.ErrorIs(t, <-oldDone, ErrMediaWorkerLeaseLost)
	require.NoError(t, <-newDone)
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Equal(t, int64(1), fixture.artifactWriter.calls.Load())
	require.Equal(t, 1, fixture.billing.settlementCalls())
}

func TestMediaWorkerSameInstanceReclaimReplacesActiveGeneration(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	prepareRecoverableExistingUpstream(fixture, "existing-upstream")
	pollRelease := make(chan struct{})
	var releasePoll sync.Once
	t.Cleanup(func() { releasePoll.Do(func() { close(pollRelease) }) })
	fixture.adapter.blockPollUntil(pollRelease)
	fixture.worker.cfg.LeaseRenewInterval = time.Hour

	oldDone := make(chan error, 1)
	go func() { oldDone <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	require.Eventually(t, func() bool { return fixture.adapter.pollCalls.Load() >= 1 }, time.Second, time.Millisecond)
	oldActive := fixture.worker.activeForTask(fixture.task.ID)
	require.NotNil(t, oldActive)
	oldClaim := fixture.repo.mustGet(fixture.task.ID).ClaimToken
	require.NotEmpty(t, oldClaim)

	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].LeaseUntil = workerTimePtr(time.Now().Add(-time.Minute))
	fixture.repo.mu.Unlock()

	newDone := make(chan error, 1)
	go func() {
		newDone <- fixture.worker.processMessage(context.Background(), &MediaQueueMessage{
			ID: "same-instance-reclaim", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync,
		})
	}()
	require.Eventually(t, func() bool {
		return fixture.repo.mustGet(fixture.task.ID).ClaimToken != oldClaim
	}, time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return fixture.adapter.pollCalls.Load() >= 2 }, time.Second, time.Millisecond)

	newActive := fixture.worker.activeForTask(fixture.task.ID)
	require.NotNil(t, newActive)
	require.NotSame(t, oldActive, newActive)
	require.Zero(t, fixture.queue.ackCalls.Load(), "newly claimed message must remain pending while its execution is active")
	require.ErrorIs(t, <-oldDone, ErrMediaWorkerLeaseLost)
	require.Same(t, newActive, fixture.worker.activeForTask(fixture.task.ID), "old defer must not unregister the new generation")

	releasePoll.Do(func() { close(pollRelease) })
	require.NoError(t, <-newDone)
	require.Equal(t, int64(1), fixture.queue.ackCalls.Load())
	require.Nil(t, fixture.worker.activeForTask(fixture.task.ID))
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Equal(t, int64(1), fixture.artifactWriter.calls.Load())
	require.Equal(t, 1, fixture.billing.settlementCalls())
}

func TestMediaWorkerRecoveryDoesNotDependOnCurrentModelOrHistoricalRequestSpec(t *testing.T) {
	tests := []struct {
		name       string
		mutateTask func(*MediaTask)
		mutateDeps func(*MediaWorker)
	}{
		{
			name: "deleted_or_disabled_model",
			mutateDeps: func(worker *MediaWorker) {
				worker.deps.Models = NewMediaModelRegistry(&workerModelRepository{})
			},
		},
		{
			name: "historical_request_schema_is_no_longer_parseable",
			mutateTask: func(task *MediaTask) {
				task.RequestSpec = json.RawMessage(`{"legacy_shape":true}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			fixture.repo.mu.Lock()
			task := fixture.repo.tasks[fixture.task.ID]
			task.Status = MediaTaskStatusInProgress
			task.Stage = MediaTaskStagePolling
			task.AccountID = workerInt64Ptr(fixture.account.ID)
			task.Adapter = fixture.adapter.Name()
			task.UpstreamModel = "upstream-image"
			task.NativeAsyncMode = NativeAsyncRequired
			task.UpstreamTaskID = "existing-upstream"
			task.PollMetadata = json.RawMessage(`{"cursor":1}`)
			task.WorkerID = "dead-worker"
			task.LeaseUntil = workerTimePtr(time.Now().Add(-time.Minute))
			task.Version++
			if tt.mutateTask != nil {
				tt.mutateTask(task)
			}
			fixture.repo.mu.Unlock()
			if tt.mutateDeps != nil {
				tt.mutateDeps(fixture.worker)
			}
			fixture.adapter.allowUpstream("existing-upstream")

			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Zero(t, fixture.adapter.submitCalls.Load())
			require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
		})
	}
}

func TestMediaWorkerUsesCandidateSnapshotInsteadOfCurrentModelMapping(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.account.Extra = map[string]any{"media": map[string]any{
		"adapter":         "changed-adapter",
		"model_overrides": map[string]any{"fake-image": map[string]any{"upstream_model": "changed-model"}},
	}}

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	request := fixture.adapter.lastRequest()
	require.Equal(t, "upstream-image", request.UpstreamModel)
	require.Equal(t, fixture.adapter.Name(), fixture.repo.mustGet(fixture.task.ID).Adapter)
}

func TestMediaWorkerTaskTimeoutIsSystemFailure(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.TaskTimeout = 10 * time.Millisecond
	fixture.adapter.blockPollUntil(make(chan struct{}))

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_timeout", stored.ErrorCode)
	require.Equal(t, MediaFailureKindSystem, fixture.billing.lastFailure().Kind)
}

func TestMediaWorkerTaskTimeoutDuringStorageIsSystemTimeout(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.worker.cfg.TaskTimeout = 10 * time.Millisecond
	fixture.artifactWriter.block = make(chan struct{})

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_timeout", stored.ErrorCode)
	require.Equal(t, MediaFailureKindSystem, fixture.billing.lastFailure().Kind)
}

func TestMediaWorkerShutdownCancellationDoesNotMarkInFlightTaskFailed(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.artifactWriter.block = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(ctx, fixture.task.ID) }()
	require.Eventually(t, func() bool { return fixture.artifactWriter.calls.Load() >= 1 }, time.Second, 10*time.Millisecond)
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
}

func TestMediaWorkerSubmissionUnknownWithoutIdempotencyFailsWithoutResubmit(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.submitErr = &MediaAdapterError{
		Code: "submission_unknown", Message: "connection closed", Retryable: true, SubmissionUnknown: true,
	}

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Equal(t, MediaTaskStatusFailed, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Equal(t, float64(1), fixture.billing.lastFailure().RefundRatio)
}

func TestMediaWorkerSubmissionUnknownWithIdempotencyRetriesSameSingleCandidate(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.supportsIdempotency = true
	fixture.adapter.submitErrors = []error{&MediaAdapterError{
		Code: "submission_unknown", Message: "connection closed", Retryable: true, SubmissionUnknown: true,
	}}

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(2), fixture.adapter.submitCalls.Load())
	requests := fixture.adapter.submitRequests()
	require.Len(t, requests, 2)
	require.Equal(t, fixture.account.ID, requests[0].Account.ID)
	require.Equal(t, fixture.account.ID, requests[1].Account.ID)
	require.Equal(t, fixture.task.PublicID, requests[0].IdempotencyKey)
	require.Equal(t, requests[0].IdempotencyKey, requests[1].IdempotencyKey)
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerSubmissionUnknownWithIdempotencyNeverSwitchesCandidate(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.supportsIdempotency = true
	fixture.adapter.submitErrors = []error{&MediaAdapterError{
		Code: "submission_unknown", Message: "connection closed", Retryable: true, SubmissionUnknown: true,
	}}
	fallback := newWorkerAdapter("worker-fallback-unknown")
	fixture.worker.deps.Adapters.Register(fallback.Name(), fallback)
	fallbackAccount := &Account{ID: 9, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}
	accountRepo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)
	accountRepo.extra = append(accountRepo.extra, fallbackAccount)
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(fixture.task.CandidateSnapshot, &candidates))
	candidates = append(candidates, MediaAccountCandidateSnapshot{
		AccountID: fallbackAccount.ID, Platform: fallbackAccount.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: fallback.Name(), UpstreamModel: "upstream-fallback", NativeAsyncMode: NativeAsyncRequired,
		},
	})
	encoded, err := json.Marshal(candidates)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].CandidateSnapshot = encoded
	fixture.repo.mu.Unlock()

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(2), fixture.adapter.submitCalls.Load())
	require.Zero(t, fallback.submitCalls.Load())
	for _, request := range fixture.adapter.submitRequests() {
		require.Equal(t, fixture.account.ID, request.Account.ID)
		require.Equal(t, fixture.task.PublicID, request.IdempotencyKey)
	}
}

func TestMediaWorkerRecoversUnknownSubmissionOnFixedIdempotentAdapter(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixedAdapter, fixedAccount := prepareRecoverableSubmittingTask(t, fixture, true)

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))

	require.Equal(t, int64(1), fixture.selector.selectCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Equal(t, fixture.task.PublicID, fixture.selector.lastSessionKey())
	require.Equal(t, MediaTaskSlotID(fixture.task.ID), fixture.selector.lastStableSlotID())
	require.Equal(t, []int64{fixedAccount.ID}, fixture.selector.lastCandidateAccountIDs())
	require.Zero(t, fixture.adapter.submitCalls.Load())
	require.Equal(t, int64(1), fixedAdapter.submitCalls.Load())
	requests := fixedAdapter.submitRequests()
	require.Len(t, requests, 1)
	require.Equal(t, fixedAccount.ID, requests[0].Account.ID)
	require.Equal(t, "fixed-upstream-image", requests[0].UpstreamModel)
	require.Equal(t, fixture.task.PublicID, requests[0].IdempotencyKey)
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusCompleted, stored.Status)
	require.Equal(t, fixedAccount.ID, *stored.AccountID)
	require.Equal(t, fixedAdapter.Name(), stored.Adapter)
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
}

func TestMediaWorkerUnknownSubmissionWaitsForFixedAccountSlot(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixedAdapter, fixedAccount := prepareRecoverableSubmittingTask(t, fixture, true)
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	fixture.selector.configureWait(waitEntered, releaseWait)

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("expected fixed account slot wait")
	}
	require.Zero(t, fixedAdapter.submitCalls.Load())
	close(releaseWait)

	require.NoError(t, <-done)
	require.Equal(t, int64(1), fixture.selector.selectCalls.Load())
	require.Equal(t, int64(1), fixture.selector.waitCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Equal(t, []int64{fixedAccount.ID}, fixture.selector.lastCandidateAccountIDs())
	require.Equal(t, int64(1), fixedAdapter.submitCalls.Load())
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerUnknownSubmissionWaitCancellationDoesNotSubmitOrAck(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixedAdapter, fixedAccount := prepareRecoverableSubmittingTask(t, fixture, true)
	waitEntered := make(chan struct{})
	fixture.selector.configureWait(waitEntered, make(chan struct{}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(ctx, fixture.task.ID) }()
	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("expected fixed account slot wait")
	}
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, int64(1), fixture.selector.selectCalls.Load())
	require.Equal(t, int64(1), fixture.selector.waitCalls.Load())
	require.Zero(t, fixture.selector.releaseCalls.Load())
	require.Equal(t, []int64{fixedAccount.ID}, fixture.selector.lastCandidateAccountIDs())
	require.Zero(t, fixedAdapter.submitCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerUnknownSubmissionRejectsFixedSelectorAccountDrift(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixedAdapter, _ := prepareRecoverableSubmittingTask(t, fixture, true)
	fixture.selector.configureDrift(99)

	require.NoError(t, fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID}))
	require.Equal(t, int64(1), fixture.selector.selectCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Zero(t, fixedAdapter.submitCalls.Load())
	require.Equal(t, int64(1), fixture.queue.ackCalls.Load())
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_scheduler", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_scheduler",
	}, fixture.billing.lastFailure())
}

func TestMediaWorkerUnknownSubmissionReleasesFixedSlotWhenAdapterPanics(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixedAdapter, fixedAccount := prepareRecoverableSubmittingTask(t, fixture, true)
	fixedAdapter.panicSubmit = true

	require.Panics(t, func() {
		_ = fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	})
	require.Equal(t, int64(1), fixture.selector.selectCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Equal(t, []int64{fixedAccount.ID}, fixture.selector.lastCandidateAccountIDs())
	require.Equal(t, int64(1), fixedAdapter.submitCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
}

func TestMediaWorkerFailsUnknownSubmissionWithoutIdempotentResubmit(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixedAdapter, fixedAccount := prepareRecoverableSubmittingTask(t, fixture, false)

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))

	require.Zero(t, fixture.selector.selectCalls.Load())
	require.Zero(t, fixture.adapter.submitCalls.Load())
	require.Zero(t, fixedAdapter.submitCalls.Load())
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, fixedAccount.ID, *stored.AccountID)
	require.Equal(t, fixedAdapter.Name(), stored.Adapter)
	require.Equal(t, "upstream_submit_failed", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: "upstream_submit_failed",
	}, fixture.billing.lastFailure())
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
}

func TestMediaWorkerDoesNotMarkAccountUsedForInvalidAsyncSubmission(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.emptySubmission = true
	accountRepo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Zero(t, accountRepo.markUsed.Load())
	require.Equal(t, MediaTaskStatusFailed, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerSuccessfulTerminalPlanPersistenceFailureIsRedeliverable(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	port := &recordingMediaBillingPort{}
	fixture.worker.deps.Billing = NewMediaBillingCoordinator(fixture.repo, port)
	fixture.repo.failSettlementPlanWrites = 1
	fixture.queue.deliver(&MediaQueueMessage{ID: "plan-failure-1", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync})

	require.NoError(t, fixture.worker.Start())
	require.Eventually(t, func() bool {
		return fixture.repo.mustGet(fixture.task.ID).Status == MediaTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)
	select {
	case workerErr := <-fixture.worker.Errors():
		require.ErrorIs(t, workerErr, ErrMediaSettlementPlanNotPersisted)
	case <-time.After(time.Second):
		t.Fatal("expected unsafe settlement persistence error")
	}
	fixture.worker.Stop()
	require.Zero(t, fixture.queue.ackCalls.Load())
	first := fixture.repo.mustGet(fixture.task.ID)
	require.NotEmpty(t, first.SettlementRecovery)
	require.Empty(t, first.SettlementPlan)
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Zero(t, port.successfulSettlementCalls())

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	second := fixture.repo.mustGet(fixture.task.ID)
	require.NotEmpty(t, second.SettlementPlan)
	require.Equal(t, MediaBillingStatusSettled, second.BillingStatus)
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Equal(t, 1, port.successfulSettlementCalls())
}

func TestMediaWorkerFailedTerminalPlanPersistenceFailureRebuildsOriginalPlan(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.artifactWriter.err = errors.New("storage unavailable")
	port := &recordingMediaBillingPort{}
	fixture.worker.deps.Billing = NewMediaBillingCoordinator(fixture.repo, port)
	fixture.repo.failSettlementPlanWrites = 1

	err := fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaSettlementPlanNotPersisted)
	first := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusFailed, first.Status)
	require.NotEmpty(t, first.SettlementRecovery)
	require.Empty(t, first.SettlementPlan)
	require.Equal(t, int64(1), fixture.adapter.syncCalls.Load())
	require.Zero(t, port.successfulSettlementCalls())

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	second := fixture.repo.mustGet(fixture.task.ID)
	require.NotEmpty(t, second.SettlementPlan)
	require.Equal(t, MediaBillingStatusSettled, second.BillingStatus)
	require.Equal(t, second.PrechargedAmount, second.RefundedAmount)
	require.Equal(t, int64(1), fixture.adapter.syncCalls.Load())
	require.Equal(t, 1, port.successfulSettlementCalls())
	var recovery MediaSettlementPlan
	require.NoError(t, json.Unmarshal(second.SettlementRecovery, &recovery))
	require.Equal(t, MediaSettlementTypeFailure, recovery.Type)
	require.Equal(t, "system_storage", recovery.Failure.ErrorCode)
}

func TestMediaWorkerRejectsSettledRecoveryAndFormalPlanDivergence(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	formal, err := json.Marshal(MediaSettlementPlan{
		Type:  MediaSettlementTypeSuccess,
		Usage: &MediaUsage{ImageCount: 1},
	})
	require.NoError(t, err)
	recovery, err := json.Marshal(MediaSettlementPlan{
		Type: MediaSettlementTypeFailure,
		Failure: &MediaFailureSettlement{
			Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_storage",
		},
	})
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.Status = MediaTaskStatusCompleted
	stored.BillingStatus = MediaBillingStatusSettled
	stored.SettlementPlan = formal
	stored.SettlementRecovery = recovery
	fixture.repo.mu.Unlock()

	err = fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaSettlementPlanConflict)
	require.Zero(t, fixture.adapter.submitCalls.Load())
}

func TestMediaWorkerRejectsSettledRecoveryWithoutFormalPlan(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	recovery, err := json.Marshal(MediaSettlementPlan{
		Type:  MediaSettlementTypeSuccess,
		Usage: &MediaUsage{ImageCount: 1},
	})
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.Status = MediaTaskStatusCompleted
	stored.Stage = MediaTaskStageCompleted
	stored.BillingStatus = MediaBillingStatusSettled
	stored.SettlementRecovery = recovery
	stored.SettlementPlan = nil
	fixture.repo.mu.Unlock()

	err = fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaSettlementPlanNotPersisted)
	require.Zero(t, fixture.adapter.submitCalls.Load())
}

func TestMediaWorkerAcceptsLegacySettledTaskWithoutSettlementPlans(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.Status = MediaTaskStatusCompleted
	stored.Stage = MediaTaskStageCompleted
	stored.BillingStatus = MediaBillingStatusSettled
	stored.SettlementRecovery = nil
	stored.SettlementPlan = nil
	fixture.repo.mu.Unlock()

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Zero(t, fixture.adapter.submitCalls.Load())
}

func TestMediaWorkerConsumerSurvivesAdapterPanicWithoutAcknowledgingFailedMessage(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	var output bytes.Buffer
	fixture.worker.logger = slog.New(slog.NewJSONHandler(&output, nil))
	sensitivePanic := "adapter submit panic https://upstream.example/task token=secret"
	fixture.adapter.panicSubmit = true
	fixture.adapter.panicMessage = sensitivePanic
	second := cloneWorkerTask(fixture.task)
	second.ID = 2
	second.PublicID = "worker-after-panic"
	second.Status = MediaTaskStatusQueued
	second.Stage = MediaTaskStageQueued
	second.WorkerID = ""
	second.LeaseUntil = nil
	second.StartedAt = nil
	second.SubmittedAt = nil
	second.FinishedAt = nil
	second.AccountID = nil
	second.Adapter = ""
	second.UpstreamModel = ""
	second.UpstreamTaskID = ""
	second.PollMetadata = nil
	second.SettlementPlan = nil
	second.SettlementRecovery = nil
	second.BillingStatus = MediaBillingStatusPrecharged
	second.Version = 1
	_, err := fixture.repo.Create(context.Background(), second)
	require.NoError(t, err)
	fixture.queue.deliver(&MediaQueueMessage{ID: "panic-1", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync})

	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	select {
	case workerErr := <-fixture.worker.Errors():
		require.Contains(t, workerErr.Error(), sensitivePanic)
	case <-time.After(time.Second):
		t.Fatal("expected adapter panic to reach worker error channel")
	}
	require.Eventually(t, func() bool {
		return fixture.selector.releaseCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	stored := fixture.repo.mustGet(fixture.task.ID)
	require.Equal(t, MediaTaskStatusInProgress, stored.Status)
	require.Equal(t, MediaTaskStageSubmitting, stored.Stage)
	require.Empty(t, stored.SettlementRecovery)
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())

	fixture.queue.deliver(&MediaQueueMessage{ID: "panic-2", TaskID: second.ID, Priority: MediaQueuePriorityAsync})
	require.Eventually(t, func() bool {
		return fixture.repo.mustGet(second.ID).Status == MediaTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fixture.queue.ackCalls.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(2), fixture.selector.releaseCalls.Load())
	require.Contains(t, output.String(), `"error_code":"worker_panic"`)
	require.NotContains(t, output.String(), "https://upstream.example")
	require.NotContains(t, output.String(), "token=secret")

	fixture.worker.Stop()
	require.Eventually(t, func() bool {
		current := fixture.repo.mustGet(fixture.task.ID)
		return current.LeaseUntil != nil && current.LeaseUntil.Before(time.Now())
	}, time.Second, 10*time.Millisecond)
	fixture.queue.resetEnqueued()
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
}

func TestMediaWorkerProductionLogsNeverIncludeRawDependencyErrors(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	var output bytes.Buffer
	fixture.worker.logger = slog.New(slog.NewJSONHandler(&output, nil))
	sensitive := errors.New("https://upstream.example/task token=secret upstream_reference=private-ref")
	trace := &mediaExecutionTrace{durations: map[MediaTaskStage]time.Duration{}}

	fixture.worker.logWorkerError(fixture.task, trace, "settlement_success", "settlement_retry", sensitive)
	fixture.worker.reportError(sensitive)
	logs := output.String()
	for _, forbidden := range []string{"https://upstream.example", "token=secret", "private-ref", "upstream_reference"} {
		require.NotContains(t, logs, forbidden)
	}
	require.Contains(t, logs, `"error_category":"settlement_retry"`)
	require.Contains(t, logs, `"error_category":"background_error"`)
	require.Contains(t, logs, `"error_code"`)
}

func TestMediaWorkerRetriesExplicitRejectionOnDifferentSnapshottedAccount(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.submitErr = &MediaAdapterError{
		Code: "upstream_busy", Message: "busy", Retryable: true,
	}
	fallback := newWorkerAdapter("worker-fallback")
	fixture.worker.deps.Adapters.Register(fallback.Name(), fallback)
	fallbackAccount := &Account{ID: 8, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}
	accountRepo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)
	accountRepo.extra = append(accountRepo.extra, fallbackAccount)
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(fixture.task.CandidateSnapshot, &candidates))
	candidates = append(candidates, MediaAccountCandidateSnapshot{
		AccountID: fallbackAccount.ID, Platform: fallbackAccount.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: fallback.Name(), UpstreamModel: "upstream-fallback", NativeAsyncMode: NativeAsyncRequired,
		},
	})
	encoded, err := json.Marshal(candidates)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].CandidateSnapshot = encoded
	fixture.repo.mu.Unlock()

	require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Equal(t, int64(1), fallback.submitCalls.Load())
	require.Equal(t, fallbackAccount.ID, *fixture.repo.mustGet(fixture.task.ID).AccountID)
	require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerRetryCandidateInfrastructureErrorDoesNotSettleLastUpstreamFailure(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.submitErr = &MediaAdapterError{
		Code: "upstream_busy", Message: "busy", Retryable: true,
	}
	fallback := newWorkerAdapter("worker-fallback-infrastructure")
	fixture.worker.deps.Adapters.Register(fallback.Name(), fallback)
	fallbackAccount := &Account{ID: 8, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}
	accountRepo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)
	accountRepo.extra = append(accountRepo.extra, fallbackAccount)
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(fixture.task.CandidateSnapshot, &candidates))
	candidates = append(candidates, MediaAccountCandidateSnapshot{
		AccountID: fallbackAccount.ID, Platform: fallbackAccount.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: fallback.Name(), UpstreamModel: "upstream-fallback", NativeAsyncMode: NativeAsyncRequired,
		},
	})
	encoded, err := json.Marshal(candidates)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].CandidateSnapshot = encoded
	fixture.repo.mu.Unlock()
	infrastructureErr := errors.New("scheduler failed during retry")
	fixture.selector.selectErrAfter = 2
	fixture.selector.selectErr = infrastructureErr

	err = fixture.worker.processMessage(context.Background(), &MediaQueueMessage{TaskID: fixture.task.ID})
	require.ErrorIs(t, err, infrastructureErr)
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Zero(t, fallback.submitCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
	require.Zero(t, fixture.queue.publishCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
}

func TestMediaWorkerConsumerAcknowledgesOnlyAfterSafeCompletion(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.queue.deliver(&MediaQueueMessage{ID: "1-0", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync})

	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	require.Eventually(t, func() bool {
		return fixture.repo.mustGet(fixture.task.ID).Status == MediaTaskStatusCompleted
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fixture.queue.ackCalls.Load() == 1 }, time.Second, 10*time.Millisecond)
}

func TestMediaWorkerStopForSyncTimeoutAbortsExistingUpstreamAndStopsPolling(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.adapter.blockPollUntil(make(chan struct{}))
	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	require.Eventually(t, func() bool { return fixture.adapter.pollCalls.Load() >= 1 }, time.Second, 10*time.Millisecond)

	require.True(t, fixture.worker.StopForSyncTimeout(fixture.task.ID))
	require.ErrorIs(t, <-done, ErrMediaSyncTimeoutStopped)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerStopWhileExistingUpstreamWaitsForFixedSlotAbortsPersistedTarget(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	prepareRecoverableExistingUpstream(fixture, "existing-upstream")
	waitEntered := make(chan struct{})
	fixture.selector.configureWait(waitEntered, make(chan struct{}))

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("expected existing upstream to wait for its fixed account slot")
	}
	require.Zero(t, fixture.adapter.pollCalls.Load())

	require.True(t, fixture.worker.StopForSyncTimeout(fixture.task.ID))
	require.ErrorIs(t, <-done, ErrMediaSyncTimeoutStopped)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.Zero(t, fixture.selector.releaseCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Eventually(t, func() bool {
		stored := fixture.repo.mustGet(fixture.task.ID)
		return stored.LeaseUntil != nil && stored.LeaseUntil.Before(time.Now())
	}, time.Second, 10*time.Millisecond)
	fixture.queue.resetEnqueued()
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
}

func TestMediaWorkerExistingUpstreamRecoveryAllowsUnlimitedFixedAccountConcurrency(t *testing.T) {
	for _, concurrency := range []int{0, -1} {
		t.Run(fmt.Sprintf("concurrency_%d", concurrency), func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			fixture.account.Concurrency = concurrency
			prepareRecoverableExistingUpstream(fixture, "existing-upstream")

			require.NoError(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
			require.Equal(t, MediaTaskStatusCompleted, fixture.repo.mustGet(fixture.task.ID).Status)
			require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
			require.Zero(t, fixture.selector.waitCalls.Load())
		})
	}
}

func TestMediaWorkerStopDuringExistingUpstreamTargetResolutionStillAbortsAfterBind(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	prepareRecoverableExistingUpstream(fixture, "existing-upstream")
	accountRepo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)
	resolveEntered := make(chan struct{})
	releaseResolve := make(chan struct{})
	defer func() {
		select {
		case <-releaseResolve:
		default:
			close(releaseResolve)
		}
	}()
	accountRepo.getEntered = resolveEntered
	accountRepo.getBlock = releaseResolve

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-resolveEntered:
	case <-time.After(time.Second):
		t.Fatal("expected persisted upstream target resolution to start")
	}
	require.True(t, fixture.worker.StopForSyncTimeout(fixture.task.ID))
	require.Zero(t, fixture.adapter.abortCalls.Load(), "abort target is not bound until account resolution completes")
	select {
	case err := <-done:
		t.Fatalf("execution returned before cleanup target resolution completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseResolve)

	require.ErrorIs(t, <-done, ErrMediaSyncTimeoutStopped)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.Zero(t, fixture.selector.selectCalls.Load(), "canceled execution must not acquire a fixed slot after cleanup binding")
	require.Zero(t, fixture.queue.ackCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerAbortsSubmittedUpstreamWhenPersistenceFails(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*workerTaskRepository)
		wantErr   error
	}{
		{
			name: "repository error",
			configure: func(repo *workerTaskRepository) {
				repo.upstreamUpdateErr = errors.New("upstream id storage unavailable")
			},
		},
		{
			name: "claim rejected",
			configure: func(repo *workerTaskRepository) {
				repo.rejectUpstreamUpdate = true
			},
			wantErr: ErrMediaWorkerLeaseLost,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
			tt.configure(fixture.repo)

			err := fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
			require.Zero(t, fixture.adapter.pollCalls.Load())
			require.Zero(t, fixture.queue.ackCalls.Load())
			require.Zero(t, fixture.billing.settlementCalls())
			stored := fixture.repo.mustGet(fixture.task.ID)
			require.Equal(t, MediaTaskStatusInProgress, stored.Status)
			require.Equal(t, MediaTaskStageSubmitting, stored.Stage)
			require.Empty(t, stored.UpstreamTaskID)
			aborted := fixture.adapter.lastAbortRequest()
			require.Equal(t, fixture.account.ID, aborted.Account.ID)
			require.Equal(t, "upstream-1", aborted.UpstreamTaskID)
		})
	}
}

func TestMediaWorkerPersistenceFailureLeavesMessageUnackedAndRecoverable(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.upstreamUpdateErr = errors.New("upstream id storage unavailable")
	fixture.queue.deliver(&MediaQueueMessage{ID: "upstream-persist-failure", TaskID: fixture.task.ID, Priority: MediaQueuePriorityAsync})

	require.NoError(t, fixture.worker.Start())
	select {
	case <-fixture.worker.Errors():
	case <-time.After(time.Second):
		fixture.worker.Stop()
		t.Fatal("expected upstream id persistence error")
	}
	fixture.worker.Stop()
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Zero(t, fixture.queue.ackCalls.Load())
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Eventually(t, func() bool {
		stored := fixture.repo.mustGet(fixture.task.ID)
		return stored.LeaseUntil != nil && stored.LeaseUntil.Before(time.Now())
	}, time.Second, 10*time.Millisecond)
	fixture.queue.resetEnqueued()
	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.Equal(t, []int64{fixture.task.ID}, fixture.queue.enqueuedTaskIDs())
}

func TestMediaWorkerStopDuringSubmitAbortsLateBoundUpstream(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	submitEntered := make(chan struct{})
	releaseSubmit := make(chan struct{})
	fixture.adapter.blockSubmitUntil(submitEntered, releaseSubmit)

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-submitEntered:
	case <-time.After(time.Second):
		t.Fatal("expected Submit to start")
	}
	require.True(t, fixture.worker.StopForSyncTimeout(fixture.task.ID))
	close(releaseSubmit)

	require.ErrorIs(t, <-done, ErrMediaSyncTimeoutStopped)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.False(t, fixture.adapter.abortCtxCanceled.Load())
	aborted := fixture.adapter.lastAbortRequest()
	require.Equal(t, fixture.account.ID, aborted.Account.ID)
	require.Equal(t, "upstream-1", aborted.UpstreamTaskID)
}

func TestMediaWorkerStopAndPersistenceFailureAbortSubmittedUpstreamOnce(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	updateEntered := make(chan struct{})
	releaseUpdate := make(chan struct{})
	fixture.repo.upstreamUpdateEntered = updateEntered
	fixture.repo.releaseUpstreamUpdate = releaseUpdate
	fixture.repo.upstreamUpdateErr = errors.New("upstream id storage unavailable")

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-updateEntered:
	case <-time.After(time.Second):
		t.Fatal("expected upstream id persistence to start")
	}
	require.True(t, fixture.worker.StopForSyncTimeout(fixture.task.ID))
	close(releaseUpdate)

	require.ErrorIs(t, <-done, ErrMediaSyncTimeoutStopped)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.False(t, fixture.adapter.abortCtxCanceled.Load())
}

func TestMediaWorkerAbortFailureDoesNotLeakSensitiveErrorToLogs(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.repo.upstreamUpdateErr = errors.New("upstream id storage unavailable")
	fixture.adapter.abortErr = errors.New("https://upstream.example/abort token=secret")
	var output bytes.Buffer
	fixture.worker.logger = slog.New(slog.NewJSONHandler(&output, nil))

	require.Error(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.NotContains(t, output.String(), "https://upstream.example")
	require.NotContains(t, output.String(), "token=secret")
	require.Contains(t, output.String(), `"error_category":"abort_failed"`)
}

func TestMediaWorkerStopRecoversAbortPanicAndStillCancelsExecution(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	prepareRecoverableExistingUpstream(fixture, "existing-upstream")
	waitEntered := make(chan struct{})
	fixture.selector.configureWait(waitEntered, make(chan struct{}))
	fixture.adapter.abortPanic = true
	fixture.adapter.abortPanicMessage = "https://upstream.example/abort token=panic-secret"
	var output bytes.Buffer
	fixture.worker.logger = slog.New(slog.NewJSONHandler(&output, nil))

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("expected existing upstream fixed-slot wait")
	}
	active := fixture.worker.activeForTask(fixture.task.ID)
	require.NotNil(t, active)
	var stopped bool
	recovered := captureWorkerTestPanic(func() {
		stopped = fixture.worker.StopForSyncTimeout(fixture.task.ID)
	})
	processErr := <-done
	var abortEvent error
	select {
	case abortEvent = <-fixture.worker.Errors():
	case <-time.After(100 * time.Millisecond):
	}
	secondRecovered := captureWorkerTestPanic(func() {
		fixture.worker.abortActiveUpstream(fixture.task.ID, active)
	})

	require.Nil(t, recovered, "StopForSyncTimeout must recover adapter Abort panic")
	require.True(t, stopped)
	require.ErrorIs(t, processErr, ErrMediaSyncTimeoutStopped)
	require.Nil(t, secondRecovered)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load(), "abortOnce must be consumed exactly once")
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.Zero(t, fixture.selector.releaseCalls.Load())
	require.False(t, fixture.adapter.abortCtxCanceled.Load())
	require.False(t, fixture.worker.StopForSyncTimeout(fixture.task.ID), "active execution must be unregistered")
	require.Error(t, abortEvent)
	require.Contains(t, abortEvent.Error(), "panic-secret", "raw panic diagnostics may remain in the in-memory error channel")
	require.NotContains(t, output.String(), "https://upstream.example")
	require.NotContains(t, output.String(), "panic-secret")
	require.Contains(t, output.String(), `"error_category":"abort_failed"`)
}

func TestMediaWorkerPersistenceFailureRecoversAbortPanicAndReleasesResources(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	persistErr := errors.New("upstream id storage unavailable")
	fixture.repo.upstreamUpdateErr = persistErr
	fixture.adapter.abortPanic = true
	fixture.adapter.abortPanicMessage = "https://upstream.example/abort token=db-panic-secret"
	var output bytes.Buffer
	fixture.worker.logger = slog.New(slog.NewJSONHandler(&output, nil))

	var processErr error
	recovered := captureWorkerTestPanic(func() {
		processErr = fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	})
	var abortEvent error
	select {
	case abortEvent = <-fixture.worker.Errors():
	case <-time.After(100 * time.Millisecond):
	}

	require.Nil(t, recovered, "persistence cleanup must recover adapter Abort panic")
	require.ErrorIs(t, processErr, persistErr)
	require.Equal(t, int64(1), fixture.adapter.abortCalls.Load())
	require.Zero(t, fixture.adapter.pollCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.False(t, fixture.adapter.abortCtxCanceled.Load())
	require.False(t, fixture.worker.StopForSyncTimeout(fixture.task.ID), "active execution must be unregistered")
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Error(t, abortEvent)
	require.Contains(t, abortEvent.Error(), "db-panic-secret")
	require.NotContains(t, output.String(), "https://upstream.example")
	require.NotContains(t, output.String(), "db-panic-secret")
	require.Contains(t, output.String(), `"error_category":"abort_failed"`)
}

func captureWorkerTestPanic(run func()) (recovered any) {
	defer func() { recovered = recover() }()
	run()
	return nil
}

func TestMediaWorkerPersistenceFailureWithNonAbortableAdapterStillReleasesResources(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	inner := newWorkerAdapter("worker-non-abortable-inner")
	adapter := &nonAbortableWorkerAdapter{name: "worker-non-abortable", inner: inner}
	fixture.worker.deps.Adapters.Register(adapter.Name(), adapter)
	candidates, err := json.Marshal([]MediaAccountCandidateSnapshot{{
		AccountID: fixture.account.ID, Platform: fixture.account.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: adapter.Name(), UpstreamModel: "upstream-image", NativeAsyncMode: NativeAsyncRequired,
		},
	}})
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	fixture.repo.tasks[fixture.task.ID].CandidateSnapshot = candidates
	fixture.repo.mu.Unlock()
	fixture.repo.upstreamUpdateErr = errors.New("upstream id storage unavailable")

	require.Error(t, fixture.worker.ProcessOne(context.Background(), fixture.task.ID))
	require.Equal(t, int64(1), inner.submitCalls.Load())
	require.Zero(t, inner.pollCalls.Load())
	require.Zero(t, inner.abortCalls.Load())
	require.Equal(t, int64(1), fixture.selector.releaseCalls.Load())
	require.False(t, fixture.worker.StopForSyncTimeout(fixture.task.ID), "active execution must be unregistered")
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
}

func TestMediaWorkerLeaseRenewerPanicCancelsExecutionBeforeTerminalTransition(t *testing.T) {
	fixture := newMediaWorkerFixture(t, true, NativeAsyncRequired)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	fixture.repo.panicRenewLease.Store(true)
	fixture.adapter.blockPollUntil(make(chan struct{}))

	err := fixture.worker.ProcessOne(context.Background(), fixture.task.ID)
	require.ErrorIs(t, err, ErrMediaWorkerLeaseLost)
	require.Equal(t, MediaTaskStatusInProgress, fixture.repo.mustGet(fixture.task.ID).Status)
	require.Zero(t, fixture.billing.settlementCalls())
}

func TestMediaWorkerLeaseRenewerPanicDoesNotCancelTerminalSettlement(t *testing.T) {
	fixture := newMediaWorkerFixture(t, false, NativeAsyncUnsupported)
	fixture.worker.cfg.LeaseRenewInterval = 5 * time.Millisecond
	applied := make(chan struct{})
	releaseTransition := make(chan struct{})
	fixture.repo.terminalTransitionApplied = applied
	fixture.repo.releaseTerminalTransition = releaseTransition
	observing := &contextObservingSettlementCoordinator{inner: fixture.billing}
	fixture.worker.deps.Billing = observing

	done := make(chan error, 1)
	go func() { done <- fixture.worker.ProcessOne(context.Background(), fixture.task.ID) }()
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("expected terminal transition to be applied")
	}
	fixture.repo.panicRenewLease.Store(true)
	renewCalls := fixture.repo.renewLeaseCalls.Load()
	require.Eventually(t, func() bool {
		return fixture.repo.renewLeaseCalls.Load() > renewCalls
	}, time.Second, time.Millisecond)
	close(releaseTransition)

	require.NoError(t, <-done)
	require.False(t, observing.ctxCanceled.Load())
	require.Equal(t, MediaBillingStatusSettled, fixture.repo.mustGet(fixture.task.ID).BillingStatus)
}

type mediaWorkerFixture struct {
	worker         *MediaWorker
	repo           *workerTaskRepository
	queue          *workerQueue
	adapter        *workerAdapter
	billing        *recordingSettlementCoordinator
	metrics        *AtomicMediaTaskMetrics
	artifactWriter *workerArtifactWriter
	task           *MediaTask
	account        *Account
	selector       *workerSelector
	releasePoll    chan struct{}
}

func newMediaWorkerFixture(t *testing.T, clientAsync bool, mode NativeAsyncMode) *mediaWorkerFixture {
	t.Helper()
	adapter := newWorkerAdapter("worker-fake")
	registry := NewMediaAdapterRegistry()
	registry.Register(adapter.Name(), adapter)
	account := &Account{ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}
	accountRepo := &workerAccountRepository{account: account}
	selector := &workerSelector{refreshOwned: true}
	scheduler := NewMediaScheduler(accountRepo, selector, registry, workerGroupRepository{})
	modelRegistry := NewMediaModelRegistry(&workerModelRepository{definition: MediaModelDefinition{
		ModelID: "fake-image", Vendor: "fake-vendor", DefaultAdapter: adapter.Name(), DefaultAsyncMode: mode, MediaType: MediaTypeImage, Operations: []MediaOperation{MediaOperationTextToImage}, Enabled: true,
	}})
	require.NoError(t, modelRegistry.Refresh(context.Background()))
	candidates, err := json.Marshal([]MediaAccountCandidateSnapshot{{
		AccountID: account.ID,
		Platform:  account.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: adapter.Name(), UpstreamModel: "upstream-image", NativeAsyncMode: mode,
		},
	}})
	require.NoError(t, err)
	task := &MediaTask{
		ID: 1, PublicID: fmt.Sprintf("worker-%s-%t", mode, clientAsync), UserID: 1, APIKeyID: 2, GroupID: 3,
		MediaType: MediaTypeImage, Operation: MediaOperationTextToImage, RequestedModel: "fake-image",
		ClientAsync: clientAsync, Status: MediaTaskStatusQueued, Stage: MediaTaskStageQueued,
		RequestSpec: json.RawMessage(`{"image":{"prompt":"cat","n":1}}`), CandidateSnapshot: candidates,
		RequestFingerprint: "fingerprint", BillingStatus: MediaBillingStatusPrecharged, PrechargedAmount: 2, Version: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	repo := newWorkerTaskRepository(task)
	queue := &workerQueue{}
	billing := &recordingSettlementCoordinator{repo: repo}
	metrics := NewAtomicMediaTaskMetrics()
	artifactWriter := &workerArtifactWriter{}
	worker := NewMediaWorker(MediaWorkerConfig{
		WorkerCount: 1, TaskTimeout: time.Second, LeaseTTL: 200 * time.Millisecond,
		LeaseRenewInterval: 50 * time.Millisecond, PollInterval: time.Millisecond,
		RecoveryInterval: time.Second, RecoveryBatchSize: 10,
	}, MediaWorkerDependencies{
		Tasks: repo, Queue: queue, Scheduler: scheduler, Models: modelRegistry, Adapters: registry,
		Artifacts: artifactWriter, Precharger: DisabledMediaBilling{}, Billing: billing, Metrics: metrics,
	})
	return &mediaWorkerFixture{
		worker: worker, repo: repo, queue: queue, adapter: adapter, billing: billing, metrics: metrics,
		artifactWriter: artifactWriter, task: task, account: account, selector: selector, releasePoll: make(chan struct{}),
	}
}

func prepareRecoverableSubmittingTask(t *testing.T, fixture *mediaWorkerFixture, supportsIdempotency bool) (*workerAdapter, *Account) {
	t.Helper()
	fixedAdapter := newWorkerAdapter("worker-fixed-submitter")
	fixedAdapter.supportsIdempotency = supportsIdempotency
	fixture.worker.deps.Adapters.Register(fixedAdapter.Name(), fixedAdapter)
	fixedAccount := &Account{ID: 9, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}
	accountRepo := fixture.worker.deps.Scheduler.accountRepo.(*workerAccountRepository)
	accountRepo.extra = append(accountRepo.extra, fixedAccount)
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(fixture.task.CandidateSnapshot, &candidates))
	candidates = append(candidates, MediaAccountCandidateSnapshot{
		AccountID: fixedAccount.ID,
		Platform:  fixedAccount.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: fixedAdapter.Name(), UpstreamModel: "fixed-upstream-image", NativeAsyncMode: NativeAsyncRequired,
		},
	})
	encoded, err := json.Marshal(candidates)
	require.NoError(t, err)
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.Status = MediaTaskStatusInProgress
	stored.Stage = MediaTaskStageSubmitting
	stored.AccountID = workerInt64Ptr(fixedAccount.ID)
	stored.Adapter = fixedAdapter.Name()
	stored.UpstreamModel = "fixed-upstream-image"
	stored.NativeAsyncMode = NativeAsyncRequired
	stored.UpstreamTaskID = ""
	stored.CandidateSnapshot = encoded
	stored.WorkerID = "dead-worker"
	stored.LeaseUntil = workerTimePtr(time.Now().Add(-time.Minute))
	stored.StartedAt = workerTimePtr(time.Now().UTC())
	stored.Version++
	fixture.repo.mu.Unlock()
	return fixedAdapter, fixedAccount
}

func prepareRecoverableExistingUpstream(fixture *mediaWorkerFixture, upstreamTaskID string) {
	fixture.repo.mu.Lock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.Status = MediaTaskStatusInProgress
	stored.Stage = MediaTaskStagePolling
	stored.AccountID = workerInt64Ptr(fixture.account.ID)
	stored.Adapter = fixture.adapter.Name()
	stored.UpstreamModel = "upstream-image"
	stored.NativeAsyncMode = NativeAsyncRequired
	stored.UpstreamTaskID = upstreamTaskID
	stored.PollMetadata = json.RawMessage(`{"cursor":1}`)
	stored.WorkerID = "dead-worker"
	stored.LeaseUntil = workerTimePtr(time.Now().Add(-time.Minute))
	stored.StartedAt = workerTimePtr(time.Now().UTC())
	stored.Version++
	fixture.repo.mu.Unlock()
	fixture.adapter.allowUpstream(upstreamTaskID)
}

func prepareRecoverableSyncStage(fixture *mediaWorkerFixture, stage MediaTaskStage) {
	fixture.repo.mu.Lock()
	defer fixture.repo.mu.Unlock()
	stored := fixture.repo.tasks[fixture.task.ID]
	stored.Status = MediaTaskStatusInProgress
	stored.Stage = stage
	stored.AccountID = workerInt64Ptr(fixture.account.ID)
	stored.Adapter = fixture.adapter.Name()
	stored.UpstreamModel = "upstream-image"
	stored.NativeAsyncMode = NativeAsyncUnsupported
	stored.UpstreamTaskID = ""
	stored.WorkerID = "dead-worker"
	stored.LeaseUntil = workerTimePtr(time.Now().Add(-time.Minute))
	stored.StartedAt = workerTimePtr(time.Now().UTC())
	stored.Version++
}

type workerTaskRepository struct {
	mu                        sync.Mutex
	tasks                     map[int64]*MediaTask
	renewLeaseCalls           atomic.Int64
	rejectRenewLease          atomic.Bool
	terminalTransitionApplied chan struct{}
	releaseTerminalTransition <-chan struct{}
	terminalTransitionOnce    sync.Once
	upstreamUpdateErr         error
	rejectUpstreamUpdate      bool
	upstreamUpdateEntered     chan struct{}
	releaseUpstreamUpdate     <-chan struct{}
	upstreamUpdateOnce        sync.Once
	panicRenewLease           atomic.Bool
	failSettlementPlanWrites  int
}

type workerInitializationBillingPort struct {
	mu                              sync.Mutex
	seen                            map[string]struct{}
	prechargeMutations              int
	prechargeCalls                  int
	settleFailureCalls              int
	balance                         float64
	failAfterFirstPrechargeMutation bool
	prechargeErr                    error
}

func newWorkerInitializationBillingPort() *workerInitializationBillingPort {
	return &workerInitializationBillingPort{seen: make(map[string]struct{})}
}

func (p *workerInitializationBillingPort) Precharge(_ context.Context, task *MediaTask, snapshot MediaBillingSnapshot) (MediaPrechargeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prechargeCalls++
	if p.prechargeErr != nil {
		return MediaPrechargeResult{}, p.prechargeErr
	}
	key, err := MediaBillingIdempotencyKey(task, MediaBillingOperationPrecharge)
	if err != nil {
		return MediaPrechargeResult{}, err
	}
	if _, exists := p.seen[key]; !exists {
		p.seen[key] = struct{}{}
		p.prechargeMutations++
		p.balance -= snapshot.EstimatedAmount
		if p.failAfterFirstPrechargeMutation {
			p.failAfterFirstPrechargeMutation = false
			return MediaPrechargeResult{}, fmt.Errorf("precharge result unavailable: %w", ErrMediaPrechargeResultUnknown)
		}
	}
	return MediaPrechargeResult{PrechargedAmount: snapshot.EstimatedAmount}, nil
}

func (p *workerInitializationBillingPort) SettleSuccess(_ context.Context, task *MediaTask, _ MediaUsage) (MediaSettlementResult, error) {
	return MediaSettlementResult{FinalAmount: task.PrechargedAmount}, nil
}

func (p *workerInitializationBillingPort) SettleFailure(_ context.Context, task *MediaTask, settlement MediaFailureSettlement) (MediaSettlementResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settleFailureCalls++
	key, err := MediaBillingIdempotencyKey(task, MediaBillingOperationFailure)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	if _, exists := p.seen[key]; !exists {
		p.seen[key] = struct{}{}
		p.balance += task.PrechargedAmount * settlement.RefundRatio
	}
	return MediaSettlementResult{
		FinalAmount:    task.PrechargedAmount * (1 - settlement.RefundRatio),
		RefundedAmount: task.PrechargedAmount * settlement.RefundRatio,
	}, nil
}

func (p *workerInitializationBillingPort) prechargeMutationCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prechargeMutations
}

func (p *workerInitializationBillingPort) prechargeCallsCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prechargeCalls
}

func (p *workerInitializationBillingPort) settleFailureCallsCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.settleFailureCalls
}

func (p *workerInitializationBillingPort) balanceAmount() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.balance
}

func newWorkerTaskRepository(tasks ...*MediaTask) *workerTaskRepository {
	repo := &workerTaskRepository{tasks: make(map[int64]*MediaTask, len(tasks))}
	for _, task := range tasks {
		repo.tasks[task.ID] = cloneWorkerTask(task)
	}
	return repo
}

func (r *workerTaskRepository) Create(_ context.Context, task *MediaTask) (*MediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if task.ID == 0 {
		task.ID = int64(len(r.tasks) + 1)
	}
	r.tasks[task.ID] = cloneWorkerTask(task)
	return cloneWorkerTask(task), nil
}
func (r *workerTaskRepository) GetByID(_ context.Context, id int64) (*MediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil {
		return nil, errors.New("task not found")
	}
	return cloneWorkerTask(task), nil
}
func (r *workerTaskRepository) GetByPublicIDForUser(context.Context, string, int64) (*MediaTask, error) {
	return nil, errors.New("not implemented")
}
func (r *workerTaskRepository) GetByIdempotencyKey(context.Context, int64, int64, string) (*MediaTask, error) {
	return nil, errors.New("not implemented")
}
func (r *workerTaskRepository) UpdateQueued(_ context.Context, id, version int64, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status != MediaTaskStatusQueued || task.Version != version {
		return false, nil
	}
	applyWorkerTaskUpdates(task, updates)
	task.Version++
	return true, nil
}
func (r *workerTaskRepository) TransitionQueued(_ context.Context, id, expectedVersion int64, to MediaTaskStatus, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status != MediaTaskStatusQueued || task.Version != expectedVersion || to != MediaTaskStatusFailed {
		return false, nil
	}
	applyWorkerTaskUpdates(task, updates)
	task.Status = to
	task.Version++
	return true, nil
}
func (r *workerTaskRepository) TransitionSyncTimeout(_ context.Context, id, expectedVersion int64, expectedStage MediaTaskStage, from MediaTaskStatus, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status != from || task.Stage != expectedStage || task.Version != expectedVersion || task.SyncFallback || !from.CanTransitionTo(MediaTaskStatusFailed) {
		return false, nil
	}
	applyWorkerTaskUpdates(task, updates)
	task.Status = MediaTaskStatusFailed
	task.Version++
	return true, nil
}
func (r *workerTaskRepository) Claim(_ context.Context, id int64, workerID, claimToken string, leaseUntil time.Time, version int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Version != version || task.Status.IsTerminal() {
		return false, nil
	}
	if task.Status == MediaTaskStatusQueued &&
		(task.BillingStatus != MediaBillingStatusPrecharged || (task.LeaseUntil != nil && task.LeaseUntil.After(time.Now()))) {
		return false, nil
	}
	if task.Status == MediaTaskStatusInProgress && task.LeaseUntil != nil && task.LeaseUntil.After(time.Now()) {
		return false, nil
	}
	task.Status, task.WorkerID, task.ClaimToken, task.LeaseUntil = MediaTaskStatusInProgress, workerID, claimToken, workerTimePtr(leaseUntil)
	task.Version++
	return true, nil
}
func (r *workerTaskRepository) RenewLease(_ context.Context, id int64, claimToken string, leaseUntil time.Time) (bool, error) {
	r.renewLeaseCalls.Add(1)
	if r.panicRenewLease.Load() {
		panic("lease renew panic")
	}
	if r.rejectRenewLease.Load() {
		return false, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status != MediaTaskStatusInProgress || task.ClaimToken != claimToken || task.LeaseUntil == nil || !task.LeaseUntil.After(time.Now()) {
		return false, nil
	}
	task.LeaseUntil = workerTimePtr(leaseUntil)
	return true, nil
}
func (r *workerTaskRepository) UpdateClaimed(_ context.Context, id int64, claimToken string, expectedVersion int64, expectedStage MediaTaskStage, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status != MediaTaskStatusInProgress || task.ClaimToken != claimToken || task.Version != expectedVersion || task.Stage != expectedStage || task.LeaseUntil == nil || !task.LeaseUntil.After(time.Now()) {
		return false, nil
	}
	if next, ok := updates["stage"].(MediaTaskStage); ok && next != expectedStage && !expectedStage.CanTransitionTo(next) {
		return false, nil
	}
	if _, persistsUpstreamID := updates["upstream_task_id"]; persistsUpstreamID {
		if r.upstreamUpdateEntered != nil {
			r.upstreamUpdateOnce.Do(func() { close(r.upstreamUpdateEntered) })
		}
		if r.releaseUpstreamUpdate != nil {
			<-r.releaseUpstreamUpdate
		}
		if r.upstreamUpdateErr != nil {
			return false, r.upstreamUpdateErr
		}
		if r.rejectUpstreamUpdate {
			return false, nil
		}
	}
	applyWorkerTaskUpdates(task, updates)
	task.Version++
	return true, nil
}
func (r *workerTaskRepository) Transition(_ context.Context, id int64, from, to MediaTaskStatus, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status != from || !from.CanTransitionTo(to) {
		return false, nil
	}
	applyWorkerTaskUpdates(task, updates)
	task.Status = to
	return true, nil
}
func (r *workerTaskRepository) TransitionClaimed(_ context.Context, id int64, claimToken string, expectedVersion int64, expectedStage MediaTaskStage, from, to MediaTaskStatus, updates map[string]any) (bool, error) {
	r.mu.Lock()
	task := r.tasks[id]
	if task == nil || task.Status != from || task.ClaimToken != claimToken || task.Version != expectedVersion || task.Stage != expectedStage || task.LeaseUntil == nil || !task.LeaseUntil.After(time.Now()) || !from.CanTransitionTo(to) {
		r.mu.Unlock()
		return false, nil
	}
	applyWorkerTaskUpdates(task, updates)
	task.Status = to
	task.Version++
	applied := r.terminalTransitionApplied
	release := r.releaseTerminalTransition
	r.mu.Unlock()
	if applied != nil {
		r.terminalTransitionOnce.Do(func() { close(applied) })
	}
	if release != nil {
		<-release
	}
	return true, nil
}
func (r *workerTaskRepository) MarkSyncFallback(_ context.Context, id int64, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.Status.IsTerminal() || task.SyncFallback {
		return false, nil
	}
	task.SyncFallback, task.SyncFallbackAt = true, workerTimePtr(at)
	return true, nil
}
func (r *workerTaskRepository) ListRecoverable(_ context.Context, now time.Time, limit int) ([]MediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]MediaTask, 0, limit)
	for _, task := range r.tasks {
		if len(result) >= limit {
			break
		}
		recoverableQueued := task.Status == MediaTaskStatusQueued &&
			(task.BillingStatus == MediaBillingStatusPending || task.BillingStatus == MediaBillingStatusPrecharged)
		expiredInProgress := task.Status == MediaTaskStatusInProgress
		if (recoverableQueued || expiredInProgress) && (task.LeaseUntil == nil || !task.LeaseUntil.After(now)) {
			result = append(result, *cloneWorkerTask(task))
		}
	}
	return result, nil
}
func (r *workerTaskRepository) ListSettlementPending(_ context.Context, limit int) ([]MediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]MediaTask, 0, limit)
	for _, task := range r.tasks {
		if len(result) >= limit {
			break
		}
		hasRecovery := task.Status.IsTerminal() && len(task.SettlementRecovery) > 0
		if (len(task.SettlementPlan) > 0 || hasRecovery) && task.BillingStatus != MediaBillingStatusSettled {
			result = append(result, *cloneWorkerTask(task))
		}
	}
	return result, nil
}
func (r *workerTaskRepository) UpdateBilling(_ context.Context, id int64, fromStatus string, updates map[string]any) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil || task.BillingStatus != fromStatus {
		return false, nil
	}
	if _, persistsPlan := updates["settlement_plan"]; persistsPlan && r.failSettlementPlanWrites > 0 {
		r.failSettlementPlanWrites--
		return false, errors.New("settlement plan storage unavailable")
	}
	applyWorkerTaskUpdates(task, updates)
	return true, nil
}
func (r *workerTaskRepository) mustGet(id int64) *MediaTask {
	task, err := r.GetByID(context.Background(), id)
	if err != nil {
		panic(err)
	}
	return task
}
func (r *workerTaskRepository) setExpiredLease(id int64, workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	task.Status, task.WorkerID, task.ClaimToken, task.LeaseUntil = MediaTaskStatusInProgress, workerID, "expired-claim", workerTimePtr(time.Now().Add(-time.Minute))
	task.Version++
}

func applyWorkerTaskUpdates(task *MediaTask, updates map[string]any) {
	for field, value := range updates {
		switch field {
		case "account_id":
			v := value.(int64)
			task.AccountID = &v
		case "adapter":
			task.Adapter = value.(string)
		case "upstream_model":
			task.UpstreamModel = value.(string)
		case "native_async_mode":
			task.NativeAsyncMode = value.(NativeAsyncMode)
		case "upstream_task_id":
			task.UpstreamTaskID = value.(string)
		case "poll_metadata":
			task.PollMetadata = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "stage":
			task.Stage = value.(MediaTaskStage)
		case "progress":
			task.Progress = value.(int)
		case "submitted_at":
			task.SubmittedAt = workerTimePtr(value.(time.Time))
		case "started_at":
			task.StartedAt = workerTimePtr(value.(time.Time))
		case "finished_at":
			task.FinishedAt = workerTimePtr(value.(time.Time))
		case "retry_count":
			task.RetryCount = value.(int)
		case "error_code":
			task.ErrorCode = value.(string)
		case "error_message":
			task.ErrorMessage = value.(string)
		case "settlement_plan":
			switch typed := value.(type) {
			case json.RawMessage:
				task.SettlementPlan = append(json.RawMessage(nil), typed...)
			case []byte:
				task.SettlementPlan = append(json.RawMessage(nil), typed...)
			}
		case "settlement_recovery":
			task.SettlementRecovery = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "billing_status":
			task.BillingStatus = value.(string)
		case "precharged_amount":
			task.PrechargedAmount = value.(float64)
		case "final_amount":
			task.FinalAmount = value.(float64)
		case "refunded_amount":
			task.RefundedAmount = value.(float64)
		case "additional_charged_amount":
			task.AdditionalChargedAmount = value.(float64)
		case "lease_until":
			if value == nil {
				task.LeaseUntil = nil
			} else {
				task.LeaseUntil = workerTimePtr(value.(time.Time))
			}
		}
	}
}

func cloneWorkerTask(task *MediaTask) *MediaTask {
	if task == nil {
		return nil
	}
	copy := *task
	copy.AccountID = cloneWorkerInt64(task.AccountID)
	copy.ChannelID = cloneWorkerInt64(task.ChannelID)
	copy.LeaseUntil = cloneWorkerTime(task.LeaseUntil)
	copy.StartedAt = cloneWorkerTime(task.StartedAt)
	copy.SubmittedAt = cloneWorkerTime(task.SubmittedAt)
	copy.FinishedAt = cloneWorkerTime(task.FinishedAt)
	copy.SyncFallbackAt = cloneWorkerTime(task.SyncFallbackAt)
	copy.RequestSpec = append(json.RawMessage(nil), task.RequestSpec...)
	copy.CandidateSnapshot = append(json.RawMessage(nil), task.CandidateSnapshot...)
	copy.PollMetadata = append(json.RawMessage(nil), task.PollMetadata...)
	copy.SettlementPlan = append(json.RawMessage(nil), task.SettlementPlan...)
	copy.SettlementRecovery = append(json.RawMessage(nil), task.SettlementRecovery...)
	return &copy
}
func cloneWorkerInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}
func cloneWorkerTime(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}
func workerTimePtr(v time.Time) *time.Time { return &v }
func workerInt64Ptr(v int64) *int64        { return &v }

func workerCompletedTask(id int64, publicID string) *MediaTask {
	now := time.Now().UTC()
	return &MediaTask{ID: id, PublicID: publicID, Status: MediaTaskStatusCompleted, Stage: MediaTaskStageCompleted, BillingStatus: MediaBillingStatusPrecharged, PrechargedAmount: 3, Version: 1, FinishedAt: &now}
}

type workerQueue struct {
	mu           sync.Mutex
	ids          []int64
	priorities   []MediaQueuePriority
	receive      chan *MediaQueueMessage
	ackCalls     atomic.Int64
	publishCalls atomic.Int64
}

func (q *workerQueue) EnsureGroups(context.Context) error { return nil }
func (q *workerQueue) Enqueue(_ context.Context, id int64, priority MediaQueuePriority) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, id)
	q.priorities = append(q.priorities, priority)
	return nil
}
func (q *workerQueue) Receive(ctx context.Context, block time.Duration) (*MediaQueueMessage, error) {
	q.mu.Lock()
	receive := q.receive
	q.mu.Unlock()
	if receive == nil {
		return nil, ErrMediaQueueReceiveTimeout
	}
	timer := time.NewTimer(block)
	defer timer.Stop()
	select {
	case message := <-receive:
		return message, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, ErrMediaQueueReceiveTimeout
	}
}
func (q *workerQueue) Ack(context.Context, *MediaQueueMessage) error {
	q.ackCalls.Add(1)
	return nil
}
func (q *workerQueue) PublishTerminal(context.Context, int64, MediaTaskStatus) error {
	q.publishCalls.Add(1)
	return nil
}
func (q *workerQueue) SubscribeTerminal(context.Context, int64) (<-chan MediaTaskStatus, func(), error) {
	ch := make(chan MediaTaskStatus)
	close(ch)
	return ch, func() {}, nil
}
func (q *workerQueue) enqueuedTaskIDs() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]int64(nil), q.ids...)
}
func (q *workerQueue) enqueuedPriorities() []MediaQueuePriority {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]MediaQueuePriority(nil), q.priorities...)
}
func (q *workerQueue) resetEnqueued() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids, q.priorities = nil, nil
}
func (q *workerQueue) deliver(message *MediaQueueMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.receive == nil {
		q.receive = make(chan *MediaQueueMessage, 1)
	}
	q.receive <- message
}

type workerAccountRepository struct {
	account        *Account
	extra          []*Account
	listErr        error
	getErr         error
	markUsed       atomic.Int64
	getCalls       atomic.Int64
	getEntered     chan struct{}
	getBlock       <-chan struct{}
	getEnteredOnce sync.Once
}

type workerGroupRepository struct{}

func (workerGroupRepository) GetByID(ctx context.Context, id int64) (*Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Group{ID: id, Platform: PlatformOpenAI, MediaCrossPlatformEnabled: true}, nil
}

func (r *workerAccountRepository) ListSchedulableByGroupID(ctx context.Context, _ int64) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.listErr != nil {
		return nil, r.listErr
	}
	accounts := []Account{*r.account}
	for _, account := range r.extra {
		accounts = append(accounts, *account)
	}
	return accounts, nil
}
func (r *workerAccountRepository) GetByID(ctx context.Context, id int64) (*Account, error) {
	r.getCalls.Add(1)
	if r.getEntered != nil {
		r.getEnteredOnce.Do(func() { close(r.getEntered) })
	}
	if r.getBlock != nil {
		select {
		case <-r.getBlock:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.account.ID == id {
		copy := *r.account
		return &copy, nil
	}
	for _, account := range r.extra {
		if account.ID == id {
			copy := *account
			return &copy, nil
		}
	}
	return nil, errors.New("account not found")
}
func (r *workerAccountRepository) UpdateLastUsed(context.Context, int64) error {
	r.markUsed.Add(1)
	return nil
}

type workerSelector struct {
	selectCalls            atomic.Int64
	releaseCalls           atomic.Int64
	refreshCalls           atomic.Int64
	refreshActive          atomic.Int64
	releaseWhileRefreshing atomic.Bool
	waitCalls              atomic.Int64
	waitMode               atomic.Bool
	driftID                atomic.Int64
	selectErrAfter         int64
	selectErr              error
	mu                     sync.Mutex
	lastSession            string
	lastSlotID             string
	lastIDs                []int64
	waitEntered            chan struct{}
	waitBlock              <-chan struct{}
	waitOnce               sync.Once
	refreshOwned           bool
	refreshErr             error
	refreshPanic           string
}

func (s *workerSelector) Select(_ context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error) {
	selectCall := s.selectCalls.Add(1)
	if s.selectErr != nil && selectCall >= s.selectErrAfter {
		return nil, s.selectErr
	}
	s.mu.Lock()
	s.lastSession = req.SessionHash
	s.lastSlotID = req.SlotID
	s.lastIDs = s.lastIDs[:0]
	for _, candidate := range req.Candidates {
		if candidate != nil {
			s.lastIDs = append(s.lastIDs, candidate.ID)
		}
	}
	s.mu.Unlock()
	if driftID := s.driftID.Load(); driftID > 0 {
		return &AccountSelectionResult{
			Account: &Account{ID: driftID}, Acquired: true,
			ReleaseFunc: func() { s.releaseCalls.Add(1) },
		}, nil
	}
	if s.waitMode.Load() {
		return &AccountSelectionResult{
			Account: req.Candidates[0],
			WaitPlan: &AccountWaitPlan{
				AccountID: req.Candidates[0].ID, MaxConcurrency: req.Candidates[0].Concurrency,
				Timeout: time.Second, MaxWaiting: 1, SlotID: req.SlotID,
			},
		}, nil
	}
	return &AccountSelectionResult{
		Account: req.Candidates[0], Acquired: true,
		ReleaseFunc: s.release, RefreshFunc: s.refresh,
	}, nil
}
func (s *workerSelector) SelectFixed(ctx context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error) {
	return s.Select(ctx, req)
}
func (s *workerSelector) Wait(ctx context.Context, _ *AccountWaitPlan) (func(), error) {
	s.waitCalls.Add(1)
	s.mu.Lock()
	entered := s.waitEntered
	block := s.waitBlock
	s.mu.Unlock()
	if entered != nil {
		s.waitOnce.Do(func() { close(entered) })
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.release, nil
}

func (s *workerSelector) WaitStable(ctx context.Context, plan *AccountWaitPlan) (*AcquireResult, error) {
	release, err := s.Wait(ctx, plan)
	if err != nil {
		return nil, err
	}
	return &AcquireResult{Acquired: true, ReleaseFunc: release, RefreshFunc: s.refresh}, nil
}

func (s *workerSelector) refresh(context.Context) (bool, error) {
	s.refreshCalls.Add(1)
	s.refreshActive.Add(1)
	defer s.refreshActive.Add(-1)
	s.mu.Lock()
	owned, err, panicMessage := s.refreshOwned, s.refreshErr, s.refreshPanic
	s.mu.Unlock()
	if panicMessage != "" {
		panic(panicMessage)
	}
	return owned, err
}

func (s *workerSelector) release() {
	if s.refreshActive.Load() != 0 {
		s.releaseWhileRefreshing.Store(true)
	}
	s.releaseCalls.Add(1)
}

func (s *workerSelector) configureRefresh(owned bool, err error, panicMessage string) {
	s.mu.Lock()
	s.refreshOwned = owned
	s.refreshErr = err
	s.refreshPanic = panicMessage
	s.mu.Unlock()
}
func (s *workerSelector) configureWait(entered chan struct{}, block <-chan struct{}) {
	s.waitMode.Store(true)
	s.mu.Lock()
	s.waitEntered = entered
	s.waitBlock = block
	s.mu.Unlock()
}
func (s *workerSelector) configureDrift(accountID int64) { s.driftID.Store(accountID) }
func (s *workerSelector) lastSessionKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSession
}
func (s *workerSelector) lastCandidateAccountIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.lastIDs...)
}
func (s *workerSelector) lastStableSlotID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSlotID
}

type workerModelRepository struct{ definition MediaModelDefinition }

func (r *workerModelRepository) ListEnabled(context.Context) ([]MediaModelDefinition, error) {
	return []MediaModelDefinition{r.definition}, nil
}

type workerAdapter struct {
	name                string
	syncCalls           atomic.Int64
	submitCalls         atomic.Int64
	pollCalls           atomic.Int64
	abortCalls          atomic.Int64
	mu                  sync.Mutex
	pollResult          MediaPollResult
	pollBlock           <-chan struct{}
	submitErr           error
	submitErrors        []error
	supportsIdempotency bool
	emptySubmission     bool
	panicSubmit         bool
	panicMessage        string
	submitEntered       chan struct{}
	submitBlock         <-chan struct{}
	submitEnteredOnce   sync.Once
	abortErr            error
	abortPanic          bool
	abortPanicMessage   string
	abortRequests       []MediaPollRequest
	abortCtxCanceled    atomic.Bool
	allowed             map[string]struct{}
	request             MediaExecutionRequest
	requests            []MediaExecutionRequest
	generateHook        func(MediaExecutionRequest)
}

type nonAbortableWorkerAdapter struct {
	name  string
	inner *workerAdapter
}

func (a *nonAbortableWorkerAdapter) Name() string { return a.name }
func (a *nonAbortableWorkerAdapter) Submit(ctx context.Context, req MediaExecutionRequest) (*MediaAsyncSubmission, error) {
	return a.inner.Submit(ctx, req)
}
func (a *nonAbortableWorkerAdapter) Poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error) {
	return a.inner.Poll(ctx, req)
}
func (a *nonAbortableWorkerAdapter) SupportsIdempotentSubmit() bool {
	return a.inner.SupportsIdempotentSubmit()
}

func newWorkerAdapter(name string) *workerAdapter {
	return &workerAdapter{name: name, pollResult: MediaPollResult{State: MediaPollStateCompleted, Result: &MediaGenerateResult{Artifacts: []MediaArtifactInput{{Direction: "output", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("png")}}, Usage: MediaUsage{ImageCount: 1}}}, allowed: map[string]struct{}{}}
}
func (a *workerAdapter) Name() string { return a.name }
func (a *workerAdapter) Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.syncCalls.Add(1)
	a.mu.Lock()
	a.request = req
	result := a.pollResult.Result
	hook := a.generateHook
	a.mu.Unlock()
	if hook != nil {
		hook(req)
	}
	return result, nil
}
func (a *workerAdapter) Submit(ctx context.Context, req MediaExecutionRequest) (*MediaAsyncSubmission, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.submitCalls.Add(1)
	a.mu.Lock()
	submitEntered := a.submitEntered
	submitBlock := a.submitBlock
	a.mu.Unlock()
	if submitEntered != nil {
		a.submitEnteredOnce.Do(func() { close(submitEntered) })
	}
	if submitBlock != nil {
		<-submitBlock
	}
	a.mu.Lock()
	panicSubmit := a.panicSubmit
	panicMessage := a.panicMessage
	a.panicSubmit = false
	if panicMessage == "" {
		panicMessage = "adapter submit panic"
	}
	if panicSubmit {
		a.mu.Unlock()
		panic(panicMessage)
	}
	defer a.mu.Unlock()
	a.request = req
	a.requests = append(a.requests, req)
	if len(a.submitErrors) > 0 {
		err := a.submitErrors[0]
		a.submitErrors = a.submitErrors[1:]
		return nil, err
	}
	if a.submitErr != nil {
		return nil, a.submitErr
	}
	if a.emptySubmission {
		return &MediaAsyncSubmission{}, nil
	}
	a.allowed["upstream-1"] = struct{}{}
	return &MediaAsyncSubmission{UpstreamTaskID: "upstream-1", PollMetadata: json.RawMessage(`{"cursor":0}`)}, nil
}
func (a *workerAdapter) SupportsIdempotentSubmit() bool {
	return a.supportsIdempotency
}
func (a *workerAdapter) Poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error) {
	a.pollCalls.Add(1)
	a.mu.Lock()
	block := a.pollBlock
	result := a.pollResult
	_, allowed := a.allowed[req.UpstreamTaskID]
	a.mu.Unlock()
	if !allowed {
		return nil, errors.New("unknown upstream")
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	copy := result
	return &copy, nil
}
func (a *workerAdapter) Abort(ctx context.Context, req MediaPollRequest) error {
	if err := ctx.Err(); err != nil {
		a.abortCtxCanceled.Store(true)
		return err
	}
	a.abortCalls.Add(1)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.abortRequests = append(a.abortRequests, req)
	if a.abortPanic {
		panic(a.abortPanicMessage)
	}
	if a.abortErr != nil {
		return a.abortErr
	}
	if _, allowed := a.allowed[req.UpstreamTaskID]; !allowed {
		return errors.New("unknown upstream")
	}
	return nil
}
func (a *workerAdapter) setPollResult(result MediaPollResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pollResult = result
}
func (a *workerAdapter) blockPollUntil(ch <-chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pollBlock = ch
}
func (a *workerAdapter) blockSubmitUntil(entered chan struct{}, release <-chan struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.submitEntered = entered
	a.submitBlock = release
}
func (a *workerAdapter) allowUpstream(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed[id] = struct{}{}
}
func (a *workerAdapter) lastRequest() MediaExecutionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.request
}
func (a *workerAdapter) submitRequests() []MediaExecutionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]MediaExecutionRequest(nil), a.requests...)
}
func (a *workerAdapter) lastAbortRequest() MediaPollRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.abortRequests) == 0 {
		return MediaPollRequest{}
	}
	return a.abortRequests[len(a.abortRequests)-1]
}

type workerArtifactWriter struct {
	mu      sync.Mutex
	err     error
	block   <-chan struct{}
	calls   atomic.Int64
	outputs map[int64][]MediaArtifact
}

func (w *workerArtifactWriter) PersistOutputs(ctx context.Context, task *MediaTask, inputs []MediaArtifactInput) ([]MediaArtifact, error) {
	w.calls.Add(1)
	if w.block != nil {
		select {
		case <-w.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return nil, w.err
	}
	if w.outputs == nil {
		w.outputs = map[int64][]MediaArtifact{}
	}
	if existing := w.outputs[task.ID]; existing != nil {
		return append([]MediaArtifact(nil), existing...), nil
	}
	artifacts := make([]MediaArtifact, len(inputs))
	for i, input := range inputs {
		artifacts[i] = MediaArtifact{TaskID: task.ID, Direction: input.Direction, Position: input.Position, MediaType: input.MediaType, ContentType: input.ContentType, SizeBytes: int64(len(input.Data)), StorageStatus: "stored"}
	}
	w.outputs[task.ID] = artifacts
	return append([]MediaArtifact(nil), artifacts...), nil
}

type recordingSettlementCoordinator struct {
	mu          sync.Mutex
	repo        *workerTaskRepository
	settlements int
	retries     int
	failure     MediaFailureSettlement
}

type blockingSettlementCoordinator struct {
	inner       *recordingSettlementCoordinator
	entered     chan struct{}
	release     chan struct{}
	enteredOnce sync.Once
	ctxCanceled atomic.Bool
}

type contextObservingSettlementCoordinator struct {
	inner       *recordingSettlementCoordinator
	ctxCanceled atomic.Bool
}

func (c *contextObservingSettlementCoordinator) SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error {
	if err := ctx.Err(); err != nil {
		c.ctxCanceled.Store(true)
		return err
	}
	return c.inner.SettleSuccess(ctx, task, usage)
}

func (c *contextObservingSettlementCoordinator) SettleFailure(ctx context.Context, task *MediaTask, failure MediaFailureSettlement) error {
	if err := ctx.Err(); err != nil {
		c.ctxCanceled.Store(true)
		return err
	}
	return c.inner.SettleFailure(ctx, task, failure)
}

func (c *contextObservingSettlementCoordinator) RetryPending(ctx context.Context, taskID int64) error {
	return c.inner.RetryPending(ctx, taskID)
}

func (c *blockingSettlementCoordinator) SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error {
	c.enteredOnce.Do(func() { close(c.entered) })
	select {
	case <-c.release:
		return c.inner.SettleSuccess(ctx, task, usage)
	case <-ctx.Done():
		c.ctxCanceled.Store(true)
		return ctx.Err()
	}
}

func (c *blockingSettlementCoordinator) SettleFailure(ctx context.Context, task *MediaTask, failure MediaFailureSettlement) error {
	return c.inner.SettleFailure(ctx, task, failure)
}

func (c *blockingSettlementCoordinator) RetryPending(ctx context.Context, taskID int64) error {
	return c.inner.RetryPending(ctx, taskID)
}

func (c *recordingSettlementCoordinator) SettleSuccess(ctx context.Context, task *MediaTask, usage MediaUsage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settlements++
	return c.persist(ctx, task, MediaSettlementPlan{Type: MediaSettlementTypeSuccess, Usage: &usage})
}
func (c *recordingSettlementCoordinator) SettleFailure(ctx context.Context, task *MediaTask, failure MediaFailureSettlement) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settlements++
	c.failure = failure
	return c.persist(ctx, task, MediaSettlementPlan{Type: MediaSettlementTypeFailure, Failure: &failure})
}
func (c *recordingSettlementCoordinator) RetryPending(context.Context, int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.retries++
	return nil
}
func (c *recordingSettlementCoordinator) settlementCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settlements
}
func (c *recordingSettlementCoordinator) lastFailure() MediaFailureSettlement {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failure
}
func (c *recordingSettlementCoordinator) persist(ctx context.Context, task *MediaTask, plan MediaSettlementPlan) error {
	if c.repo == nil {
		return nil
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	stored := c.repo.mustGet(task.ID)
	_, err = c.repo.UpdateBilling(ctx, task.ID, stored.BillingStatus, map[string]any{
		"settlement_plan": json.RawMessage(encoded),
		"billing_status":  MediaBillingStatusSettled,
	})
	return err
}
