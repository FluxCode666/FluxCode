package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/url"
	"slices"
	"strings"
	"time"
)

var (
	ErrMediaTaskNotFound               = errors.New("media task not found")
	ErrMediaIdempotencyConflict        = errors.New("media idempotency key conflicts with the original request")
	ErrMediaContentRejected            = errors.New("media content rejected")
	ErrMediaGenerationNotAllowed       = errors.New("media generation is not allowed for the group")
	ErrMediaInputNotRecoverable        = errors.New("media input is not recoverable")
	ErrMediaTerminalSubscriptionClosed = errors.New("media terminal subscription closed")
	ErrMediaOrchestratorStateConflict  = errors.New("media orchestrator state CAS conflict")
	ErrMediaTaskInitializing           = errors.New("media task initialization is still in progress")
)

const (
	mediaOrchestratorDetachedTimeout = 30 * time.Second
	mediaTaskInitializationLease     = 5 * time.Minute
	mediaSyncTimeoutTransitionTries  = 8
)

type MediaCreateRequest struct {
	UserID         int64
	APIKeyID       int64
	GroupID        int64
	MediaType      MediaType
	Operation      MediaOperation
	RequestedModel string
	Spec           MediaSpec
	Inputs         []MediaArtifactInput
	ClientAsync    bool
	SessionHash    string
	IdempotencyKey string
}

type MediaCreateDisposition string

const (
	MediaCreateDispositionCompleted      MediaCreateDisposition = "completed"
	MediaCreateDispositionFailed         MediaCreateDisposition = "failed"
	MediaCreateDispositionAccepted       MediaCreateDisposition = "accepted"
	MediaCreateDispositionFallbackAsync  MediaCreateDisposition = "fallback_async"
	MediaCreateDispositionGatewayTimeout MediaCreateDisposition = "gateway_timeout"
)

type MediaCreateResult struct {
	Task          *MediaTask
	Artifacts     []MediaArtifact
	Disposition   MediaCreateDisposition
	InputsAdopted bool
}

type MediaSettingsProvider interface {
	GetAllSettings(ctx context.Context) (*SystemSettings, error)
}

type MediaContentPolicy interface {
	Check(ctx context.Context, userID int64, mediaType MediaType, spec MediaSpec) error
}

type AllowAllMediaContentPolicy struct{}

func (AllowAllMediaContentPolicy) Check(context.Context, int64, MediaType, MediaSpec) error {
	return nil
}

type MediaPricingPort interface {
	Snapshot(ctx context.Context, req MediaCreateRequest, definition *MediaModelDefinition, candidates []MediaAccountCandidateSnapshot) (MediaBillingSnapshot, error)
}

type ZeroMediaPricing struct{}

func (ZeroMediaPricing) Snapshot(_ context.Context, req MediaCreateRequest, _ *MediaModelDefinition, _ []MediaAccountCandidateSnapshot) (MediaBillingSnapshot, error) {
	return MediaBillingSnapshot{RequestedModel: req.RequestedModel}, nil
}

type MediaTimer interface {
	Channel() <-chan time.Time
	Stop() bool
}

type MediaClock interface {
	Now() time.Time
	NewTimer(d time.Duration) MediaTimer
}

type realMediaTimer struct {
	timer *time.Timer
}

func (t realMediaTimer) Channel() <-chan time.Time { return t.timer.C }
func (t realMediaTimer) Stop() bool                { return t.timer.Stop() }

type realMediaClock struct{}

func (realMediaClock) Now() time.Time { return time.Now() }
func (realMediaClock) NewTimer(d time.Duration) MediaTimer {
	return realMediaTimer{timer: time.NewTimer(d)}
}

type MediaCandidateSnapshotter interface {
	SnapshotCandidates(ctx context.Context, groupID int64, requestedModel string) ([]MediaAccountCandidateSnapshot, error)
}

type MediaGroupProvider interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

type MediaTaskController interface {
	StopForSyncTimeout(taskID int64) bool
}

type MediaOrchestratorDependencies struct {
	Registry          *MediaModelRegistry
	Groups            MediaGroupProvider
	Scheduler         MediaCandidateSnapshotter
	Settings          MediaSettingsProvider
	ContentPolicy     MediaContentPolicy
	Pricing           MediaPricingPort
	Tasks             MediaTaskRepository
	Artifacts         MediaArtifactRepository
	Billing           MediaBillingPort
	Settlement        MediaSettlementCoordinator
	Queue             MediaTaskQueue
	Controller        MediaTaskController
	Clock             MediaClock
	PublicIDGenerator func() (string, error)
}

type MediaOrchestrator struct {
	deps MediaOrchestratorDependencies
}

func NewMediaOrchestrator(deps MediaOrchestratorDependencies) *MediaOrchestrator {
	if deps.ContentPolicy == nil {
		deps.ContentPolicy = AllowAllMediaContentPolicy{}
	}
	if deps.Pricing == nil {
		deps.Pricing = ZeroMediaPricing{}
	}
	if deps.Clock == nil {
		deps.Clock = realMediaClock{}
	}
	if deps.PublicIDGenerator == nil {
		deps.PublicIDGenerator = newMediaTaskPublicID
	}
	return &MediaOrchestrator{deps: deps}
}

