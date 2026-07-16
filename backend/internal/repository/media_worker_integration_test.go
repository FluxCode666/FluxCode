//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaWorkerIntegrationDuplicateDeliverySettlesOnce(t *testing.T) {
	fixture := newIntegrationMediaWorker(t)
	task := fixture.createQueuedTask(t, true)
	require.NoError(t, fixture.queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))
	require.NoError(t, fixture.queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))
	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	fixture.waitForSettledAndAcked(t, task.ID, service.MediaQueuePriorityAsync, 1)
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Equal(t, int64(1), fixture.billing.settlementCount())
	require.Equal(t, int64(1), fixture.billing.settlementAttempts())
	stored, err := fixture.taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	require.NotEmpty(t, stored.SettlementPlan)
	artifacts, err := fixture.artifactRepo.ListByTaskID(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
}

func TestMediaWorkerIntegrationResumesPollWithoutResubmit(t *testing.T) {
	fixture := newIntegrationMediaWorker(t)
	task := fixture.createQueuedTask(t, true)
	claimed, err := fixture.taskRepo.Claim(context.Background(), task.ID, "dead-worker", time.Now().Add(time.Minute), task.Version)
	require.NoError(t, err)
	require.True(t, claimed)
	claimedTask, err := fixture.taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	updated, err := fixture.taskRepo.UpdateClaimed(context.Background(), task.ID, "dead-worker", map[string]any{
		"account_id": fixture.account.ID, "adapter": fixture.adapter.Name(), "upstream_model": "upstream-image",
		"native_async_mode": service.NativeAsyncRequired, "upstream_task_id": "upstream-existing",
		"poll_metadata": json.RawMessage(`{"cursor":1}`), "stage": service.MediaTaskStagePolling,
	})
	require.NoError(t, err)
	require.True(t, updated)
	fixture.adapter.allow("upstream-existing")
	_, err = testEntClient(t).MediaTask.UpdateOneID(task.ID).SetLeaseUntil(time.Now().Add(-time.Minute)).Save(context.Background())
	require.NoError(t, err)
	_ = claimedTask

	require.NoError(t, fixture.worker.RecoverOnce(context.Background()))
	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	fixture.waitForSettledAndAcked(t, task.ID, service.MediaQueuePriorityAsync, 0)
	require.Zero(t, fixture.adapter.submitCalls.Load())
	require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
	require.Equal(t, int64(1), fixture.billing.settlementCount())
	require.Equal(t, int64(1), fixture.billing.settlementAttempts())
	stored, err := fixture.taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusCompleted, stored.Status)
	require.Equal(t, service.MediaBillingStatusSettled, stored.BillingStatus)
	require.NotEmpty(t, stored.SettlementPlan)
	artifacts, err := fixture.artifactRepo.ListByTaskID(context.Background(), task.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
}

func TestMediaWorkerIntegrationFixtureQueueRoundTrip(t *testing.T) {
	fixture := newIntegrationMediaWorker(t)
	task := fixture.createQueuedTask(t, true)
	require.NoError(t, fixture.queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))

	message, err := fixture.queue.Receive(context.Background(), 200*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, task.ID, message.TaskID)
	require.Equal(t, service.MediaQueuePriorityAsync, message.Priority)
	require.NoError(t, fixture.queue.Ack(context.Background(), message))
}

type integrationMediaWorkerFixture struct {
	worker       *service.MediaWorker
	queue        *MediaTaskStream
	taskRepo     service.MediaTaskRepository
	artifactRepo service.MediaArtifactRepository
	adapter      *integrationWorkerAdapter
	billing      *integrationWorkerBillingPort
	metrics      *service.AtomicMediaTaskMetrics
	account      *service.Account
	candidates   json.RawMessage
}

