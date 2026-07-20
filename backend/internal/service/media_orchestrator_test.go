package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMediaOrchestratorRejectsInvalidPricingSnapshotBeforeTaskCreation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		amount float64
	}{
		{name: "negative", amount: -0.00000001},
		{name: "nan", amount: math.NaN()},
		{name: "positive infinity", amount: math.Inf(1)},
		{name: "negative infinity", amount: math.Inf(-1)},
		{name: "numeric overflow", amount: 1_000_000_000_000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			fixture.pricing.snapshot.EstimatedAmount = tt.amount

			result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrInvalidMediaBillingSnapshot)
			require.Zero(t, fixture.repo.createCalls())
			require.Zero(t, fixture.billing.prechargeCalls())
			require.Zero(t, fixture.queue.enqueueCalls())
		})
	}
}

func TestMediaOrchestratorNormalizesPricingSnapshotBeforeTaskCreation(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.pricing.snapshot.EstimatedAmount = 1.234567894

	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, 1, fixture.repo.createCalls())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	stored := fixture.repo.mustGet(result.Task.ID)
	var snapshot MediaBillingSnapshot
	require.NoError(t, json.Unmarshal(stored.BillingSnapshot, &snapshot))
	require.Equal(t, 1.23456789, snapshot.EstimatedAmount)
	require.Equal(t, 1.23456789, stored.PrechargedAmount)
}

func TestMediaOrchestratorAsyncReturnsAfterDurableEnqueue(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionAccepted, result.Disposition)
	require.True(t, result.InputsAdopted)
	require.Equal(t, MediaQueuePriorityAsync, fixture.queue.lastPriority())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	require.Equal(t, 2, fixture.queue.enqueueCalls())
	require.Equal(t, MediaBillingStatusPrecharged, fixture.repo.mustGet(result.Task.ID).BillingStatus)
}

func TestMediaOrchestratorEnqueuesBeforePublishingReadyAndClearsInitializationLease(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.billing.inspectPrecharge = func(task *MediaTask) {
		stored := fixture.repo.mustGet(task.ID)
		require.Equal(t, MediaBillingStatusPending, stored.BillingStatus)
		require.NotNil(t, stored.LeaseUntil)
		require.True(t, stored.LeaseUntil.After(fixture.clock.Now()))
	}
	fixture.queue.enqueueFunc = func(call int, taskID int64) error {
		stored := fixture.repo.mustGet(taskID)
		if call == 1 {
			require.Equal(t, MediaBillingStatusPending, stored.BillingStatus)
			require.NotNil(t, stored.LeaseUntil)
		} else {
			require.Equal(t, 2, call)
			require.Equal(t, MediaBillingStatusPrecharged, stored.BillingStatus)
			require.Nil(t, stored.LeaseUntil)
		}
		return nil
	}

	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	ready := fixture.repo.mustGet(result.Task.ID)
	require.Equal(t, MediaBillingStatusPrecharged, ready.BillingStatus)
	require.Nil(t, ready.LeaseUntil)
	require.Equal(t, 2, fixture.queue.enqueueCalls())
}

func TestMediaOrchestratorReadyWakeupClaimsImmediatelyAfterEarlyMessageAck(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.settings.MediaSyncWaitTimeoutSeconds = 1
	firstAcked := false
	secondClaimed := false
	fixture.queue.enqueueFunc = func(call int, taskID int64) error {
		stored := fixture.repo.mustGet(taskID)
		claimed, err := fixture.repo.Claim(context.Background(), taskID, "worker-ready-wakeup", "ready-wakeup-claim", time.Now().Add(time.Minute), stored.Version)
		require.NoError(t, err)
		if call == 1 {
			require.False(t, claimed)
			require.Equal(t, MediaBillingStatusPending, stored.BillingStatus)
			firstAcked = true
			return nil
		}
		require.Equal(t, 2, call)
		require.True(t, claimed)
		secondClaimed = true
		fixture.repo.mutate(taskID, func(task *MediaTask) {
			task.Status = MediaTaskStatusCompleted
			task.Stage = MediaTaskStageCompleted
			task.Progress = 100
			task.LeaseUntil = nil
			task.Version++
		})
		fixture.artifacts.mu.Lock()
		fixture.artifacts.items[taskID] = []MediaArtifact{{ID: 51, TaskID: taskID, Direction: "output", Position: 0}}
		fixture.artifacts.mu.Unlock()
		return nil
	}

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.True(t, firstAcked)
	require.True(t, secondClaimed)
	require.Equal(t, 2, fixture.queue.enqueueCalls())
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, int64(51), result.Artifacts[0].ID)
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorReadyWakeupFailureKeepsReadyTaskWithoutRefund(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.pricing.snapshot.EstimatedAmount = 3.25
	actualPrecharge := 2.75
	fixture.billing.actualPrechargeAmount = &actualPrecharge
	wakeupErr := errors.New("ready wakeup unavailable")
	fixture.queue.enqueueFunc = func(call int, _ int64) error {
		if call == 2 {
			return wakeupErr
		}
		return nil
	}

	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionAccepted, result.Disposition)
	require.Equal(t, 2, fixture.queue.enqueueCalls())
	stored := fixture.repo.mustGet(result.Task.ID)
	require.Equal(t, MediaTaskStatusQueued, stored.Status)
	require.Equal(t, MediaBillingStatusPrecharged, stored.BillingStatus)
	require.Equal(t, actualPrecharge, stored.PrechargedAmount)
	require.Nil(t, stored.LeaseUntil)
	require.Empty(t, stored.SettlementRecovery)
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorReadyPublishWriteAppliedButReturnedErrorReusesReadyTask(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.repo.readyWriteAppliedErrors = 1

	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionAccepted, result.Disposition)
	stored := fixture.repo.mustGet(result.Task.ID)
	require.Equal(t, MediaTaskStatusQueued, stored.Status)
	require.Equal(t, MediaBillingStatusPrecharged, stored.BillingStatus)
	require.Equal(t, fixture.pricing.snapshot.EstimatedAmount, stored.PrechargedAmount)
	require.Nil(t, stored.LeaseUntil)
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorDurableEnqueueConservativelyAdoptsInputsWhenReadyOutcomeIsUncertain(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.repo.readyWriteAppliedErrors = 1
	fixture.repo.getErrors = 1
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.Error(t, err)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)

	stored := fixture.repo.mustGet(result.Task.ID)
	require.Equal(t, MediaTaskStatusQueued, stored.Status)
	require.Equal(t, MediaBillingStatusPrecharged, stored.BillingStatus)
	require.Nil(t, stored.LeaseUntil)
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), stored.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
	require.Equal(t, req.Inputs[0].ObjectKey, artifacts[0].ObjectKey)
}

func TestMediaOrchestratorReadyPublishAppliedErrorReloadsRealTerminalResult(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.repo.readyWriteAppliedErrors = 1
	fixture.repo.readyPublishedHook = func(task *MediaTask) {
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		fixture.artifacts.mu.Lock()
		fixture.artifacts.items[task.ID] = []MediaArtifact{{ID: 61, TaskID: task.ID, Direction: "output", Position: 0}}
		fixture.artifacts.mu.Unlock()
	}

	result, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, int64(61), result.Artifacts[0].ID)
	require.Equal(t, 2, fixture.queue.enqueueCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorIdempotencyRetryDoesNotAcceptActiveInitialization(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-initializing"
	fixture.repo.put(orchestratorPendingTaskForRequest(t, fixture, req, fixture.clock.Now().Add(time.Minute)))

	_, err := fixture.orchestrator.Create(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, 0, fixture.billing.prechargeCalls())
	require.Equal(t, 0, fixture.queue.enqueueCalls())
}

func TestMediaOrchestratorIdempotencyRetryTakesOverExpiredInitialization(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-expired-initialization"
	winner := orchestratorPendingTaskForRequest(t, fixture, req, fixture.clock.Now().Add(-time.Minute))
	fixture.repo.put(winner)

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, winner.PublicID, result.Task.PublicID)
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	require.Equal(t, 2, fixture.queue.enqueueCalls())
	ready := fixture.repo.mustGet(winner.ID)
	require.Equal(t, MediaBillingStatusPrecharged, ready.BillingStatus)
	require.Nil(t, ready.LeaseUntil)
}

