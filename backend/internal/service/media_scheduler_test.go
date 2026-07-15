package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mediaSchedulerAccountRepoStub struct {
	mu sync.Mutex

	accounts       []Account
	listErr        error
	getErr         error
	updateErr      error
	getCalls       int
	updateLastUsed []int64
}

func (s *mediaSchedulerAccountRepoStub) ListSchedulableByGroupID(ctx context.Context, _ int64) ([]Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]Account(nil), s.accounts...), nil
}

func (s *mediaSchedulerAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	if s.getErr != nil {
		return nil, s.getErr
	}
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			copy := s.accounts[i]
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *mediaSchedulerAccountRepoStub) UpdateLastUsed(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updateLastUsed = append(s.updateLastUsed, id)
	return nil
}

func (s *mediaSchedulerAccountRepoStub) replaceAccounts(accounts []Account) {
	s.mu.Lock()
	s.accounts = append([]Account(nil), accounts...)
	s.mu.Unlock()
}

type mediaSchedulerSelectorStub struct {
	mu sync.Mutex

	selectedID int64
	result     *AccountSelectionResult
	err        error
	candidates []*Account
	requests   int
	waitCalls  int
	waitFunc   func()
	waitErr    error
	returnNil  bool
}

func (s *mediaSchedulerSelectorStub) Select(_ context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	s.candidates = append([]*Account(nil), req.Candidates...)
	if s.err != nil {
		return nil, s.err
	}
	if s.returnNil {
		return nil, nil
	}
	if s.result != nil {
		copy := *s.result
		return &copy, nil
	}
	for _, candidate := range req.Candidates {
		if candidate.ID == s.selectedID {
			return &AccountSelectionResult{Account: candidate, Acquired: true, ReleaseFunc: func() {}}, nil
		}
	}
	return nil, ErrNoAvailableAccounts
}

func (s *mediaSchedulerSelectorStub) Wait(context.Context, *AccountWaitPlan) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waitCalls++
	return s.waitFunc, s.waitErr
}

func task12Account(id int64, platform, model, upstream, adapter string, mode NativeAsyncMode) Account {
	return Account{
		ID: id, Platform: platform, Priority: int(id), Concurrency: 2,
		Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{model: upstream}},
		Extra: map[string]any{"media_config": map[string]any{
			"adapter": adapter, "native_async_mode": string(mode),
			"model_overrides": map[string]any{model: map[string]any{"upstream_model": upstream}},
		}},
	}
}

func TestMediaSchedulerSelectsAcrossPlatformsForSameModel(t *testing.T) {
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{
		task12Account(1, PlatformGemini, "veo-3.1", "veo-gemini", "gemini", NativeAsyncOptional),
		task12Account(2, PlatformOpenAI, "veo-3.1", "veo-xai", "xai", NativeAsyncRequired),
	}}
	selector := &mediaSchedulerSelectorStub{selectedID: 2}
	registry := NewMediaAdapterRegistry()
	registry.Register("gemini", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "gemini", NativeAsyncMode: NativeAsyncOptional}))
	registry.Register("xai", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "xai", NativeAsyncMode: NativeAsyncRequired}))
	scheduler := NewMediaScheduler(repo, selector, registry)

	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 7, "veo-3.1")
	require.NoError(t, err)
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{
		GroupID: 7, RequestedModel: "veo-3.1", Operation: MediaOperationTextToVideo, CandidateSnapshot: snapshot,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	require.Equal(t, "xai", selection.ResolvedModel.Adapter)
	require.Equal(t, "veo-xai", selection.ResolvedModel.UpstreamModel)
	require.Len(t, selector.candidates, 2)
}

func TestMediaSchedulerSnapshotFiltersAvailabilityModelAndAdapterMethodSets(t *testing.T) {
	future := time.Now().Add(time.Hour)
	validSync := task12Account(1, PlatformOpenAI, "image", "image-up", "sync", NativeAsyncUnsupported)
	missingAdapter := task12Account(2, PlatformOpenAI, "image", "image-up", "missing", NativeAsyncUnsupported)
	wrongRequired := task12Account(3, PlatformOpenAI, "image", "image-up", "sync", NativeAsyncRequired)
	wrongOptional := task12Account(4, PlatformOpenAI, "image", "image-up", "async", NativeAsyncOptional)
	disabled := task12Account(5, PlatformOpenAI, "image", "image-up", "sync", NativeAsyncUnsupported)
	disabled.Status = StatusDisabled
	cooling := task12Account(6, PlatformOpenAI, "image", "image-up", "sync", NativeAsyncUnsupported)
	cooling.TempUnschedulableUntil = &future
	unsupportedModel := task12Account(7, PlatformOpenAI, "other", "other-up", "sync", NativeAsyncUnsupported)
	noConfig := task12Account(8, PlatformOpenAI, "image", "image-up", "", NativeAsyncUnsupported)
	noConfig.Extra = nil

	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	registry.Register("async", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "async", NativeAsyncMode: NativeAsyncRequired}))
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{validSync, missingAdapter, wrongRequired, wrongOptional, disabled, cooling, unsupportedModel, noConfig}}
	scheduler := NewMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)

	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)
	require.Equal(t, []MediaAccountCandidateSnapshot{{
		AccountID: 1, Platform: PlatformOpenAI,
		ResolvedModel: ResolvedMediaAccountModel{Adapter: "sync", UpstreamModel: "image-up", NativeAsyncMode: NativeAsyncUnsupported},
	}}, snapshot)
}

