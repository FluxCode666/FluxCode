package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/mediaartifact"
	"github.com/Wei-Shaw/sub2api/ent/mediatask"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type mediaTaskRepository struct {
	client *dbent.Client
}

var (
	updateQueuedFields = mediaTaskUpdateFieldSet(
		"channel_id",
		"account_id",
		"upstream_model",
		"adapter",
		"native_async_mode",
		"stage",
		"progress",
		"request_spec",
		"candidate_snapshot",
		"billing_snapshot",
		"settlement_plan",
		"billing_status",
		"precharged_amount",
		"lease_until",
		"submitted_at",
	)
	transitionQueuedFields = mediaTaskUpdateFieldSet(
		"stage",
		"error_code",
		"error_message",
		"finished_at",
		"billing_status",
		"precharged_amount",
		"settlement_recovery",
		"lease_until",
	)
	updateClaimedFields = mediaTaskUpdateFieldSet(
		"channel_id",
		"account_id",
		"upstream_model",
		"adapter",
		"native_async_mode",
		"upstream_task_id",
		"poll_metadata",
		"stage",
		"progress",
		"submitted_at",
		"started_at",
		"retry_count",
		"error_code",
		"error_message",
	)
	transitionFields = mediaTaskUpdateFieldSet(
		"stage",
		"progress",
		"retry_count",
		"error_code",
		"error_message",
		"finished_at",
		"settlement_recovery",
	)
	transitionClaimedFields = mediaTaskUpdateFieldSet(
		"stage",
		"progress",
		"retry_count",
		"error_code",
		"error_message",
		"finished_at",
		"settlement_recovery",
	)
	updateBillingFields = mediaTaskUpdateFieldSet(
		"billing_snapshot",
		"settlement_plan",
		"billing_status",
		"precharged_amount",
		"final_amount",
		"refunded_amount",
	)
)

func NewMediaTaskRepository(client *dbent.Client) service.MediaTaskRepository {
	return &mediaTaskRepository{client: client}
}

func (r *mediaTaskRepository) Create(ctx context.Context, task *service.MediaTask) (*service.MediaTask, error) {
	create := r.client.MediaTask.Create().
		SetPublicID(task.PublicID).
		SetUserID(task.UserID).
		SetAPIKeyID(task.APIKeyID).
		SetGroupID(task.GroupID).
		SetNillableChannelID(task.ChannelID).
		SetNillableAccountID(task.AccountID).
		SetMediaType(string(task.MediaType)).
		SetOperation(string(task.Operation)).
		SetRequestedModel(task.RequestedModel).
		SetClientAsync(task.ClientAsync).
		SetSyncFallback(task.SyncFallback).
		SetProgress(task.Progress).
		SetRequestSpec(cloneRawMessage(task.RequestSpec)).
		SetCandidateSnapshot(cloneRawMessage(task.CandidateSnapshot)).
		SetRequestFingerprint(task.RequestFingerprint).
		SetPrechargedAmount(task.PrechargedAmount).
		SetFinalAmount(task.FinalAmount).
		SetRefundedAmount(task.RefundedAmount).
		SetRetryCount(task.RetryCount).
		SetNillableLeaseUntil(utcTimePointer(task.LeaseUntil)).
		SetNillableSubmittedAt(utcTimePointer(task.SubmittedAt)).
		SetNillableStartedAt(utcTimePointer(task.StartedAt)).
		SetNillableFinishedAt(utcTimePointer(task.FinishedAt)).
		SetNillableSyncFallbackAt(utcTimePointer(task.SyncFallbackAt))
	if task.UpstreamModel != "" {
		create.SetUpstreamModel(task.UpstreamModel)
	}
	if task.Adapter != "" {
		create.SetAdapter(task.Adapter)
	}
	if task.NativeAsyncMode != "" {
		create.SetNativeAsyncMode(string(task.NativeAsyncMode))
	}
	if task.Status != "" {
		create.SetStatus(string(task.Status))
	}
	if task.Stage != "" {
		create.SetStage(string(task.Stage))
	}
	if task.IdempotencyKey != "" {
		create.SetIdempotencyKey(task.IdempotencyKey)
	}
	if task.UpstreamTaskID != "" {
		create.SetUpstreamTaskID(task.UpstreamTaskID)
	}
	if len(task.PollMetadata) != 0 {
		create.SetPollMetadata(cloneRawMessage(task.PollMetadata))
	}
	if len(task.BillingSnapshot) != 0 {
		create.SetBillingSnapshot(cloneRawMessage(task.BillingSnapshot))
	}
	if len(task.SettlementPlan) != 0 {
		create.SetSettlementPlan(cloneRawMessage(task.SettlementPlan))
	}
	if len(task.SettlementRecovery) != 0 {
		create.SetSettlementRecovery(cloneRawMessage(task.SettlementRecovery))
	}
	if task.BillingStatus != "" {
		create.SetBillingStatus(task.BillingStatus)
	}
	if task.ErrorCode != "" {
		create.SetErrorCode(task.ErrorCode)
	}
	if task.ErrorMessage != "" {
		create.SetErrorMessage(task.ErrorMessage)
	}
	if task.WorkerID != "" {
		create.SetWorkerID(task.WorkerID)
	}
	if task.Version != 0 {
		create.SetVersion(task.Version)
	}
	if !task.CreatedAt.IsZero() {
		create.SetCreatedAt(task.CreatedAt)
	}
	if !task.UpdatedAt.IsZero() {
		create.SetUpdatedAt(task.UpdatedAt)
	}

	created, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}
	return mediaTaskFromEnt(created), nil
}

