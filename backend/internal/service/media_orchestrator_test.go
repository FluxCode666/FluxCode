package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMediaOrchestratorAsyncReturnsAfterDurableEnqueue(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionAccepted, result.Disposition)
	require.Equal(t, MediaQueuePriorityAsync, fixture.queue.lastPriority())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	require.Equal(t, MediaBillingStatusPrecharged, fixture.repo.mustGet(result.Task.ID).BillingStatus)
}

func TestMediaOrchestratorPersistsResolvedCandidateAndDurableInputSnapshot(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.scheduler.candidates = []MediaAccountCandidateSnapshot{{
		AccountID: 7, Platform: PlatformGemini,
		ResolvedModel: ResolvedMediaAccountModel{
			Adapter: "gemini", UpstreamModel: "imagen-4", NativeAsyncMode: NativeAsyncRequired,
		},
	}}
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{{
		Position: 0, MediaType: MediaTypeImage, ContentType: "image/png", ObjectKey: "media/input/source.png",
	}}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	stored := fixture.repo.mustGet(result.Task.ID)
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(stored.CandidateSnapshot, &candidates))
	require.Equal(t, fixture.scheduler.candidates, candidates)
	var spec MediaSpec
	require.NoError(t, json.Unmarshal(stored.RequestSpec, &spec))
	require.Len(t, spec.Image.InputArtifactIDs, 1)
	artifacts, err := fixture.artifacts.ListByTaskID(context.Background(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, "input", artifacts[0].Direction)
	require.Equal(t, "media/input/source.png", artifacts[0].ObjectKey)
}

func TestMediaOrchestratorRejectsRawOrNonRecoverableInputsBeforeTaskAndCharge(t *testing.T) {
	tests := []struct {
		name  string
		input MediaArtifactInput
	}{
		{name: "raw data", input: MediaArtifactInput{Position: 0, MediaType: MediaTypeImage, Data: []byte("raw"), ObjectKey: "key"}},
		{name: "no durable reference", input: MediaArtifactInput{Position: 0, MediaType: MediaTypeImage}},
		{name: "ambiguous references", input: MediaArtifactInput{Position: 0, MediaType: MediaTypeImage, ObjectKey: "key", UpstreamReference: "upstream"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			req := validAsyncMediaCreateRequest()
			req.Inputs = []MediaArtifactInput{tt.input}
			_, err := fixture.orchestrator.Create(context.Background(), req)
			require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
			require.Equal(t, 0, fixture.repo.createCalls())
			require.Equal(t, 0, fixture.billing.prechargeCalls())
		})
	}
}

func TestMediaOrchestratorRejectsPreExistingArtifactIDs(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.Spec.Image.InputArtifactIDs = []int64{999}
	_, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
	require.Equal(t, 0, fixture.repo.createCalls())
}

func TestMediaOrchestratorRejectsContentPolicyBeforeTaskAndCharge(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.contentPolicy.err = ErrMediaContentRejected
	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorIs(t, err, ErrMediaContentRejected)
	require.Equal(t, 0, fixture.repo.createCalls())
	require.Equal(t, 0, fixture.billing.prechargeCalls())
}

func TestMediaOrchestratorChecksGroupMediaPermission(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.groups.group.AllowImageGeneration = false
	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorIs(t, err, ErrMediaGenerationNotAllowed)
	require.Equal(t, 0, fixture.contentPolicy.calls)
	require.Equal(t, 0, fixture.repo.createCalls())
}

func TestMediaOrchestratorIdempotencyKeyReusesTaskWithoutSecondCharge(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-1"
	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.Task.PublicID, second.Task.PublicID)
	require.Equal(t, 1, fixture.billing.prechargeCalls())

	req.Spec.Image.Prompt = "different"
	_, err = fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaIdempotencyConflict)
}

