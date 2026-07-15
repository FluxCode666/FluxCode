package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

var ErrFakeMediaUpstreamTaskNotFound = errors.New("fake media upstream task not found")

// FakeMediaAdapterCallCounts exposes race-safe invocation and progress counts
// to tests that need to verify at-least-once execution behavior.
type FakeMediaAdapterCallCounts struct {
	generateCalls   atomic.Int64
	submitCalls     atomic.Int64
	pollCalls       atomic.Int64
	successfulPolls atomic.Int64
}

func (c *FakeMediaAdapterCallCounts) GenerateCalls() int64 {
	if c == nil {
		return 0
	}
	return c.generateCalls.Load()
}

func (c *FakeMediaAdapterCallCounts) SubmitCalls() int64 {
	if c == nil {
		return 0
	}
	return c.submitCalls.Load()
}

func (c *FakeMediaAdapterCallCounts) PollCalls() int64 {
	if c == nil {
		return 0
	}
	return c.pollCalls.Load()
}

func (c *FakeMediaAdapterCallCounts) SuccessfulPolls() int64 {
	if c == nil {
		return 0
	}
	return c.successfulPolls.Load()
}

type FakeMediaAdapterOptions struct {
	Name            string
	NativeAsyncMode NativeAsyncMode
	GenerateResult  *MediaGenerateResult
	PollsBeforeDone int
	Artifacts       []MediaArtifactInput
	Usage           MediaUsage
	UpstreamTaskID  string
	PollMetadata    json.RawMessage
	GenerateError   error
	SubmitError     error
	PollError       error
	CallCounts      *FakeMediaAdapterCallCounts
}

type fakeMediaAdapterState struct {
	mu sync.Mutex

	name            string
	generateResult  *MediaGenerateResult
	pollsBeforeDone int
	artifacts       []MediaArtifactInput
	usage           MediaUsage
	upstreamTaskID  string
	pollMetadata    json.RawMessage
	generateError   error
	submitError     error
	pollError       error
	callCounts      *FakeMediaAdapterCallCounts
	nextTaskID      int64
	taskPolls       map[string]int
}

type fakeSyncMediaAdapter struct {
	state *fakeMediaAdapterState
}

type fakeAsyncMediaAdapter struct {
	state *fakeMediaAdapterState
}

type fakeOptionalMediaAdapter struct {
	state *fakeMediaAdapterState
}

var (
	_ MediaAdapter        = (*fakeSyncMediaAdapter)(nil)
	_ MediaSyncGenerator  = (*fakeSyncMediaAdapter)(nil)
	_ MediaAdapter        = (*fakeAsyncMediaAdapter)(nil)
	_ MediaAsyncSubmitter = (*fakeAsyncMediaAdapter)(nil)
	_ MediaAsyncPoller    = (*fakeAsyncMediaAdapter)(nil)
	_ MediaAdapter        = (*fakeOptionalMediaAdapter)(nil)
	_ MediaSyncGenerator  = (*fakeOptionalMediaAdapter)(nil)
	_ MediaAsyncSubmitter = (*fakeOptionalMediaAdapter)(nil)
	_ MediaAsyncPoller    = (*fakeOptionalMediaAdapter)(nil)
)

func NewFakeMediaAdapter(options FakeMediaAdapterOptions) MediaAdapter {
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "fake"
	}
	callCounts := options.CallCounts
	if callCounts == nil {
		callCounts = &FakeMediaAdapterCallCounts{}
	}
	state := &fakeMediaAdapterState{
		name:            name,
		generateResult:  cloneMediaGenerateResult(options.GenerateResult),
		pollsBeforeDone: options.PollsBeforeDone,
		artifacts:       cloneMediaArtifactInputs(options.Artifacts),
		usage:           options.Usage,
		upstreamTaskID:  strings.TrimSpace(options.UpstreamTaskID),
		pollMetadata:    cloneMediaRawMessage(options.PollMetadata),
		generateError:   options.GenerateError,
		submitError:     options.SubmitError,
		pollError:       options.PollError,
		callCounts:      callCounts,
		taskPolls:       make(map[string]int),
	}

	switch options.NativeAsyncMode {
	case NativeAsyncRequired:
		return &fakeAsyncMediaAdapter{state: state}
	case NativeAsyncOptional:
		return &fakeOptionalMediaAdapter{state: state}
	default:
		return &fakeSyncMediaAdapter{state: state}
	}
}