func (o *MediaOrchestrator) Create(ctx context.Context, req MediaCreateRequest) (*MediaCreateResult, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	definition, inputs, err := o.validateRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := o.deps.ContentPolicy.Check(ctx, req.UserID, req.MediaType, req.Spec); err != nil {
		return nil, fmt.Errorf("check media content policy: %w", err)
	}

	fingerprint, err := mediaCreateFingerprint(req)
	if err != nil {
		return nil, err
	}
	if req.IdempotencyKey != "" {
		existing, lookupErr := o.deps.Tasks.GetByIdempotencyKey(ctx, req.UserID, req.APIKeyID, req.IdempotencyKey)
		switch {
		case lookupErr == nil:
			return o.reuseOrResumeTask(ctx, existing, req, inputs, fingerprint, nil)
		case errors.Is(lookupErr, ErrMediaTaskNotFound):
		case lookupErr != nil:
			return nil, fmt.Errorf("lookup media idempotency key: %w", lookupErr)
		}
	}

	var settings *SystemSettings
	if !req.ClientAsync {
		settings, err = o.loadSettings(ctx)
		if err != nil {
			return nil, err
		}
	}

	candidates, err := o.deps.Scheduler.SnapshotCandidates(ctx, req.GroupID, strings.TrimSpace(req.RequestedModel))
	if err != nil {
		return nil, fmt.Errorf("snapshot media candidates: %w", err)
	}
	candidateJSON, err := json.Marshal(candidates)
	if err != nil {
		return nil, fmt.Errorf("encode media candidate snapshot: %w", err)
	}
	billingSnapshot, err := o.deps.Pricing.Snapshot(ctx, req, definition, candidates)
	if err != nil {
		return nil, fmt.Errorf("snapshot media pricing: %w", err)
	}
	billingJSON, err := json.Marshal(billingSnapshot)
	if err != nil {
		return nil, fmt.Errorf("encode media billing snapshot: %w", err)
	}
	publicID, err := o.deps.PublicIDGenerator()
	if err != nil {
		return nil, fmt.Errorf("generate media task public id: %w", err)
	}
	if !strings.HasPrefix(publicID, "task_") || len(publicID) <= len("task_") {
		return nil, errors.New("generated media task public id is invalid")
	}
	initialSpec, err := json.Marshal(req.Spec)
	if err != nil {
		return nil, fmt.Errorf("encode media request spec: %w", err)
	}
	now := o.deps.Clock.Now().UTC()
	initializationLease := now.Add(mediaTaskInitializationLease)
	task := &MediaTask{
		PublicID: publicID, UserID: req.UserID, APIKeyID: req.APIKeyID, GroupID: req.GroupID,
		MediaType: req.MediaType, Operation: req.Operation, RequestedModel: strings.TrimSpace(req.RequestedModel),
		ClientAsync: req.ClientAsync, Status: MediaTaskStatusQueued, Stage: MediaTaskStageQueued,
		RequestSpec: initialSpec, CandidateSnapshot: candidateJSON, RequestFingerprint: fingerprint,
		IdempotencyKey: req.IdempotencyKey, BillingSnapshot: billingJSON, BillingStatus: MediaBillingStatusPending,
		LeaseUntil: &initializationLease, CreatedAt: now, UpdatedAt: now,
	}
	created, createErr := o.deps.Tasks.Create(ctx, task)
	if createErr != nil {
		if req.IdempotencyKey == "" {
			return nil, fmt.Errorf("create media task: %w", createErr)
		}
		winner, readErr := o.deps.Tasks.GetByIdempotencyKey(ctx, req.UserID, req.APIKeyID, req.IdempotencyKey)
		switch {
		case readErr == nil:
			return o.reuseOrResumeTask(ctx, winner, req, inputs, fingerprint, settings)
		case errors.Is(readErr, ErrMediaTaskNotFound):
			return nil, fmt.Errorf("create media task: %w", createErr)
		default:
			return nil, errors.Join(
				fmt.Errorf("create media task: %w", createErr),
				fmt.Errorf("reload media idempotency winner: %w", readErr),
			)
		}
	}
	task = created
	return o.initializeAndEnqueue(ctx, task, req, inputs, billingSnapshot, settings)
}

func (o *MediaOrchestrator) GetForUser(ctx context.Context, publicID string, userID int64) (*MediaTask, []MediaArtifact, error) {
	if err := o.validateRead(); err != nil {
		return nil, nil, err
	}
	task, err := o.deps.Tasks.GetByPublicIDForUser(ctx, publicID, userID)
	if err != nil {
		if errors.Is(err, ErrMediaTaskNotFound) {
			return nil, nil, ErrMediaTaskNotFound
		}
		return nil, nil, fmt.Errorf("load media task for user: %w", err)
	}
	artifacts, err := o.deps.Artifacts.ListByTaskID(ctx, task.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list media task artifacts: %w", err)
	}
	return sanitizeMediaTaskForUser(task), sanitizeMediaArtifactsForUser(artifacts), nil
}

func (o *MediaOrchestrator) waitSync(ctx context.Context, task *MediaTask, settings *SystemSettings) (*MediaCreateResult, error) {
	terminal, unsubscribe, err := o.deps.Queue.SubscribeTerminal(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("subscribe media terminal state: %w", err)
	}
	if unsubscribe == nil {
		return nil, errors.New("media terminal unsubscribe function is nil")
	}
	defer unsubscribe()

	var timeout <-chan time.Time
	var timer MediaTimer
	if settings.MediaSyncWaitTimeoutSeconds > 0 {
		timer = o.deps.Clock.NewTimer(time.Duration(settings.MediaSyncWaitTimeoutSeconds) * time.Second)
		if timer == nil {
			return nil, errors.New("media clock returned a nil timer")
		}
		timeout = timer.Channel()
		defer timer.Stop()
	}

	for {
		current, err := o.deps.Tasks.GetByID(ctx, task.ID)
		if err != nil {
			return nil, fmt.Errorf("load media task while waiting: %w", err)
		}
		if current.Status.IsTerminal() {
			return o.terminalResult(ctx, current)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			timeoutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaOrchestratorDetachedTimeout)
			result, timeoutErr := o.handleSyncTimeout(timeoutCtx, current, settings)
			cancel()
			return result, timeoutErr
		case _, ok := <-terminal:
			if !ok {
				current, readErr := o.deps.Tasks.GetByID(ctx, task.ID)
				if readErr == nil && current.Status.IsTerminal() {
					continue
				}
				if readErr != nil {
					return nil, errors.Join(ErrMediaTerminalSubscriptionClosed, fmt.Errorf("recheck media task: %w", readErr))
				}
				return nil, ErrMediaTerminalSubscriptionClosed
			}
		}
	}
}