func TestMediaOrchestratorIdempotencyCreateRaceLoserNeverPrecharges(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-race"
	fingerprint, err := mediaCreateFingerprint(req)
	require.NoError(t, err)
	fixture.repo.createRaceWinner = &MediaTask{
		ID: 88, PublicID: "task_winner", UserID: req.UserID, APIKeyID: req.APIKeyID, GroupID: req.GroupID,
		MediaType: req.MediaType, Operation: req.Operation, RequestedModel: req.RequestedModel,
		ClientAsync: true, RequestFingerprint: fingerprint, IdempotencyKey: req.IdempotencyKey,
		Status: MediaTaskStatusQueued, Stage: MediaTaskStageQueued, BillingStatus: MediaBillingStatusPrecharged, Version: 2,
	}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "task_winner", result.Task.PublicID)
	require.Equal(t, 0, fixture.billing.prechargeCalls())
	require.Equal(t, 0, fixture.queue.enqueueCalls())
}

func TestMediaOrchestratorQueueFailureMarksFailedAndRefunds(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.pricing.snapshot.EstimatedAmount = 12.5
	fixture.queue.enqueueErr = errors.New("redis unavailable")

	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorContains(t, err, "redis unavailable")
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, MediaTaskStageFailed, stored.Stage)
	require.Equal(t, "system_queue", stored.ErrorCode)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_queue",
	}, fixture.billing.lastFailure())
}

func TestMediaOrchestratorQueueFailureCompensatesAfterClientCancellation(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.queue.enqueueHook = func(int64) { cancel() }
	fixture.queue.enqueueErr = context.Canceled

	_, err := fixture.orchestrator.Create(ctx, validAsyncMediaCreateRequest())
	require.ErrorIs(t, err, context.Canceled)
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_queue", stored.ErrorCode)
	require.Equal(t, float64(1), fixture.billing.lastFailure().RefundRatio)
}

func TestMediaOrchestratorPrechargeFailureMarksTaskFailedWithoutEnqueue(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.billing.prechargeErr = errors.New("balance unavailable")
	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorContains(t, err, "balance unavailable")
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "billing_precharge", stored.ErrorCode)
	require.Equal(t, 0, fixture.queue.enqueueCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorSyncSubscribePrecedesFirstDBRead(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.queue.enqueueHook = func(taskID int64) {
		fixture.repo.setTerminal(taskID, MediaTaskStatusCompleted)
	}
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Equal(t, []string{"subscribe", "get"}, fixture.events.snapshotTail(2))
}

func TestMediaOrchestratorSyncTimeoutFallbackKeepsTaskRunning(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, result.Disposition)
	stored := fixture.repo.mustGet(result.Task.ID)
	require.True(t, stored.SyncFallback)
	require.NotNil(t, stored.SyncFallbackAt)
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
	require.Equal(t, 0, fixture.controller.stopCalls())
}

func TestMediaOrchestratorFallbackCASLossReturnsRealCompletion(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	fixture.repo.completeOnNextFallback = true
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorSyncTimeoutBeforeSubmitAlwaysRefunds(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.settings.settings.MediaSyncTimeoutBillingPolicy = MediaTimeoutBillingPolicyPenalty
	fixture.settings.settings.MediaSyncTimeoutPenaltyRatio = 0.8
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, result.Disposition)
	require.Empty(t, result.Task.PublicID)
	require.Equal(t, MediaFailureSettlement{
		Kind: MediaFailureKindSyncTimeout, RefundRatio: 1, ErrorCode: "sync_timeout",
	}, fixture.billing.lastFailure())
}