func (r *mediaTaskRepository) GetByID(ctx context.Context, id int64) (*service.MediaTask, error) {
	task, err := r.client.MediaTask.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return mediaTaskFromEnt(task), nil
}

func (r *mediaTaskRepository) GetByPublicIDForUser(ctx context.Context, publicID string, userID int64) (*service.MediaTask, error) {
	task, err := r.client.MediaTask.Query().
		Where(mediatask.PublicIDEQ(publicID), mediatask.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %w", service.ErrMediaTaskNotFound, err)
		}
		return nil, err
	}
	return mediaTaskFromEnt(task), nil
}

func (r *mediaTaskRepository) GetByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*service.MediaTask, error) {
	task, err := r.client.MediaTask.Query().
		Where(
			mediatask.UserIDEQ(userID),
			mediatask.APIKeyIDEQ(apiKeyID),
			mediatask.IdempotencyKeyEQ(key),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %w", service.ErrMediaTaskNotFound, err)
		}
		return nil, err
	}
	return mediaTaskFromEnt(task), nil
}

func (r *mediaTaskRepository) UpdateQueued(ctx context.Context, id, version int64, updates map[string]any) (bool, error) {
	if err := validateMediaTaskUpdateFields("UpdateQueued", updates, updateQueuedFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.StatusEQ(string(service.MediaTaskStatusQueued)),
			mediatask.VersionEQ(version),
		)
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.AddVersion(1).Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) TransitionQueued(
	ctx context.Context,
	id, expectedVersion int64,
	to service.MediaTaskStatus,
	updates map[string]any,
) (bool, error) {
	if to != service.MediaTaskStatusFailed || !service.MediaTaskStatusQueued.CanTransitionTo(to) {
		return false, nil
	}
	if err := validateMediaTaskUpdateFields("TransitionQueued", updates, transitionQueuedFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.StatusEQ(string(service.MediaTaskStatusQueued)),
			mediatask.VersionEQ(expectedVersion),
		)
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.
		SetStatus(string(to)).
		AddVersion(1).
		Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) Claim(ctx context.Context, id int64, workerID string, leaseUntil time.Time, version int64) (bool, error) {
	now := time.Now().UTC()
	leaseAvailable := mediatask.Or(mediatask.LeaseUntilIsNil(), mediatask.LeaseUntilLTE(now))
	claimable := mediatask.Or(
		mediatask.And(
			mediatask.StatusEQ(string(service.MediaTaskStatusQueued)),
			mediatask.BillingStatusEQ(service.MediaBillingStatusPrecharged),
			leaseAvailable,
		),
		mediatask.And(
			mediatask.StatusEQ(string(service.MediaTaskStatusInProgress)),
			leaseAvailable,
		),
	)
	updated, err := r.client.MediaTask.Update().
		Where(mediatask.IDEQ(id), mediatask.VersionEQ(version), claimable).
		SetStatus(string(service.MediaTaskStatusInProgress)).
		SetWorkerID(workerID).
		SetLeaseUntil(leaseUntil.UTC()).
		AddVersion(1).
		Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) RenewLease(ctx context.Context, id int64, workerID string, leaseUntil time.Time) (bool, error) {
	updated, err := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.StatusEQ(string(service.MediaTaskStatusInProgress)),
			mediatask.WorkerIDEQ(workerID),
			mediatask.LeaseUntilNotNil(),
			mediatask.LeaseUntilGT(time.Now().UTC()),
		).
		SetLeaseUntil(leaseUntil.UTC()).
		Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) UpdateClaimed(ctx context.Context, id int64, workerID string, updates map[string]any) (bool, error) {
	if err := validateMediaTaskUpdateFields("UpdateClaimed", updates, updateClaimedFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.StatusEQ(string(service.MediaTaskStatusInProgress)),
			mediatask.WorkerIDEQ(workerID),
			mediatask.LeaseUntilNotNil(),
			mediatask.LeaseUntilGT(time.Now().UTC()),
		)
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.AddVersion(1).Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) Transition(ctx context.Context, id int64, from, to service.MediaTaskStatus, updates map[string]any) (bool, error) {
	if !from.CanTransitionTo(to) {
		return false, nil
	}
	if err := validateMediaTaskUpdateFields("Transition", updates, transitionFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(mediatask.IDEQ(id), mediatask.StatusEQ(string(from)))
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.SetStatus(string(to)).Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) TransitionSyncTimeout(
	ctx context.Context,
	id, expectedVersion int64,
	from service.MediaTaskStatus,
	updates map[string]any,
) (bool, error) {
	if !from.CanTransitionTo(service.MediaTaskStatusFailed) {
		return false, nil
	}
	if err := validateMediaTaskUpdateFields("TransitionSyncTimeout", updates, transitionFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.StatusEQ(string(from)),
			mediatask.VersionEQ(expectedVersion),
			mediatask.SyncFallbackEQ(false),
		)
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.
		SetStatus(string(service.MediaTaskStatusFailed)).
		AddVersion(1).
		Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) TransitionClaimed(
	ctx context.Context,
	id int64,
	workerID string,
	expectedVersion int64,
	from, to service.MediaTaskStatus,
	updates map[string]any,
) (bool, error) {
	if !from.CanTransitionTo(to) {
		return false, nil
	}
	if err := validateMediaTaskUpdateFields("TransitionClaimed", updates, transitionClaimedFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.StatusEQ(string(from)),
			mediatask.WorkerIDEQ(workerID),
			mediatask.VersionEQ(expectedVersion),
			mediatask.LeaseUntilNotNil(),
			mediatask.LeaseUntilGT(time.Now().UTC()),
		)
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.
		SetStatus(string(to)).
		AddVersion(1).
		Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) MarkSyncFallback(ctx context.Context, id int64, at time.Time) (bool, error) {
	updated, err := r.client.MediaTask.Update().
		Where(
			mediatask.IDEQ(id),
			mediatask.SyncFallbackEQ(false),
			mediatask.StatusNotIn(
				string(service.MediaTaskStatusCompleted),
				string(service.MediaTaskStatusFailed),
			),
		).
		SetSyncFallback(true).
		SetSyncFallbackAt(at.UTC()).
		Save(ctx)
	return updated == 1, err
}

func (r *mediaTaskRepository) ListRecoverable(ctx context.Context, now time.Time, limit int) ([]service.MediaTask, error) {
	tasks, err := r.client.MediaTask.Query().
		Where(mediatask.Or(
			mediatask.And(
				mediatask.StatusEQ(string(service.MediaTaskStatusQueued)),
				mediatask.BillingStatusIn(service.MediaBillingStatusPending, service.MediaBillingStatusPrecharged),
				mediatask.Or(mediatask.LeaseUntilIsNil(), mediatask.LeaseUntilLTE(now.UTC())),
			),
			mediatask.And(
				mediatask.StatusEQ(string(service.MediaTaskStatusInProgress)),
				mediatask.Or(mediatask.LeaseUntilIsNil(), mediatask.LeaseUntilLTE(now.UTC())),
			),
		)).
		Order(mediatask.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mediaTasksFromEnt(tasks), nil
}

func (r *mediaTaskRepository) ListSettlementPending(ctx context.Context, limit int) ([]service.MediaTask, error) {
	tasks, err := r.client.MediaTask.Query().
		Where(
			mediatask.BillingStatusIn("precharged", "settling", "retry"),
			mediatask.Or(
				mediatask.SettlementPlanNotNil(),
				mediatask.And(
					mediatask.StatusIn(
						string(service.MediaTaskStatusCompleted),
						string(service.MediaTaskStatusFailed),
					),
					mediatask.SettlementRecoveryNotNil(),
				),
			),
		).
		Order(mediatask.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return mediaTasksFromEnt(tasks), nil
}

func (r *mediaTaskRepository) UpdateBilling(ctx context.Context, id int64, fromStatus string, updates map[string]any) (bool, error) {
	if err := validateMediaTaskUpdateFields("UpdateBilling", updates, updateBillingFields); err != nil {
		return false, err
	}
	update := r.client.MediaTask.Update().
		Where(mediatask.IDEQ(id), mediatask.BillingStatusEQ(fromStatus))
	if err := applyMediaTaskUpdates(update, updates); err != nil {
		return false, err
	}
	updated, err := update.Save(ctx)
	return updated == 1, err
}

type mediaArtifactRepository struct {
	client *dbent.Client
}

func NewMediaArtifactRepository(client *dbent.Client) service.MediaArtifactRepository {
	return &mediaArtifactRepository{client: client}
}

func (r *mediaArtifactRepository) Create(ctx context.Context, artifact *service.MediaArtifact) (*service.MediaArtifact, error) {
	create := r.client.MediaArtifact.Create().
		SetTaskID(artifact.TaskID).
		SetDirection(artifact.Direction).
		SetPosition(artifact.Position).
		SetMediaType(string(artifact.MediaType)).
		SetContentType(artifact.ContentType).
		SetSizeBytes(artifact.SizeBytes).
		SetChecksumSha256(artifact.ChecksumSHA256).
		SetNillableWidth(artifact.Width).
		SetNillableHeight(artifact.Height).
		SetNillableDurationSeconds(artifact.DurationSeconds).
		SetResolution(artifact.Resolution).
		SetNillableFps(artifact.FPS).
		SetNillableExpiresAt(utcTimePointer(artifact.ExpiresAt))
	if artifact.StorageStatus != "" {
		create.SetStorageStatus(artifact.StorageStatus)
	}
	if artifact.ObjectKey != "" {
		create.SetObjectKey(artifact.ObjectKey)
	}
	if artifact.PublicURL != "" {
		create.SetPublicURL(artifact.PublicURL)
	}
	if artifact.UpstreamReference != "" {
		create.SetUpstreamReference(artifact.UpstreamReference)
	}
	if !artifact.CreatedAt.IsZero() {
		create.SetCreatedAt(artifact.CreatedAt)
	}
	if !artifact.UpdatedAt.IsZero() {
		create.SetUpdatedAt(artifact.UpdatedAt)
	}

	created, err := create.Save(ctx)
	if err == nil {
		return mediaArtifactFromEnt(created), nil
	}
	if !dbent.IsConstraintError(err) {
		return nil, err
	}
	existing, queryErr := r.client.MediaArtifact.Query().
		Where(
			mediaartifact.TaskIDEQ(artifact.TaskID),
			mediaartifact.DirectionEQ(artifact.Direction),
			mediaartifact.PositionEQ(artifact.Position),
		).
		Only(ctx)
	if queryErr != nil {
		return nil, err
	}
	return mediaArtifactFromEnt(existing), nil
}

func (r *mediaArtifactRepository) ListByTaskID(ctx context.Context, taskID int64) ([]service.MediaArtifact, error) {
	artifacts, err := r.client.MediaArtifact.Query().
		Where(mediaartifact.TaskIDEQ(taskID)).
		Order(mediaartifact.ByDirection(), mediaartifact.ByPosition(), mediaartifact.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.MediaArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, *mediaArtifactFromEnt(artifact))
	}
	return out, nil
}

func mediaTaskFromEnt(task *dbent.MediaTask) *service.MediaTask {
	if task == nil {
		return nil
	}
	return &service.MediaTask{
		ID:                 task.ID,
		PublicID:           task.PublicID,
		UserID:             task.UserID,
		APIKeyID:           task.APIKeyID,
		GroupID:            task.GroupID,
		ChannelID:          cloneInt64Pointer(task.ChannelID),
		AccountID:          cloneInt64Pointer(task.AccountID),
		MediaType:          service.MediaType(task.MediaType),
		Operation:          service.MediaOperation(task.Operation),
		RequestedModel:     task.RequestedModel,
		UpstreamModel:      task.UpstreamModel,
		Adapter:            task.Adapter,
		NativeAsyncMode:    service.NativeAsyncMode(task.NativeAsyncMode),
		ClientAsync:        task.ClientAsync,
		SyncFallback:       task.SyncFallback,
		Status:             service.MediaTaskStatus(task.Status),
		Stage:              service.MediaTaskStage(task.Stage),
		Progress:           task.Progress,
		RequestSpec:        cloneRawMessage(task.RequestSpec),
		CandidateSnapshot:  cloneRawMessage(task.CandidateSnapshot),
		RequestFingerprint: task.RequestFingerprint,
		IdempotencyKey:     task.IdempotencyKey,
		UpstreamTaskID:     stringFromPointer(task.UpstreamTaskID),
		PollMetadata:       cloneRawMessage(task.PollMetadata),
		BillingSnapshot:    cloneRawMessage(task.BillingSnapshot),
		SettlementPlan:     cloneRawMessage(task.SettlementPlan),
		SettlementRecovery: cloneRawMessage(task.SettlementRecovery),
		BillingStatus:      task.BillingStatus,
		PrechargedAmount:   task.PrechargedAmount,
		FinalAmount:        task.FinalAmount,
		RefundedAmount:     task.RefundedAmount,
		RetryCount:         task.RetryCount,
		ErrorCode:          task.ErrorCode,
		ErrorMessage:       task.ErrorMessage,
		WorkerID:           task.WorkerID,
		LeaseUntil:         cloneTimePointer(task.LeaseUntil),
		Version:            task.Version,
		SubmittedAt:        cloneTimePointer(task.SubmittedAt),
		StartedAt:          cloneTimePointer(task.StartedAt),
		FinishedAt:         cloneTimePointer(task.FinishedAt),
		SyncFallbackAt:     cloneTimePointer(task.SyncFallbackAt),
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func mediaTasksFromEnt(tasks []*dbent.MediaTask) []service.MediaTask {
	out := make([]service.MediaTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, *mediaTaskFromEnt(task))
	}
	return out
}

func mediaArtifactFromEnt(artifact *dbent.MediaArtifact) *service.MediaArtifact {
	if artifact == nil {
		return nil
	}
	return &service.MediaArtifact{
		ID:                artifact.ID,
		TaskID:            artifact.TaskID,
		Direction:         artifact.Direction,
		Position:          artifact.Position,
		MediaType:         service.MediaType(artifact.MediaType),
		ContentType:       artifact.ContentType,
		SizeBytes:         artifact.SizeBytes,
		ChecksumSHA256:    artifact.ChecksumSha256,
		Width:             cloneIntPointer(artifact.Width),
		Height:            cloneIntPointer(artifact.Height),
		DurationSeconds:   cloneFloatPointer(artifact.DurationSeconds),
		Resolution:        artifact.Resolution,
		FPS:               cloneFloatPointer(artifact.Fps),
		StorageStatus:     artifact.StorageStatus,
		ObjectKey:         stringFromPointer(artifact.ObjectKey),
		PublicURL:         stringFromPointer(artifact.PublicURL),
		UpstreamReference: stringFromPointer(artifact.UpstreamReference),
		ExpiresAt:         cloneTimePointer(artifact.ExpiresAt),
		CreatedAt:         artifact.CreatedAt,
		UpdatedAt:         artifact.UpdatedAt,
	}
}

func applyMediaTaskUpdates(update *dbent.MediaTaskUpdate, updates map[string]any) error {
	for field, value := range updates {
		switch field {
		case "channel_id":
			v, err := int64PointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearChannelID()
			} else {
				update.SetChannelID(*v)
			}
		case "account_id":
			v, err := int64PointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearAccountID()
			} else {
				update.SetAccountID(*v)
			}
		case "upstream_model":
			v, err := stringUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetUpstreamModel(v)
		case "adapter":
			v, err := stringUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetAdapter(v)
		case "native_async_mode":
			v, err := nativeAsyncModeUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetNativeAsyncMode(v)
		case "stage":
			v, err := mediaTaskStageUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetStage(v)
		case "progress":
			v, err := intUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetProgress(v)
		case "request_spec":
			v, ok := value.(json.RawMessage)
			if !ok {
				return updateTypeError(field, fmt.Errorf("got %T, want json.RawMessage", value))
			}
			if !json.Valid(v) {
				return updateTypeError(field, errors.New("invalid JSON"))
			}
			update.SetRequestSpec(cloneRawMessage(v))
		case "candidate_snapshot":
			v, err := rawMessageValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetCandidateSnapshot(v)
		case "upstream_task_id":
			v, err := stringPointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearUpstreamTaskID()
			} else {
				update.SetUpstreamTaskID(*v)
			}
		case "poll_metadata":
			v, err := rawMessagePointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearPollMetadata()
			} else {
				update.SetPollMetadata(*v)
			}
		case "billing_snapshot":
			v, err := rawMessagePointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearBillingSnapshot()
			} else {
				update.SetBillingSnapshot(*v)
			}
		case "settlement_plan":
			v, err := rawMessagePointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearSettlementPlan()
			} else {
				update.SetSettlementPlan(*v)
			}
		case "settlement_recovery":
			v, err := rawMessagePointerValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			if v == nil {
				update.ClearSettlementRecovery()
			} else {
				update.SetSettlementRecovery(*v)
			}
		case "billing_status":
			v, err := stringUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetBillingStatus(v)
		case "precharged_amount":
			v, err := floatUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetPrechargedAmount(v)
		case "final_amount":
			v, err := floatUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetFinalAmount(v)
		case "refunded_amount":
			v, err := floatUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetRefundedAmount(v)
		case "retry_count":
			v, err := intUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetRetryCount(v)
		case "error_code":
			v, err := stringUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetErrorCode(v)
		case "error_message":
			v, err := stringUpdateValue(value)
			if err != nil {
				return updateTypeError(field, err)
			}
			update.SetErrorMessage(v)
		case "submitted_at":
			if err := setOptionalTime(update.SetSubmittedAt, update.ClearSubmittedAt, value); err != nil {
				return updateTypeError(field, err)
			}
		case "started_at":
			if err := setOptionalTime(update.SetStartedAt, update.ClearStartedAt, value); err != nil {
				return updateTypeError(field, err)
			}
		case "finished_at":
			if err := setOptionalTime(update.SetFinishedAt, update.ClearFinishedAt, value); err != nil {
				return updateTypeError(field, err)
			}
		case "lease_until":
			if err := setOptionalTime(update.SetLeaseUntil, update.ClearLeaseUntil, value); err != nil {
				return updateTypeError(field, err)
			}
		default:
			return fmt.Errorf("unsupported media task update field %q", field)
		}
	}
	return nil
}