func (o *MediaOrchestrator) handleSyncTimeout(ctx context.Context, task *MediaTask, settings *SystemSettings) (*MediaCreateResult, error) {
	current, err := o.deps.Tasks.GetByID(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("recheck media task at sync timeout: %w", err)
	}
	if current.Status.IsTerminal() {
		return o.terminalResult(ctx, current)
	}
	if current.SyncFallback {
		return &MediaCreateResult{Task: current, Disposition: MediaCreateDispositionFallbackAsync}, nil
	}
	if settings.MediaSyncTimeoutFallbackAsyncEnabled {
		marked, markErr := o.deps.Tasks.MarkSyncFallback(ctx, current.ID, o.deps.Clock.Now().UTC())
		fresh, readErr := o.deps.Tasks.GetByID(ctx, current.ID)
		if readErr != nil {
			if markErr != nil {
				return nil, errors.Join(
					fmt.Errorf("mark media sync fallback: %w", markErr),
					fmt.Errorf("reload media sync fallback task: %w", readErr),
				)
			}
			return nil, fmt.Errorf("reload media sync fallback task: %w", readErr)
		}
		if fresh.Status.IsTerminal() {
			return o.terminalResult(ctx, fresh)
		}
		if fresh.SyncFallback {
			return &MediaCreateResult{Task: fresh, Disposition: MediaCreateDispositionFallbackAsync}, nil
		}
		if markErr != nil {
			return nil, fmt.Errorf("mark media sync fallback: %w", markErr)
		}
		if !marked && !fresh.SyncFallback {
			return nil, fmt.Errorf("%w: mark media sync fallback", ErrMediaOrchestratorStateConflict)
		}
		return &MediaCreateResult{Task: fresh, Disposition: MediaCreateDispositionFallbackAsync}, nil
	}

	var settlement MediaFailureSettlement
	var recovery []byte
	var finishedAt time.Time
	transitioned := false
	loadedPersistedTimeout := false
	for attempt := 0; attempt < mediaSyncTimeoutTransitionTries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		settlement = mediaSyncTimeoutSettlement(current, settings)
		var marshalErr error
		recovery, marshalErr = json.Marshal(MediaSettlementPlan{
			Type:    MediaSettlementTypeFailure,
			Failure: &settlement,
		})
		if marshalErr != nil {
			return nil, fmt.Errorf("encode media sync timeout settlement recovery: %w", marshalErr)
		}
		finishedAt = o.deps.Clock.Now().UTC()
		var transitionErr error
		transitioned, transitionErr = o.deps.Tasks.TransitionSyncTimeout(
			ctx,
			current.ID,
			current.Version,
			current.Stage,
			current.Status,
			map[string]any{
				"stage": MediaTaskStageFailed, "error_code": "sync_timeout",
				"error_message": "synchronous media wait timed out", "finished_at": finishedAt,
				"settlement_recovery": json.RawMessage(recovery),
			},
		)
		if transitionErr != nil {
			fresh, readErr := o.deps.Tasks.GetByID(ctx, current.ID)
			if readErr != nil {
				return nil, errors.Join(
					fmt.Errorf("fail media task at sync timeout: %w", transitionErr),
					fmt.Errorf("reload media task after timeout transition error: %w", readErr),
				)
			}
			matchingTimeout := fresh.Status == MediaTaskStatusFailed && fresh.Stage == MediaTaskStageFailed &&
				fresh.ErrorCode == "sync_timeout" && mediaSettlementPlansEqual(fresh.SettlementRecovery, recovery)
			if matchingTimeout {
				current = fresh
				transitioned = true
				loadedPersistedTimeout = true
				break
			}
			if fresh.Status.IsTerminal() {
				return o.terminalResult(ctx, fresh)
			}
			if fresh.SyncFallback {
				return &MediaCreateResult{Task: fresh, Disposition: MediaCreateDispositionFallbackAsync}, nil
			}
			return nil, fmt.Errorf("fail media task at sync timeout: %w", transitionErr)
		}
		if transitioned {
			break
		}
		fresh, readErr := o.deps.Tasks.GetByID(ctx, current.ID)
		if readErr != nil {
			return nil, fmt.Errorf("reload media task after timeout CAS: %w", readErr)
		}
		if fresh.Status.IsTerminal() {
			return o.terminalResult(ctx, fresh)
		}
		if fresh.SyncFallback {
			return &MediaCreateResult{Task: fresh, Disposition: MediaCreateDispositionFallbackAsync}, nil
		}
		current = fresh
	}
	if !transitioned {
		return nil, fmt.Errorf("%w: fail media task at sync timeout", ErrMediaOrchestratorStateConflict)
	}
	if !loadedPersistedTimeout {
		current.Status = MediaTaskStatusFailed
		current.Stage = MediaTaskStageFailed
		current.ErrorCode = "sync_timeout"
		current.ErrorMessage = "synchronous media wait timed out"
		current.FinishedAt = &finishedAt
		current.SettlementRecovery = append(json.RawMessage(nil), recovery...)
		current.Version++
	}
	if o.deps.Controller != nil {
		o.deps.Controller.StopForSyncTimeout(current.ID)
	}

	settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaOrchestratorDetachedTimeout)
	defer cancel()
	_ = o.deps.Settlement.SettleFailure(settlementCtx, current, settlement)
	if fresh, readErr := o.deps.Tasks.GetByID(settlementCtx, current.ID); readErr == nil {
		current = fresh
	}
	gatewayTask := *current
	gatewayTask.PublicID = ""
	return &MediaCreateResult{Task: &gatewayTask, Disposition: MediaCreateDispositionGatewayTimeout}, nil
}

func mediaSyncTimeoutSettlement(task *MediaTask, settings *SystemSettings) MediaFailureSettlement {
	settlement := MediaFailureSettlement{
		Kind: MediaFailureKindSyncTimeout, RefundRatio: 1, ErrorCode: "sync_timeout",
	}
	if task == nil || task.SubmittedAt == nil || settings.MediaSyncTimeoutBillingPolicy != MediaTimeoutBillingPolicyPenalty {
		return settlement
	}
	switch task.Stage {
	case MediaTaskStageSubmitting, MediaTaskStageGenerating, MediaTaskStagePolling:
		penalty := settings.MediaSyncTimeoutPenaltyRatio
		if penalty < 0 || penalty > 1 {
			penalty = DefaultMediaSyncTimeoutPenaltyRatio
		}
		settlement.PenaltyRatio = penalty
		settlement.RefundRatio = 1 - penalty
	}
	return settlement
}