func TestMediaOrchestratorSyncTimeoutPenaltyRequiresSubmittedEligibleStage(t *testing.T) {
	now := time.Now()
	for _, stage := range []MediaTaskStage{MediaTaskStageSubmitting, MediaTaskStageGenerating, MediaTaskStagePolling} {
		t.Run(string(stage), func(t *testing.T) {
			fixture := newTimedOutMediaOrchestratorFixture(t, stage, &now)
			fixture.settings.settings.MediaSyncTimeoutBillingPolicy = MediaTimeoutBillingPolicyPenalty
			fixture.settings.settings.MediaSyncTimeoutPenaltyRatio = 0.8
			result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
			require.NoError(t, err)
			require.Equal(t, MediaCreateDispositionGatewayTimeout, result.Disposition)
			require.InDelta(t, 0.2, fixture.billing.lastFailure().RefundRatio, 1e-9)
			require.InDelta(t, 0.8, fixture.billing.lastFailure().PenaltyRatio, 1e-9)
		})
	}

	for _, stage := range []MediaTaskStage{MediaTaskStageQueued, MediaTaskStageScheduling, MediaTaskStageStoring, MediaTaskStageSettling} {
		t.Run("refund_"+string(stage), func(t *testing.T) {
			fixture := newTimedOutMediaOrchestratorFixture(t, stage, &now)
			fixture.settings.settings.MediaSyncTimeoutBillingPolicy = MediaTimeoutBillingPolicyPenalty
			fixture.settings.settings.MediaSyncTimeoutPenaltyRatio = 0.8
			_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
			require.NoError(t, err)
			require.Equal(t, float64(1), fixture.billing.lastFailure().RefundRatio)
			require.Zero(t, fixture.billing.lastFailure().PenaltyRatio)
		})
	}
}

func TestMediaOrchestratorSyncTimeoutRefundPolicyAlwaysFullyRefunds(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.settings.settings.MediaSyncTimeoutBillingPolicy = MediaTimeoutBillingPolicyRefund
	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, float64(1), fixture.billing.lastFailure().RefundRatio)
	require.Zero(t, fixture.billing.lastFailure().PenaltyRatio)
}

func TestMediaOrchestratorTimeoutBillingUsesFreshReReadState(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.repo.getHook = func(call int, task *MediaTask) {
		if call == 2 {
			task.Stage = MediaTaskStageGenerating
			task.SubmittedAt = mediaTimePointer(time.Now())
			task.Status = MediaTaskStatusInProgress
		}
	}
	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.InDelta(t, 0.8, fixture.billing.lastFailure().PenaltyRatio, 1e-9)
}

func TestMediaOrchestratorZeroTimeoutHasNoApplicationTimer(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.settings.MediaSyncWaitTimeoutSeconds = 0
	ctx, cancel := context.WithCancel(context.Background())
	fixture.queue.subscribeHook = cancel
	_, err := fixture.orchestrator.Create(ctx, validSyncMediaCreateRequest())
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 0, fixture.clock.newTimerCalls())
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorIdempotentRetryAfterFallbackReturnsImmediately(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	req := validSyncMediaCreateRequest()
	req.IdempotencyKey = "idem-fallback"
	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, first.Disposition)
	timers := fixture.clock.newTimerCalls()

	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, second.Disposition)
	require.Equal(t, timers, fixture.clock.newTimerCalls())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
}

func TestMediaOrchestratorTimeoutCASLossReturnsRealCompletion(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.repo.completeOnNextTransition = true
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorClosedSubscriptionRechecksDBThenErrors(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.queue.closeSubscription = true
	fixture.clock.autoFire = false
	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.ErrorIs(t, err, ErrMediaTerminalSubscriptionClosed)
	require.GreaterOrEqual(t, fixture.repo.getCalls(), 2)
}

func TestMediaOrchestratorSettlementFailureDoesNotChangeGatewayTimeoutDecision(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.billing.settleFailureErr = errors.New("billing unavailable")
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, result.Disposition)
	require.Empty(t, result.Task.PublicID)
	require.Equal(t, MediaBillingStatusRetry, fixture.repo.mustGet(result.Task.ID).BillingStatus)
}