func TestMediaOrchestratorConcurrentIdempotencyLoserCannotBypassReadiness(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-concurrent-initialization"
	fixture.repo.put(orchestratorPendingTaskForRequest(t, fixture, req, fixture.clock.Now().Add(-time.Minute)))
	entered := make(chan struct{})
	release := make(chan struct{})
	fixture.billing.prechargeEntered = entered
	fixture.billing.prechargeBlock = release

	firstDone := make(chan error, 1)
	go func() {
		_, err := fixture.orchestrator.Create(context.Background(), req)
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("takeover never reached precharge")
	}
	_, secondErr := fixture.orchestrator.Create(context.Background(), req)
	require.Error(t, secondErr)
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	require.Equal(t, 0, fixture.queue.enqueueCalls())
	close(release)
	require.NoError(t, <-firstDone)
	require.Equal(t, 2, fixture.queue.enqueueCalls())
}

func TestMediaOrchestratorStaleInitializationOwnerCannotFailOrRefundTakeoverWinner(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "stale precharge success"},
		{name: "stale precharge failure", err: errors.New("stale precharge response failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			req := validAsyncMediaCreateRequest()
			req.IdempotencyKey = "idem-owner-fence-" + tt.name
			entered := make(chan struct{})
			release := make(chan struct{})
			fixture.billing.prechargeFunc = func(call int, _ *MediaTask) error {
				if call == 1 {
					close(entered)
					<-release
					return tt.err
				}
				return nil
			}

			type createOutcome struct {
				result *MediaCreateResult
				err    error
			}
			firstDone := make(chan createOutcome, 1)
			go func() {
				result, err := fixture.orchestrator.Create(context.Background(), req)
				firstDone <- createOutcome{result: result, err: err}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("first initializer did not reach precharge")
			}
			stored := fixture.repo.onlyTask()
			fixture.repo.mutate(stored.ID, func(task *MediaTask) {
				task.LeaseUntil = mediaTimePointer(fixture.clock.Now().Add(-time.Minute))
			})

			winner, err := fixture.orchestrator.Create(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, MediaCreateDispositionAccepted, winner.Disposition)
			close(release)
			loser := <-firstDone
			if tt.err == nil {
				require.NoError(t, loser.err)
				require.Equal(t, MediaCreateDispositionAccepted, loser.result.Disposition)
			} else {
				require.ErrorIs(t, loser.err, tt.err)
				require.NotNil(t, loser.result)
				require.False(t, loser.result.InputsAdopted)
			}

			ready := fixture.repo.mustGet(stored.ID)
			require.Equal(t, MediaTaskStatusQueued, ready.Status)
			require.Equal(t, MediaBillingStatusPrecharged, ready.BillingStatus)
			require.Nil(t, ready.LeaseUntil)
			require.Empty(t, ready.SettlementRecovery)
			require.Equal(t, 0, fixture.billing.settleFailureCalls())
		})
	}
}

func TestMediaOrchestratorStaleEnqueueFailureCannotFailOrRefundTakeoverWinner(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-stale-enqueue"
	entered := make(chan struct{})
	release := make(chan struct{})
	queueErr := errors.New("stale enqueue response failed")
	fixture.queue.enqueueFunc = func(call int, _ int64) error {
		if call == 1 {
			close(entered)
			<-release
			return queueErr
		}
		return nil
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := fixture.orchestrator.Create(context.Background(), req)
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first initializer did not reach enqueue")
	}
	stored := fixture.repo.onlyTask()
	fixture.repo.mutate(stored.ID, func(task *MediaTask) {
		task.LeaseUntil = mediaTimePointer(fixture.clock.Now().Add(-time.Minute))
	})

	winner, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionAccepted, winner.Disposition)
	close(release)
	require.ErrorIs(t, <-firstDone, queueErr)
	ready := fixture.repo.mustGet(stored.ID)
	require.Equal(t, MediaTaskStatusQueued, ready.Status)
	require.Equal(t, MediaBillingStatusPrecharged, ready.BillingStatus)
	require.Nil(t, ready.LeaseUntil)
	require.Empty(t, ready.SettlementRecovery)
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
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
		SizeBytes: 128, ChecksumSHA256: strings.Repeat("D", 64),
	}}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	stored := fixture.repo.mustGet(result.Task.ID)
	var candidates []MediaAccountCandidateSnapshot
	require.NoError(t, json.Unmarshal(stored.CandidateSnapshot, &candidates))
	require.Len(t, candidates, 1)
	require.Equal(t, fixture.scheduler.candidates[0].AccountID, candidates[0].AccountID)
	require.Equal(t, fixture.scheduler.candidates[0].ResolvedModel, candidates[0].ResolvedModel)
	require.JSONEq(t, `{"image":{"input_artifact_ids":[1],"n":1,"prompt":"cat"}}`, string(candidates[0].ResolvedRequest))
	var spec MediaSpec
	require.NoError(t, json.Unmarshal(stored.RequestSpec, &spec))
	require.Len(t, spec.Image.InputArtifactIDs, 1)
	require.NotContains(t, string(stored.RequestSpec), "checksum")
	require.NotContains(t, string(stored.RequestSpec), "size_bytes")
	artifacts, err := fixture.artifacts.ListByTaskID(context.Background(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, "input", artifacts[0].Direction)
	require.Equal(t, "media/input/source.png", artifacts[0].ObjectKey)
	require.Equal(t, int64(128), artifacts[0].SizeBytes)
	require.Equal(t, strings.Repeat("d", 64), artifacts[0].ChecksumSHA256)
	require.Equal(t, 2, fixture.queue.enqueueCalls())
}

func TestMediaOrchestratorRejectsMappingForOppositeMediaEnvelope(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.scheduler.candidates = []MediaAccountCandidateSnapshot{{
		AccountID: 7, Platform: PlatformGemini,
		ResolvedModel: ResolvedMediaAccountModel{Adapter: "gemini", UpstreamModel: "veo-up", NativeAsyncMode: NativeAsyncRequired,
			RequestMapping: MediaRequestMapping{Rules: []MediaMappingRule{{Source: "video.prompt", Target: "video.text", Operation: "rename"}}}},
	}}
	req := validAsyncMediaCreateRequest()
	req.MediaType = MediaTypeImage
	req.Operation = MediaOperationTextToImage

	_, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrInvalidMediaRequestMapping)
	require.Zero(t, fixture.repo.createCalls())
}

func TestMediaOrchestratorRejectsRawOrNonRecoverableInputsBeforeTaskAndCharge(t *testing.T) {
	tests := []struct {
		name  string
		input MediaArtifactInput
	}{
		{name: "raw data", input: MediaArtifactInput{Position: 0, MediaType: MediaTypeImage, ContentType: "image/png", Data: []byte("raw"), ObjectKey: "key"}},
		{name: "no durable reference", input: MediaArtifactInput{Position: 0, MediaType: MediaTypeImage, ContentType: "image/png"}},
		{name: "ambiguous references", input: MediaArtifactInput{Position: 0, MediaType: MediaTypeImage, ContentType: "image/png", ObjectKey: "key", UpstreamReference: "upstream"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			req := validAsyncMediaCreateRequest()
			req.Operation = MediaOperationImageToImage
			req.Inputs = []MediaArtifactInput{tt.input}
			_, err := fixture.orchestrator.Create(context.Background(), req)
			require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
			require.Equal(t, 0, fixture.repo.createCalls())
			require.Equal(t, 0, fixture.billing.prechargeCalls())
		})
	}
}