func (o *MediaOrchestrator) validateRequest(ctx context.Context, req MediaCreateRequest) (*MediaModelDefinition, []MediaArtifactInput, error) {
	if req.UserID <= 0 || req.APIKeyID <= 0 || req.GroupID <= 0 {
		return nil, nil, ErrInvalidMediaSpec
	}
	if err := req.Spec.Validate(req.MediaType); err != nil {
		return nil, nil, err
	}
	definition, err := o.deps.Registry.Resolve(req.RequestedModel, req.Operation)
	if err != nil {
		return nil, nil, err
	}
	if definition.MediaType != req.MediaType {
		return nil, nil, ErrMediaSpecOutsideModelConstraints
	}
	placeholderIDs := make([]int64, len(req.Inputs))
	for i := range placeholderIDs {
		placeholderIDs[i] = int64(i + 1)
	}
	validationSpec, err := mediaSpecWithInputArtifacts(req.Spec, req.Operation, placeholderIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := o.deps.Registry.ValidateSpec(req.RequestedModel, req.Operation, validationSpec); err != nil {
		return nil, nil, err
	}
	group, err := o.deps.Groups.GetByID(ctx, req.GroupID)
	if err != nil {
		return nil, nil, fmt.Errorf("load media group: %w", err)
	}
	if group == nil || group.ID != req.GroupID {
		return nil, nil, ErrGroupNotFound
	}
	if (req.MediaType == MediaTypeImage && !group.AllowImageGeneration) ||
		(req.MediaType == MediaTypeVideo && !group.AllowVideoGeneration) {
		return nil, nil, ErrMediaGenerationNotAllowed
	}
	if mediaSpecHasPersistedArtifactReferences(req.Spec) {
		return nil, nil, fmt.Errorf("%w: request spec must not contain pre-existing artifact ids", ErrMediaInputNotRecoverable)
	}
	inputs, err := normalizeMediaInputs(req.Inputs, req.Operation)
	if err != nil {
		return nil, nil, err
	}
	return definition, inputs, nil
}

func normalizeMediaInputs(inputs []MediaArtifactInput, operation MediaOperation) ([]MediaArtifactInput, error) {
	if len(inputs) > MaxMediaReferenceInputs {
		return nil, fmt.Errorf("%w: input count exceeds %d", ErrMediaInputNotRecoverable, MaxMediaReferenceInputs)
	}
	var sourceMediaType MediaType
	switch operation {
	case MediaOperationTextToImage, MediaOperationTextToVideo:
		if len(inputs) != 0 {
			return nil, fmt.Errorf("%w: operation %q does not accept inputs", ErrMediaInputNotRecoverable, operation)
		}
		return nil, nil
	case MediaOperationImageToImage, MediaOperationImageEdit, MediaOperationImageToVideo,
		MediaOperationReferenceVideo:
		sourceMediaType = MediaTypeImage
	case MediaOperationVideoExtend, MediaOperationVideoRemix:
		sourceMediaType = MediaTypeVideo
	default:
		return nil, fmt.Errorf("%w: unsupported operation %q", ErrMediaInputNotRecoverable, operation)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: operation %q requires input", ErrMediaInputNotRecoverable, operation)
	}
	result := append([]MediaArtifactInput(nil), inputs...)
	slices.SortFunc(result, func(a, b MediaArtifactInput) int {
		switch {
		case a.Position < b.Position:
			return -1
		case a.Position > b.Position:
			return 1
		default:
			return 0
		}
	})
	for i := range result {
		input := &result[i]
		if len(input.Data) != 0 || input.Position < 0 || (input.Direction != "" && input.Direction != "input") ||
			(input.MediaType != MediaTypeImage && input.MediaType != MediaTypeVideo) {
			return nil, fmt.Errorf("%w: invalid input at position %d", ErrMediaInputNotRecoverable, input.Position)
		}
		if i > 0 && result[i-1].Position == input.Position {
			return nil, fmt.Errorf("%w: duplicate input position %d", ErrMediaInputNotRecoverable, input.Position)
		}
		expectedMediaType := MediaTypeImage
		if i == 0 {
			expectedMediaType = sourceMediaType
		}
		if input.MediaType != expectedMediaType {
			return nil, fmt.Errorf("%w: input position %d must be %s", ErrMediaInputNotRecoverable, input.Position, expectedMediaType)
		}
		input.ContentType = strings.TrimSpace(input.ContentType)
		parsedContentType, _, parseErr := mime.ParseMediaType(input.ContentType)
		slash := strings.IndexByte(parsedContentType, '/')
		if parseErr != nil || slash <= 0 || slash == len(parsedContentType)-1 || parsedContentType[slash+1:] == "*" ||
			parsedContentType[:slash] != string(input.MediaType) {
			return nil, fmt.Errorf("%w: invalid content type at position %d", ErrMediaInputNotRecoverable, input.Position)
		}
		input.ObjectKey = strings.TrimSpace(input.ObjectKey)
		input.ExternalURL = strings.TrimSpace(input.ExternalURL)
		input.UpstreamReference = strings.TrimSpace(input.UpstreamReference)
		references := 0
		if input.ObjectKey != "" {
			references++
			if strings.HasPrefix(input.ObjectKey, "/") || slices.Contains(strings.Split(input.ObjectKey, "/"), "..") {
				return nil, fmt.Errorf("%w: invalid object key at position %d", ErrMediaInputNotRecoverable, input.Position)
			}
		}
		if input.ExternalURL != "" {
			references++
			parsed, err := url.Parse(input.ExternalURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
				return nil, fmt.Errorf("%w: invalid external url at position %d", ErrMediaInputNotRecoverable, input.Position)
			}
		}
		if input.UpstreamReference != "" {
			references++
		}
		if references != 1 {
			return nil, fmt.Errorf("%w: input position %d must have exactly one durable reference", ErrMediaInputNotRecoverable, input.Position)
		}
		if input.ExternalURL == "" {
			checksum := strings.TrimSpace(input.ChecksumSHA256)
			decodedChecksum, err := hex.DecodeString(checksum)
			if input.SizeBytes <= 0 || err != nil || len(decodedChecksum) != sha256.Size {
				return nil, fmt.Errorf("%w: input position %d requires stable upload content identity", ErrMediaInputNotRecoverable, input.Position)
			}
			input.ChecksumSHA256 = strings.ToLower(checksum)
		}
		input.Direction = "input"
		input.Data = nil
	}
	return result, nil
}

func mediaSpecWithInputArtifacts(spec MediaSpec, operation MediaOperation, artifactIDs []int64) (MediaSpec, error) {
	copy := MediaSpec{}
	if spec.Image != nil {
		image := *spec.Image
		image.InputArtifactIDs = append([]int64(nil), spec.Image.InputArtifactIDs...)
		copy.Image = &image
	}
	if spec.Video != nil {
		video := *spec.Video
		video.ReferenceArtifactIDs = append([]int64(nil), spec.Video.ReferenceArtifactIDs...)
		if spec.Video.SourceArtifactID != nil {
			source := *spec.Video.SourceArtifactID
			video.SourceArtifactID = &source
		}
		copy.Video = &video
	}
	if len(artifactIDs) == 0 {
		return copy, nil
	}
	switch operation {
	case MediaOperationImageToImage, MediaOperationImageEdit:
		if copy.Image == nil {
			return MediaSpec{}, ErrInvalidMediaSpec
		}
		copy.Image.InputArtifactIDs = append([]int64(nil), artifactIDs...)
	case MediaOperationImageToVideo, MediaOperationReferenceVideo:
		if copy.Video == nil {
			return MediaSpec{}, ErrInvalidMediaSpec
		}
		copy.Video.ReferenceArtifactIDs = append([]int64(nil), artifactIDs...)
	case MediaOperationVideoExtend, MediaOperationVideoRemix:
		if copy.Video == nil {
			return MediaSpec{}, ErrInvalidMediaSpec
		}
		copy.Video.SourceArtifactID = mediaInt64Pointer(artifactIDs[0])
		copy.Video.ReferenceArtifactIDs = append([]int64(nil), artifactIDs[1:]...)
	default:
		return MediaSpec{}, fmt.Errorf("%w: operation %q does not accept inputs", ErrMediaInputNotRecoverable, operation)
	}
	return copy, nil
}

func mediaSpecHasPersistedArtifactReferences(spec MediaSpec) bool {
	return (spec.Image != nil && len(spec.Image.InputArtifactIDs) > 0) ||
		(spec.Video != nil && (len(spec.Video.ReferenceArtifactIDs) > 0 || spec.Video.SourceArtifactID != nil))
}

func mediaCreateFingerprint(req MediaCreateRequest) (string, error) {
	inputs, err := normalizeMediaInputs(req.Inputs, req.Operation)
	if err != nil && len(req.Inputs) > 0 {
		return "", err
	}
	type stableInputIdentity struct {
		Position        int       `json:"position"`
		MediaType       MediaType `json:"media_type"`
		ContentType     string    `json:"content_type"`
		SizeBytes       int64     `json:"size_bytes"`
		ChecksumSHA256  string    `json:"checksum_sha256"`
		ExternalURL     string    `json:"external_url,omitempty"`
		Width           int       `json:"width,omitempty"`
		Height          int       `json:"height,omitempty"`
		DurationSeconds float64   `json:"duration_seconds,omitempty"`
		Resolution      string    `json:"resolution,omitempty"`
		FPS             float64   `json:"fps,omitempty"`
	}
	stableInputs := make([]stableInputIdentity, len(inputs))
	for i, input := range inputs {
		stableInputs[i] = stableInputIdentity{
			Position: input.Position, MediaType: input.MediaType, ContentType: input.ContentType,
			SizeBytes: input.SizeBytes, ChecksumSHA256: input.ChecksumSHA256, ExternalURL: input.ExternalURL,
			Width: input.Width, Height: input.Height, DurationSeconds: input.DurationSeconds,
			Resolution: input.Resolution, FPS: input.FPS,
		}
	}
	canonical := struct {
		UserID         int64                 `json:"user_id"`
		APIKeyID       int64                 `json:"api_key_id"`
		GroupID        int64                 `json:"group_id"`
		MediaType      MediaType             `json:"media_type"`
		Operation      MediaOperation        `json:"operation"`
		RequestedModel string                `json:"requested_model"`
		Spec           MediaSpec             `json:"spec"`
		Inputs         []stableInputIdentity `json:"inputs,omitempty"`
		ClientAsync    bool                  `json:"client_async"`
		SessionHash    string                `json:"session_hash,omitempty"`
	}{
		UserID: req.UserID, APIKeyID: req.APIKeyID, GroupID: req.GroupID,
		MediaType: req.MediaType, Operation: req.Operation, RequestedModel: strings.TrimSpace(req.RequestedModel),
		Spec: req.Spec, Inputs: stableInputs, ClientAsync: req.ClientAsync, SessionHash: req.SessionHash,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode canonical media request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (o *MediaOrchestrator) initializeAndEnqueue(
	ctx context.Context,
	task *MediaTask,
	req MediaCreateRequest,
	inputs []MediaArtifactInput,
	billingSnapshot MediaBillingSnapshot,
	settings *SystemSettings,
) (*MediaCreateResult, error) {
	inputsDurable := false
	if len(inputs) > 0 {
		artifactIDs, persistErr := o.persistInputs(ctx, task, inputs)
		if persistErr != nil {
			return &MediaCreateResult{Task: task}, o.failWithReconciledPrecharge(ctx, task, billingSnapshot, "system_input", persistErr)
		}
		durableSpec, specErr := mediaSpecWithInputArtifacts(req.Spec, req.Operation, artifactIDs)
		if specErr != nil {
			return &MediaCreateResult{Task: task}, o.failWithReconciledPrecharge(ctx, task, billingSnapshot, "system_input", specErr)
		}
		encoded, encodeErr := json.Marshal(durableSpec)
		if encodeErr != nil {
			return &MediaCreateResult{Task: task}, o.failWithReconciledPrecharge(ctx, task, billingSnapshot, "system_input", fmt.Errorf("encode durable media request spec: %w", encodeErr))
		}
		updated, updateErr := o.deps.Tasks.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"request_spec": json.RawMessage(encoded)})
		if updateErr != nil {
			return &MediaCreateResult{Task: task}, o.failWithReconciledPrecharge(ctx, task, billingSnapshot, "system_input", fmt.Errorf("persist durable media request spec: %w", updateErr))
		}
		if !updated {
			return &MediaCreateResult{Task: task}, fmt.Errorf("%w: persist durable media request spec", ErrMediaOrchestratorStateConflict)
		}
		task.RequestSpec = encoded
		task.Version++
		inputsDurable = true
	}

	billingSnapshot, err := normalizeMediaBillingSnapshot(billingSnapshot)
	var prechargeResult MediaPrechargeResult
	if err == nil {
		prechargeResult, err = o.deps.Billing.Precharge(ctx, task, billingSnapshot)
	}
	if err == nil {
		prechargeResult, err = normalizeMediaPrechargeResult(prechargeResult)
	}
	if err != nil {
		prechargeErr := fmt.Errorf("precharge media task: %w", err)
		if mediaPrechargeNeedsReconciliation(err) {
			return &MediaCreateResult{Task: task, InputsAdopted: inputsDurable}, prechargeErr
		}
		failureCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaOrchestratorDetachedTimeout)
		defer cancel()
		_, transitionErr := o.transitionInitializationFailed(failureCtx, task, "billing_precharge", nil, 0)
		return &MediaCreateResult{Task: task}, errors.Join(prechargeErr, transitionErr)
	}
	priority := MediaQueuePrioritySync
	if req.ClientAsync {
		priority = MediaQueuePriorityAsync
	}
	if err := o.deps.Queue.Enqueue(ctx, task.ID, priority); err != nil {
		return &MediaCreateResult{Task: task}, o.failAfterPrecharge(ctx, task, prechargeResult.PrechargedAmount, "system_queue", fmt.Errorf("enqueue media task: %w", err))
	}
	updated, updateErr := o.deps.Tasks.UpdateQueued(ctx, task.ID, task.Version, map[string]any{
		"billing_status": MediaBillingStatusPrecharged, "precharged_amount": prechargeResult.PrechargedAmount,
		"lease_until": nil,
	})
	readyReconciled := false
	if updateErr != nil || !updated {
		fresh, readErr := o.deps.Tasks.GetByID(ctx, task.ID)
		readyObserved := readErr == nil && fresh != nil && fresh.BillingStatus == MediaBillingStatusPrecharged &&
			fresh.PrechargedAmount == prechargeResult.PrechargedAmount && fresh.LeaseUntil == nil
		if readyObserved {
			task = fresh
			readyReconciled = true
		} else if readErr != nil {
			updateErr = errors.Join(updateErr, fmt.Errorf("reload media task after ready CAS: %w", readErr))
		} else if updateErr == nil {
			updateErr = ErrMediaOrchestratorStateConflict
		}
		if !readyObserved {
			return &MediaCreateResult{Task: task, InputsAdopted: true}, o.failAfterPrecharge(ctx, task, prechargeResult.PrechargedAmount, "system_billing_state", fmt.Errorf("persist media precharge state: %w", updateErr))
		}
	} else {
		task.BillingStatus = MediaBillingStatusPrecharged
		task.PrechargedAmount = prechargeResult.PrechargedAmount
		task.LeaseUntil = nil
		task.Version++
	}

	// The first enqueue is the durable execution record while the task is still
	// fenced pending. This best-effort second enqueue wakes a consumer after the
	// ready CAS; failure leaves the durable ready task for the first message or recovery.
	_ = o.deps.Queue.Enqueue(ctx, task.ID, priority)
	if readyReconciled {
		result, err := o.reuseTask(ctx, task, task.RequestFingerprint, req.ClientAsync, settings)
		return mediaCreateResultWithInputOwnership(result, task, true), err
	}

	if req.ClientAsync {
		return &MediaCreateResult{Task: task, Disposition: MediaCreateDispositionAccepted, InputsAdopted: true}, nil
	}
	if settings == nil {
		var err error
		settings, err = o.loadSettings(ctx)
		if err != nil {
			return &MediaCreateResult{Task: task, InputsAdopted: true}, err
		}
	}
	result, err := o.waitSync(ctx, task, settings)
	return mediaCreateResultWithInputOwnership(result, task, true), err
}

