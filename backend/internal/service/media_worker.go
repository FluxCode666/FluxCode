package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrMediaTaskNotClaimed       = errors.New("media task was not claimed")
	ErrMediaWorkerLeaseLost      = errors.New("media worker lease lost")
	ErrMediaTaskDeadlineExceeded = errors.New("media task deployment timeout exceeded")
	ErrMediaSyncTimeoutStopped   = errors.New("media sync timeout stopped local execution")
	errMediaWorkerMessagePanic   = errors.New("media worker message panic")
)

type MediaWorkerConfig struct {
	WorkerCount        int
	TaskTimeout        time.Duration
	LeaseTTL           time.Duration
	LeaseRenewInterval time.Duration
	PollInterval       time.Duration
	RecoveryInterval   time.Duration
	RecoveryBatchSize  int
}

type MediaExecutionPath string

const (
	MediaExecutionPathSync        MediaExecutionPath = "sync"
	MediaExecutionPathNativeAsync MediaExecutionPath = "native_async"
)

type MediaExecutionController interface {
	StopForSyncTimeout(taskID int64) bool
}

type MediaTaskMetrics interface {
	ObserveStage(mediaType MediaType, stage MediaTaskStage, elapsed time.Duration)
	IncrementRecovery(mediaType MediaType)
	IncrementDuplicateMessage(mediaType MediaType)
	IncrementStorageFailure(mediaType MediaType)
	IncrementSettlementRetry(mediaType MediaType)
}

type MediaArtifactWriter interface {
	PersistOutputs(ctx context.Context, task *MediaTask, inputs []MediaArtifactInput) ([]MediaArtifact, error)
}

type MediaWorkerDependencies struct {
	Tasks     MediaTaskRepository
	Queue     MediaTaskQueue
	Scheduler *MediaScheduler
	Models    *MediaModelRegistry
	Adapters  *MediaAdapterRegistry
	Artifacts MediaArtifactWriter
	Billing   MediaSettlementCoordinator
	Metrics   MediaTaskMetrics
	Logger    *slog.Logger
}

type MediaWorker struct {
	cfg       MediaWorkerConfig
	deps      MediaWorkerDependencies
	workerID  string
	logger    *slog.Logger
	errEvents chan error

	activeMu sync.RWMutex
	active   map[int64]*mediaActiveExecution

	lifecycleMu sync.Mutex
	runCancel   context.CancelFunc
	runWG       sync.WaitGroup
	started     bool
}

type mediaActiveExecution struct {
	ctx           context.Context
	cancel        context.CancelCauseFunc
	terminalizing atomic.Bool
	abortOnce     sync.Once

	mu             sync.RWMutex
	path           MediaExecutionPath
	account        *Account
	adapter        MediaAdapter
	upstreamTaskID string
	pollMetadata   json.RawMessage
	stopRequested  bool
}

type mediaActiveAbortTarget struct {
	ctx            context.Context
	account        *Account
	adapter        MediaAdapter
	upstreamTaskID string
	pollMetadata   json.RawMessage
}

type mediaExecutionFailure struct {
	settlement MediaFailureSettlement
	message    string
	category   string
}

type mediaAdapterRetryMode uint8

const (
	mediaAdapterRetryNone mediaAdapterRetryMode = iota
	mediaAdapterRetryDifferentAccount
	mediaAdapterRetrySameSelection
)

type mediaTaskRecoveryMode uint8

const (
	mediaTaskRecoveryReschedule mediaTaskRecoveryMode = iota
	mediaTaskRecoveryExistingUpstream
	mediaTaskRecoveryUnknownSubmission
	mediaTaskRecoveryUnknownResult
)

type mediaExecutionTrace struct {
	durations map[MediaTaskStage]time.Duration
	pollCount int
}

var mediaWorkerSequence atomic.Uint64

const mediaUpstreamCleanupTimeout = 5 * time.Second

func NewMediaWorker(cfg MediaWorkerConfig, deps MediaWorkerDependencies) *MediaWorker {
	cfg = normalizeMediaWorkerConfig(cfg)
	if deps.Metrics == nil {
		deps.Metrics = NewAtomicMediaTaskMetrics()
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &MediaWorker{
		cfg:       cfg,
		deps:      deps,
		workerID:  fmt.Sprintf("media-worker-%d-%d", os.Getpid(), mediaWorkerSequence.Add(1)),
		logger:    logger,
		errEvents: make(chan error, 64),
		active:    make(map[int64]*mediaActiveExecution),
	}
}

func normalizeMediaWorkerConfig(cfg MediaWorkerConfig) MediaWorkerConfig {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}
	if cfg.TaskTimeout <= 0 {
		cfg.TaskTimeout = 15 * time.Minute
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = time.Minute
	}
	if cfg.LeaseRenewInterval <= 0 || cfg.LeaseRenewInterval >= cfg.LeaseTTL {
		cfg.LeaseRenewInterval = cfg.LeaseTTL / 3
		if cfg.LeaseRenewInterval <= 0 {
			cfg.LeaseRenewInterval = time.Second
		}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.RecoveryInterval <= 0 {
		cfg.RecoveryInterval = 30 * time.Second
	}
	if cfg.RecoveryBatchSize <= 0 {
		cfg.RecoveryBatchSize = 100
	}
	return cfg
}

func (w *MediaWorker) Start() error {
	if err := w.validateDependencies(); err != nil {
		return err
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	if w.started {
		return nil
	}
	startupCtx, startupCancel := context.WithTimeout(context.Background(), w.cfg.LeaseTTL)
	err := w.deps.Queue.EnsureGroups(startupCtx)
	startupCancel()
	if err != nil {
		return fmt.Errorf("ensure media worker consumer groups: %w", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	w.runCancel = cancel
	w.started = true
	for index := 0; index < w.cfg.WorkerCount; index++ {
		workerIndex := index
		w.runWG.Add(1)
		go w.runOwned(runCtx, fmt.Sprintf("consumer-%d", workerIndex), w.consumeLoop)
	}
	w.runWG.Add(1)
	go w.runOwned(runCtx, "recovery", w.recoveryLoop)
	return nil
}

func (w *MediaWorker) Stop() {
	if w == nil {
		return
	}
	w.lifecycleMu.Lock()
	if !w.started {
		w.lifecycleMu.Unlock()
		return
	}
	cancel := w.runCancel
	cancel()
	w.cancelAllActive(context.Canceled)
	w.runWG.Wait()
	w.started = false
	w.runCancel = nil
	w.lifecycleMu.Unlock()
}

func (w *MediaWorker) Errors() <-chan error {
	if w == nil {
		return nil
	}
	return w.errEvents
}

func (w *MediaWorker) runOwned(ctx context.Context, owner string, run func(context.Context) error) {
	defer w.runWG.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			w.reportError(fmt.Errorf("media worker %s panic: %v\n%s", owner, recovered, debug.Stack()))
		}
	}()
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		w.reportError(fmt.Errorf("media worker %s stopped: %w", owner, err))
	}
}

func (w *MediaWorker) consumeLoop(ctx context.Context) error {
	block := w.cfg.PollInterval
	if block < 10*time.Millisecond {
		block = 10 * time.Millisecond
	}
	for {
		message, err := w.deps.Queue.Receive(ctx, block)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, ErrMediaQueueReceiveTimeout) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			w.reportError(fmt.Errorf("receive media task: %w", err))
			continue
		}
		if err := w.processMessage(ctx, message); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.reportError(err)
		}
	}
}