func (a *fakeSyncMediaAdapter) Name() string {
	return a.state.name
}

func (a *fakeSyncMediaAdapter) Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error) {
	return a.state.generate(ctx, req)
}

func (a *fakeAsyncMediaAdapter) Name() string {
	return a.state.name
}

func (a *fakeAsyncMediaAdapter) Submit(ctx context.Context, req MediaExecutionRequest) (*MediaAsyncSubmission, error) {
	return a.state.submit(ctx, req)
}

func (a *fakeAsyncMediaAdapter) Poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error) {
	return a.state.poll(ctx, req)
}

func (a *fakeOptionalMediaAdapter) Name() string {
	return a.state.name
}

func (a *fakeOptionalMediaAdapter) Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error) {
	return a.state.generate(ctx, req)
}

func (a *fakeOptionalMediaAdapter) Submit(ctx context.Context, req MediaExecutionRequest) (*MediaAsyncSubmission, error) {
	return a.state.submit(ctx, req)
}

func (a *fakeOptionalMediaAdapter) Poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error) {
	return a.state.poll(ctx, req)
}

func (s *fakeMediaAdapterState) generate(ctx context.Context, _ MediaExecutionRequest) (*MediaGenerateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.callCounts.generateCalls.Add(1)
	if s.generateError != nil {
		return nil, s.generateError
	}
	if s.generateResult != nil {
		return cloneMediaGenerateResult(s.generateResult), nil
	}
	return &MediaGenerateResult{
		Artifacts: cloneMediaArtifactInputs(s.artifacts),
		Usage:     s.usage,
	}, nil
}

func (s *fakeMediaAdapterState) submit(ctx context.Context, _ MediaExecutionRequest) (*MediaAsyncSubmission, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.callCounts.submitCalls.Add(1)
	if s.submitError != nil {
		return nil, s.submitError
	}

	s.mu.Lock()
	upstreamTaskID := s.upstreamTaskID
	if upstreamTaskID == "" {
		s.nextTaskID++
		upstreamTaskID = fmt.Sprintf("fake-media-task-%d", s.nextTaskID)
	}
	if _, exists := s.taskPolls[upstreamTaskID]; !exists {
		s.taskPolls[upstreamTaskID] = 0
	}
	s.mu.Unlock()

	return &MediaAsyncSubmission{
		UpstreamTaskID: upstreamTaskID,
		PollMetadata:   cloneMediaRawMessage(s.pollMetadata),
	}, nil
}

func (s *fakeMediaAdapterState) poll(ctx context.Context, req MediaPollRequest) (*MediaPollResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.callCounts.pollCalls.Add(1)

	s.mu.Lock()
	polls, exists := s.taskPolls[req.UpstreamTaskID]
	if !exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrFakeMediaUpstreamTaskNotFound, req.UpstreamTaskID)
	}
	if s.pollError != nil {
		s.mu.Unlock()
		return nil, s.pollError
	}
	polls++
	s.taskPolls[req.UpstreamTaskID] = polls
	s.mu.Unlock()
	s.callCounts.successfulPolls.Add(1)

	pollsBeforeDone := s.pollsBeforeDone
	if pollsBeforeDone < 1 {
		pollsBeforeDone = 1
	}
	if polls < pollsBeforeDone {
		return &MediaPollResult{
			State:    MediaPollStateRunning,
			Progress: polls * 100 / pollsBeforeDone,
		}, nil
	}
	return &MediaPollResult{
		State:    MediaPollStateCompleted,
		Progress: 100,
		Result: &MediaGenerateResult{
			Artifacts: cloneMediaArtifactInputs(s.artifacts),
			Usage:     s.usage,
		},
	}, nil
}

func cloneMediaGenerateResult(result *MediaGenerateResult) *MediaGenerateResult {
	if result == nil {
		return nil
	}
	return &MediaGenerateResult{
		Artifacts: cloneMediaArtifactInputs(result.Artifacts),
		Usage:     result.Usage,
	}
}

func cloneMediaArtifactInputs(artifacts []MediaArtifactInput) []MediaArtifactInput {
	if artifacts == nil {
		return nil
	}
	cloned := make([]MediaArtifactInput, len(artifacts))
	copy(cloned, artifacts)
	for index := range cloned {
		cloned[index].Data = append([]byte(nil), artifacts[index].Data...)
	}
	return cloned
}

func cloneMediaRawMessage(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}