func TestMediaOrchestratorGetForUserSanitizesInternalTaskFields(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	created, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	fixture.repo.mutate(created.Task.ID, func(task *MediaTask) {
		accountID := int64(9)
		task.AccountID = &accountID
		task.UpstreamTaskID = "upstream-secret"
		task.PollMetadata = json.RawMessage(`{"secret":true}`)
		task.BillingSnapshot = json.RawMessage(`{"amount":1}`)
	})
	fixture.artifacts.items[created.Task.ID] = []MediaArtifact{{
		ID: 1, TaskID: created.Task.ID, ObjectKey: "private/object", UpstreamReference: "private-upstream", PublicURL: "https://example.test/media",
	}}

	result, err := fixture.orchestrator.GetForUser(context.Background(), created.Task.PublicID, created.Task.UserID)
	require.NoError(t, err)
	require.Nil(t, result.Task.AccountID)
	require.Empty(t, result.Task.UpstreamTaskID)
	require.Empty(t, result.Task.PollMetadata)
	require.Empty(t, result.Task.BillingSnapshot)
	require.Empty(t, result.Task.RequestFingerprint)
	require.Empty(t, result.Task.IdempotencyKey)
	require.Empty(t, result.Artifacts[0].ObjectKey)
	require.Empty(t, result.Artifacts[0].UpstreamReference)
	require.Equal(t, "https://example.test/media", result.Artifacts[0].PublicURL)

	_, err = fixture.orchestrator.GetForUser(context.Background(), created.Task.PublicID, created.Task.UserID+1)
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
}

func validAsyncMediaCreateRequest() MediaCreateRequest {
	return MediaCreateRequest{
		UserID: 1, APIKeyID: 2, GroupID: 3,
		MediaType: MediaTypeImage, Operation: MediaOperationTextToImage,
		RequestedModel: "fake-image", Spec: MediaSpec{Image: &ImageSpec{Prompt: "cat", Count: 1}},
		ClientAsync: true,
	}
}

func validSyncMediaCreateRequest() MediaCreateRequest {
	req := validAsyncMediaCreateRequest()
	req.ClientAsync = false
	return req
}

type mediaOrchestratorFixture struct {
	orchestrator  *MediaOrchestrator
	repo          *orchestratorTaskRepository
	artifacts     *orchestratorArtifactRepository
	queue         *orchestratorQueue
	scheduler     *orchestratorScheduler
	settings      *orchestratorSettings
	groups        *orchestratorGroups
	contentPolicy *orchestratorContentPolicy
	pricing       *orchestratorPricing
	billing       *orchestratorBilling
	controller    *orchestratorController
	clock         *orchestratorClock
	events        *orchestratorEvents
}

func newMediaOrchestratorFixture(t *testing.T) *mediaOrchestratorFixture {
	t.Helper()
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{validImageModelDefinition()}})
	require.NoError(t, registry.Refresh(context.Background()))
	repo := newOrchestratorTaskRepository()
	artifacts := newOrchestratorArtifactRepository()
	events := &orchestratorEvents{}
	repo.events = events
	queue := &orchestratorQueue{repo: repo, events: events}
	scheduler := &orchestratorScheduler{candidates: []MediaAccountCandidateSnapshot{{
		AccountID: 7, Platform: "fake", ResolvedModel: ResolvedMediaAccountModel{
			Adapter: "fake", UpstreamModel: "fake-image-upstream", NativeAsyncMode: NativeAsyncOptional,
		},
	}}}
	settings := &orchestratorSettings{settings: SystemSettings{
		MediaSyncWaitTimeoutSeconds: 1, MediaSyncTimeoutFallbackAsyncEnabled: false,
		MediaSyncTimeoutBillingPolicy: MediaTimeoutBillingPolicyPenalty, MediaSyncTimeoutPenaltyRatio: 0.8,
	}}
	groups := &orchestratorGroups{group: &Group{ID: 3, AllowImageGeneration: true, AllowVideoGeneration: true}}
	contentPolicy := &orchestratorContentPolicy{}
	pricing := &orchestratorPricing{snapshot: MediaBillingSnapshot{RequestedModel: "fake-image"}}
	billing := &orchestratorBilling{}
	controller := &orchestratorController{}
	clock := &orchestratorClock{autoFire: true, now: time.Now()}
	fixture := &mediaOrchestratorFixture{
		repo: repo, artifacts: artifacts, queue: queue, scheduler: scheduler, settings: settings,
		groups: groups, contentPolicy: contentPolicy, pricing: pricing, billing: billing,
		controller: controller, clock: clock, events: events,
	}
	fixture.orchestrator = NewMediaOrchestrator(MediaOrchestratorDependencies{
		Registry: registry, Groups: groups, Scheduler: scheduler, Settings: settings,
		ContentPolicy: contentPolicy, Pricing: pricing, Tasks: repo, Artifacts: artifacts,
		Billing: billing, Settlement: NewMediaBillingCoordinator(repo, billing), Queue: queue,
		Controller: controller, Clock: clock,
		PublicIDGenerator: func() (string, error) { return fmt.Sprintf("task_%d", repo.nextPublicID()), nil },
	})
	return fixture
}