func (w *MediaWorker) processMessage(ctx context.Context, message *MediaQueueMessage) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: media task %d: %v\n%s", errMediaWorkerMessagePanic, message.TaskID, recovered, debug.Stack())
		}
	}()
	if err := w.ProcessOne(ctx, message.TaskID); err != nil {
		return fmt.Errorf("process media task %d: %w", message.TaskID, err)
	}
	if err := w.deps.Queue.Ack(ctx, message); err != nil {
		return fmt.Errorf("ack media task %d: %w", message.TaskID, err)
	}
	return nil
}

func (w *MediaWorker) recoveryLoop(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.RecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.RecoverOnce(ctx); err != nil && ctx.Err() == nil {
				w.reportError(fmt.Errorf("recover media tasks: %w", err))
			}
		}
	}
}

func (w *MediaWorker) ProcessOne(ctx context.Context, taskID int64) error {
	if err := w.validateDependencies(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	task, err := w.deps.Tasks.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load media task %d: %w", taskID, err)
	}
	if task.Status.IsTerminal() {
		w.deps.Metrics.IncrementDuplicateMessage(task.MediaType)
		return w.retryTerminalSettlement(ctx, task)
	}
	recoveryMode := classifyMediaTaskRecovery(task)

	now := time.Now().UTC()
	claimed, err := w.deps.Tasks.Claim(ctx, task.ID, w.workerID, now.Add(w.cfg.LeaseTTL), task.Version)
	if err != nil {
		return fmt.Errorf("claim media task %d: %w", task.ID, err)
	}
	if !claimed {
		fresh, loadErr := w.deps.Tasks.GetByID(ctx, task.ID)
		if loadErr != nil {
			return fmt.Errorf("reload unclaimed media task %d: %w", task.ID, loadErr)
		}
		if fresh.Status.IsTerminal() {
			w.deps.Metrics.IncrementDuplicateMessage(fresh.MediaType)
			return w.retryTerminalSettlement(ctx, fresh)
		}
		return fmt.Errorf("%w: task %d", ErrMediaTaskNotClaimed, task.ID)
	}
	task.Status = MediaTaskStatusInProgress
	task.WorkerID = w.workerID
	task.LeaseUntil = mediaTimePointer(now.Add(w.cfg.LeaseTTL))
	task.Version++

	executionCtx, executionCancel := context.WithCancelCause(ctx)
	active := &mediaActiveExecution{ctx: executionCtx, cancel: executionCancel}
	if !w.registerActive(task.ID, active) {
		executionCancel(ErrMediaTaskNotClaimed)
		return fmt.Errorf("%w: task %d already active", ErrMediaTaskNotClaimed, task.ID)
	}
	leaseCtx, leaseCancel := context.WithCancel(executionCtx)
	renewerDone := w.startLeaseRenewer(leaseCtx, active, task.ID)
	var stopLeaseRenewerOnce sync.Once
	stopLeaseRenewer := func() {
		stopLeaseRenewerOnce.Do(func() {
			leaseCancel()
			<-renewerDone
		})
	}
	defer func() {
		stopLeaseRenewer()
		executionCancel(context.Canceled)
		w.unregisterActive(task.ID, active)
	}()

	startedAt := now
	if task.StartedAt != nil {
		startedAt = task.StartedAt.UTC()
	}
	updates := make(map[string]any, 2)
	if recoveryMode == mediaTaskRecoveryReschedule {
		updates["stage"] = MediaTaskStageScheduling
	}
	if task.StartedAt == nil {
		updates["started_at"] = startedAt
	}
	if len(updates) > 0 {
		if err := w.updateClaimed(executionCtx, task, updates); err != nil {
			return err
		}
	}
	if recoveryMode == mediaTaskRecoveryReschedule {
		task.Stage = MediaTaskStageScheduling
	}
	task.StartedAt = mediaTimePointer(startedAt)

	taskCtx, taskCancel := context.WithDeadlineCause(executionCtx, startedAt.Add(w.cfg.TaskTimeout), ErrMediaTaskDeadlineExceeded)
	defer taskCancel()
	trace := &mediaExecutionTrace{durations: make(map[MediaTaskStage]time.Duration)}
	var result *MediaGenerateResult
	var failure *mediaExecutionFailure
	var executeErr error
	if recoveryMode == mediaTaskRecoveryUnknownResult {
		failure = upstreamMediaFailure("upstream_generate_failed", "media generation result is unknown")
	} else {
		result, failure, executeErr = w.execute(taskCtx, task, active, trace)
	}
	if executeErr != nil {
		cause := context.Cause(taskCtx)
		switch {
		case errors.Is(cause, ErrMediaTaskDeadlineExceeded) && ctx.Err() == nil:
			failure = &mediaExecutionFailure{
				settlement: MediaFailureSettlement{Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_timeout"},
				message:    "media task exceeded deployment timeout",
				category:   "system_timeout",
			}
		case errors.Is(context.Cause(executionCtx), ErrMediaSyncTimeoutStopped):
			return fmt.Errorf("task %d: %w", task.ID, ErrMediaSyncTimeoutStopped)
		case errors.Is(context.Cause(executionCtx), ErrMediaWorkerLeaseLost):
			return fmt.Errorf("task %d: %w", task.ID, context.Cause(executionCtx))
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			return executeErr
		}
	}
	if failure != nil {
		return w.completeFailure(ctx, task, failure, trace, active, stopLeaseRenewer)
	}
	if result == nil {
		return errors.New("media adapter completed without a result")
	}
	completeErr := w.completeSuccess(taskCtx, task, result, trace, active, stopLeaseRenewer)
	if completeErr != nil && errors.Is(context.Cause(taskCtx), ErrMediaTaskDeadlineExceeded) && ctx.Err() == nil && !task.Status.IsTerminal() {
		return w.completeFailure(ctx, task, &mediaExecutionFailure{
			settlement: MediaFailureSettlement{Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: "system_timeout"},
			message:    "media task exceeded deployment timeout",
			category:   "system_timeout",
		}, trace, active, stopLeaseRenewer)
	}
	return completeErr
}

func (w *MediaWorker) execute(ctx context.Context, task *MediaTask, active *mediaActiveExecution, trace *mediaExecutionTrace) (*MediaGenerateResult, *mediaExecutionFailure, error) {
	stageStarted := time.Now()
	if task.UpstreamTaskID != "" {
		w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
		return w.resumePolling(ctx, task, active, trace)
	}
	if isUnknownSubmissionRecovery(task) {
		w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
		return w.resumeUnknownSubmission(ctx, task, active, trace)
	}
	spec, err := decodeWorkerMediaSpec(task.RequestSpec, task.MediaType)
	if err != nil {
		w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
		return nil, systemMediaFailure("system_request", "stored media request is invalid"), nil
	}
	definition, err := w.deps.Models.Resolve(task.RequestedModel, task.Operation)
	if err != nil {
		w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
		return nil, systemMediaFailure("system_model", "stored media model is unavailable"), nil
	}
	candidates, err := decodeWorkerCandidateSnapshot(task.CandidateSnapshot)
	if err != nil {
		w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
		return nil, systemMediaFailure("system_scheduler", "media candidate snapshot is invalid"), nil
	}

	excluded := make(map[int64]struct{})
	var lastFailure *mediaExecutionFailure
	for len(excluded) < len(candidates) {
		selection, selectErr := w.deps.Scheduler.Select(ctx, MediaScheduleRequest{
			GroupID: task.GroupID, RequestedModel: task.RequestedModel, Operation: task.Operation,
			SessionHash: task.RequestFingerprint, SlotID: MediaTaskSlotID(task.PublicID),
			ExcludedAccountIDs: excluded, CandidateSnapshot: candidates,
		})
		if selectErr != nil {
			w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if lastFailure != nil {
				return nil, lastFailure, nil
			}
			return nil, systemMediaFailure("system_scheduler", "no media account is available"), nil
		}
		release, acquireErr := w.acquireSelection(ctx, selection)
		if acquireErr != nil {
			w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			return nil, systemMediaFailure("system_scheduler", "media account concurrency is unavailable"), nil
		}
		w.observeStage(task, trace, MediaTaskStageScheduling, stageStarted)
		result, failure, retry, executeErr := func() (*MediaGenerateResult, *mediaExecutionFailure, bool, error) {
			defer release()
			return w.executeSelected(ctx, task, active, trace, spec, definition, selection)
		}()
		if executeErr != nil {
			return nil, nil, executeErr
		}
		if !retry {
			return result, failure, nil
		}
		lastFailure = failure
		excluded[selection.Account.ID] = struct{}{}
		task.RetryCount++
		if err := w.updateClaimed(ctx, task, map[string]any{
			"retry_count": task.RetryCount,
			"stage":       MediaTaskStageScheduling,
		}); err != nil {
			return nil, nil, err
		}
		task.Stage = MediaTaskStageScheduling
		stageStarted = time.Now()
	}
	if lastFailure != nil {
		return nil, lastFailure, nil
	}
	return nil, systemMediaFailure("system_scheduler", "no media account is available"), nil
}

func isUnknownSubmissionRecovery(task *MediaTask) bool {
	return task != nil &&
		task.Status == MediaTaskStatusInProgress &&
		task.Stage == MediaTaskStageSubmitting &&
		task.UpstreamTaskID == "" &&
		task.AccountID != nil &&
		task.Adapter != "" &&
		task.UpstreamModel != ""
}

func classifyMediaTaskRecovery(task *MediaTask) mediaTaskRecoveryMode {
	if task == nil || task.Status != MediaTaskStatusInProgress {
		return mediaTaskRecoveryReschedule
	}
	if task.UpstreamTaskID != "" {
		return mediaTaskRecoveryExistingUpstream
	}
	if isUnknownSubmissionRecovery(task) {
		return mediaTaskRecoveryUnknownSubmission
	}
	switch task.Stage {
	case MediaTaskStageGenerating, MediaTaskStageStoring, MediaTaskStageSettling:
		return mediaTaskRecoveryUnknownResult
	default:
		return mediaTaskRecoveryReschedule
	}
}

func (w *MediaWorker) resumeUnknownSubmission(
	ctx context.Context,
	task *MediaTask,
	active *mediaActiveExecution,
	trace *mediaExecutionTrace,
) (*MediaGenerateResult, *mediaExecutionFailure, error) {
	adapter, err := w.deps.Adapters.Resolve(task.Adapter)
	if err != nil {
		return nil, systemMediaFailure("system_adapter", "fixed media adapter is unavailable"), nil
	}
	idempotent, supportsIdempotency := adapter.(MediaIdempotentSubmitter)
	if !supportsIdempotency || !idempotent.SupportsIdempotentSubmit() {
		return nil, upstreamMediaFailure("upstream_submit_failed", "media submission result is unknown"), nil
	}
	if chooseMediaExecutionPath(task.ClientAsync, task.NativeAsyncMode) != MediaExecutionPathNativeAsync {
		return nil, systemMediaFailure("system_recovery", "stored submitting task has invalid native async mode"), nil
	}
	spec, err := decodeWorkerMediaSpec(task.RequestSpec, task.MediaType)
	if err != nil {
		return nil, systemMediaFailure("system_request", "stored media request is invalid"), nil
	}
	definition, err := w.deps.Models.Resolve(task.RequestedModel, task.Operation)
	if err != nil {
		return nil, systemMediaFailure("system_model", "stored media model is unavailable"), nil
	}
	selection, release, err := w.acquireFixedSelection(ctx, task)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	result, failure, _, executeErr := w.executeSelected(ctx, task, active, trace, spec, definition, selection)
	return result, failure, executeErr
}

func (w *MediaWorker) executeSelected(
	ctx context.Context,
	task *MediaTask,
	active *mediaActiveExecution,
	trace *mediaExecutionTrace,
	spec MediaSpec,
	definition *MediaModelDefinition,
	selection *MediaAccountSelection,
) (*MediaGenerateResult, *mediaExecutionFailure, bool, error) {
	adapter, err := w.deps.Adapters.Resolve(selection.ResolvedModel.Adapter)
	if err != nil {
		return nil, systemMediaFailure("system_adapter", "media adapter is unavailable"), false, nil
	}
	path := chooseMediaExecutionPath(task.ClientAsync, selection.ResolvedModel.NativeAsyncMode)
	active.bind(path, selection.Account, adapter, "", nil)
	if err := ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	stage := MediaTaskStageGenerating
	if path == MediaExecutionPathNativeAsync {
		stage = MediaTaskStageSubmitting
	}
	if err := w.updateClaimed(ctx, task, map[string]any{
		"account_id":        selection.Account.ID,
		"adapter":           selection.ResolvedModel.Adapter,
		"upstream_model":    selection.ResolvedModel.UpstreamModel,
		"native_async_mode": selection.ResolvedModel.NativeAsyncMode,
		"stage":             stage,
	}); err != nil {
		return nil, nil, false, err
	}
	task.AccountID = mediaInt64Pointer(selection.Account.ID)
	task.Adapter = selection.ResolvedModel.Adapter
	task.UpstreamModel = selection.ResolvedModel.UpstreamModel
	task.NativeAsyncMode = selection.ResolvedModel.NativeAsyncMode
	task.Stage = stage
	request := MediaExecutionRequest{
		Task: task, Account: selection.Account, Definition: definition, Spec: spec,
		UpstreamModel: selection.ResolvedModel.UpstreamModel, IdempotencyKey: task.PublicID,
	}

	stageStarted := time.Now()
	if path == MediaExecutionPathSync {
		generator, ok := adapter.(MediaSyncGenerator)
		if !ok {
			w.observeStage(task, trace, MediaTaskStageGenerating, stageStarted)
			return nil, systemMediaFailure("system_adapter", "media adapter does not support synchronous generation"), false, nil
		}
		result, generateErr := generator.Generate(ctx, request)
		w.observeStage(task, trace, MediaTaskStageGenerating, stageStarted)
		w.markAccountUsed(ctx, task, selection.Account.ID)
		if generateErr != nil {
			if ctx.Err() != nil {
				return nil, nil, false, ctx.Err()
			}
			failure, adapterErr := classifyWorkerAdapterFailure(generateErr, "upstream_generate_failed")
			retryDifferentAccount := adapterErr != nil && adapterErr.Retryable && !adapterErr.SubmissionUnknown && task.UpstreamTaskID == ""
			return nil, failure, retryDifferentAccount, nil
		}
		return result, nil, false, nil
	}

	submitter, submitOK := adapter.(MediaAsyncSubmitter)
	if !submitOK {
		w.observeStage(task, trace, MediaTaskStageSubmitting, stageStarted)
		return nil, systemMediaFailure("system_adapter", "media adapter does not support native async submission"), false, nil
	}
	var submission *MediaAsyncSubmission
	for {
		var submitErr error
		submission, submitErr = submitter.Submit(ctx, request)
		w.observeStage(task, trace, MediaTaskStageSubmitting, stageStarted)
		if submitErr == nil {
			break
		}
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		failure, adapterErr := classifyWorkerAdapterFailure(submitErr, "upstream_submit_failed")
		switch w.mediaAdapterRetryMode(task, adapter, adapterErr) {
		case mediaAdapterRetryDifferentAccount:
			return nil, failure, true, nil
		case mediaAdapterRetrySameSelection:
			task.RetryCount++
			if err := w.updateClaimed(ctx, task, map[string]any{"retry_count": task.RetryCount}); err != nil {
				return nil, nil, false, err
			}
			if err := waitMediaInterval(ctx, w.cfg.PollInterval); err != nil {
				return nil, nil, false, err
			}
			stageStarted = time.Now()
			continue
		default:
			return nil, failure, false, nil
		}
	}
	if submission == nil || strings.TrimSpace(submission.UpstreamTaskID) == "" {
		return nil, systemMediaFailure("system_adapter", "media adapter returned an empty upstream task id"), false, nil
	}
	if active.bind(path, selection.Account, adapter, submission.UpstreamTaskID, submission.PollMetadata) {
		w.abortActiveUpstream(task.ID, active)
		if err := ctx.Err(); err != nil {
			return nil, nil, false, err
		}
		return nil, nil, false, ErrMediaSyncTimeoutStopped
	}
	w.markAccountUsed(ctx, task, selection.Account.ID)
	submittedAt := time.Now().UTC()
	if err := w.updateClaimed(ctx, task, map[string]any{
		"account_id":        selection.Account.ID,
		"adapter":           selection.ResolvedModel.Adapter,
		"upstream_model":    selection.ResolvedModel.UpstreamModel,
		"native_async_mode": selection.ResolvedModel.NativeAsyncMode,
		"upstream_task_id":  submission.UpstreamTaskID,
		"poll_metadata":     append(json.RawMessage(nil), submission.PollMetadata...),
		"submitted_at":      submittedAt,
		"stage":             MediaTaskStagePolling,
	}); err != nil {
		w.abortActiveUpstream(task.ID, active)
		return nil, nil, false, err
	}
	task.UpstreamTaskID = submission.UpstreamTaskID
	task.PollMetadata = append(json.RawMessage(nil), submission.PollMetadata...)
	task.SubmittedAt = mediaTimePointer(submittedAt)
	task.Stage = MediaTaskStagePolling
	if err := ctx.Err(); err != nil {
		w.abortActiveUpstream(task.ID, active)
		return nil, nil, false, err
	}
	result, failure, pollErr := w.poll(ctx, task, selection.Account, adapter, trace)
	return result, failure, false, pollErr
}

func (w *MediaWorker) resumePolling(ctx context.Context, task *MediaTask, active *mediaActiveExecution, trace *mediaExecutionTrace) (*MediaGenerateResult, *mediaExecutionFailure, error) {
	if task.AccountID == nil || task.Adapter == "" || task.UpstreamModel == "" {
		return nil, systemMediaFailure("system_recovery", "submitted media task is missing fixed execution data"), nil
	}
	adapter, err := w.deps.Adapters.Resolve(task.Adapter)
	if err != nil {
		return nil, systemMediaFailure("system_adapter", "fixed media adapter is unavailable"), nil
	}
	if err := w.bindPersistedUpstreamForCleanup(ctx, task, active, adapter); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	selection, release, err := w.acquireFixedSelection(ctx, task)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	if active.bind(MediaExecutionPathNativeAsync, selection.Account, adapter, task.UpstreamTaskID, task.PollMetadata) {
		w.abortActiveUpstream(task.ID, active)
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrMediaSyncTimeoutStopped
	}
	if task.Stage != MediaTaskStagePolling {
		if err := w.updateClaimed(ctx, task, map[string]any{"stage": MediaTaskStagePolling}); err != nil {
			return nil, nil, err
		}
		task.Stage = MediaTaskStagePolling
	}
	return w.poll(ctx, task, selection.Account, adapter, trace)
}

// bindPersistedUpstreamForCleanup resolves and binds the persisted abort target
// before fixed-slot selection can block. The cleanup lookup deliberately
// survives execution cancellation, but remains bounded, so a concurrent stop
// cannot lose the upstream Abort handshake while waiting for an account slot.
func (w *MediaWorker) bindPersistedUpstreamForCleanup(ctx context.Context, task *MediaTask, active *mediaActiveExecution, adapter MediaAdapter) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaUpstreamCleanupTimeout)
	defer cancel()
	account, err := w.deps.Scheduler.GetFixedAccount(cleanupCtx, *task.AccountID)
	if err != nil {
		return fmt.Errorf("resolve persisted upstream cleanup account %d: %w", *task.AccountID, err)
	}
	if account == nil || account.ID != *task.AccountID {
		return fmt.Errorf("resolve persisted upstream cleanup account %d: %w", *task.AccountID, ErrNoAvailableAccounts)
	}
	if active.bind(MediaExecutionPathNativeAsync, account, adapter, task.UpstreamTaskID, task.PollMetadata) {
		w.abortActiveUpstream(task.ID, active)
	}
	return nil
}