func TestMediaOrchestratorEnforcesOperationSpecificInputContractsBeforeTaskAndCharge(t *testing.T) {
	tests := []struct {
		name      string
		operation MediaOperation
		inputs    []MediaArtifactInput
	}{
		{name: "text to image rejects inputs", operation: MediaOperationTextToImage, inputs: []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}},
		{name: "text to video rejects inputs", operation: MediaOperationTextToVideo, inputs: []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}},
		{name: "image to image requires input", operation: MediaOperationImageToImage},
		{name: "image edit requires input", operation: MediaOperationImageEdit},
		{name: "image to video requires input", operation: MediaOperationImageToVideo},
		{name: "reference to video requires input", operation: MediaOperationReferenceVideo},
		{name: "video extend requires source", operation: MediaOperationVideoExtend},
		{name: "video remix requires source", operation: MediaOperationVideoRemix},
		{name: "image operation rejects video", operation: MediaOperationImageToImage, inputs: []MediaArtifactInput{validOrchestratorVideoInput(0, "video/mp4")}},
		{name: "image type rejects video mime", operation: MediaOperationImageEdit, inputs: []MediaArtifactInput{validOrchestratorImageInput(0, "video/mp4")}},
		{name: "image operation rejects malformed mime", operation: MediaOperationImageToVideo, inputs: []MediaArtifactInput{validOrchestratorImageInput(0, "image/")}},
		{name: "image operation rejects wildcard mime", operation: MediaOperationReferenceVideo, inputs: []MediaArtifactInput{validOrchestratorImageInput(0, "image/*")}},
		{name: "reference to video rejects video input", operation: MediaOperationReferenceVideo, inputs: []MediaArtifactInput{validOrchestratorVideoInput(0, "video/mp4")}},
		{name: "video extend requires video first after sorting", operation: MediaOperationVideoExtend, inputs: []MediaArtifactInput{
			validOrchestratorVideoInput(2, "video/mp4"), validOrchestratorImageInput(1, "image/png"),
		}},
		{name: "video source rejects image mime", operation: MediaOperationVideoExtend, inputs: []MediaArtifactInput{validOrchestratorVideoInput(0, "image/png")}},
		{name: "video remix rejects video reference", operation: MediaOperationVideoRemix, inputs: []MediaArtifactInput{
			validOrchestratorVideoInput(0, "video/mp4"), validOrchestratorVideoInput(1, "video/webm"),
		}},
		{name: "video remix image reference rejects video mime", operation: MediaOperationVideoRemix, inputs: []MediaArtifactInput{
			validOrchestratorVideoInput(0, "video/mp4"), validOrchestratorImageInput(1, "video/mp4"),
		}},
		{name: "declared image rejects non image top level", operation: MediaOperationImageToImage, inputs: []MediaArtifactInput{validOrchestratorImageInput(0, "application/octet-stream")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			req := validMediaCreateRequestForOperation(tt.operation)
			req.Inputs = tt.inputs

			_, err := fixture.orchestrator.Create(context.Background(), req)
			require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
			require.Equal(t, 0, fixture.repo.createCalls())
			require.Equal(t, 0, fixture.billing.prechargeCalls())
		})
	}
}

func TestMediaOrchestratorRejectsReferenceInputsAboveGlobalLimit(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validMediaCreateRequestForOperation(MediaOperationReferenceVideo)
	for position := 0; position < MaxMediaReferenceInputs+1; position++ {
		req.Inputs = append(req.Inputs, validOrchestratorImageInput(position, "image/png"))
	}

	_, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
	require.Equal(t, 0, fixture.repo.createCalls())
	require.Equal(t, 0, fixture.billing.prechargeCalls())
}

func TestMediaOrchestratorVideoInputMappingUsesSortedSourceAndImageReferences(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validMediaCreateRequestForOperation(MediaOperationVideoExtend)
	req.Inputs = []MediaArtifactInput{
		validOrchestratorImageInput(2, "image/jpeg; name=reference.jpg"),
		validOrchestratorVideoInput(1, "video/mp4; codecs=h264"),
		validOrchestratorImageInput(3, "image/png"),
	}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	stored := fixture.repo.mustGet(result.Task.ID)
	var spec MediaSpec
	require.NoError(t, json.Unmarshal(stored.RequestSpec, &spec))
	require.NotNil(t, spec.Video)
	require.NotNil(t, spec.Video.SourceArtifactID)
	artifacts, err := fixture.artifacts.ListByTaskID(context.Background(), stored.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 3)
	require.Equal(t, MediaTypeVideo, artifacts[0].MediaType)
	require.Equal(t, artifacts[0].ID, *spec.Video.SourceArtifactID)
	require.Equal(t, []int64{artifacts[1].ID, artifacts[2].ID}, spec.Video.ReferenceArtifactIDs)
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

func TestMediaOrchestratorIdempotencyRetryReusesUploadWhenObjectKeyChanges(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.IdempotencyKey = "idem-stable-upload"
	req.Inputs = []MediaArtifactInput{{
		Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
		SizeBytes: 128, ChecksumSHA256: strings.Repeat("a", 64), ObjectKey: "staged/random-object-first",
		Width: 16, Height: 8, Resolution: "16x8",
	}}

	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.True(t, first.InputsAdopted)
	req.Inputs[0].ObjectKey = "staged/random-object-second"

	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.Task.PublicID, second.Task.PublicID)
	require.False(t, second.InputsAdopted)
	require.Equal(t, 1, fixture.repo.createCalls())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), first.Task.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
	require.Equal(t, "staged/random-object-first", artifacts[0].ObjectKey)

	differentContent := req
	differentContent.Inputs = append([]MediaArtifactInput(nil), req.Inputs...)
	differentContent.Inputs[0].ObjectKey = "staged/random-object-third"
	differentContent.Inputs[0].ChecksumSHA256 = strings.Repeat("b", 64)
	_, err = fixture.orchestrator.Create(context.Background(), differentContent)
	require.ErrorIs(t, err, ErrMediaIdempotencyConflict)
	require.Equal(t, 1, fixture.billing.prechargeCalls())
}

func TestMediaOrchestratorIdempotencyRetryReusesUploadWhenUpstreamReferenceChanges(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.IdempotencyKey = "idem-stable-upstream-reference"
	req.Inputs = []MediaArtifactInput{{
		Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
		SizeBytes: 128, ChecksumSHA256: strings.Repeat("c", 64),
		UpstreamReference: "temporary-upstream-reference-first", Width: 16, Height: 8,
	}}

	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), first.Task.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
	require.Empty(t, artifacts[0].ObjectKey)
	require.Equal(t, "temporary-upstream-reference-first", artifacts[0].UpstreamReference)
	require.Empty(t, artifacts[0].PublicURL)
	require.Equal(t, int64(128), artifacts[0].SizeBytes)
	require.Equal(t, strings.Repeat("c", 64), artifacts[0].ChecksumSHA256)
	req.Inputs[0].UpstreamReference = "temporary-upstream-reference-second"
	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.Task.PublicID, second.Task.PublicID)
	require.False(t, second.InputsAdopted)
	require.Equal(t, 1, fixture.repo.createCalls())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
}

func TestMediaOrchestratorRejectsUnidentifiedNonURLInputsBeforeIdempotentReuse(t *testing.T) {
	for _, tt := range []struct {
		name      string
		firstRef  func(*MediaArtifactInput)
		secondRef func(*MediaArtifactInput)
	}{
		{
			name: "object key",
			firstRef: func(input *MediaArtifactInput) {
				input.ObjectKey = "staged/unidentified-object-first"
			},
			secondRef: func(input *MediaArtifactInput) {
				input.ObjectKey = "staged/unidentified-object-second"
			},
		},
		{
			name: "upstream reference",
			firstRef: func(input *MediaArtifactInput) {
				input.UpstreamReference = "unidentified-upstream-first"
			},
			secondRef: func(input *MediaArtifactInput) {
				input.UpstreamReference = "unidentified-upstream-second"
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			req := validAsyncMediaCreateRequest()
			req.Operation = MediaOperationImageToImage
			req.IdempotencyKey = "idem-unidentified-" + tt.name
			req.Inputs = []MediaArtifactInput{{
				Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
			}}
			tt.firstRef(&req.Inputs[0])

			first, firstErr := fixture.orchestrator.Create(context.Background(), req)
			req.Inputs[0].ObjectKey = ""
			req.Inputs[0].UpstreamReference = ""
			tt.secondRef(&req.Inputs[0])
			second, secondErr := fixture.orchestrator.Create(context.Background(), req)

			require.ErrorIs(t, firstErr, ErrMediaInputNotRecoverable)
			require.ErrorIs(t, secondErr, ErrMediaInputNotRecoverable)
			require.Nil(t, first)
			require.Nil(t, second)
			require.Equal(t, 0, fixture.repo.createCalls())
			require.Equal(t, 0, fixture.billing.prechargeCalls())
		})
	}
}