func newIntegrationMediaWorker(t *testing.T) *integrationMediaWorkerFixture {
	t.Helper()
	client := testEntClient(t)
	resetMediaTaskStreamKeys(t)
	queue := NewMediaTaskStream(integrationRedis, fmt.Sprintf("worker-integration-%d", time.Now().UnixNano()), 100*time.Millisecond)
	require.NoError(t, queue.EnsureGroups(context.Background()))
	taskRepo := NewMediaTaskRepository(client)
	artifactRepo := NewMediaArtifactRepository(client)
	adapter := &integrationWorkerAdapter{allowed: map[string]struct{}{}}
	adapters := service.NewMediaAdapterRegistry()
	adapters.Register(adapter.Name(), adapter)
	account := &service.Account{ID: 707, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, Concurrency: 1}
	scheduler := service.NewMediaScheduler(&integrationWorkerAccounts{account: account}, &integrationWorkerSelector{}, adapters)
	models := service.NewMediaModelRegistry(&integrationWorkerModels{})
	require.NoError(t, models.Refresh(context.Background()))
	candidates, err := json.Marshal([]service.MediaAccountCandidateSnapshot{{AccountID: account.ID, Platform: account.Platform, ResolvedModel: service.ResolvedMediaAccountModel{Adapter: adapter.Name(), UpstreamModel: "upstream-image", NativeAsyncMode: service.NativeAsyncRequired}}})
	require.NoError(t, err)
	billing := &integrationWorkerBillingPort{settled: make(map[string]struct{})}
	metrics := service.NewAtomicMediaTaskMetrics()
	worker := service.NewMediaWorker(service.MediaWorkerConfig{WorkerCount: 1, TaskTimeout: time.Second, LeaseTTL: 200 * time.Millisecond, LeaseRenewInterval: 50 * time.Millisecond, PollInterval: time.Millisecond, RecoveryInterval: time.Second, RecoveryBatchSize: 10}, service.MediaWorkerDependencies{
		Tasks: taskRepo, Queue: queue, Scheduler: scheduler, Models: models, Adapters: adapters,
		Artifacts: &integrationArtifactWriter{repo: artifactRepo},
		Billing:   service.NewMediaBillingCoordinator(taskRepo, billing), Metrics: metrics,
	})
	return &integrationMediaWorkerFixture{worker: worker, queue: queue, taskRepo: taskRepo, artifactRepo: artifactRepo, adapter: adapter, billing: billing, metrics: metrics, account: account, candidates: candidates}
}

func (f *integrationMediaWorkerFixture) createQueuedTask(t *testing.T, clientAsync bool) *service.MediaTask {
	t.Helper()
	publicID := fmt.Sprintf("worker-it-%d", time.Now().UnixNano())
	task, err := f.taskRepo.Create(context.Background(), &service.MediaTask{PublicID: publicID, UserID: 1, APIKeyID: 2, GroupID: 3, MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage, RequestedModel: "fake-image", ClientAsync: clientAsync, Status: service.MediaTaskStatusQueued, Stage: service.MediaTaskStageQueued, RequestSpec: json.RawMessage(`{"image":{"prompt":"cat","n":1}}`), CandidateSnapshot: f.candidates, RequestFingerprint: publicID, BillingStatus: service.MediaBillingStatusPrecharged, PrechargedAmount: 1})
	require.NoError(t, err)
	return task
}
func (f *integrationMediaWorkerFixture) waitForSettledAndAcked(t *testing.T, id int64, priority service.MediaQueuePriority, duplicateMessages int64) {
	t.Helper()
	if assert.Eventually(t, func() bool {
		task, err := f.taskRepo.GetByID(context.Background(), id)
		if err != nil || task.Status != service.MediaTaskStatusCompleted || task.BillingStatus != service.MediaBillingStatusSettled {
			return false
		}
		if f.metrics.DuplicateMessages() < duplicateMessages {
			return false
		}
		pending, err := f.queue.PendingCount(context.Background(), priority)
		return err == nil && pending == 0
	}, 3*time.Second, 10*time.Millisecond) {
		return
	}
	task, taskErr := f.taskRepo.GetByID(context.Background(), id)
	pending, pendingErr := f.queue.PendingCount(context.Background(), priority)
	var workerErr error
	select {
	case workerErr = <-f.worker.Errors():
	default:
	}
	t.Fatalf("media worker did not settle and ACK: task=%+v task_err=%v pending=%d pending_err=%v worker_err=%v", task, taskErr, pending, pendingErr, workerErr)
}

type integrationWorkerAdapter struct {
	submitCalls atomic.Int64
	pollCalls   atomic.Int64
	seq         atomic.Int64
	mu          sync.Mutex
	allowed     map[string]struct{}
}