func (w *MediaWorker) acquireFixedSelection(ctx context.Context, task *MediaTask) (*MediaAccountSelection, func(), error) {
	if task == nil || task.AccountID == nil {
		return nil, nil, ErrNoAvailableAccounts
	}
	selection, err := w.deps.Scheduler.SelectFixed(ctx, MediaFixedAccountRequest{
		AccountID: *task.AccountID, GroupID: task.GroupID, SessionHash: task.PublicID,
		SlotID: MediaTaskSlotID(task.PublicID),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("select fixed media account %d: %w", *task.AccountID, err)
	}
	selection.ResolvedModel = ResolvedMediaAccountModel{
		Adapter: task.Adapter, UpstreamModel: task.UpstreamModel, NativeAsyncMode: task.NativeAsyncMode,
	}
	release, err := w.acquireSelection(ctx, selection)
	if err != nil {
		return nil, nil, err
	}
	return selection, release, nil
}

func (w *MediaWorker) poll(ctx context.Context, task *MediaTask, account *Account, adapter MediaAdapter, trace *mediaExecutionTrace) (*MediaGenerateResult, *mediaExecutionFailure, error) {
	poller, ok := adapter.(MediaAsyncPoller)
	if !ok {
		return nil, systemMediaFailure("system_adapter", "media adapter does not support polling"), nil
	}
	stageStarted := time.Now()
	for {
		trace.pollCount++
		result, err := poller.Poll(ctx, MediaPollRequest{Account: account, UpstreamTaskID: task.UpstreamTaskID, PollMetadata: task.PollMetadata})
		if err != nil {
			if ctx.Err() != nil {
				w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
				return nil, nil, ctx.Err()
			}
			var adapterErr *MediaAdapterError
			if errors.As(err, &adapterErr) && !adapterErr.Retryable {
				w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
				failure, _ := classifyWorkerAdapterFailure(err, "upstream_poll_failed")
				return nil, failure, nil
			}
			task.RetryCount++
			if updateErr := w.updateClaimed(ctx, task, map[string]any{"retry_count": task.RetryCount}); updateErr != nil {
				w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
				return nil, nil, updateErr
			}
			if waitErr := waitMediaInterval(ctx, w.cfg.PollInterval); waitErr != nil {
				w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
				return nil, nil, waitErr
			}
			continue
		}
		if result == nil {
			w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
			return nil, systemMediaFailure("system_adapter", "media adapter returned an empty poll result"), nil
		}
		switch result.State {
		case MediaPollStateRunning:
			progress := min(max(result.Progress, 0), 99)
			if err := w.updateClaimed(ctx, task, map[string]any{"progress": progress}); err != nil {
				w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
				return nil, nil, err
			}
			task.Progress = progress
			if err := waitMediaInterval(ctx, w.cfg.PollInterval); err != nil {
				w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
				return nil, nil, err
			}
		case MediaPollStateCompleted:
			w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
			if result.Result == nil {
				return nil, systemMediaFailure("system_adapter", "media adapter completed without output"), nil
			}
			return result.Result, nil, nil
		case MediaPollStateCanceled:
			w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
			return nil, &mediaExecutionFailure{
				settlement: MediaFailureSettlement{Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: "upstream_canceled"},
				message:    "upstream media task was canceled",
				category:   "upstream_canceled",
			}, nil
		case MediaPollStateFailed:
			w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
			if result.Error == nil {
				return nil, upstreamMediaFailure("upstream_failed", "upstream media task failed"), nil
			}
			failure, _ := classifyWorkerAdapterFailure(result.Error, "upstream_failed")
			return nil, failure, nil
		default:
			w.observeStage(task, trace, MediaTaskStagePolling, stageStarted)
			return nil, systemMediaFailure("system_adapter", "media adapter returned an unknown poll state"), nil
		}
	}
}

func (w *MediaWorker) completeSuccess(
	ctx context.Context,
	task *MediaTask,
	result *MediaGenerateResult,
	trace *mediaExecutionTrace,
	active *mediaActiveExecution,
	stopLeaseRenewer func(),
) error {
	stageStarted := time.Now()
	if err := w.updateClaimed(ctx, task, map[string]any{"stage": MediaTaskStageStoring}); err != nil {
		return err
	}
	task.Stage = MediaTaskStageStoring
	_, err := w.deps.Artifacts.PersistOutputs(ctx, task, result.Artifacts)
	w.observeStage(task, trace, MediaTaskStageStoring, stageStarted)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.deps.Metrics.IncrementStorageFailure(task.MediaType)
		return w.completeFailure(ctx, task, systemMediaFailure("system_storage", "media output storage failed"), trace, active, stopLeaseRenewer)
	}

	stageStarted = time.Now()
	if err := w.updateClaimed(ctx, task, map[string]any{"stage": MediaTaskStageSettling}); err != nil {
		return err
	}
	task.Stage = MediaTaskStageSettling
	recovery, err := json.Marshal(MediaSettlementPlan{Type: MediaSettlementTypeSuccess, Usage: &result.Usage})
	if err != nil {
		return fmt.Errorf("encode media task %d success settlement recovery: %w", task.ID, err)
	}
	finishedAt := time.Now().UTC()
	active.terminalizing.Store(true)
	transitioned, err := w.deps.Tasks.TransitionClaimed(ctx, task.ID, w.workerID, task.Version,
		MediaTaskStatusInProgress, MediaTaskStatusCompleted,
		map[string]any{
			"stage": MediaTaskStageCompleted, "progress": 100, "finished_at": finishedAt,
			"settlement_recovery": json.RawMessage(recovery),
		},
	)
	if err != nil {
		return fmt.Errorf("complete media task %d: %w", task.ID, err)
	}
	if !transitioned {
		return fmt.Errorf("%w: complete media task %d", ErrMediaWorkerLeaseLost, task.ID)
	}
	stopLeaseRenewer()
	task.Status = MediaTaskStatusCompleted
	task.Stage = MediaTaskStageCompleted
	task.Progress = 100
	task.FinishedAt = mediaTimePointer(finishedAt)
	task.SettlementRecovery = append(json.RawMessage(nil), recovery...)
	task.Version++
	settlementErr := w.deps.Billing.SettleSuccess(ctx, task, result.Usage)
	if settlementErr != nil {
		w.deps.Metrics.IncrementSettlementRetry(task.MediaType)
		w.logWorkerError(task, trace, "settlement_success", "settlement_retry", settlementErr)
	}
	w.observeStage(task, trace, MediaTaskStageSettling, stageStarted)
	w.publishTerminal(ctx, task, trace, "")
	return w.settlementAckError(ctx, task, settlementErr)
}