func TestMediaCreateFingerprintRejectsInvalidNonURLContentIdentity(t *testing.T) {
	validChecksum := strings.Repeat("e", 64)
	for _, tt := range []struct {
		name     string
		size     int64
		checksum string
	}{
		{name: "missing size", checksum: validChecksum},
		{name: "missing checksum", size: 128},
		{name: "short checksum", size: 128, checksum: strings.Repeat("a", 63)},
		{name: "non hex checksum", size: 128, checksum: strings.Repeat("z", 64)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := validMediaCreateRequestForOperation(MediaOperationImageToImage)
			req.Inputs = []MediaArtifactInput{{
				Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
				SizeBytes: tt.size, ChecksumSHA256: tt.checksum, ObjectKey: "staged/input",
			}}
			_, err := mediaCreateFingerprint(req)
			require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
		})
	}
}

func TestMediaOrchestratorExternalURLDoesNotRequireUploadContentIdentity(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validMediaCreateRequestForOperation(MediaOperationImageToImage)
	req.IdempotencyKey = "idem-external-url-without-upload-checksum"
	req.Inputs = []MediaArtifactInput{{
		Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
		ExternalURL: "https://media.example/input.png?version=1",
	}}

	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), first.Task.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
	require.Empty(t, artifacts[0].ObjectKey)
	require.Empty(t, artifacts[0].UpstreamReference)
	require.Equal(t, req.Inputs[0].ExternalURL, artifacts[0].PublicURL)
	require.Zero(t, artifacts[0].SizeBytes)
	require.Empty(t, artifacts[0].ChecksumSHA256)
	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.Task.PublicID, second.Task.PublicID)
	require.False(t, second.InputsAdopted)
	require.Equal(t, 1, fixture.repo.createCalls())
	require.Equal(t, 1, fixture.billing.prechargeCalls())
}

func TestMediaCreateFingerprintPreservesStableInputDifferences(t *testing.T) {
	req := validMediaCreateRequestForOperation(MediaOperationReferenceVideo)
	req.Inputs = []MediaArtifactInput{
		{
			Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
			SizeBytes: 100, ChecksumSHA256: strings.Repeat("1", 64), ObjectKey: "staged/first",
		},
		{
			Direction: "input", Position: 1, MediaType: MediaTypeImage, ContentType: "image/jpeg",
			SizeBytes: 200, ChecksumSHA256: strings.Repeat("2", 64), ObjectKey: "staged/second",
		},
	}
	base, err := mediaCreateFingerprint(req)
	require.NoError(t, err)

	reversed := req
	reversed.Inputs = []MediaArtifactInput{req.Inputs[1], req.Inputs[0]}
	reversedFingerprint, err := mediaCreateFingerprint(reversed)
	require.NoError(t, err)
	require.Equal(t, base, reversedFingerprint)

	positionsChanged := req
	positionsChanged.Inputs = append([]MediaArtifactInput(nil), req.Inputs...)
	positionsChanged.Inputs[0].Position = 1
	positionsChanged.Inputs[1].Position = 0
	positionsFingerprint, err := mediaCreateFingerprint(positionsChanged)
	require.NoError(t, err)
	require.NotEqual(t, base, positionsFingerprint)

	metadataChanged := req
	metadataChanged.Inputs = append([]MediaArtifactInput(nil), req.Inputs...)
	metadataChanged.Inputs[0].SizeBytes++
	metadataFingerprint, err := mediaCreateFingerprint(metadataChanged)
	require.NoError(t, err)
	require.NotEqual(t, base, metadataFingerprint)

	wrongMediaType := req
	wrongMediaType.Inputs = append([]MediaArtifactInput(nil), req.Inputs...)
	wrongMediaType.Inputs[0].MediaType = MediaTypeVideo
	wrongMediaType.Inputs[0].ContentType = "video/mp4"
	_, err = mediaCreateFingerprint(wrongMediaType)
	require.ErrorIs(t, err, ErrMediaInputNotRecoverable)
}

func TestMediaCreateFingerprintDistinguishesNormalizedExternalURLs(t *testing.T) {
	req := validMediaCreateRequestForOperation(MediaOperationImageToImage)
	req.Inputs = []MediaArtifactInput{{
		Direction: "input", Position: 0, MediaType: MediaTypeImage, ContentType: "image/png",
		ExternalURL: "https://media.example/input.png?version=1",
	}}
	first, err := mediaCreateFingerprint(req)
	require.NoError(t, err)

	req.Inputs[0].ExternalURL = "https://media.example/input.png?version=2"
	second, err := mediaCreateFingerprint(req)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
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

func TestMediaOrchestratorPrechargedInitializationFailurePersistsRecoveryBeforePlanWrite(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		arrange func(*mediaOrchestratorFixture) MediaCreateRequest
	}{
		{name: "queue", code: "system_queue", arrange: func(fixture *mediaOrchestratorFixture) MediaCreateRequest {
			fixture.queue.enqueueErr = errors.New("redis unavailable")
			return validAsyncMediaCreateRequest()
		}},
		{name: "ready publish", code: "system_billing_state", arrange: func(fixture *mediaOrchestratorFixture) MediaCreateRequest {
			fixture.repo.readyWriteErrors = 1
			return validAsyncMediaCreateRequest()
		}},
		{name: "input persistence", code: "system_input", arrange: func(fixture *mediaOrchestratorFixture) MediaCreateRequest {
			fixture.artifacts.createErr = errors.New("object metadata unavailable")
			req := validAsyncMediaCreateRequest()
			req.Operation = MediaOperationImageToImage
			req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}
			return req
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			fixture.pricing.snapshot.EstimatedAmount = 3.25
			fixture.repo.failSettlementPlanWrites = 1
			req := tt.arrange(fixture)

			_, err := fixture.orchestrator.Create(context.Background(), req)
			require.Error(t, err)
			stored := fixture.repo.onlyTask()
			require.Equal(t, MediaTaskStatusFailed, stored.Status)
			require.Equal(t, tt.code, stored.ErrorCode)
			require.Equal(t, MediaBillingStatusPrecharged, stored.BillingStatus)
			require.Equal(t, 3.25, stored.PrechargedAmount)
			require.Empty(t, stored.SettlementPlan)
			require.NotEmpty(t, stored.SettlementRecovery)
			recovery, decodeErr := decodeMediaSettlementPlan(stored.SettlementRecovery)
			require.NoError(t, decodeErr)
			require.Equal(t, MediaSettlementPlan{
				Type: MediaSettlementTypeFailure,
				Failure: &MediaFailureSettlement{
					Kind:        MediaFailureKindSystem,
					RefundRatio: 1,
					ErrorCode:   tt.code,
				},
			}, recovery)
			pending, pendingErr := fixture.repo.ListSettlementPending(context.Background(), 10)
			require.NoError(t, pendingErr)
			require.Len(t, pending, 1)
			require.Equal(t, stored.ID, pending[0].ID)
		})
	}
}

func TestMediaOrchestratorPrechargedInitializationFailureRefundsActualAmount(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(*mediaOrchestratorFixture) MediaCreateRequest
	}{
		{name: "queue", arrange: func(fixture *mediaOrchestratorFixture) MediaCreateRequest {
			fixture.queue.enqueueErr = errors.New("redis unavailable")
			return validAsyncMediaCreateRequest()
		}},
		{name: "ready publish", arrange: func(fixture *mediaOrchestratorFixture) MediaCreateRequest {
			fixture.repo.readyWriteErrors = 1
			return validAsyncMediaCreateRequest()
		}},
		{name: "input persistence", arrange: func(fixture *mediaOrchestratorFixture) MediaCreateRequest {
			fixture.artifacts.createErr = errors.New("object metadata unavailable")
			req := validAsyncMediaCreateRequest()
			req.Operation = MediaOperationImageToImage
			req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}
			return req
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMediaOrchestratorFixture(t)
			fixture.pricing.snapshot.EstimatedAmount = 3.25
			actualPrecharge := 2.75
			fixture.billing.actualPrechargeAmount = &actualPrecharge
			req := tt.arrange(fixture)

			_, err := fixture.orchestrator.Create(context.Background(), req)
			require.Error(t, err)
			stored := fixture.repo.onlyTask()
			require.Equal(t, MediaTaskStatusFailed, stored.Status)
			require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
			require.Equal(t, actualPrecharge, stored.PrechargedAmount)
			require.Equal(t, actualPrecharge, stored.RefundedAmount)
			require.Zero(t, stored.FinalAmount)
		})
	}
}