func newTimedOutMediaOrchestratorFixture(t *testing.T, stage MediaTaskStage, submittedAt *time.Time) *mediaOrchestratorFixture {
	t.Helper()
	fixture := newMediaOrchestratorFixture(t)
	fixture.queue.enqueueHook = func(taskID int64) {
		fixture.repo.mutate(taskID, func(task *MediaTask) {
			task.Stage = stage
			task.SubmittedAt = submittedAt
			if stage != MediaTaskStageQueued {
				task.Status = MediaTaskStatusInProgress
			}
		})
	}
	return fixture
}

type orchestratorTaskRepository struct {
	mu                       sync.Mutex
	tasks                    map[int64]*MediaTask
	nextID                   int64
	publicSequence           int64
	creates                  int
	gets                     int
	createRaceWinner         *MediaTask
	completeOnNextTransition bool
	completeOnNextFallback   bool
	events                   *orchestratorEvents
	getHook                  func(int, *MediaTask)
}

func newOrchestratorTaskRepository() *orchestratorTaskRepository {
	return &orchestratorTaskRepository{tasks: make(map[int64]*MediaTask), nextID: 1}
}

func (r *orchestratorTaskRepository) Create(_ context.Context, task *MediaTask) (*MediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creates++
	if r.createRaceWinner != nil {
		winner := cloneOrchestratorTask(r.createRaceWinner)
		r.tasks[winner.ID] = winner
		r.createRaceWinner = nil
		return nil, errors.New("unique idempotency constraint")
	}
	for _, existing := range r.tasks {
		if task.IdempotencyKey != "" && existing.UserID == task.UserID && existing.APIKeyID == task.APIKeyID && existing.IdempotencyKey == task.IdempotencyKey {
			return nil, errors.New("unique idempotency constraint")
		}
	}
	copy := cloneOrchestratorTask(task)
	copy.ID = r.nextID
	r.nextID++
	if copy.Version == 0 {
		copy.Version = 1
	}
	r.tasks[copy.ID] = copy
	return cloneOrchestratorTask(copy), nil
}

func (r *orchestratorTaskRepository) GetByID(ctx context.Context, id int64) (*MediaTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gets++
	if r.events != nil {
		r.events.add("get")
	}
	task, ok := r.tasks[id]
	if !ok {
		return nil, ErrMediaTaskNotFound
	}
	if r.getHook != nil {
		r.getHook(r.gets, task)
	}
	return cloneOrchestratorTask(task), nil
}

func (r *orchestratorTaskRepository) GetByPublicIDForUser(ctx context.Context, publicID string, userID int64) (*MediaTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.PublicID == publicID && task.UserID == userID {
			return cloneOrchestratorTask(task), nil
		}
	}
	return nil, ErrMediaTaskNotFound
}