func (w *MediaWorker) completeFailure(
	ctx context.Context,
	task *MediaTask,
	failure *mediaExecutionFailure,
	trace *mediaExecutionTrace,
	active *mediaActiveExecution,
	stopLeaseRenewer func(),
) error {
	if failure == nil {
		failure = systemMediaFailure("system_worker", "media worker failed")
	}
	recovery, err := json.Marshal(MediaSettlementPlan{Type: MediaSettlementTypeFailure, Failure: &failure.settlement})
	if err != nil {
		return fmt.Errorf("encode media task %d failure settlement recovery: %w", task.ID, err)
	}
	finishedAt := time.Now().UTC()
	active.terminalizing.Store(true)
	transitioned, err := w.deps.Tasks.TransitionClaimed(ctx, task.ID, w.workerID, task.Version,
		MediaTaskStatusInProgress, MediaTaskStatusFailed,
		map[string]any{
			"stage": MediaTaskStageFailed, "finished_at": finishedAt,
			"error_code": failure.settlement.ErrorCode, "error_message": failure.message,
			"retry_count": task.RetryCount, "settlement_recovery": json.RawMessage(recovery),
		},
	)
	if err != nil {
		return fmt.Errorf("fail media task %d: %w", task.ID, err)
	}
	if !transitioned {
		return fmt.Errorf("%w: fail media task %d", ErrMediaWorkerLeaseLost, task.ID)
	}
	stopLeaseRenewer()
	task.Status = MediaTaskStatusFailed
	task.Stage = MediaTaskStageFailed
	task.ErrorCode = failure.settlement.ErrorCode
	task.ErrorMessage = failure.message
	task.FinishedAt = mediaTimePointer(finishedAt)
	task.SettlementRecovery = append(json.RawMessage(nil), recovery...)
	task.Version++
	stageStarted := time.Now()
	settlementErr := w.deps.Billing.SettleFailure(ctx, task, failure.settlement)
	if settlementErr != nil {
		w.deps.Metrics.IncrementSettlementRetry(task.MediaType)
		w.logWorkerError(task, trace, "settlement_failure", "settlement_retry", settlementErr)
	}
	w.observeStage(task, trace, MediaTaskStageSettling, stageStarted)
	w.publishTerminal(ctx, task, trace, failure.category)
	return w.settlementAckError(ctx, task, settlementErr)
}