func TestMediaOrchestratorCompensationWriteAppliedButReturnedErrorStillSettles(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.pricing.snapshot.EstimatedAmount = 3.25
	fixture.queue.enqueueErr = errors.New("redis unavailable")
	fixture.repo.transitionQueuedAppliedErrors = 1

	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorContains(t, err, "redis unavailable")
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "system_queue", stored.ErrorCode)
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
	require.Equal(t, 3.25, stored.PrechargedAmount)
	require.Equal(t, 3.25, stored.RefundedAmount)
	require.Zero(t, stored.FinalAmount)
}

func TestMediaOrchestratorDeterministicPrechargeFailureTerminatesWithoutRefund(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.pricing.snapshot.EstimatedAmount = 3.25
	fixture.billing.prechargeErr = errors.New("insufficient balance")
	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorContains(t, err, "insufficient balance")
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, MediaTaskStageFailed, stored.Stage)
	require.Equal(t, "billing_precharge", stored.ErrorCode)
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
	require.Zero(t, stored.PrechargedAmount)
	require.Nil(t, stored.LeaseUntil)
	require.Empty(t, stored.SettlementRecovery)
	require.Equal(t, 0, fixture.queue.enqueueCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorDeterministicReconciledPrechargeFailureTerminatesWithoutRefund(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.artifacts.createErr = errors.New("object metadata unavailable")
	fixture.billing.prechargeErr = errors.New("insufficient balance")
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	_, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorContains(t, err, "insufficient balance")
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "billing_precharge", stored.ErrorCode)
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
	require.Zero(t, stored.PrechargedAmount)
	require.Nil(t, stored.LeaseUntil)
	require.Empty(t, stored.SettlementRecovery)
	require.Equal(t, 0, fixture.queue.enqueueCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorUnknownPrechargeResultLeavesFencedPendingForIdempotentRecovery(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.billing.prechargeErr = fmt.Errorf("balance outcome unavailable: %w", ErrMediaPrechargeResultUnknown)
	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorIs(t, err, ErrMediaPrechargeResultUnknown)
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusQueued, stored.Status)
	require.Equal(t, MediaBillingStatusPending, stored.BillingStatus)
	require.NotNil(t, stored.LeaseUntil)
	require.Empty(t, stored.ErrorCode)
	require.Equal(t, 0, fixture.queue.enqueueCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorInvalidSuccessfulPrechargeResultLeavesFencedPendingForRecovery(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	invalidAmount := -1.0
	fixture.billing.actualPrechargeAmount = &invalidAmount

	_, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.ErrorIs(t, err, ErrInvalidMediaBillingResult)
	stored := fixture.repo.onlyTask()
	require.Equal(t, MediaTaskStatusQueued, stored.Status)
	require.Equal(t, MediaBillingStatusPending, stored.BillingStatus)
	require.NotNil(t, stored.LeaseUntil)
	require.Empty(t, stored.ErrorCode)
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	require.Equal(t, 0, fixture.queue.enqueueCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorUnknownPrechargeResultKeepsDurableInputOwnership(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.billing.prechargeErr = fmt.Errorf("balance outcome unavailable: %w", ErrMediaPrechargeResultUnknown)
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaPrechargeResultUnknown)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), result.Task.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
}

func TestMediaOrchestratorSyncSubscribePrecedesFirstDBRead(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.repo.readyPublishedHook = func(task *MediaTask) {
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
	}
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.True(t, result.InputsAdopted)
	require.Equal(t, []string{"subscribe", "get"}, fixture.events.snapshotTail(2))
}

func TestMediaOrchestratorSyncTimeoutFallbackKeepsTaskRunning(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, result.Disposition)
	require.True(t, result.InputsAdopted)
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

func TestMediaOrchestratorFallbackMarkSuccessReturnsCompletionThatWinsReload(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	fixture.repo.completeAfterSuccessfulFallback = true
	fixture.repo.readyPublishedHook = func(task *MediaTask) {
		task.Status = MediaTaskStatusInProgress
		task.Stage = MediaTaskStageGenerating
		task.SubmittedAt = mediaTimePointer(time.Now())
		fixture.artifacts.mu.Lock()
		fixture.artifacts.items[task.ID] = []MediaArtifact{{ID: 41, TaskID: task.ID, Direction: "output", Position: 0}}
		fixture.artifacts.mu.Unlock()
	}

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, int64(41), result.Artifacts[0].ID)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorFallbackMarkAppliedErrorReloadsPersistedFallback(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	fixture.repo.markFallbackAppliedErr = errors.New("mark fallback result unavailable")

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, result.Disposition)
	stored := fixture.repo.mustGet(result.Task.ID)
	require.True(t, stored.SyncFallback)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorFallbackMarkAppliedErrorReloadsCompletion(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	fixture.repo.completeAfterSuccessfulFallback = true
	fixture.repo.markFallbackAppliedErr = errors.New("mark fallback result unavailable")
	fixture.repo.readyPublishedHook = func(task *MediaTask) {
		task.Status = MediaTaskStatusInProgress
		task.Stage = MediaTaskStageGenerating
		task.SubmittedAt = mediaTimePointer(time.Now())
		fixture.artifacts.mu.Lock()
		fixture.artifacts.items[task.ID] = []MediaArtifact{{ID: 71, TaskID: task.ID, Direction: "output", Position: 0}}
		fixture.artifacts.mu.Unlock()
	}

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, int64(71), result.Artifacts[0].ID)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorFallbackMarkUnappliedErrorPreservesOriginalError(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.settings.settings.MediaSyncTimeoutFallbackAsyncEnabled = true
	markErr := errors.New("mark fallback write failed")
	fixture.repo.markFallbackErr = markErr

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.ErrorIs(t, err, markErr)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)
	stored := fixture.repo.onlyTask()
	require.False(t, stored.Status.IsTerminal())
	require.False(t, stored.SyncFallback)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorPersistedFallbackWinsAcrossWaitersWithDifferentSettings(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	created, err := fixture.orchestrator.Create(context.Background(), validAsyncMediaCreateRequest())
	require.NoError(t, err)
	fixture.repo.mutate(created.Task.ID, func(task *MediaTask) {
		task.ClientAsync = false
		task.Status = MediaTaskStatusInProgress
		task.Stage = MediaTaskStageGenerating
		task.SubmittedAt = mediaTimePointer(time.Now())
		task.Version++
	})
	staleForWaiterA := fixture.repo.mustGet(created.Task.ID)

	waiterBSettings := fixture.settings.settings
	waiterBSettings.MediaSyncTimeoutFallbackAsyncEnabled = true
	waiterB, err := fixture.orchestrator.handleSyncTimeout(context.Background(), staleForWaiterA, &waiterBSettings)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, waiterB.Disposition)

	waiterASettings := waiterBSettings
	waiterASettings.MediaSyncTimeoutFallbackAsyncEnabled = false
	waiterA, err := fixture.orchestrator.handleSyncTimeout(context.Background(), staleForWaiterA, &waiterASettings)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, waiterA.Disposition)
	stored := fixture.repo.mustGet(created.Task.ID)
	require.False(t, stored.Status.IsTerminal())
	require.True(t, stored.SyncFallback)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorTimeoutVersionConflictReloadsPersistedFallback(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.repo.beforeNextVersionedTransition = func(task *MediaTask) {
		task.SyncFallback = true
		task.SyncFallbackAt = mediaTimePointer(time.Now())
	}

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, result.Disposition)
	stored := fixture.repo.mustGet(result.Task.ID)
	require.False(t, stored.Status.IsTerminal())
	require.True(t, stored.SyncFallback)
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

func TestMediaOrchestratorTimeoutVersionConflictRecomputesPenaltyFromFreshState(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.repo.beforeNextVersionedTransition = func(task *MediaTask) {
		task.Stage = MediaTaskStageGenerating
		task.SubmittedAt = mediaTimePointer(time.Now())
		task.Version++
	}

	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.InDelta(t, 0.2, fixture.billing.lastFailure().RefundRatio, 1e-9)
	require.InDelta(t, 0.8, fixture.billing.lastFailure().PenaltyRatio, 1e-9)
}

func TestMediaOrchestratorTimeoutVersionConflictRecomputesFullRefundFromFreshState(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.repo.beforeNextVersionedTransition = func(task *MediaTask) {
		task.Stage = MediaTaskStageScheduling
		task.SubmittedAt = nil
		task.Version++
	}

	_, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, float64(1), fixture.billing.lastFailure().RefundRatio)
	require.Zero(t, fixture.billing.lastFailure().PenaltyRatio)
}

func TestMediaOrchestratorZeroTimeoutHasNoApplicationTimer(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.settings.MediaSyncWaitTimeoutSeconds = 0
	ctx, cancel := context.WithCancel(context.Background())
	fixture.queue.subscribeHook = cancel
	result, err := fixture.orchestrator.Create(ctx, validSyncMediaCreateRequest())
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)
	require.Equal(t, 0, fixture.clock.newTimerCalls())
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorParentCancelWinsWhenTimeoutAlsoReady(t *testing.T) {
	for iteration := 0; iteration < 64; iteration++ {
		fixture := newMediaOrchestratorFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		fixture.repo.getHook = func(call int, _ *MediaTask) {
			if call == 1 {
				cancel()
			}
		}

		result, err := fixture.orchestrator.Create(ctx, validSyncMediaCreateRequest())
		require.ErrorIs(t, err, context.Canceled, "iteration %d", iteration)
		require.NotNil(t, result, "iteration %d", iteration)
		require.True(t, result.InputsAdopted, "iteration %d", iteration)
		require.Equal(t, 0, fixture.controller.stopCalls(), "iteration %d", iteration)
		require.Equal(t, 0, fixture.billing.settleFailureCalls(), "iteration %d", iteration)
	}
}

func TestMediaOrchestratorReadyTaskAdoptsInputsWhenSyncWaitIsCanceled(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.settings.MediaSyncWaitTimeoutSeconds = 0
	ctx, cancel := context.WithCancel(context.Background())
	fixture.queue.subscribeHook = cancel
	req := validSyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	result, err := fixture.orchestrator.Create(ctx, req)
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), result.Task.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
	require.Equal(t, req.Inputs[0].ObjectKey, artifacts[0].ObjectKey)
}