func (a *integrationWorkerAdapter) Name() string { return "integration-fake" }
func (a *integrationWorkerAdapter) Submit(context.Context, service.MediaExecutionRequest) (*service.MediaAsyncSubmission, error) {
	a.submitCalls.Add(1)
	id := fmt.Sprintf("upstream-%d", a.seq.Add(1))
	a.allow(id)
	return &service.MediaAsyncSubmission{UpstreamTaskID: id}, nil
}
func (a *integrationWorkerAdapter) Poll(_ context.Context, req service.MediaPollRequest) (*service.MediaPollResult, error) {
	a.pollCalls.Add(1)
	a.mu.Lock()
	_, ok := a.allowed[req.UpstreamTaskID]
	a.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown upstream %s", req.UpstreamTaskID)
	}
	return &service.MediaPollResult{State: service.MediaPollStateCompleted, Progress: 100, Result: &service.MediaGenerateResult{Artifacts: []service.MediaArtifactInput{{Direction: "output", Position: 0, MediaType: service.MediaTypeImage, ContentType: "image/png", Data: []byte("png")}}, Usage: service.MediaUsage{ImageCount: 1}}}, nil
}
func (a *integrationWorkerAdapter) allow(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed[id] = struct{}{}
}

type integrationWorkerAccounts struct{ account *service.Account }

func (r *integrationWorkerAccounts) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return []service.Account{*r.account}, nil
}
func (r *integrationWorkerAccounts) GetByID(context.Context, int64) (*service.Account, error) {
	copy := *r.account
	return &copy, nil
}
func (*integrationWorkerAccounts) UpdateLastUsed(context.Context, int64) error { return nil }

type integrationWorkerSelector struct{}

func (*integrationWorkerSelector) Select(_ context.Context, req service.AccountCandidateSelectionRequest) (*service.AccountSelectionResult, error) {
	return &service.AccountSelectionResult{Account: req.Candidates[0], Acquired: true, ReleaseFunc: func() {}}, nil
}
func (*integrationWorkerSelector) Wait(context.Context, *service.AccountWaitPlan) (func(), error) {
	return func() {}, nil
}

type integrationWorkerModels struct{}

func (*integrationWorkerModels) ListEnabled(context.Context) ([]service.MediaModelDefinition, error) {
	return []service.MediaModelDefinition{{ModelID: "fake-image", MediaType: service.MediaTypeImage, Operations: []service.MediaOperation{service.MediaOperationTextToImage}, Enabled: true}}, nil
}

type integrationArtifactWriter struct {
	repo service.MediaArtifactRepository
}

func (w *integrationArtifactWriter) PersistOutputs(ctx context.Context, task *service.MediaTask, inputs []service.MediaArtifactInput) ([]service.MediaArtifact, error) {
	artifacts := make([]service.MediaArtifact, 0, len(inputs))
	for _, input := range inputs {
		sum := sha256.Sum256(input.Data)
		artifact, err := w.repo.Create(ctx, &service.MediaArtifact{TaskID: task.ID, Direction: input.Direction, Position: input.Position, MediaType: input.MediaType, ContentType: input.ContentType, SizeBytes: int64(len(input.Data)), ChecksumSHA256: hex.EncodeToString(sum[:]), StorageStatus: "stored"})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, *artifact)
	}
	return artifacts, nil
}

type integrationWorkerBillingPort struct {
	mu       sync.Mutex
	settled  map[string]struct{}
	attempts atomic.Int64
}

func (*integrationWorkerBillingPort) Precharge(context.Context, *service.MediaTask, service.MediaBillingSnapshot) error {
	return nil
}
func (b *integrationWorkerBillingPort) SettleSuccess(_ context.Context, task *service.MediaTask, _ service.MediaUsage) error {
	b.attempts.Add(1)
	b.settle(task, service.MediaSettlementTypeSuccess)
	return nil
}
func (b *integrationWorkerBillingPort) SettleFailure(_ context.Context, task *service.MediaTask, _ service.MediaFailureSettlement) error {
	b.attempts.Add(1)
	b.settle(task, service.MediaSettlementTypeFailure)
	return nil
}
func (b *integrationWorkerBillingPort) settle(task *service.MediaTask, planType service.MediaSettlementType) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.settled[task.PublicID+":"+string(planType)] = struct{}{}
}
func (b *integrationWorkerBillingPort) settlementCount() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return int64(len(b.settled))
}
func (b *integrationWorkerBillingPort) settlementAttempts() int64 {
	return b.attempts.Load()
}