func TestMediaSchedulerSnapshotReturnsStableNoAvailableError(t *testing.T) {
	scheduler := NewMediaScheduler(&mediaSchedulerAccountRepoStub{}, &mediaSchedulerSelectorStub{}, NewMediaAdapterRegistry())
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.Nil(t, snapshot)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

func TestMediaSchedulerSelectExcludesAndUsesFrozenResolvedModel(t *testing.T) {
	original := task12Account(11, PlatformOpenAI, "image", "upstream-v1", "xai", NativeAsyncRequired)
	backup := task12Account(12, PlatformGemini, "image", "upstream-backup", "gemini", NativeAsyncOptional)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{original, backup}}
	selector := &mediaSchedulerSelectorStub{selectedID: 11}
	registry := NewMediaAdapterRegistry()
	registry.Register("xai", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "xai", NativeAsyncMode: NativeAsyncRequired}))
	registry.Register("gemini", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "gemini", NativeAsyncMode: NativeAsyncOptional}))
	scheduler := NewMediaScheduler(repo, selector, registry)
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)

	changed := task12Account(11, PlatformOpenAI, "different-model", "upstream-v2", "gemini", NativeAsyncOptional)
	repo.replaceAccounts([]Account{changed, backup})
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{
		GroupID: 1, RequestedModel: "image", Operation: MediaOperationTextToImage,
		ExcludedAccountIDs: map[int64]struct{}{12: {}}, CandidateSnapshot: snapshot,
	})
	require.NoError(t, err)
	require.Equal(t, int64(11), selection.Account.ID)
	require.Equal(t, ResolvedMediaAccountModel{Adapter: "xai", UpstreamModel: "upstream-v1", NativeAsyncMode: NativeAsyncRequired}, selection.ResolvedModel)
}

func TestMediaSchedulerSelectRechecksRealtimeAvailabilityAndAdapter(t *testing.T) {
	future := time.Now().Add(time.Hour)
	accounts := []Account{
		task12Account(1, PlatformOpenAI, "image", "up-1", "sync", NativeAsyncUnsupported),
		task12Account(2, PlatformOpenAI, "image", "up-2", "sync", NativeAsyncUnsupported),
		task12Account(3, PlatformOpenAI, "image", "up-3", "sync", NativeAsyncUnsupported),
	}
	repo := &mediaSchedulerAccountRepoStub{accounts: accounts}
	selector := &mediaSchedulerSelectorStub{selectedID: 3}
	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	scheduler := NewMediaScheduler(repo, selector, registry)
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)

	accounts[0].Schedulable = false
	accounts[1].TempUnschedulableUntil = &future
	repo.replaceAccounts(accounts)
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
	require.NoError(t, err)
	require.Equal(t, int64(3), selection.Account.ID)
	require.Len(t, selector.candidates, 1)

	missingRegistryScheduler := NewMediaScheduler(repo, selector, NewMediaAdapterRegistry())
	selection, err = missingRegistryScheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

func TestMediaSchedulerSelectRejectsDuplicateMalformedAndSelectorEscape(t *testing.T) {
	account := task12Account(1, PlatformOpenAI, "image", "up", "sync", NativeAsyncUnsupported)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))

	t.Run("duplicate snapshot id", func(t *testing.T) {
		scheduler := NewMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)
		duplicate := []MediaAccountCandidateSnapshot{
			{AccountID: 1, Platform: PlatformOpenAI, ResolvedModel: ResolvedMediaAccountModel{Adapter: "sync", UpstreamModel: "one", NativeAsyncMode: NativeAsyncUnsupported}},
			{AccountID: 1, Platform: PlatformOpenAI, ResolvedModel: ResolvedMediaAccountModel{Adapter: "sync", UpstreamModel: "two", NativeAsyncMode: NativeAsyncUnsupported}},
		}
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: duplicate})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("empty resolved model", func(t *testing.T) {
		scheduler := NewMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: []MediaAccountCandidateSnapshot{{AccountID: 1, Platform: PlatformOpenAI}}})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("selector outside candidate set", func(t *testing.T) {
		selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{Account: &Account{ID: 99}, Acquired: true, ReleaseFunc: func() {}}}
		scheduler := NewMediaScheduler(repo, selector, registry)
		snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
		require.NoError(t, err)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("nil selector result", func(t *testing.T) {
		selector := &mediaSchedulerSelectorStub{returnNil: true}
		scheduler := NewMediaScheduler(repo, selector, registry)
		snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
		require.NoError(t, err)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})
}

func TestMediaSchedulerWaitMarkUsedAndFixedAccountBoundaries(t *testing.T) {
	repoErr := errors.New("repository unavailable")
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{{ID: 7, Status: StatusDisabled, Schedulable: false}}}
	waitRelease := func() {}
	selector := &mediaSchedulerSelectorStub{waitFunc: waitRelease}
	scheduler := NewMediaScheduler(repo, selector, NewMediaAdapterRegistry())

	release, err := scheduler.WaitForSlot(context.Background(), &MediaAccountSelection{WaitPlan: &AccountWaitPlan{AccountID: 7}})
	require.NoError(t, err)
	require.NotNil(t, release)
	require.Equal(t, 1, selector.waitCalls)

	require.NoError(t, scheduler.MarkUsed(context.Background(), 7))
	require.Equal(t, []int64{7}, repo.updateLastUsed)

	fixed, err := scheduler.GetFixedAccount(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, int64(7), fixed.ID)
	require.Equal(t, StatusDisabled, fixed.Status, "固定账号读取不得因禁用而改选")
	require.Zero(t, selector.requests, "已有 upstream_task_id 的固定账号恢复边界不得调用 Select")

	repo.updateErr = repoErr
	require.ErrorIs(t, scheduler.MarkUsed(context.Background(), 7), repoErr)
	repo.getErr = repoErr
	_, err = scheduler.GetFixedAccount(context.Background(), 7)
	require.ErrorIs(t, err, repoErr)
}