func TestMediaOrchestratorReadyTaskAdoptsInputsWhenTerminalSubscriptionCloses(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.settings.settings.MediaSyncWaitTimeoutSeconds = 0
	fixture.queue.closeSubscription = true
	req := validSyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorIs(t, err, ErrMediaTerminalSubscriptionClosed)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)
	artifacts, readErr := fixture.artifacts.ListByTaskID(context.Background(), result.Task.ID)
	require.NoError(t, readErr)
	require.Len(t, artifacts, 1)
}

func TestMediaOrchestratorQueueFailureDoesNotAdoptInputs(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	fixture.queue.enqueueErr = errors.New("redis unavailable")
	req := validAsyncMediaCreateRequest()
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	result, err := fixture.orchestrator.Create(context.Background(), req)
	require.ErrorContains(t, err, "redis unavailable")
	require.NotNil(t, result)
	require.False(t, result.InputsAdopted)
}

func TestMediaOrchestratorIdempotentReuseDoesNotAdoptRetryInputs(t *testing.T) {
	fixture := newMediaOrchestratorFixture(t)
	req := validAsyncMediaCreateRequest()
	req.IdempotencyKey = "idem-input-owner"
	req.Operation = MediaOperationImageToImage
	req.Inputs = []MediaArtifactInput{validOrchestratorImageInput(0, "image/png")}

	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.True(t, first.InputsAdopted)
	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.False(t, second.InputsAdopted)
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

func TestMediaOrchestratorIdempotentRetryAfterSyncTimeoutKeepsGatewayTimeoutPrivate(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	req := validSyncMediaCreateRequest()
	req.IdempotencyKey = "idem-sync-timeout"

	first, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, first.Disposition)
	require.Empty(t, first.Task.PublicID)

	second, err := fixture.orchestrator.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, second.Disposition)
	require.Empty(t, second.Task.PublicID)
	require.Equal(t, 1, fixture.billing.prechargeCalls())
	require.Equal(t, 2, fixture.queue.enqueueCalls())
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

func TestMediaOrchestratorTimeoutTransitionAppliedErrorContinuesStopAndSettlement(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.repo.transitionVersionedAppliedErr = errors.New("timeout transition result unavailable")

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, result.Disposition)
	require.Empty(t, result.Task.PublicID)
	require.Equal(t, 1, fixture.controller.stopCalls())
	require.Equal(t, 1, fixture.billing.settleFailureCalls())
	stored := fixture.repo.mustGet(result.Task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "sync_timeout", stored.ErrorCode)
	require.NotEmpty(t, stored.SettlementRecovery)
	require.Equal(t, MediaBillingStatusSettled, stored.BillingStatus)
}

func TestMediaOrchestratorTimeoutTransitionErrorReloadsCompletedWinner(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.repo.transitionVersionedErr = errors.New("timeout transition failed")
	fixture.repo.beforeNextVersionedTransition = func(task *MediaTask) {
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		task.Version++
		fixture.artifacts.mu.Lock()
		fixture.artifacts.items[task.ID] = []MediaArtifact{{ID: 81, TaskID: task.ID, Direction: "output", Position: 0}}
		fixture.artifacts.mu.Unlock()
	}

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionCompleted, result.Disposition)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, int64(81), result.Artifacts[0].ID)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorTimeoutTransitionErrorReloadsPersistedFallback(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageGenerating, mediaTimePointer(time.Now()))
	fixture.repo.transitionVersionedErr = errors.New("timeout transition failed")
	fixture.repo.beforeNextVersionedTransition = func(task *MediaTask) {
		task.SyncFallback = true
		task.SyncFallbackAt = mediaTimePointer(time.Now())
	}

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionFallbackAsync, result.Disposition)
	stored := fixture.repo.mustGet(result.Task.ID)
	require.False(t, stored.Status.IsTerminal())
	require.True(t, stored.SyncFallback)
	require.Equal(t, 0, fixture.controller.stopCalls())
	require.Equal(t, 0, fixture.billing.settleFailureCalls())
}