func mediaCreateResultWithInputOwnership(result *MediaCreateResult, task *MediaTask, adopted bool) *MediaCreateResult {
	if result == nil {
		result = &MediaCreateResult{Task: task}
	} else if result.Task == nil {
		result.Task = task
	}
	result.InputsAdopted = adopted
	return result
}

func (o *MediaOrchestrator) persistInputs(ctx context.Context, task *MediaTask, inputs []MediaArtifactInput) ([]int64, error) {
	ids := make([]int64, 0, len(inputs))
	for _, input := range inputs {
		artifact := &MediaArtifact{
			TaskID: task.ID, Direction: "input", Position: input.Position, MediaType: input.MediaType,
			ContentType: input.ContentType, SizeBytes: input.SizeBytes, ChecksumSHA256: input.ChecksumSHA256,
			ObjectKey: input.ObjectKey, PublicURL: input.ExternalURL, UpstreamReference: input.UpstreamReference,
			Resolution: input.Resolution,
		}
		if input.Width > 0 {
			artifact.Width = mediaIntPointer(input.Width)
		}
		if input.Height > 0 {
			artifact.Height = mediaIntPointer(input.Height)
		}
		if input.DurationSeconds > 0 {
			artifact.DurationSeconds = mediaFloatPointer(input.DurationSeconds)
		}
		if input.FPS > 0 {
			artifact.FPS = mediaFloatPointer(input.FPS)
		}
		created, err := o.deps.Artifacts.Create(ctx, artifact)
		if err != nil {
			return nil, fmt.Errorf("persist media input position %d: %w", input.Position, err)
		}
		if created == nil || created.ID <= 0 {
			return nil, fmt.Errorf("persist media input position %d: repository returned invalid artifact", input.Position)
		}
		ids = append(ids, created.ID)
	}
	return ids, nil
}

