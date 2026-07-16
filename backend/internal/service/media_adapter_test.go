package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaAdapterTestStub struct {
	name string
}

func (a *mediaAdapterTestStub) Name() string {
	return a.name
}

func TestMediaAdapterRegistryReportsSmallCapabilities(t *testing.T) {
	t.Parallel()

	registry := NewMediaAdapterRegistry()
	registry.Register("fake-sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncUnsupported,
	}))
	registry.Register("fake-async", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
	}))
	registry.Register("fake-optional", NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncOptional,
	}))

	syncAdapter, err := registry.Resolve("fake-sync")
	require.NoError(t, err)
	require.IsType(t, (*fakeSyncMediaAdapter)(nil), syncAdapter)
	require.Implements(t, (*MediaAdapter)(nil), syncAdapter)
	require.Implements(t, (*MediaSyncGenerator)(nil), syncAdapter)
	require.NotImplements(t, (*MediaAsyncSubmitter)(nil), syncAdapter)
	require.NotImplements(t, (*MediaAsyncPoller)(nil), syncAdapter)
	require.NotImplements(t, (*MediaIdempotentSubmitter)(nil), syncAdapter)
	require.NotImplements(t, (*MediaAborter)(nil), syncAdapter)

	asyncAdapter, err := registry.Resolve("fake-async")
	require.NoError(t, err)
	require.IsType(t, (*fakeAsyncMediaAdapter)(nil), asyncAdapter)
	require.Implements(t, (*MediaAdapter)(nil), asyncAdapter)
	require.NotImplements(t, (*MediaSyncGenerator)(nil), asyncAdapter)
	require.Implements(t, (*MediaAsyncSubmitter)(nil), asyncAdapter)
	require.Implements(t, (*MediaAsyncPoller)(nil), asyncAdapter)
	require.NotImplements(t, (*MediaIdempotentSubmitter)(nil), asyncAdapter)
	require.NotImplements(t, (*MediaAborter)(nil), asyncAdapter)

	optionalAdapter, err := registry.Resolve("fake-optional")
	require.NoError(t, err)
	require.IsType(t, (*fakeOptionalMediaAdapter)(nil), optionalAdapter)
	require.Implements(t, (*MediaAdapter)(nil), optionalAdapter)
	require.Implements(t, (*MediaSyncGenerator)(nil), optionalAdapter)
	require.Implements(t, (*MediaAsyncSubmitter)(nil), optionalAdapter)
	require.Implements(t, (*MediaAsyncPoller)(nil), optionalAdapter)
	require.NotImplements(t, (*MediaIdempotentSubmitter)(nil), optionalAdapter)
	require.NotImplements(t, (*MediaAborter)(nil), optionalAdapter)
}

func TestMediaAdapterRegistryNormalizesAndRejectsInvalidRegistration(t *testing.T) {
	t.Parallel()

	registry := NewMediaAdapterRegistry()
	adapter := &mediaAdapterTestStub{name: "adapter-name-is-not-consulted"}
	registry.Register("  XAI  ", adapter)

	resolved, err := registry.Resolve(" xAi ")
	require.NoError(t, err)
	require.Same(t, adapter, resolved)

	require.PanicsWithValue(t, "media adapter name and implementation are required", func() {
		registry.Register("  ", adapter)
	})
	require.PanicsWithValue(t, "media adapter name and implementation are required", func() {
		registry.Register("nil", nil)
	})
	var typedNil *mediaAdapterTestStub
	require.PanicsWithValue(t, "media adapter name and implementation are required", func() {
		registry.Register("typed-nil", typedNil)
	})
	require.PanicsWithValue(t, "duplicate media adapter: xai", func() {
		registry.Register("xai", &mediaAdapterTestStub{})
	})
}

func TestMediaAdapterRegistryReturnsStableNotFoundError(t *testing.T) {
	t.Parallel()

	registry := NewMediaAdapterRegistry()
	for _, name := range []string{"missing", "  MISSING  ", ""} {
		adapter, err := registry.Resolve(name)
		require.Nil(t, adapter)
		require.ErrorIs(t, err, ErrMediaAdapterNotFound)
	}

	var zero MediaAdapterRegistry
	adapter, err := zero.Resolve("missing")
	require.Nil(t, adapter)
	require.ErrorIs(t, err, ErrMediaAdapterNotFound)
	zero.Register("late", &mediaAdapterTestStub{name: "late"})
	adapter, err = zero.Resolve("late")
	require.NoError(t, err)
	require.Equal(t, "late", adapter.Name())
}