func (w *MediaWorker) RecoverOnce(ctx context.Context) error {
	if err := w.validateDependencies(); err != nil {
		return err
	}
	tasks, err := w.deps.Tasks.ListRecoverable(ctx, time.Now().UTC(), w.cfg.RecoveryBatchSize)
	if err != nil {
		return fmt.Errorf("list recoverable media tasks: %w", err)
	}
	for index := range tasks {
		task := &tasks[index]
		priority := MediaQueuePriorityAsync
		if !task.ClientAsync && !task.SyncFallback {
			priority = MediaQueuePrioritySync
		}
		if err := w.deps.Queue.Enqueue(ctx, task.ID, priority); err != nil {
			return fmt.Errorf("requeue recoverable media task %d: %w", task.ID, err)
		}
		w.deps.Metrics.IncrementRecovery(task.MediaType)
	}
	pending, err := w.deps.Tasks.ListSettlementPending(ctx, w.cfg.RecoveryBatchSize)
	if err != nil {
		return fmt.Errorf("list settlement-pending media tasks: %w", err)
	}
	for index := range pending {
		if err := w.deps.Queue.Enqueue(ctx, pending[index].ID, MediaQueuePriorityAsync); err != nil {
			return fmt.Errorf("requeue settlement-pending media task %d: %w", pending[index].ID, err)
		}
	}
	return nil
}