func mediaTaskUpdateFieldSet(fields ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		set[field] = struct{}{}
	}
	return set
}

func validateMediaTaskUpdateFields(operation string, updates map[string]any, allowed map[string]struct{}) error {
	for field := range updates {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("media task %s field %q is not allowed", operation, field)
		}
	}
	return nil
}

func stringUpdateValue(value any) (string, error) {
	if v, ok := value.(string); ok {
		return v, nil
	}
	return "", fmt.Errorf("got %T, want string", value)
}

func nativeAsyncModeUpdateValue(value any) (string, error) {
	v, ok := value.(service.NativeAsyncMode)
	if !ok {
		return "", fmt.Errorf("got %T, want service.NativeAsyncMode", value)
	}
	switch v {
	case service.NativeAsyncUnsupported, service.NativeAsyncOptional, service.NativeAsyncRequired:
		return string(v), nil
	default:
		return "", fmt.Errorf("unknown service.NativeAsyncMode %q", v)
	}
}

func mediaTaskStageUpdateValue(value any) (string, error) {
	v, ok := value.(service.MediaTaskStage)
	if !ok {
		return "", fmt.Errorf("got %T, want service.MediaTaskStage", value)
	}
	switch v {
	case service.MediaTaskStageQueued,
		service.MediaTaskStageScheduling,
		service.MediaTaskStageSubmitting,
		service.MediaTaskStageGenerating,
		service.MediaTaskStagePolling,
		service.MediaTaskStageStoring,
		service.MediaTaskStageSettling,
		service.MediaTaskStageCompleted,
		service.MediaTaskStageFailed:
		return string(v), nil
	default:
		return "", fmt.Errorf("unknown service.MediaTaskStage %q", v)
	}
}

