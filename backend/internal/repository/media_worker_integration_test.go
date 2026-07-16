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
	"github.com/stretchr/testify/require"
)

func TestMediaWorkerIntegrationDuplicateDeliverySettlesOnce(t *testing.T) {
	fixture := newIntegrationMediaWorker(t)
	task := fixture.createQueuedTask(t, true)
	require.NoError(t, fixture.queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))
	require.NoError(t, fixture.queue.Enqueue(context.Background(), task.ID, service.MediaQueuePriorityAsync))
	require.NoError(t, fixture.worker.Start())
	t.Cleanup(fixture.worker.Stop)
	fixture.waitForStatus(t, task.ID, service.MediaTaskStatusCompleted)
	require.Equal(t, int64(1), fixture.adapter.submitCalls.Load())
	require.Equal(t, int64(1), fixture.billing.settlements.Load())
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
	require.NoError(t, fixture.worker.ProcessOne(context.Background(), task.ID))
	require.Zero(t, fixture.adapter.submitCalls.Load())
	require.GreaterOrEqual(t, fixture.adapter.pollCalls.Load(), int64(1))
	stored, err := fixture.taskRepo.GetByID(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, service.MediaTaskStatusCompleted, stored.Status)
}

type integrationMediaWorkerFixture struct {
	worker       *service.MediaWorker
	queue        *MediaTaskStream
	taskRepo     service.MediaTaskRepository
	artifactRepo service.MediaArtifactRepository
	adapter      *integrationWorkerAdapter
	billing      *integrationWorkerBilling
	account      *service.Account
	candidates   json.RawMessage
}

func newIntegrationMediaWorker(t *testing.T) *integrationMediaWorkerFixture {
	t.Helper()
	client := testEntClient(t)
	queue := NewMediaTaskStream(testRedis(t), "worker-integration", 100*time.Millisecond)
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
	billing := &integrationWorkerBilling{}
	worker := service.NewMediaWorker(service.MediaWorkerConfig{WorkerCount: 1, TaskTimeout: time.Second, LeaseTTL: 200 * time.Millisecond, LeaseRenewInterval: 50 * time.Millisecond, PollInterval: time.Millisecond, RecoveryInterval: time.Second, RecoveryBatchSize: 10}, service.MediaWorkerDependencies{
		Tasks: taskRepo, Queue: queue, Scheduler: scheduler, Models: models, Adapters: adapters,
		Artifacts: &integrationArtifactWriter{repo: artifactRepo}, Billing: billing, Metrics: service.NewAtomicMediaTaskMetrics(),
	})
	return &integrationMediaWorkerFixture{worker: worker, queue: queue, taskRepo: taskRepo, artifactRepo: artifactRepo, adapter: adapter, billing: billing, account: account, candidates: candidates}
}

func (f *integrationMediaWorkerFixture) createQueuedTask(t *testing.T, clientAsync bool) *service.MediaTask {
	t.Helper()
	publicID := fmt.Sprintf("worker-it-%d", time.Now().UnixNano())
	task, err := f.taskRepo.Create(context.Background(), &service.MediaTask{PublicID: publicID, UserID: 1, APIKeyID: 2, GroupID: 3, MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage, RequestedModel: "fake-image", ClientAsync: clientAsync, Status: service.MediaTaskStatusQueued, Stage: service.MediaTaskStageQueued, RequestSpec: json.RawMessage(`{"image":{"prompt":"cat","n":1}}`), CandidateSnapshot: f.candidates, RequestFingerprint: publicID, BillingStatus: service.MediaBillingStatusPrecharged, PrechargedAmount: 1})
	require.NoError(t, err)
	return task
}
func (f *integrationMediaWorkerFixture) waitForStatus(t *testing.T, id int64, status service.MediaTaskStatus) {
	t.Helper()
	require.Eventually(t, func() bool {
		task, err := f.taskRepo.GetByID(context.Background(), id)
		return err == nil && task.Status == status
	}, 3*time.Second, 10*time.Millisecond)
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

type integrationWorkerBilling struct{ settlements atomic.Int64 }

func (b *integrationWorkerBilling) SettleSuccess(context.Context, *service.MediaTask, service.MediaUsage) error {
	b.settlements.Add(1)
	return nil
}
func (b *integrationWorkerBilling) SettleFailure(context.Context, *service.MediaTask, service.MediaFailureSettlement) error {
	b.settlements.Add(1)
	return nil
}
func (*integrationWorkerBilling) RetryPending(context.Context, int64) error { return nil }