func (w *MediaWorker) StopForSyncTimeout(taskID int64) bool {
	if w == nil {
		return false
	}
	w.activeMu.RLock()
	active := w.active[taskID]
	w.activeMu.RUnlock()
	if active == nil {
		return false
	}
	shouldAbort := active.requestStop()
	active.cancel(ErrMediaSyncTimeoutStopped)
	if shouldAbort {
		w.abortActiveUpstream(taskID, active)
	}
	return true
}

func (w *MediaWorker) abortActiveUpstream(taskID int64, active *mediaActiveExecution) {
	if active == nil {
		return
	}
	active.abortBoundUpstream(func(target mediaActiveAbortTarget) {
		aborter, ok := target.adapter.(MediaAborter)
		if !ok {
			return
		}
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(target.ctx), mediaUpstreamCleanupTimeout)
		defer cancel()
		err := callMediaAborter(aborter, abortCtx, MediaPollRequest{
			Account: target.account, UpstreamTaskID: target.upstreamTaskID, PollMetadata: target.pollMetadata,
		})
		if err == nil {
			return
		}
		wrapped := fmt.Errorf("abort media task %d: %w", taskID, err)
		w.logger.Warn("media task abort failed", "task_id", taskID, "error_category", "abort_failed")
		select {
		case w.errEvents <- wrapped:
		default:
		}
	})
}

func callMediaAborter(aborter MediaAborter, ctx context.Context, request MediaPollRequest) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("media adapter abort panic: %v", recovered)
		}
	}()
	return aborter.Abort(ctx, request)
}

func chooseMediaExecutionPath(clientAsync bool, mode NativeAsyncMode) MediaExecutionPath {
	if mode == NativeAsyncRequired || (clientAsync && mode == NativeAsyncOptional) {
		return MediaExecutionPathNativeAsync
	}
	return MediaExecutionPathSync
}

func (w *MediaWorker) mayResubmit(task *MediaTask, adapter MediaAdapter, err *MediaAdapterError) bool {
	return w.mediaAdapterRetryMode(task, adapter, err) != mediaAdapterRetryNone
}

func (w *MediaWorker) mediaAdapterRetryMode(task *MediaTask, adapter MediaAdapter, err *MediaAdapterError) mediaAdapterRetryMode {
	if task.UpstreamTaskID != "" {
		return mediaAdapterRetryNone
	}
	if err == nil || !err.Retryable {
		return mediaAdapterRetryNone
	}
	if err.SubmissionUnknown {
		idempotent, supportsIdempotency := adapter.(MediaIdempotentSubmitter)
		if supportsIdempotency && idempotent.SupportsIdempotentSubmit() {
			return mediaAdapterRetrySameSelection
		}
		return mediaAdapterRetryNone
	}
	return mediaAdapterRetryDifferentAccount
}

func (w *MediaWorker) validateDependencies() error {
	if w == nil || w.deps.Tasks == nil || w.deps.Queue == nil || w.deps.Scheduler == nil ||
		w.deps.Models == nil || w.deps.Adapters == nil || w.deps.Artifacts == nil ||
		w.deps.Billing == nil || w.deps.Metrics == nil {
		return errors.New("media worker dependencies are incomplete")
	}
	return nil
}

func (w *MediaWorker) acquireSelection(ctx context.Context, selection *MediaAccountSelection) (func(), error) {
	if selection == nil {
		return nil, ErrNoAvailableAccounts
	}
	if selection.Acquired {
		if selection.ReleaseFunc == nil {
			return nil, ErrAccountConcurrencySaturated
		}
		return selection.ReleaseFunc, nil
	}
	release, err := w.deps.Scheduler.WaitForSlot(ctx, selection)
	if err != nil {
		return nil, fmt.Errorf("wait for media account slot: %w", err)
	}
	if release == nil {
		return nil, ErrAccountConcurrencySaturated
	}
	return release, nil
}

func (w *MediaWorker) updateClaimed(ctx context.Context, task *MediaTask, updates map[string]any) error {
	updated, err := w.deps.Tasks.UpdateClaimed(ctx, task.ID, w.workerID, updates)
	if err != nil {
		return fmt.Errorf("update claimed media task %d: %w", task.ID, err)
	}
	if !updated {
		return fmt.Errorf("%w: update media task %d", ErrMediaWorkerLeaseLost, task.ID)
	}
	task.Version++
	return nil
}