func (r *orchestratorTaskRepository) GetByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*MediaTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.UserID == userID && task.APIKeyID == apiKeyID && task.IdempotencyKey == key {
			return cloneOrchestratorTask(task), nil
		}
	}
	return nil, ErrMediaTaskNotFound
}

func (r *orchestratorTaskRepository) UpdateQueued(ctx context.Context, id, version int64, updates map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.Status != MediaTaskStatusQueued || task.Version != version {
		return false, nil
	}
	applyOrchestratorTaskUpdates(task, updates)
	task.Version++
	return true, nil
}

func (r *orchestratorTaskRepository) Claim(context.Context, int64, string, time.Time, int64) (bool, error) {
	return false, errors.New("not implemented")
}
func (r *orchestratorTaskRepository) RenewLease(context.Context, int64, string, time.Time) (bool, error) {
	return false, errors.New("not implemented")
}
func (r *orchestratorTaskRepository) UpdateClaimed(context.Context, int64, string, map[string]any) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *orchestratorTaskRepository) Transition(ctx context.Context, id int64, from, to MediaTaskStatus, updates map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.Status != from {
		return false, nil
	}
	if r.completeOnNextTransition {
		r.completeOnNextTransition = false
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		return false, nil
	}
	task.Status = to
	applyOrchestratorTaskUpdates(task, updates)
	return true, nil
}

func (r *orchestratorTaskRepository) TransitionClaimed(context.Context, int64, string, int64, MediaTaskStatus, MediaTaskStatus, map[string]any) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *orchestratorTaskRepository) MarkSyncFallback(ctx context.Context, id int64, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.Status.IsTerminal() || task.SyncFallback {
		return false, nil
	}
	if r.completeOnNextFallback {
		r.completeOnNextFallback = false
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		return false, nil
	}
	task.SyncFallback = true
	task.SyncFallbackAt = mediaTimePointer(at)
	return true, nil
}

func (r *orchestratorTaskRepository) ListRecoverable(context.Context, time.Time, int) ([]MediaTask, error) {
	return nil, nil
}
func (r *orchestratorTaskRepository) ListSettlementPending(context.Context, int) ([]MediaTask, error) {
	return nil, nil
}

func (r *orchestratorTaskRepository) UpdateBilling(ctx context.Context, id int64, fromStatus string, updates map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.BillingStatus != fromStatus {
		return false, nil
	}
	applyOrchestratorTaskUpdates(task, updates)
	return true, nil
}

func (r *orchestratorTaskRepository) nextPublicID() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publicSequence++
	return r.publicSequence
}
func (r *orchestratorTaskRepository) createCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.creates
}
func (r *orchestratorTaskRepository) getCalls() int { r.mu.Lock(); defer r.mu.Unlock(); return r.gets }
func (r *orchestratorTaskRepository) mustGet(id int64) *MediaTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneOrchestratorTask(r.tasks[id])
}
func (r *orchestratorTaskRepository) onlyTask() *MediaTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		return cloneOrchestratorTask(task)
	}
	return nil
}
func (r *orchestratorTaskRepository) mutate(id int64, mutate func(*MediaTask)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mutate(r.tasks[id])
}
func (r *orchestratorTaskRepository) setTerminal(id int64, status MediaTaskStatus) {
	r.mutate(id, func(task *MediaTask) {
		task.Status = status
		if status == MediaTaskStatusCompleted {
			task.Stage = MediaTaskStageCompleted
		} else {
			task.Stage = MediaTaskStageFailed
		}
	})
}