func stringPointerValue(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if v, ok := value.(*string); ok {
		return v, nil
	}
	v, err := stringUpdateValue(value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func intUpdateValue(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("got %T, want int", value)
	}
}

func int64UpdateValue(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("got %T, want int64", value)
	}
}

func int64PointerValue(value any) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if v, ok := value.(*int64); ok {
		return v, nil
	}
	v, err := int64UpdateValue(value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func floatUpdateValue(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("got %T, want number", value)
	}
}

func rawMessageValue(value any) (json.RawMessage, error) {
	switch v := value.(type) {
	case json.RawMessage:
		return cloneRawMessage(v), nil
	case []byte:
		return cloneRawMessage(v), nil
	default:
		return nil, fmt.Errorf("got %T, want json.RawMessage", value)
	}
}

func rawMessagePointerValue(value any) (*json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	v, err := rawMessageValue(value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func timePointerValue(value any) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case time.Time:
		utc := v.UTC()
		return &utc, nil
	case *time.Time:
		return utcTimePointer(v), nil
	default:
		return nil, fmt.Errorf("got %T, want time.Time", value)
	}
}

func setOptionalTime(set func(time.Time) *dbent.MediaTaskUpdate, clear func() *dbent.MediaTaskUpdate, value any) error {
	v, err := timePointerValue(value)
	if err != nil {
		return err
	}
	if v == nil {
		clear()
	} else {
		set(*v)
	}
	return nil
}

func updateTypeError(field string, err error) error {
	return fmt.Errorf("invalid media task update %q: %w", field, err)
}

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func stringFromPointer(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