func (o *MediaOrchestrator) reuseTask(ctx context.Context, task *MediaTask, fingerprint string, clientAsync bool, settings *SystemSettings) (*MediaCreateResult, error) {
	if task == nil || task.RequestFingerprint != fingerprint {
		return nil, ErrMediaIdempotencyConflict
	}
	if task.Status.IsTerminal() {
		return o.terminalResult(ctx, task)
	}
	if task.SyncFallback {
		return &MediaCreateResult{Task: task, Disposition: MediaCreateDispositionFallbackAsync}, nil
	}
	if clientAsync {
		return &MediaCreateResult{Task: task, Disposition: MediaCreateDispositionAccepted}, nil
	}
	if settings == nil {
		var err error
		settings, err = o.loadSettings(ctx)
		if err != nil {
			return nil, err
		}
	}
	return o.waitSync(ctx, task, settings)
}

func (o *MediaOrchestrator) reuseOrResumeTask(
	ctx context.Context,
	task *MediaTask,
	req MediaCreateRequest,
	inputs []MediaArtifactInput,
	fingerprint string,
	settings *SystemSettings,
) (*MediaCreateResult, error) {
	if task == nil || task.RequestFingerprint != fingerprint {
		return nil, ErrMediaIdempotencyConflict
	}
	if task.Status.IsTerminal() {
		return o.terminalResult(ctx, task)
	}
	if task.Status != MediaTaskStatusQueued {
		if task.BillingStatus != MediaBillingStatusPrecharged {
			return nil, ErrMediaTaskInitializing
		}
		return o.reuseTask(ctx, task, fingerprint, req.ClientAsync, settings)
	}
	if task.BillingStatus == MediaBillingStatusPrecharged && task.LeaseUntil == nil {
		return o.reuseTask(ctx, task, fingerprint, req.ClientAsync, settings)
	}
	now := o.deps.Clock.Now().UTC()
	if task.LeaseUntil != nil && task.LeaseUntil.After(now) {
		return nil, ErrMediaTaskInitializing
	}
	if task.BillingStatus != MediaBillingStatusPending && task.BillingStatus != MediaBillingStatusPrecharged {
		return nil, fmt.Errorf("%w: task %d has billing status %q", ErrMediaOrchestratorStateConflict, task.ID, task.BillingStatus)
	}
	initializationLease := now.Add(mediaTaskInitializationLease)
	acquired, err := o.deps.Tasks.UpdateQueued(ctx, task.ID, task.Version, map[string]any{"lease_until": initializationLease})
	if err != nil {
		return nil, fmt.Errorf("acquire media initialization lease: %w", err)
	}
	if !acquired {
		fresh, readErr := o.deps.Tasks.GetByID(ctx, task.ID)
		if readErr != nil {
			return nil, fmt.Errorf("reload media initialization lease: %w", readErr)
		}
		if fresh.BillingStatus == MediaBillingStatusPrecharged && fresh.LeaseUntil == nil {
			return o.reuseTask(ctx, fresh, fingerprint, req.ClientAsync, settings)
		}
		return nil, ErrMediaTaskInitializing
	}
	task.LeaseUntil = &initializationLease
	task.Version++
	var billingSnapshot MediaBillingSnapshot
	if err := json.Unmarshal(task.BillingSnapshot, &billingSnapshot); err != nil {
		return nil, fmt.Errorf("decode media billing snapshot: %w", err)
	}
	return o.initializeAndEnqueue(ctx, task, req, inputs, billingSnapshot, settings)
}