func TestMediaOrchestratorTimeoutTransitionUnappliedErrorPreservesOriginalError(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	transitionErr := errors.New("timeout transition failed")
	fixture.repo.transitionVersionedErr = transitionErr

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.ErrorIs(t, err, transitionErr)
	require.NotNil(t, result)
	require.True(t, result.InputsAdopted)
	stored := fixture.repo.onlyTask()
	require.False(t, stored.Status.IsTerminal())
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

func TestMediaOrchestratorSyncTimeoutTransitionPersistsRecoveryBeforeSettlementPlan(t *testing.T) {
	fixture := newTimedOutMediaOrchestratorFixture(t, MediaTaskStageScheduling, nil)
	fixture.repo.failSettlementPlanWrites = 1

	result, err := fixture.orchestrator.Create(context.Background(), validSyncMediaCreateRequest())
	require.NoError(t, err)
	require.Equal(t, MediaCreateDispositionGatewayTimeout, result.Disposition)
	require.Empty(t, result.Task.PublicID)

	stored := fixture.repo.mustGet(result.Task.ID)
	require.Equal(t, MediaTaskStatusFailed, stored.Status)
	require.Equal(t, "sync_timeout", stored.ErrorCode)
	require.Equal(t, MediaBillingStatusPrecharged, stored.BillingStatus)
	require.Empty(t, stored.SettlementPlan)
	require.NotEmpty(t, stored.SettlementRecovery)
	recovery, err := decodeMediaSettlementPlan(stored.SettlementRecovery)
	require.NoError(t, err)
	require.Equal(t, MediaSettlementPlan{
		Type: MediaSettlementTypeFailure,
		Failure: &MediaFailureSettlement{
			Kind:        MediaFailureKindSyncTimeout,
			RefundRatio: 1,
			ErrorCode:   "sync_timeout",
		},
	}, recovery)
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
		task.ErrorMessage = "Authorization Bearer secret at https://upstream.example"
		task.RequestSpec = json.RawMessage(`{"internal":true}`)
		task.BillingStatus = MediaBillingStatusPrecharged
		task.PrechargedAmount = 12
		task.FinalAmount = 10
		task.RefundedAmount = 2
		task.AdditionalChargedAmount = 3
		task.ClaimToken = "claim-secret"
		task.Stage = MediaTaskStageGenerating
		now := time.Unix(1784112001, 0)
		task.SubmittedAt = &now
		task.StartedAt = &now
	})
	fixture.artifacts.items[created.Task.ID] = []MediaArtifact{{
		ID: 1, TaskID: created.Task.ID, ObjectKey: "private/object", UpstreamReference: "private-upstream", PublicURL: "https://example.test/media",
	}}

	task, artifacts, err := fixture.orchestrator.GetForUser(context.Background(), created.Task.PublicID, created.Task.UserID)
	require.NoError(t, err)
	require.Nil(t, task.AccountID)
	require.Empty(t, task.UpstreamTaskID)
	require.Empty(t, task.PollMetadata)
	require.Empty(t, task.BillingSnapshot)
	require.Empty(t, task.RequestFingerprint)
	require.Empty(t, task.IdempotencyKey)
	require.Zero(t, task.ID)
	require.Zero(t, task.UserID)
	require.Zero(t, task.APIKeyID)
	require.Zero(t, task.GroupID)
	require.Empty(t, task.ErrorMessage)
	require.Empty(t, task.RequestSpec)
	require.Empty(t, task.BillingStatus)
	require.Zero(t, task.PrechargedAmount)
	require.Zero(t, task.FinalAmount)
	require.Zero(t, task.RefundedAmount)
	require.Zero(t, task.AdditionalChargedAmount)
	require.Empty(t, task.ClaimToken)
	require.Empty(t, task.Stage)
	require.Nil(t, task.SubmittedAt)
	require.Nil(t, task.StartedAt)
	require.Empty(t, artifacts[0].ObjectKey)
	require.Empty(t, artifacts[0].UpstreamReference)
	require.Equal(t, "https://example.test/media", artifacts[0].PublicURL)

	_, _, err = fixture.orchestrator.GetForUser(context.Background(), created.Task.PublicID, created.Task.UserID+1)
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

func validMediaCreateRequestForOperation(operation MediaOperation) MediaCreateRequest {
	mediaType, ok := mediaTypeForOperation(operation)
	if !ok {
		panic(fmt.Sprintf("unsupported test media operation %q", operation))
	}
	req := validAsyncMediaCreateRequest()
	req.Operation = operation
	req.MediaType = mediaType
	if mediaType == MediaTypeVideo {
		req.RequestedModel = "fake-video"
		req.Spec = MediaSpec{Video: &VideoSpec{Prompt: "animate a cat", DurationSeconds: 5, FPS: 24}}
	}
	return req
}

func validOrchestratorImageInput(position int, contentType string) MediaArtifactInput {
	return MediaArtifactInput{
		Position: position, MediaType: MediaTypeImage, ContentType: contentType,
		ObjectKey: fmt.Sprintf("media/input/image-%d", position), SizeBytes: int64(100 + position),
		ChecksumSHA256: fmt.Sprintf("%064x", position+1),
	}
}

func validOrchestratorVideoInput(position int, contentType string) MediaArtifactInput {
	return MediaArtifactInput{
		Position: position, MediaType: MediaTypeVideo, ContentType: contentType,
		ObjectKey: fmt.Sprintf("media/input/video-%d", position), SizeBytes: int64(100 + position),
		ChecksumSHA256: fmt.Sprintf("%064x", position+1),
	}
}

func orchestratorPendingTaskForRequest(t *testing.T, fixture *mediaOrchestratorFixture, req MediaCreateRequest, leaseUntil time.Time) *MediaTask {
	t.Helper()
	fingerprint, err := mediaCreateFingerprint(req)
	require.NoError(t, err)
	requestSpec, err := json.Marshal(req.Spec)
	require.NoError(t, err)
	candidates, err := json.Marshal(fixture.scheduler.candidates)
	require.NoError(t, err)
	billingSnapshot, err := json.Marshal(fixture.pricing.snapshot)
	require.NoError(t, err)
	return &MediaTask{
		ID: 88, PublicID: "task_initializing_winner", UserID: req.UserID, APIKeyID: req.APIKeyID, GroupID: req.GroupID,
		MediaType: req.MediaType, Operation: req.Operation, RequestedModel: req.RequestedModel,
		ClientAsync: req.ClientAsync, Status: MediaTaskStatusQueued, Stage: MediaTaskStageQueued,
		RequestSpec: requestSpec, CandidateSnapshot: candidates, RequestFingerprint: fingerprint,
		IdempotencyKey: req.IdempotencyKey, BillingSnapshot: billingSnapshot,
		BillingStatus: MediaBillingStatusPending, LeaseUntil: mediaTimePointer(leaseUntil), Version: 1,
	}
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
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{
		validImageModelDefinition(),
		validVideoModelDefinition(),
	}})
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
	fixture.repo.readyPublishedHook = func(task *MediaTask) {
		task.Stage = stage
		task.SubmittedAt = submittedAt
		if stage != MediaTaskStageQueued {
			task.Status = MediaTaskStatusInProgress
		}
	}
	return fixture
}