func applyOrchestratorTaskUpdates(task *MediaTask, updates map[string]any) {
	for field, value := range updates {
		switch field {
		case "request_spec":
			task.RequestSpec = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "billing_status":
			task.BillingStatus = value.(string)
		case "billing_snapshot":
			task.BillingSnapshot = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "settlement_plan":
			task.SettlementPlan = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "precharged_amount":
			task.PrechargedAmount = value.(float64)
		case "final_amount":
			task.FinalAmount = value.(float64)
		case "refunded_amount":
			task.RefundedAmount = value.(float64)
		case "stage":
			task.Stage = value.(MediaTaskStage)
		case "progress":
			task.Progress = value.(int)
		case "error_code":
			task.ErrorCode = value.(string)
		case "error_message":
			task.ErrorMessage = value.(string)
		case "finished_at":
			task.FinishedAt = mediaTimePointer(value.(time.Time))
		}
	}
}

func cloneOrchestratorTask(task *MediaTask) *MediaTask {
	if task == nil {
		return nil
	}
	copy := *task
	copy.RequestSpec = append(json.RawMessage(nil), task.RequestSpec...)
	copy.CandidateSnapshot = append(json.RawMessage(nil), task.CandidateSnapshot...)
	copy.BillingSnapshot = append(json.RawMessage(nil), task.BillingSnapshot...)
	copy.SettlementPlan = append(json.RawMessage(nil), task.SettlementPlan...)
	copy.PollMetadata = append(json.RawMessage(nil), task.PollMetadata...)
	copy.AccountID = cloneOrchestratorInt64(task.AccountID)
	copy.SyncFallbackAt = cloneOrchestratorTime(task.SyncFallbackAt)
	copy.SubmittedAt = cloneOrchestratorTime(task.SubmittedAt)
	copy.FinishedAt = cloneOrchestratorTime(task.FinishedAt)
	return &copy
}

func cloneOrchestratorInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func cloneOrchestratorTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type orchestratorArtifactRepository struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64][]MediaArtifact
}

func newOrchestratorArtifactRepository() *orchestratorArtifactRepository {
	return &orchestratorArtifactRepository{nextID: 1, items: make(map[int64][]MediaArtifact)}
}
func (r *orchestratorArtifactRepository) Create(ctx context.Context, artifact *MediaArtifact) (*MediaArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *artifact
	copy.ID = r.nextID
	r.nextID++
	r.items[copy.TaskID] = append(r.items[copy.TaskID], copy)
	return &copy, nil
}
func (r *orchestratorArtifactRepository) ListByTaskID(ctx context.Context, taskID int64) ([]MediaArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]MediaArtifact(nil), r.items[taskID]...), nil
}

type orchestratorQueue struct {
	mu                sync.Mutex
	repo              *orchestratorTaskRepository
	events            *orchestratorEvents
	priority          MediaQueuePriority
	enqueues          int
	enqueueErr        error
	enqueueHook       func(int64)
	subscribeHook     func()
	closeSubscription bool
}

func (q *orchestratorQueue) EnsureGroups(context.Context) error { return nil }
func (q *orchestratorQueue) Enqueue(ctx context.Context, taskID int64, priority MediaQueuePriority) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	q.priority = priority
	q.enqueues++
	hook, err := q.enqueueHook, q.enqueueErr
	q.mu.Unlock()
	if hook != nil {
		hook(taskID)
	}
	return err
}
func (q *orchestratorQueue) Receive(context.Context, time.Duration) (*MediaQueueMessage, error) {
	return nil, errors.New("not implemented")
}
func (q *orchestratorQueue) Ack(context.Context, *MediaQueueMessage) error {
	return errors.New("not implemented")
}
func (q *orchestratorQueue) PublishTerminal(context.Context, int64, MediaTaskStatus) error {
	return nil
}
func (q *orchestratorQueue) SubscribeTerminal(ctx context.Context, _ int64) (<-chan MediaTaskStatus, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	q.events.add("subscribe")
	if q.subscribeHook != nil {
		q.subscribeHook()
	}
	ch := make(chan MediaTaskStatus)
	if q.closeSubscription {
		close(ch)
	}
	return ch, func() {}, nil
}
func (q *orchestratorQueue) lastPriority() MediaQueuePriority {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.priority
}
func (q *orchestratorQueue) enqueueCalls() int { q.mu.Lock(); defer q.mu.Unlock(); return q.enqueues }