func (w *MediaWorker) startLeaseRenewer(ctx context.Context, active *mediaActiveExecution, taskID int64) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("%w: lease renew panic: %v\n%s", ErrMediaWorkerLeaseLost, recovered, debug.Stack())
				w.reportError(err)
				if !active.terminalizing.Load() {
					active.cancel(err)
				}
			}
		}()
		ticker := time.NewTicker(w.cfg.LeaseRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				callCtx, cancel := context.WithTimeout(ctx, w.cfg.LeaseRenewInterval)
				renewed, err := w.deps.Tasks.RenewLease(callCtx, taskID, w.workerID, time.Now().UTC().Add(w.cfg.LeaseTTL))
				cancel()
				if ctx.Err() != nil || active.terminalizing.Load() {
					return
				}
				if err != nil {
					active.cancel(fmt.Errorf("%w: renew media task %d: %w", ErrMediaWorkerLeaseLost, taskID, err))
					return
				}
				if !renewed {
					active.cancel(fmt.Errorf("%w: renew media task %d rejected", ErrMediaWorkerLeaseLost, taskID))
					return
				}
			}
		}
	}()
	return done
}

func (w *MediaWorker) registerActive(taskID int64, active *mediaActiveExecution) bool {
	w.activeMu.Lock()
	defer w.activeMu.Unlock()
	if _, exists := w.active[taskID]; exists {
		return false
	}
	w.active[taskID] = active
	return true
}

func (w *MediaWorker) unregisterActive(taskID int64, active *mediaActiveExecution) {
	w.activeMu.Lock()
	defer w.activeMu.Unlock()
	if w.active[taskID] == active {
		delete(w.active, taskID)
	}
}

func (w *MediaWorker) cancelAllActive(cause error) {
	w.activeMu.RLock()
	items := make([]*mediaActiveExecution, 0, len(w.active))
	for _, active := range w.active {
		items = append(items, active)
	}
	w.activeMu.RUnlock()
	for _, active := range items {
		active.cancel(cause)
	}
}

func (a *mediaActiveExecution) bind(path MediaExecutionPath, account *Account, adapter MediaAdapter, upstreamTaskID string, pollMetadata json.RawMessage) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.path = path
	a.account = account
	a.adapter = adapter
	a.upstreamTaskID = upstreamTaskID
	a.pollMetadata = append(json.RawMessage(nil), pollMetadata...)
	return a.stopRequested && a.hasAbortTargetLocked()
}

func (a *mediaActiveExecution) requestStop() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopRequested = true
	return a.hasAbortTargetLocked()
}

func (a *mediaActiveExecution) hasAbortTargetLocked() bool {
	return a.path == MediaExecutionPathNativeAsync &&
		a.account != nil &&
		a.adapter != nil &&
		strings.TrimSpace(a.upstreamTaskID) != ""
}

func (a *mediaActiveExecution) abortBoundUpstream(abort func(mediaActiveAbortTarget)) {
	if a == nil || abort == nil {
		return
	}
	a.mu.RLock()
	if !a.hasAbortTargetLocked() {
		a.mu.RUnlock()
		return
	}
	target := mediaActiveAbortTarget{
		ctx:            a.ctx,
		account:        a.account,
		adapter:        a.adapter,
		upstreamTaskID: a.upstreamTaskID,
		pollMetadata:   append(json.RawMessage(nil), a.pollMetadata...),
	}
	a.mu.RUnlock()
	a.abortOnce.Do(func() { abort(target) })
}

func (a *mediaActiveExecution) snapshot() (MediaExecutionPath, *Account, MediaAdapter, string, json.RawMessage, context.Context) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.path, a.account, a.adapter, a.upstreamTaskID, append(json.RawMessage(nil), a.pollMetadata...), a.ctx
}

func (w *MediaWorker) observeStage(task *MediaTask, trace *mediaExecutionTrace, stage MediaTaskStage, started time.Time) {
	elapsed := time.Since(started)
	w.deps.Metrics.ObserveStage(task.MediaType, stage, elapsed)
	trace.durations[stage] += elapsed
}

func (w *MediaWorker) retryTerminalSettlement(ctx context.Context, task *MediaTask) error {
	if err := validatePersistedMediaSettlementConsistency(task); err != nil {
		return err
	}
	if task.BillingStatus == MediaBillingStatusSettled {
		return nil
	}
	if len(task.SettlementPlan) > 0 {
		if err := w.deps.Billing.RetryPending(ctx, task.ID); err != nil {
			w.deps.Metrics.IncrementSettlementRetry(task.MediaType)
			w.logWorkerError(task, &mediaExecutionTrace{durations: map[MediaTaskStage]time.Duration{}}, "retry_settlement", "settlement_retry", err)
		}
		return nil
	}
	if len(task.SettlementRecovery) == 0 {
		return fmt.Errorf("%w: terminal task %d has no recovery intent", ErrMediaSettlementPlanNotPersisted, task.ID)
	}
	plan, err := decodeMediaSettlementPlan(task.SettlementRecovery)
	if err != nil {
		return fmt.Errorf("%w: decode task %d recovery intent: %w", ErrMediaSettlementPlanNotPersisted, task.ID, err)
	}
	switch plan.Type {
	case MediaSettlementTypeSuccess:
		if plan.Usage == nil || plan.Failure != nil {
			return fmt.Errorf("%w: task %d has invalid success recovery intent", ErrMediaSettlementPlanNotPersisted, task.ID)
		}
		err = w.deps.Billing.SettleSuccess(ctx, task, *plan.Usage)
	case MediaSettlementTypeFailure:
		if plan.Failure == nil || plan.Usage != nil {
			return fmt.Errorf("%w: task %d has invalid failure recovery intent", ErrMediaSettlementPlanNotPersisted, task.ID)
		}
		err = w.deps.Billing.SettleFailure(ctx, task, *plan.Failure)
	default:
		return fmt.Errorf("%w: task %d has unknown recovery intent type %q", ErrMediaSettlementPlanNotPersisted, task.ID, plan.Type)
	}
	if err != nil {
		w.deps.Metrics.IncrementSettlementRetry(task.MediaType)
		w.logWorkerError(task, &mediaExecutionTrace{durations: map[MediaTaskStage]time.Duration{}}, "retry_settlement", "settlement_retry", err)
	}
	return w.settlementAckError(ctx, task, err)
}

func (w *MediaWorker) settlementAckError(ctx context.Context, task *MediaTask, settlementErr error) error {
	fresh, err := w.deps.Tasks.GetByID(ctx, task.ID)
	if err != nil {
		return errors.Join(
			fmt.Errorf("%w: verify task %d formal plan: %w", ErrMediaSettlementPlanNotPersisted, task.ID, err),
			settlementErr,
		)
	}
	if len(fresh.SettlementPlan) == 0 {
		if settlementErr == nil {
			settlementErr = errors.New("settlement coordinator returned without persisting a formal plan")
		}
		return fmt.Errorf("%w: task %d: %w", ErrMediaSettlementPlanNotPersisted, task.ID, settlementErr)
	}
	if len(fresh.SettlementRecovery) == 0 || !mediaSettlementPlansEqual(fresh.SettlementRecovery, fresh.SettlementPlan) {
		return fmt.Errorf("%w: task %d recovery intent differs from formal plan", ErrMediaSettlementPlanConflict, task.ID)
	}
	task.SettlementPlan = append(json.RawMessage(nil), fresh.SettlementPlan...)
	task.BillingStatus = fresh.BillingStatus
	return nil
}