type orchestratorTaskRepository struct {
	mu                              sync.Mutex
	tasks                           map[int64]*MediaTask
	nextID                          int64
	publicSequence                  int64
	creates                         int
	gets                            int
	createRaceWinner                *MediaTask
	completeOnNextTransition        bool
	completeOnNextFallback          bool
	completeAfterSuccessfulFallback bool
	markFallbackErr                 error
	markFallbackAppliedErr          error
	beforeNextVersionedTransition   func(*MediaTask)
	transitionVersionedErr          error
	transitionVersionedAppliedErr   error
	failSettlementPlanWrites        int
	readyWriteErrors                int
	readyWriteAppliedErrors         int
	getErrors                       int
	transitionQueuedAppliedErrors   int
	readyPublishedHook              func(*MediaTask)
	events                          *orchestratorEvents
	getHook                         func(int, *MediaTask)
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
	if r.getErrors > 0 {
		r.getErrors--
		return nil, errors.New("task read unavailable")
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
	if _, publishesReady := updates["billing_status"]; publishesReady && r.readyWriteErrors > 0 {
		r.readyWriteErrors--
		return false, errors.New("ready write failed")
	}
	applyOrchestratorTaskUpdates(task, updates)
	task.Version++
	if _, publishesReady := updates["billing_status"]; publishesReady && r.readyPublishedHook != nil {
		r.readyPublishedHook(task)
	}
	if _, publishesReady := updates["billing_status"]; publishesReady && r.readyWriteAppliedErrors > 0 {
		r.readyWriteAppliedErrors--
		return false, errors.New("ready write result unavailable")
	}
	return true, nil
}

func (r *orchestratorTaskRepository) TransitionQueued(ctx context.Context, id, expectedVersion int64, to MediaTaskStatus, updates map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.Status != MediaTaskStatusQueued || task.Version != expectedVersion || to != MediaTaskStatusFailed {
		return false, nil
	}
	applyOrchestratorTaskUpdates(task, updates)
	task.Status = to
	task.Version++
	if r.transitionQueuedAppliedErrors > 0 {
		r.transitionQueuedAppliedErrors--
		return true, errors.New("transition result unavailable")
	}
	return true, nil
}

func (r *orchestratorTaskRepository) Claim(ctx context.Context, id int64, workerID, claimToken string, leaseUntil time.Time, version int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok || task.Version != version || task.Status.IsTerminal() {
		return false, nil
	}
	if task.Status == MediaTaskStatusQueued &&
		(task.BillingStatus != MediaBillingStatusPrecharged || task.LeaseUntil != nil) {
		return false, nil
	}
	task.Status = MediaTaskStatusInProgress
	task.WorkerID = workerID
	task.ClaimToken = claimToken
	task.LeaseUntil = mediaTimePointer(leaseUntil)
	task.Version++
	return true, nil
}
func (r *orchestratorTaskRepository) RenewLease(context.Context, int64, string, time.Time) (bool, error) {
	return false, errors.New("not implemented")
}
func (r *orchestratorTaskRepository) UpdateClaimed(context.Context, int64, string, int64, MediaTaskStage, map[string]any) (bool, error) {
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

func (r *orchestratorTaskRepository) TransitionSyncTimeout(ctx context.Context, id, expectedVersion int64, expectedStage MediaTaskStage, from MediaTaskStatus, updates map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return false, nil
	}
	if r.completeOnNextTransition {
		r.completeOnNextTransition = false
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		task.Version++
		return false, nil
	}
	if r.beforeNextVersionedTransition != nil {
		hook := r.beforeNextVersionedTransition
		r.beforeNextVersionedTransition = nil
		hook(task)
	}
	if r.transitionVersionedErr != nil {
		err := r.transitionVersionedErr
		r.transitionVersionedErr = nil
		return false, err
	}
	if task.Status != from || task.Stage != expectedStage || task.Version != expectedVersion || task.SyncFallback || !from.CanTransitionTo(MediaTaskStatusFailed) {
		return false, nil
	}
	task.Status = MediaTaskStatusFailed
	applyOrchestratorTaskUpdates(task, updates)
	task.Version++
	if r.transitionVersionedAppliedErr != nil {
		err := r.transitionVersionedAppliedErr
		r.transitionVersionedAppliedErr = nil
		return true, err
	}
	return true, nil
}

func (r *orchestratorTaskRepository) TransitionClaimed(context.Context, int64, string, int64, MediaTaskStage, MediaTaskStatus, MediaTaskStatus, map[string]any) (bool, error) {
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
	if r.markFallbackErr != nil {
		err := r.markFallbackErr
		r.markFallbackErr = nil
		return false, err
	}
	if r.completeOnNextFallback {
		r.completeOnNextFallback = false
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		return false, nil
	}
	task.SyncFallback = true
	task.SyncFallbackAt = mediaTimePointer(at)
	if r.completeAfterSuccessfulFallback {
		r.completeAfterSuccessfulFallback = false
		task.Status = MediaTaskStatusCompleted
		task.Stage = MediaTaskStageCompleted
		task.Version++
	}
	if r.markFallbackAppliedErr != nil {
		err := r.markFallbackAppliedErr
		r.markFallbackAppliedErr = nil
		return true, err
	}
	return true, nil
}

func (r *orchestratorTaskRepository) ListRecoverable(context.Context, time.Time, int) ([]MediaTask, error) {
	return nil, nil
}
func (r *orchestratorTaskRepository) ListSettlementPending(_ context.Context, limit int) ([]MediaTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]MediaTask, 0, limit)
	for _, task := range r.tasks {
		billingPending := task.BillingStatus == MediaBillingStatusPrecharged ||
			task.BillingStatus == MediaBillingStatusSettling || task.BillingStatus == MediaBillingStatusRetry
		hasRecovery := task.Status.IsTerminal() && len(task.SettlementRecovery) > 0
		if billingPending && (len(task.SettlementPlan) > 0 || hasRecovery) {
			result = append(result, *cloneOrchestratorTask(task))
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
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
	if _, persistsPlan := updates["settlement_plan"]; persistsPlan && r.failSettlementPlanWrites > 0 {
		r.failSettlementPlanWrites--
		return false, errors.New("settlement plan write failed")
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
func (r *orchestratorTaskRepository) put(task *MediaTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[task.ID] = cloneOrchestratorTask(task)
	if task.ID >= r.nextID {
		r.nextID = task.ID + 1
	}
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
		case "candidate_snapshot":
			task.CandidateSnapshot = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "billing_status":
			task.BillingStatus = value.(string)
		case "billing_snapshot":
			task.BillingSnapshot = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "settlement_plan":
			task.SettlementPlan = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "settlement_recovery":
			task.SettlementRecovery = append(json.RawMessage(nil), value.(json.RawMessage)...)
		case "precharged_amount":
			task.PrechargedAmount = value.(float64)
		case "final_amount":
			task.FinalAmount = value.(float64)
		case "refunded_amount":
			task.RefundedAmount = value.(float64)
		case "additional_charged_amount":
			task.AdditionalChargedAmount = value.(float64)
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
		case "lease_until":
			if value == nil {
				task.LeaseUntil = nil
			} else {
				task.LeaseUntil = mediaTimePointer(value.(time.Time))
			}
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
	copy.SettlementRecovery = append(json.RawMessage(nil), task.SettlementRecovery...)
	copy.PollMetadata = append(json.RawMessage(nil), task.PollMetadata...)
	copy.AccountID = cloneOrchestratorInt64(task.AccountID)
	copy.SyncFallbackAt = cloneOrchestratorTime(task.SyncFallbackAt)
	copy.LeaseUntil = cloneOrchestratorTime(task.LeaseUntil)
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
	mu        sync.Mutex
	nextID    int64
	items     map[int64][]MediaArtifact
	createErr error
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
	if r.createErr != nil {
		return nil, r.createErr
	}
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
	enqueueFunc       func(int, int64) error
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
	call := q.enqueues
	hook, enqueueFunc, err := q.enqueueHook, q.enqueueFunc, q.enqueueErr
	q.mu.Unlock()
	if hook != nil {
		hook(taskID)
	}
	if enqueueFunc != nil {
		return enqueueFunc(call, taskID)
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
	mu                    sync.Mutex
	precharges            int
	failures              []MediaFailureSettlement
	settleFailureErr      error
	prechargeErr          error
	prechargeFunc         func(int, *MediaTask) error
	inspectPrecharge      func(*MediaTask)
	prechargeEntered      chan struct{}
	prechargeBlock        <-chan struct{}
	prechargeEnterOnce    sync.Once
	actualPrechargeAmount *float64
}

func (b *orchestratorBilling) Precharge(_ context.Context, task *MediaTask, snapshot MediaBillingSnapshot) (MediaPrechargeResult, error) {
	b.mu.Lock()
	b.precharges++
	call := b.precharges
	inspect, prechargeFunc, entered, block, err := b.inspectPrecharge, b.prechargeFunc, b.prechargeEntered, b.prechargeBlock, b.prechargeErr
	b.mu.Unlock()
	if inspect != nil {
		inspect(task)
	}
	if entered != nil {
		b.prechargeEnterOnce.Do(func() { close(entered) })
	}
	if block != nil {
		<-block
	}
	if prechargeFunc != nil {
		return MediaPrechargeResult{}, prechargeFunc(call, task)
	}
	amount := snapshot.EstimatedAmount
	if b.actualPrechargeAmount != nil {
		amount = *b.actualPrechargeAmount
	}
	return MediaPrechargeResult{PrechargedAmount: amount}, err
}
func (b *orchestratorBilling) SettleSuccess(_ context.Context, task *MediaTask, _ MediaUsage) (MediaSettlementResult, error) {
	return MediaSettlementResult{FinalAmount: task.PrechargedAmount}, nil
}
func (b *orchestratorBilling) SettleFailure(_ context.Context, task *MediaTask, settlement MediaFailureSettlement) (MediaSettlementResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = append(b.failures, settlement)
	return MediaSettlementResult{
		FinalAmount:    task.PrechargedAmount * (1 - settlement.RefundRatio),
		RefundedAmount: task.PrechargedAmount * settlement.RefundRatio,
	}, b.settleFailureErr
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