type orchestratorScheduler struct {
	candidates []MediaAccountCandidateSnapshot
	err        error
}

func (s *orchestratorScheduler) SnapshotCandidates(context.Context, int64, string) ([]MediaAccountCandidateSnapshot, error) {
	return append([]MediaAccountCandidateSnapshot(nil), s.candidates...), s.err
}

type orchestratorSettings struct {
	settings SystemSettings
	err      error
}

func (s *orchestratorSettings) GetAllSettings(context.Context) (*SystemSettings, error) {
	copy := s.settings
	return &copy, s.err
}

type orchestratorGroups struct {
	group *Group
	err   error
}

func (g *orchestratorGroups) GetByID(context.Context, int64) (*Group, error) {
	if g.err != nil {
		return nil, g.err
	}
	copy := *g.group
	return &copy, nil
}

type orchestratorContentPolicy struct {
	err   error
	calls int
}

func (p *orchestratorContentPolicy) Check(context.Context, int64, MediaType, MediaSpec) error {
	p.calls++
	return p.err
}

type orchestratorPricing struct {
	snapshot MediaBillingSnapshot
	err      error
}

func (p *orchestratorPricing) Snapshot(context.Context, MediaCreateRequest, *MediaModelDefinition, []MediaAccountCandidateSnapshot) (MediaBillingSnapshot, error) {
	return p.snapshot, p.err
}

type orchestratorBilling struct {
	mu               sync.Mutex
	precharges       int
	failures         []MediaFailureSettlement
	settleFailureErr error
	prechargeErr     error
}

func (b *orchestratorBilling) Precharge(context.Context, *MediaTask, MediaBillingSnapshot) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.precharges++
	return b.prechargeErr
}
func (b *orchestratorBilling) SettleSuccess(context.Context, *MediaTask, MediaUsage) error {
	return nil
}
func (b *orchestratorBilling) SettleFailure(_ context.Context, _ *MediaTask, settlement MediaFailureSettlement) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = append(b.failures, settlement)
	return b.settleFailureErr
}
func (b *orchestratorBilling) prechargeCalls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.precharges
}
func (b *orchestratorBilling) settleFailureCalls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.failures)
}
func (b *orchestratorBilling) lastFailure() MediaFailureSettlement {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.failures) == 0 {
		return MediaFailureSettlement{}
	}
	return b.failures[len(b.failures)-1]
}

type orchestratorController struct {
	mu    sync.Mutex
	stops int
}

func (c *orchestratorController) StopForSyncTimeout(int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stops++
	return true
}
func (c *orchestratorController) stopCalls() int { c.mu.Lock(); defer c.mu.Unlock(); return c.stops }

type orchestratorTimer struct {
	channel <-chan time.Time
	stopped bool
	mu      sync.Mutex
}

func (t *orchestratorTimer) Channel() <-chan time.Time { return t.channel }
func (t *orchestratorTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasActive := !t.stopped
	t.stopped = true
	return wasActive
}

type orchestratorClock struct {
	mu         sync.Mutex
	now        time.Time
	autoFire   bool
	timerCalls int
}

func (c *orchestratorClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *orchestratorClock) NewTimer(time.Duration) MediaTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timerCalls++
	ch := make(chan time.Time, 1)
	if c.autoFire {
		ch <- c.now
	}
	return &orchestratorTimer{channel: ch}
}
func (c *orchestratorClock) newTimerCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timerCalls
}

type orchestratorEvents struct {
	mu     sync.Mutex
	values []string
}

func (e *orchestratorEvents) add(value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.values = append(e.values, value)
}
func (e *orchestratorEvents) snapshotTail(count int) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if count > len(e.values) {
		count = len(e.values)
	}
	return append([]string(nil), e.values[len(e.values)-count:]...)
}