func TestMediaArtifactInputCarriesDurableObjectKey(t *testing.T) {
	input := MediaArtifactInput{
		Direction: "input",
		Position:  0,
		MediaType: MediaTypeImage,
		ObjectKey: "media/input/task-1/source.png",
	}

	cloned := cloneMediaArtifactInputs([]MediaArtifactInput{input})
	require.Equal(t, input.ObjectKey, cloned[0].ObjectKey)
}

func TestMediaAdapterRegistrySupportsConcurrentRegisterAndResolve(t *testing.T) {
	t.Parallel()

	const adapterCount = 64
	registry := NewMediaAdapterRegistry()
	start := make(chan struct{})
	errCh := make(chan error, adapterCount*2)
	var wg sync.WaitGroup

	for index := 0; index < adapterCount; index++ {
		name := fmt.Sprintf("fake-%d", index)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			registry.Register(name, &mediaAdapterTestStub{name: name})
		}()
		go func() {
			defer wg.Done()
			<-start
			adapter, err := registry.Resolve("  " + strings.ToUpper(name) + "  ")
			if err != nil && !errors.Is(err, ErrMediaAdapterNotFound) {
				errCh <- err
				return
			}
			if err == nil && adapter == nil {
				errCh <- errors.New("registry returned a nil adapter without an error")
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	for index := 0; index < adapterCount; index++ {
		name := fmt.Sprintf("fake-%d", index)
		adapter, err := registry.Resolve(name)
		require.NoError(t, err)
		require.Equal(t, name, adapter.Name())
	}
}

func TestMediaAdapterErrorKeepsSafeClassificationAndErrorChain(t *testing.T) {
	t.Parallel()

	cause := errors.New("authorization=secret-upstream-token")
	adapterErr := &MediaAdapterError{
		Code:              "upstream_temporarily_unavailable",
		Message:           "上游服务暂时不可用",
		Retryable:         true,
		SubmissionUnknown: true,
		SystemFailure:     false,
		Cause:             cause,
	}
	wrapped := fmt.Errorf("submit media: %w", adapterErr)

	var classified *MediaAdapterError
	require.ErrorAs(t, wrapped, &classified)
	require.Same(t, adapterErr, classified)
	require.ErrorIs(t, wrapped, cause)
	require.Equal(t, "upstream_temporarily_unavailable", classified.Code)
	require.Equal(t, "上游服务暂时不可用", classified.Message)
	require.True(t, classified.Retryable)
	require.True(t, classified.SubmissionUnknown)
	require.False(t, classified.SystemFailure)
	require.Equal(t, classified.Message, classified.Error())
	require.NotContains(t, classified.Error(), "secret-upstream-token")

	encoded, err := json.Marshal(classified)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"code":"upstream_temporarily_unavailable",
		"message":"上游服务暂时不可用",
		"retryable":true,
		"submission_unknown":true,
		"system_failure":false
	}`, string(encoded))
	require.NotContains(t, string(encoded), "secret-upstream-token")

	require.Equal(t, "media adapter error", (*MediaAdapterError)(nil).Error())
	require.Equal(t, "media adapter error", (&MediaAdapterError{Cause: cause}).Error())
}

func TestFakeNativeAsyncAdapterCompletesAfterPoll(t *testing.T) {
	t.Parallel()

	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		PollsBeforeDone: 2,
		Artifacts:       []MediaArtifactInput{{MediaType: MediaTypeVideo, ContentType: "video/mp4"}},
	})
	submitter := adapter.(MediaAsyncSubmitter)
	poller := adapter.(MediaAsyncPoller)
	submission, err := submitter.Submit(context.Background(), MediaExecutionRequest{IdempotencyKey: "task_fake"})
	require.NoError(t, err)
	first, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, MediaPollStateRunning, first.State)
	second, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, MediaPollStateCompleted, second.State)
}

func TestMediaFakeNativeAsyncAdapterCompletesAfterConfiguredPolls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		pollsBeforeDone int
		wantStates      []MediaPollState
	}{
		{name: "zero completes on first poll", pollsBeforeDone: 0, wantStates: []MediaPollState{MediaPollStateCompleted}},
		{name: "one completes on first poll", pollsBeforeDone: 1, wantStates: []MediaPollState{MediaPollStateCompleted}},
		{name: "two runs then completes", pollsBeforeDone: 2, wantStates: []MediaPollState{MediaPollStateRunning, MediaPollStateCompleted}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := &FakeMediaAdapterCallCounts{}
			adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
				Name:            "fake-video",
				NativeAsyncMode: NativeAsyncRequired,
				PollsBeforeDone: tt.pollsBeforeDone,
				UpstreamTaskID:  "upstream-42",
				PollMetadata:    json.RawMessage(`{"cursor":"first"}`),
				Artifacts:       []MediaArtifactInput{{MediaType: MediaTypeVideo, ContentType: "video/mp4", Data: []byte("video")}},
				Usage:           MediaUsage{VideoSeconds: 5, VideoResolution: "720p"},
				CallCounts:      calls,
			})
			require.Equal(t, "fake-video", adapter.Name())
			submitter := adapter.(MediaAsyncSubmitter)
			poller := adapter.(MediaAsyncPoller)

			submission, err := submitter.Submit(context.Background(), MediaExecutionRequest{IdempotencyKey: "task_fake"})
			require.NoError(t, err)
			require.Equal(t, "upstream-42", submission.UpstreamTaskID)
			require.JSONEq(t, `{"cursor":"first"}`, string(submission.PollMetadata))

			for index, wantState := range tt.wantStates {
				result, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
				require.NoError(t, err)
				require.Equal(t, wantState, result.State)
				if wantState == MediaPollStateRunning {
					require.Nil(t, result.Result)
					require.Less(t, result.Progress, 100)
					continue
				}
				require.Equal(t, 100, result.Progress)
				require.NotNil(t, result.Result)
				require.Equal(t, MediaUsage{VideoSeconds: 5, VideoResolution: "720p"}, result.Result.Usage)
				require.Equal(t, []byte("video"), result.Result.Artifacts[0].Data, "poll %d", index+1)
			}
			require.EqualValues(t, 1, calls.SubmitCalls())
			require.EqualValues(t, len(tt.wantStates), calls.PollCalls())
			require.EqualValues(t, len(tt.wantStates), calls.SuccessfulPolls())
		})
	}
}

func TestMediaFakeSyncResultAndMutableDataAreDefensivelyCopied(t *testing.T) {
	t.Parallel()

	original := &MediaGenerateResult{
		Artifacts: []MediaArtifactInput{{
			Direction:   "output",
			Position:    1,
			MediaType:   MediaTypeImage,
			ContentType: "image/png",
			Data:        []byte("original-image"),
		}},
		Usage: MediaUsage{ImageCount: 1, ImageSize: "1024x1024"},
	}
	calls := &FakeMediaAdapterCallCounts{}
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncUnsupported,
		GenerateResult:  original,
		CallCounts:      calls,
	})
	generator := adapter.(MediaSyncGenerator)

	original.Artifacts[0].Data[0] = 'X'
	original.Artifacts[0].ContentType = "mutated/before-call"
	first, err := generator.Generate(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)
	require.Equal(t, "image/png", first.Artifacts[0].ContentType)
	require.Equal(t, []byte("original-image"), first.Artifacts[0].Data)

	first.Artifacts[0].Data[0] = 'Y'
	first.Artifacts[0].ContentType = "mutated/after-call"
	second, err := generator.Generate(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)
	require.Equal(t, "image/png", second.Artifacts[0].ContentType)
	require.Equal(t, []byte("original-image"), second.Artifacts[0].Data)
	require.EqualValues(t, 2, calls.GenerateCalls())
}

func TestMediaFakeAsyncResultsAndMetadataAreDefensivelyCopied(t *testing.T) {
	t.Parallel()

	artifacts := []MediaArtifactInput{{MediaType: MediaTypeVideo, ContentType: "video/mp4", Data: []byte("immutable-video")}}
	metadata := json.RawMessage(`{"cursor":"immutable"}`)
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		PollsBeforeDone: 1,
		UpstreamTaskID:  "upstream-copy",
		PollMetadata:    metadata,
		Artifacts:       artifacts,
	})
	artifacts[0].Data[0] = 'X'
	metadata[0] = '['
	submitter := adapter.(MediaAsyncSubmitter)
	poller := adapter.(MediaAsyncPoller)

	firstSubmission, err := submitter.Submit(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)
	require.JSONEq(t, `{"cursor":"immutable"}`, string(firstSubmission.PollMetadata))
	firstSubmission.PollMetadata[0] = '['
	secondSubmission, err := submitter.Submit(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)
	require.JSONEq(t, `{"cursor":"immutable"}`, string(secondSubmission.PollMetadata))

	first, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: firstSubmission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, []byte("immutable-video"), first.Result.Artifacts[0].Data)
	first.Result.Artifacts[0].Data[0] = 'Y'
	first.Result.Artifacts[0].ContentType = "mutated/video"
	second, err := poller.Poll(context.Background(), MediaPollRequest{UpstreamTaskID: firstSubmission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, "video/mp4", second.Result.Artifacts[0].ContentType)
	require.Equal(t, []byte("immutable-video"), second.Result.Artifacts[0].Data)
}

func TestMediaFakeErrorsPreserveChainsAndDoNotAdvanceSuccessfulPolls(t *testing.T) {
	t.Parallel()

	generateCause := errors.New("generate cause")
	generateCalls := &FakeMediaAdapterCallCounts{}
	generator := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncUnsupported,
		GenerateError:   fmt.Errorf("fake generate: %w", generateCause),
		CallCounts:      generateCalls,
	}).(MediaSyncGenerator)
	_, err := generator.Generate(context.Background(), MediaExecutionRequest{})
	require.ErrorIs(t, err, generateCause)
	require.EqualValues(t, 1, generateCalls.GenerateCalls())

	submitCause := errors.New("submit cause")
	submitCalls := &FakeMediaAdapterCallCounts{}
	submitter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		SubmitError:     fmt.Errorf("fake submit: %w", submitCause),
		CallCounts:      submitCalls,
	}).(MediaAsyncSubmitter)
	_, err = submitter.Submit(context.Background(), MediaExecutionRequest{})
	require.ErrorIs(t, err, submitCause)
	require.EqualValues(t, 1, submitCalls.SubmitCalls())

	pollCause := errors.New("poll cause")
	pollCalls := &FakeMediaAdapterCallCounts{}
	asyncAdapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		UpstreamTaskID:  "poll-error-task",
		PollError:       fmt.Errorf("fake poll: %w", pollCause),
		CallCounts:      pollCalls,
	})
	submission, err := asyncAdapter.(MediaAsyncSubmitter).Submit(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)
	_, err = asyncAdapter.(MediaAsyncPoller).Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.ErrorIs(t, err, pollCause)
	require.EqualValues(t, 1, pollCalls.PollCalls())
	require.Zero(t, pollCalls.SuccessfulPolls())
}

func TestMediaFakeRejectsUnknownUpstreamTask(t *testing.T) {
	t.Parallel()

	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{NativeAsyncMode: NativeAsyncRequired})
	result, err := adapter.(MediaAsyncPoller).Poll(context.Background(), MediaPollRequest{UpstreamTaskID: "unknown"})
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrFakeMediaUpstreamTaskNotFound)
}

func TestMediaFakePreservesContextCancellationWithoutSideEffects(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	syncCalls := &FakeMediaAdapterCallCounts{}
	generator := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncUnsupported,
		CallCounts:      syncCalls,
	}).(MediaSyncGenerator)
	_, err := generator.Generate(canceled, MediaExecutionRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, syncCalls.GenerateCalls())

	asyncCalls := &FakeMediaAdapterCallCounts{}
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		UpstreamTaskID:  "context-task",
		CallCounts:      asyncCalls,
	})
	_, err = adapter.(MediaAsyncSubmitter).Submit(canceled, MediaExecutionRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, asyncCalls.SubmitCalls())

	submission, err := adapter.(MediaAsyncSubmitter).Submit(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)
	_, err = adapter.(MediaAsyncPoller).Poll(canceled, MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, asyncCalls.PollCalls())
	require.Zero(t, asyncCalls.SuccessfulPolls())
}

func TestMediaFakeSupportsConcurrentPollingWithoutSharingResults(t *testing.T) {
	t.Parallel()

	const pollCount = 64
	calls := &FakeMediaAdapterCallCounts{}
	adapter := NewFakeMediaAdapter(FakeMediaAdapterOptions{
		NativeAsyncMode: NativeAsyncRequired,
		PollsBeforeDone: pollCount,
		UpstreamTaskID:  "concurrent-task",
		Artifacts:       []MediaArtifactInput{{MediaType: MediaTypeImage, Data: []byte("image")}},
		CallCounts:      calls,
	})
	submission, err := adapter.(MediaAsyncSubmitter).Submit(context.Background(), MediaExecutionRequest{})
	require.NoError(t, err)

	resultCh := make(chan *MediaPollResult, pollCount)
	errCh := make(chan error, pollCount)
	var wg sync.WaitGroup
	for range pollCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, pollErr := adapter.(MediaAsyncPoller).Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
			if pollErr != nil {
				errCh <- pollErr
				return
			}
			resultCh <- result
		}()
	}
	wg.Wait()
	close(resultCh)
	close(errCh)
	for pollErr := range errCh {
		require.NoError(t, pollErr)
	}

	completed := 0
	for result := range resultCh {
		if result.State == MediaPollStateCompleted {
			completed++
			result.Result.Artifacts[0].Data[0] = 'X'
		}
	}
	require.Equal(t, 1, completed)
	require.EqualValues(t, pollCount, calls.PollCalls())
	require.EqualValues(t, pollCount, calls.SuccessfulPolls())

	final, err := adapter.(MediaAsyncPoller).Poll(context.Background(), MediaPollRequest{UpstreamTaskID: submission.UpstreamTaskID})
	require.NoError(t, err)
	require.Equal(t, MediaPollStateCompleted, final.State)
	require.Equal(t, []byte("image"), final.Result.Artifacts[0].Data)
}