func (o *MediaOrchestrator) loadSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := o.deps.Settings.GetAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load media runtime settings: %w", err)
	}
	if settings == nil {
		return nil, errors.New("media runtime settings are nil")
	}
	if settings.MediaSyncWaitTimeoutSeconds < 0 ||
		int64(settings.MediaSyncWaitTimeoutSeconds) > math.MaxInt64/int64(time.Second) {
		return nil, errors.New("media sync wait timeout is outside time.Duration range")
	}
	if settings.MediaSyncTimeoutBillingPolicy != MediaTimeoutBillingPolicyRefund &&
		settings.MediaSyncTimeoutBillingPolicy != MediaTimeoutBillingPolicyPenalty {
		return nil, errors.New("media sync timeout billing policy is invalid")
	}
	if math.IsNaN(settings.MediaSyncTimeoutPenaltyRatio) || math.IsInf(settings.MediaSyncTimeoutPenaltyRatio, 0) ||
		settings.MediaSyncTimeoutPenaltyRatio < 0 || settings.MediaSyncTimeoutPenaltyRatio > 1 {
		return nil, errors.New("media sync timeout penalty ratio is invalid")
	}
	return settings, nil
}

func (o *MediaOrchestrator) terminalResult(ctx context.Context, task *MediaTask) (*MediaCreateResult, error) {
	if task.Status == MediaTaskStatusCompleted {
		artifacts, err := o.deps.Artifacts.ListByTaskID(ctx, task.ID)
		if err != nil {
			return nil, fmt.Errorf("list completed media artifacts: %w", err)
		}
		return &MediaCreateResult{Task: task, Artifacts: artifacts, Disposition: MediaCreateDispositionCompleted}, nil
	}
	if task.Status == MediaTaskStatusFailed {
		if task.ErrorCode == "sync_timeout" {
			gatewayTask := *task
			gatewayTask.PublicID = ""
			return &MediaCreateResult{Task: &gatewayTask, Disposition: MediaCreateDispositionGatewayTimeout}, nil
		}
		return &MediaCreateResult{Task: task, Disposition: MediaCreateDispositionFailed}, nil
	}
	return nil, fmt.Errorf("media task %d is not terminal", task.ID)
}

func (o *MediaOrchestrator) failWithReconciledPrecharge(
	ctx context.Context,
	task *MediaTask,
	billingSnapshot MediaBillingSnapshot,
	code string,
	cause error,
) error {
	compensationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaOrchestratorDetachedTimeout)
	defer cancel()
	billingSnapshot, err := normalizeMediaBillingSnapshot(billingSnapshot)
	var prechargeResult MediaPrechargeResult
	if err == nil {
		prechargeResult, err = o.deps.Billing.Precharge(compensationCtx, task, billingSnapshot)
	}
	if err == nil {
		prechargeResult, err = normalizeMediaPrechargeResult(prechargeResult)
	}
	if err != nil {
		prechargeErr := fmt.Errorf("reconcile media task precharge before failure: %w", err)
		if mediaPrechargeNeedsReconciliation(err) {
			return errors.Join(cause, prechargeErr)
		}
		_, transitionErr := o.transitionInitializationFailed(compensationCtx, task, "billing_precharge", nil, 0)
		return errors.Join(cause, prechargeErr, transitionErr)
	}
	return o.failAfterPrecharge(compensationCtx, task, prechargeResult.PrechargedAmount, code, cause)
}

func (o *MediaOrchestrator) failAfterPrecharge(ctx context.Context, task *MediaTask, prechargedAmount float64, code string, cause error) error {
	compensationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaOrchestratorDetachedTimeout)
	defer cancel()
	settlement := MediaFailureSettlement{Kind: MediaFailureKindSystem, RefundRatio: 1, ErrorCode: code}
	transitioned, transitionErr := o.transitionInitializationFailed(compensationCtx, task, code, &settlement, prechargedAmount)
	if transitionErr != nil {
		return errors.Join(cause, transitionErr)
	}
	if !transitioned {
		return cause
	}
	settlementErr := o.deps.Settlement.SettleFailure(compensationCtx, task, settlement)
	return errors.Join(cause, settlementErr)
}