func (w *MediaWorker) markAccountUsed(ctx context.Context, task *MediaTask, accountID int64) {
	if err := w.deps.Scheduler.MarkUsed(ctx, accountID); err != nil {
		w.logWorkerError(task, &mediaExecutionTrace{durations: map[MediaTaskStage]time.Duration{}}, "mark_used", "scheduler_mark_used", err)
	}
}

func (w *MediaWorker) publishTerminal(ctx context.Context, task *MediaTask, trace *mediaExecutionTrace, errorCategory string) {
	if err := w.deps.Queue.PublishTerminal(ctx, task.ID, task.Status); err != nil {
		w.logWorkerError(task, trace, "publish_terminal", "queue_publish", err)
	}
	attrs := w.logAttrs(task, trace, "terminal", errorCategory)
	w.logger.LogAttrs(ctx, slog.LevelInfo, "media task terminal", attrs...)
}

func (w *MediaWorker) logWorkerError(task *MediaTask, trace *mediaExecutionTrace, operation, category string, err error) {
	attrs := w.logAttrs(task, trace, operation, category)
	attrs = append(attrs, slog.String("error_code", stableMediaWorkerErrorCode(err, category)))
	w.logger.LogAttrs(context.Background(), slog.LevelError, "media worker operation failed", attrs...)
}

func (w *MediaWorker) logAttrs(task *MediaTask, trace *mediaExecutionTrace, operation, category string) []slog.Attr {
	accountID := int64(0)
	platform := ""
	if task.AccountID != nil {
		accountID = *task.AccountID
	}
	if active := w.activeForTask(task.ID); active != nil {
		_, account, _, _, _, _ := active.snapshot()
		if account != nil {
			platform = account.Platform
		}
	}
	duration := func(stage MediaTaskStage) int64 {
		if trace == nil {
			return 0
		}
		return trace.durations[stage].Milliseconds()
	}
	pollCount := 0
	if trace != nil {
		pollCount = trace.pollCount
	}
	return []slog.Attr{
		slog.Int64("task_id", task.ID), slog.String("media_type", string(task.MediaType)),
		slog.String("operation", string(task.Operation)), slog.String("worker_action", operation),
		slog.String("requested_model", task.RequestedModel), slog.String("upstream_model", task.UpstreamModel),
		slog.String("adapter", task.Adapter), slog.String("platform", platform), slog.Int64("account_id", accountID),
		slog.Int64("group_id", task.GroupID), slog.Int("retry_count", task.RetryCount), slog.Int("poll_count", pollCount),
		slog.Int64("scheduling_ms", duration(MediaTaskStageScheduling)), slog.Int64("submitting_ms", duration(MediaTaskStageSubmitting)),
		slog.Int64("generating_ms", duration(MediaTaskStageGenerating)), slog.Int64("polling_ms", duration(MediaTaskStagePolling)),
		slog.Int64("storing_ms", duration(MediaTaskStageStoring)), slog.Int64("settling_ms", duration(MediaTaskStageSettling)),
		slog.String("error_category", category),
	}
}

func (w *MediaWorker) activeForTask(taskID int64) *mediaActiveExecution {
	w.activeMu.RLock()
	defer w.activeMu.RUnlock()
	return w.active[taskID]
}

func (w *MediaWorker) reportError(err error) {
	if err == nil {
		return
	}
	w.logger.Error(
		"media worker background error",
		"error_category", "background_error",
		"error_code", stableMediaWorkerErrorCode(err, "background_error"),
	)
	select {
	case w.errEvents <- err:
	default:
	}
}

func stableMediaWorkerErrorCode(err error, fallback string) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrMediaTaskDeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, ErrMediaSettlementPlanNotPersisted):
		return "settlement_plan_not_persisted"
	case errors.Is(err, ErrMediaSettlementPlanConflict):
		return "settlement_plan_conflict"
	case errors.Is(err, ErrMediaSettlementCASConflict):
		return "settlement_cas_conflict"
	case errors.Is(err, ErrMediaWorkerLeaseLost):
		return "lease_lost"
	case errors.Is(err, ErrMediaTaskNotClaimed):
		return "task_not_claimed"
	case errors.Is(err, errMediaWorkerMessagePanic):
		return "worker_panic"
	}
	var adapterErr *MediaAdapterError
	if errors.As(err, &adapterErr) && validStableMediaErrorCode(adapterErr.Code) {
		return adapterErr.Code
	}
	return fallback
}

func validStableMediaErrorCode(code string) bool {
	if code == "" {
		return false
	}
	for _, character := range code {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func decodeWorkerMediaSpec(raw json.RawMessage, mediaType MediaType) (MediaSpec, error) {
	var spec MediaSpec
	if err := decodeWorkerJSON(raw, &spec); err != nil {
		return MediaSpec{}, err
	}
	if err := spec.Validate(mediaType); err != nil {
		return MediaSpec{}, err
	}
	return spec, nil
}

func decodeWorkerCandidateSnapshot(raw json.RawMessage) ([]MediaAccountCandidateSnapshot, error) {
	var candidates []MediaAccountCandidateSnapshot
	if err := decodeWorkerJSON(raw, &candidates); err != nil {
		return nil, err
	}
	if _, err := validateMediaCandidateSnapshot(candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func decodeWorkerJSON(raw json.RawMessage, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple top-level JSON values")
		}
		return err
	}
	return nil
}

func classifyWorkerAdapterFailure(err error, fallbackCode string) (*mediaExecutionFailure, *MediaAdapterError) {
	var adapterErr *MediaAdapterError
	if !errors.As(err, &adapterErr) {
		return upstreamMediaFailure(fallbackCode, "media adapter request failed"), nil
	}
	code := strings.TrimSpace(adapterErr.Code)
	if code == "" {
		code = fallbackCode
	}
	message := strings.TrimSpace(adapterErr.Message)
	if message == "" {
		message = "media adapter request failed"
	}
	kind := MediaFailureKindUpstream
	category := "upstream"
	if adapterErr.SystemFailure {
		kind = MediaFailureKindSystem
		category = "system_adapter"
	}
	return &mediaExecutionFailure{
		settlement: MediaFailureSettlement{Kind: kind, RefundRatio: 1, ErrorCode: code},
		message:    message,
		category:   category,
	}, adapterErr
}

func systemMediaFailure(code, message string) *mediaExecutionFailure {
	return &mediaExecutionFailure{
		settlement: MediaFailureSettlement{Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: code},
		message:    message,
		category:   code,
	}
}

func upstreamMediaFailure(code, message string) *mediaExecutionFailure {
	return &mediaExecutionFailure{
		settlement: MediaFailureSettlement{Kind: MediaFailureKindUpstream, RefundRatio: 1, ErrorCode: code},
		message:    message,
		category:   code,
	}
}

func waitMediaInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func mediaTimePointer(value time.Time) *time.Time {
	copy := value
	return &copy
}

func mediaInt64Pointer(value int64) *int64 {
	copy := value
	return &copy
}

var _ MediaExecutionController = (*MediaWorker)(nil)