func (o *MediaOrchestrator) transitionInitializationFailed(
	ctx context.Context,
	task *MediaTask,
	code string,
	settlement *MediaFailureSettlement,
	prechargedAmount float64,
) (bool, error) {
	finishedAt := o.deps.Clock.Now().UTC()
	updates := map[string]any{
		"stage": MediaTaskStageFailed, "error_code": code, "error_message": code,
		"finished_at": finishedAt, "lease_until": nil,
	}
	var recovery json.RawMessage
	if settlement != nil {
		encoded, err := json.Marshal(MediaSettlementPlan{Type: MediaSettlementTypeFailure, Failure: settlement})
		if err != nil {
			return false, fmt.Errorf("encode media task %d initialization recovery: %w", task.ID, err)
		}
		recovery = json.RawMessage(encoded)
		updates["billing_status"] = MediaBillingStatusPrecharged
		updates["precharged_amount"] = prechargedAmount
		updates["settlement_recovery"] = recovery
	} else {
		updates["billing_status"] = MediaBillingStatusSettled
		updates["precharged_amount"] = float64(0)
	}
	expectedVersion := task.Version
	transitioned, transitionErr := o.deps.Tasks.TransitionQueued(
		ctx, task.ID, expectedVersion, MediaTaskStatusFailed, updates,
	)
	if transitioned {
		task.Status = MediaTaskStatusFailed
		task.Stage = MediaTaskStageFailed
		task.ErrorCode = code
		task.ErrorMessage = code
		task.FinishedAt = &finishedAt
		task.LeaseUntil = nil
		task.Version++
		if settlement != nil {
			task.BillingStatus = MediaBillingStatusPrecharged
			task.PrechargedAmount = prechargedAmount
			task.SettlementRecovery = append(json.RawMessage(nil), recovery...)
		} else {
			task.BillingStatus = MediaBillingStatusSettled
			task.PrechargedAmount = 0
		}
		return true, nil
	}
	fresh, readErr := o.deps.Tasks.GetByID(ctx, task.ID)
	if readErr != nil {
		return false, errors.Join(
			fmt.Errorf("fail media task %d: %w", task.ID, transitionErr),
			fmt.Errorf("reload media task after initialization failure CAS: %w", readErr),
		)
	}
	if mediaInitializationFailureMatches(fresh, code, recovery, prechargedAmount, settlement != nil) {
		*task = *fresh
		return true, nil
	}
	*task = *fresh
	if fresh.Status != MediaTaskStatusQueued || fresh.Version != expectedVersion {
		return false, nil
	}
	if transitionErr != nil {
		return false, fmt.Errorf("fail media task %d: %w", task.ID, transitionErr)
	}
	return false, fmt.Errorf("%w: fail media task %d", ErrMediaOrchestratorStateConflict, task.ID)
}

func mediaInitializationFailureMatches(
	task *MediaTask,
	code string,
	recovery json.RawMessage,
	prechargedAmount float64,
	precharged bool,
) bool {
	if task == nil || task.Status != MediaTaskStatusFailed || task.ErrorCode != code {
		return false
	}
	if !precharged {
		return task.BillingStatus == MediaBillingStatusSettled &&
			task.PrechargedAmount == 0 && len(task.SettlementRecovery) == 0
	}
	return task.BillingStatus == MediaBillingStatusPrecharged &&
		task.PrechargedAmount == prechargedAmount &&
		mediaSettlementPlansEqual(task.SettlementRecovery, recovery)
}

func sanitizeMediaTaskForUser(task *MediaTask) *MediaTask {
	if task == nil {
		return nil
	}
	copy := *task
	copy.ID = 0
	copy.UserID = 0
	copy.APIKeyID = 0
	copy.GroupID = 0
	copy.ChannelID = nil
	copy.AccountID = nil
	copy.UpstreamModel = ""
	copy.Adapter = ""
	copy.NativeAsyncMode = ""
	copy.ClientAsync = false
	copy.SyncFallback = false
	copy.Stage = ""
	copy.RequestSpec = nil
	copy.CandidateSnapshot = nil
	copy.RequestFingerprint = ""
	copy.IdempotencyKey = ""
	copy.UpstreamTaskID = ""
	copy.PollMetadata = nil
	copy.BillingSnapshot = nil
	copy.SettlementPlan = nil
	copy.SettlementRecovery = nil
	copy.BillingStatus = ""
	copy.PrechargedAmount = 0
	copy.FinalAmount = 0
	copy.RefundedAmount = 0
	copy.AdditionalChargedAmount = 0
	copy.RetryCount = 0
	copy.ErrorMessage = ""
	copy.WorkerID = ""
	copy.ClaimToken = ""
	copy.LeaseUntil = nil
	copy.Version = 0
	copy.SubmittedAt = nil
	copy.StartedAt = nil
	copy.FinishedAt = nil
	copy.SyncFallbackAt = nil
	copy.UpdatedAt = time.Time{}
	return &copy
}

func sanitizeMediaArtifactsForUser(artifacts []MediaArtifact) []MediaArtifact {
	result := append([]MediaArtifact(nil), artifacts...)
	for i := range result {
		result[i].ObjectKey = ""
		result[i].UpstreamReference = ""
	}
	return result
}

func (o *MediaOrchestrator) validate() error {
	if o == nil || o.deps.Registry == nil || o.deps.Groups == nil || o.deps.Scheduler == nil ||
		o.deps.Settings == nil || o.deps.ContentPolicy == nil || o.deps.Pricing == nil ||
		o.deps.Tasks == nil || o.deps.Artifacts == nil || o.deps.Billing == nil ||
		o.deps.Settlement == nil || o.deps.Queue == nil || o.deps.Clock == nil || o.deps.PublicIDGenerator == nil {
		return errors.New("media orchestrator dependencies are incomplete")
	}
	return nil
}

func (o *MediaOrchestrator) validateRead() error {
	if o == nil || o.deps.Tasks == nil || o.deps.Artifacts == nil {
		return errors.New("media orchestrator read dependencies are incomplete")
	}
	return nil
}

func newMediaTaskPublicID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "task_" + hex.EncodeToString(random[:]), nil
}

func mediaIntPointer(value int) *int           { return &value }
func mediaFloatPointer(value float64) *float64 { return &value }
