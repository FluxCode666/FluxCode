package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

type mediaSchedulerGroupRepoStub struct {
	group *Group
	err   error
	calls atomic.Int64
}

type mediaSchedulerScopeRepoStub struct {
	modelIDs []string
	err      error
}

func (s *mediaSchedulerScopeRepoStub) ListEnabledMediaModelIDs(context.Context, int64) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]string(nil), s.modelIDs...), nil
}

func (s *mediaSchedulerGroupRepoStub) GetByID(ctx context.Context, _ int64) (*Group, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	if s.group == nil {
		return nil, ErrGroupNotFound
	}
	copy := *s.group
	return &copy, nil
}

func newTestMediaScheduler(accountRepo MediaSchedulerAccountRepository, selector AccountCandidateSelector, adapters *MediaAdapterRegistry, groups ...MediaSchedulerGroupRepository) *MediaScheduler {
	groupRepo := MediaSchedulerGroupRepository(&mediaSchedulerGroupRepoStub{group: &Group{
		Platform: PlatformOpenAI, MediaCrossPlatformEnabled: true,
	}})
	if len(groups) > 0 {
		groupRepo = groups[0]
	}
	return NewMediaScheduler(accountRepo, selector, adapters, groupRepo)
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

	selectedID      int64
	result          *AccountSelectionResult
	err             error
	candidates      []*Account
	requests        int
	waitCalls       int
	waitFunc        func()
	waitRefreshFunc func(context.Context) (bool, error)
	waitErr         error
	returnNil       bool
	lastGroupID     int64
	lastSessionHash string
	lastSlotID      string
}

func (s *mediaSchedulerSelectorStub) Select(_ context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	s.candidates = append([]*Account(nil), req.Candidates...)
	s.lastGroupID = req.GroupID
	s.lastSessionHash = req.SessionHash
	s.lastSlotID = req.SlotID
	if s.returnNil {
		return nil, nil
	}
	if s.result != nil {
		copy := *s.result
		return &copy, s.err
	}
	if s.err != nil {
		return nil, s.err
	}
	for _, candidate := range req.Candidates {
		if candidate.ID == s.selectedID {
			result := &AccountSelectionResult{Account: candidate, Acquired: true, ReleaseFunc: func() {}}
			if req.SlotID != "" {
				result.RefreshFunc = func(context.Context) (bool, error) { return true, nil }
			}
			return result, nil
		}
	}
	return nil, ErrNoAvailableAccounts
}

func (s *mediaSchedulerSelectorStub) SelectFixed(ctx context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error) {
	return s.Select(ctx, req)
}

func (s *mediaSchedulerSelectorStub) Wait(context.Context, *AccountWaitPlan) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waitCalls++
	return s.waitFunc, s.waitErr
}

func (s *mediaSchedulerSelectorStub) WaitStable(context.Context, *AccountWaitPlan) (*AcquireResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waitCalls++
	refresh := s.waitRefreshFunc
	if refresh == nil {
		refresh = func(context.Context) (bool, error) { return true, nil }
	}
	return &AcquireResult{Acquired: true, ReleaseFunc: s.waitFunc, RefreshFunc: refresh}, s.waitErr
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
	scheduler := newTestMediaScheduler(repo, selector, registry)

	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 7, "veo-3.1")
	require.NoError(t, err)
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{
		GroupID: 7, RequestedModel: "veo-3.1", Operation: MediaOperationTextToVideo,
		SlotID: "media-task:first-task", CandidateSnapshot: snapshot,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	require.Equal(t, "xai", selection.ResolvedModel.Adapter)
	require.Equal(t, "veo-xai", selection.ResolvedModel.UpstreamModel)
	require.Len(t, selector.candidates, 2)
	require.Equal(t, "media-task:first-task", selector.lastSlotID)
	require.NotNil(t, selection.RefreshFunc)
	owned, refreshErr := selection.RefreshFunc(context.Background())
	require.NoError(t, refreshErr)
	require.True(t, owned)
}

func TestMediaSchedulerRoutesVersionOneMediaAccountByRegistryAndGroupScope(t *testing.T) {
	account := Account{
		ID: 21, Platform: PlatformMedia, Priority: 1, Concurrency: 1,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{"media_config": map[string]any{
			"version": 1, "provider": "volcengine", "models": map[string]any{
				"seedance": map[string]any{"enabled": true, "upstream_model_id": "doubao-seedance", "async_mode": "unsupported"},
			},
		}},
	}
	definition := MediaModelDefinition{
		ID: 31, ModelID: "seedance", Vendor: "bytedance", MediaType: MediaTypeImage,
		Operations: []MediaOperation{MediaOperationTextToImage}, DefaultAdapter: "openai-images",
		DefaultAsyncMode: NativeAsyncUnsupported, Enabled: true,
	}
	models := &mediaModelRepoStub{items: []MediaModelDefinition{definition}}
	registry := NewMediaModelRegistry(models)
	require.NoError(t, registry.Refresh(context.Background()))
	adapters := NewMediaAdapterRegistry()
	adapters.Register("openai-images", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "openai-images", NativeAsyncMode: NativeAsyncUnsupported}))
	groupRepo := &mediaSchedulerGroupRepoStub{group: &Group{ID: 9, Platform: PlatformMedia}}
	scheduler := NewMediaScheduler(
		&mediaSchedulerAccountRepoStub{accounts: []Account{account}},
		&mediaSchedulerSelectorStub{selectedID: account.ID}, adapters, groupRepo,
		registry, &mediaSchedulerScopeRepoStub{modelIDs: []string{"seedance"}},
	)

	candidates, err := scheduler.SnapshotCandidates(context.Background(), 9, "seedance")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, PlatformMedia, candidates[0].Platform)
	require.Equal(t, "openai-images", candidates[0].ResolvedModel.Adapter)
	require.Equal(t, "doubao-seedance", candidates[0].ResolvedModel.UpstreamModel)
	require.Equal(t, "volcengine", candidates[0].ResolvedModel.Provider)
}

func TestMediaSchedulerRejectsMediaModelOutsideGroupScope(t *testing.T) {
	definition := MediaModelDefinition{ID: 32, ModelID: "image", Vendor: "openai", MediaType: MediaTypeImage,
		Operations: []MediaOperation{MediaOperationTextToImage}, DefaultAdapter: "sync", DefaultAsyncMode: NativeAsyncUnsupported, Enabled: true}
	registry := NewMediaModelRegistry(&mediaModelRepoStub{items: []MediaModelDefinition{definition}})
	require.NoError(t, registry.Refresh(context.Background()))
	adapters := NewMediaAdapterRegistry()
	adapters.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	scheduler := NewMediaScheduler(&mediaSchedulerAccountRepoStub{}, &mediaSchedulerSelectorStub{}, adapters,
		&mediaSchedulerGroupRepoStub{group: &Group{ID: 10, Platform: PlatformMedia}}, registry,
		&mediaSchedulerScopeRepoStub{modelIDs: []string{"other"}})

	_, err := scheduler.SnapshotCandidates(context.Background(), 10, "image")
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

func TestMediaSchedulerSnapshotRechecksCurrentGroupPlatformPolicy(t *testing.T) {
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{
		task12Account(1, PlatformOpenAI, "image", "openai-image", "sync", NativeAsyncUnsupported),
		task12Account(2, PlatformGemini, "image", "gemini-image", "sync", NativeAsyncUnsupported),
	}}
	group := &Group{ID: 7, Platform: PlatformOpenAI, MediaCrossPlatformEnabled: false}
	groups := &mediaSchedulerGroupRepoStub{group: group}
	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	scheduler := newTestMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry, groups)

	snapshot, err := scheduler.SnapshotCandidates(context.Background(), group.ID, "image")
	require.NoError(t, err)
	require.Equal(t, []int64{1}, mediaSchedulerSnapshotAccountIDs(snapshot))

	groups.group.MediaCrossPlatformEnabled = true
	snapshot, err = scheduler.SnapshotCandidates(context.Background(), group.ID, "image")
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, mediaSchedulerSnapshotAccountIDs(snapshot))
	require.Equal(t, int64(2), groups.calls.Load())
}

func mediaSchedulerSnapshotAccountIDs(snapshot []MediaAccountCandidateSnapshot) []int64 {
	ids := make([]int64, 0, len(snapshot))
	for _, candidate := range snapshot {
		ids = append(ids, candidate.AccountID)
	}
	return ids
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
	scheduler := newTestMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)

	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)
	require.Equal(t, []MediaAccountCandidateSnapshot{{
		AccountID: 1, Platform: PlatformOpenAI,
		ResolvedModel: ResolvedMediaAccountModel{Adapter: "sync", UpstreamModel: "image-up", NativeAsyncMode: NativeAsyncUnsupported},
	}}, snapshot)
}

func TestMediaSchedulerSnapshotReturnsStableNoAvailableError(t *testing.T) {
	scheduler := newTestMediaScheduler(&mediaSchedulerAccountRepoStub{}, &mediaSchedulerSelectorStub{}, NewMediaAdapterRegistry())
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
	scheduler := newTestMediaScheduler(repo, selector, registry)
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
	scheduler := newTestMediaScheduler(repo, selector, registry)
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)

	accounts[0].Schedulable = false
	accounts[1].TempUnschedulableUntil = &future
	repo.replaceAccounts(accounts)
	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
	require.NoError(t, err)
	require.Equal(t, int64(3), selection.Account.ID)
	require.Len(t, selector.candidates, 1)

	missingRegistryScheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())
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
		scheduler := newTestMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)
		duplicate := []MediaAccountCandidateSnapshot{
			{AccountID: 1, Platform: PlatformOpenAI, ResolvedModel: ResolvedMediaAccountModel{Adapter: "sync", UpstreamModel: "one", NativeAsyncMode: NativeAsyncUnsupported}},
			{AccountID: 1, Platform: PlatformOpenAI, ResolvedModel: ResolvedMediaAccountModel{Adapter: "sync", UpstreamModel: "two", NativeAsyncMode: NativeAsyncUnsupported}},
		}
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: duplicate})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("empty resolved model", func(t *testing.T) {
		scheduler := newTestMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: []MediaAccountCandidateSnapshot{{AccountID: 1, Platform: PlatformOpenAI}}})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})

	t.Run("selector outside candidate set", func(t *testing.T) {
		var releases atomic.Int64
		selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{
			Account: &Account{ID: 99}, Acquired: true, ReleaseFunc: func() { releases.Add(1) },
		}}
		scheduler := newTestMediaScheduler(repo, selector, registry)
		snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
		require.NoError(t, err)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Equal(t, int64(1), releases.Load())
	})

	t.Run("nil selector result", func(t *testing.T) {
		selector := &mediaSchedulerSelectorStub{returnNil: true}
		scheduler := newTestMediaScheduler(repo, selector, registry)
		snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
		require.NoError(t, err)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
	})
}

func TestMediaSchedulerReleasesSelectorResultReturnedWithError(t *testing.T) {
	selectorErr := errors.New("selector failed after acquire")
	var releases atomic.Int64
	account := task12Account(1, PlatformOpenAI, "image", "up", "sync", NativeAsyncUnsupported)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	selector := &mediaSchedulerSelectorStub{
		err: selectorErr,
		result: &AccountSelectionResult{
			Account: &Account{ID: 1}, Acquired: true, ReleaseFunc: func() { releases.Add(1) },
		},
	}
	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	scheduler := newTestMediaScheduler(repo, selector, registry)
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)

	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
	require.Nil(t, selection)
	require.ErrorIs(t, err, selectorErr)
	require.Equal(t, int64(1), releases.Load())
}

func TestMediaSchedulerRejectsStableAcquiredSelectionWithoutRefresh(t *testing.T) {
	var releases atomic.Int64
	account := task12Account(1, PlatformOpenAI, "image", "up", "sync", NativeAsyncUnsupported)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{
		Account: &Account{ID: account.ID}, Acquired: true,
		ReleaseFunc: func() { releases.Add(1) },
	}}
	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	scheduler := newTestMediaScheduler(repo, selector, registry)
	snapshot, err := scheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)

	selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{
		GroupID: 1, SlotID: "media-task:1", CandidateSnapshot: snapshot,
	})
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Equal(t, int64(1), releases.Load())
}

func TestMediaSchedulerRejectsMalformedSelectorWaitPlans(t *testing.T) {
	account := task12Account(1, PlatformOpenAI, "image", "up", "sync", NativeAsyncUnsupported)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	registry := NewMediaAdapterRegistry()
	registry.Register("sync", NewFakeMediaAdapter(FakeMediaAdapterOptions{Name: "sync", NativeAsyncMode: NativeAsyncUnsupported}))
	snapshotScheduler := newTestMediaScheduler(repo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)
	snapshot, err := snapshotScheduler.SnapshotCandidates(context.Background(), 1, "image")
	require.NoError(t, err)

	validWait := func() *AccountSelectionResult {
		return &AccountSelectionResult{
			Account:  &Account{ID: 1},
			WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 1},
		}
	}
	tests := []struct {
		name   string
		mutate func(*AccountSelectionResult)
	}{
		{name: "wrong account id", mutate: func(result *AccountSelectionResult) { result.WaitPlan.AccountID = 2 }},
		{name: "wrong concurrency", mutate: func(result *AccountSelectionResult) { result.WaitPlan.MaxConcurrency = 99 }},
		{name: "wrong stable slot id", mutate: func(result *AccountSelectionResult) { result.WaitPlan.SlotID = "media-task:other" }},
		{name: "zero timeout", mutate: func(result *AccountSelectionResult) { result.WaitPlan.Timeout = 0 }},
		{name: "zero max waiting", mutate: func(result *AccountSelectionResult) { result.WaitPlan.MaxWaiting = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validWait()
			tt.mutate(result)
			selector := &mediaSchedulerSelectorStub{result: result}
			scheduler := newTestMediaScheduler(repo, selector, registry)
			selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
			require.Nil(t, selection)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
		})
	}

	t.Run("waiting result with release is released exactly once", func(t *testing.T) {
		var releases atomic.Int64
		result := validWait()
		result.ReleaseFunc = func() { releases.Add(1) }
		selector := &mediaSchedulerSelectorStub{result: result}
		scheduler := newTestMediaScheduler(repo, selector, registry)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Equal(t, int64(1), releases.Load())
	})

	t.Run("missing account with release is released exactly once", func(t *testing.T) {
		var releases atomic.Int64
		selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{
			ReleaseFunc: func() { releases.Add(1) },
			WaitPlan:    &AccountWaitPlan{AccountID: 1, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 1},
		}}
		scheduler := newTestMediaScheduler(repo, selector, registry)
		selection, err := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: snapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Equal(t, int64(1), releases.Load())
	})

	t.Run("zero canonical concurrency cannot wait", func(t *testing.T) {
		zero := task12Account(1, PlatformOpenAI, "image", "up", "sync", NativeAsyncUnsupported)
		zero.Concurrency = 0
		zeroRepo := &mediaSchedulerAccountRepoStub{accounts: []Account{zero}}
		zeroSnapshotScheduler := newTestMediaScheduler(zeroRepo, &mediaSchedulerSelectorStub{selectedID: 1}, registry)
		zeroSnapshot, snapshotErr := zeroSnapshotScheduler.SnapshotCandidates(context.Background(), 1, "image")
		require.NoError(t, snapshotErr)
		selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{
			Account:  &Account{ID: 1},
			WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 0, Timeout: time.Second, MaxWaiting: 1},
		}}
		scheduler := newTestMediaScheduler(zeroRepo, selector, registry)
		selection, selectErr := scheduler.Select(context.Background(), MediaScheduleRequest{GroupID: 1, CandidateSnapshot: zeroSnapshot})
		require.Nil(t, selection)
		require.ErrorIs(t, selectErr, ErrNoAvailableAccounts)
	})
}

func TestMediaSchedulerWaitMarkUsedAndFixedAccountBoundaries(t *testing.T) {
	repoErr := errors.New("repository unavailable")
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{{ID: 7, Status: StatusDisabled, Schedulable: false}}}
	waitRelease := func() {}
	selector := &mediaSchedulerSelectorStub{waitFunc: waitRelease}
	scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

	release, err := scheduler.WaitForSlot(context.Background(), &MediaAccountSelection{
		Account:  &Account{ID: 7, Concurrency: 2},
		WaitPlan: &AccountWaitPlan{AccountID: 7, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 1},
	})
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

func TestMediaSchedulerDeletedFixedAccountIsUnavailableAndPreservesNotFound(t *testing.T) {
	accountID := int64(7)
	repo := &mediaSchedulerAccountRepoStub{
		getErr: fmt.Errorf("soft-deleted account: %w", ErrAccountNotFound),
	}
	selector := &mediaSchedulerSelectorStub{selectedID: accountID}
	scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

	selection, err := scheduler.SelectFixed(context.Background(), MediaFixedAccountRequest{AccountID: accountID})
	require.Nil(t, selection)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.Contains(t, err.Error(), "account 7")
	require.Zero(t, selector.requests)

	account, err := scheduler.GetFixedAccount(context.Background(), accountID)
	require.Nil(t, account)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.Contains(t, err.Error(), "account 7")
}

func TestMediaSchedulerSelectFixedUsesSingleRealtimeAccountAndStableKey(t *testing.T) {
	account := task12Account(7, PlatformOpenAI, "changed-model", "changed-upstream", "changed-adapter", NativeAsyncOptional)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	var releases atomic.Int64
	selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{
		Account: &Account{ID: account.ID}, Acquired: true, ReleaseFunc: func() { releases.Add(1) },
		RefreshFunc: func(context.Context) (bool, error) { return true, nil },
	}}
	scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

	selection, err := scheduler.SelectFixed(context.Background(), MediaFixedAccountRequest{
		AccountID: account.ID, GroupID: 3, SessionHash: "task_public_id", SlotID: "media-task:task_public_id",
	})
	require.NoError(t, err)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, account.Platform, selection.Account.Platform)
	require.True(t, selection.Acquired)
	require.NotNil(t, selection.ReleaseFunc)
	require.Equal(t, ResolvedMediaAccountModel{}, selection.ResolvedModel, "固定槽位入口不得重解析当前模型映射")
	require.Len(t, selector.candidates, 1)
	require.Equal(t, account.ID, selector.candidates[0].ID)
	require.Equal(t, int64(3), selector.lastGroupID)
	require.Equal(t, "task_public_id", selector.lastSessionHash)
	require.Equal(t, "media-task:task_public_id", selector.lastSlotID)
	selection.ReleaseFunc()
	selection.ReleaseFunc()
	require.Equal(t, int64(1), releases.Load())
}

func TestMediaSchedulerSelectFixedReturnsTrustedWaitPlan(t *testing.T) {
	account := task12Account(7, PlatformOpenAI, "image", "upstream", "adapter", NativeAsyncRequired)
	repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
	waitReleaseCalls := atomic.Int64{}
	selector := &mediaSchedulerSelectorStub{
		result: &AccountSelectionResult{
			Account: &Account{ID: account.ID},
			WaitPlan: &AccountWaitPlan{
				AccountID: account.ID, MaxConcurrency: account.Concurrency, Timeout: time.Second, MaxWaiting: 2,
				SlotID: "media-task:task_public_id",
			},
		},
		waitFunc: func() { waitReleaseCalls.Add(1) },
	}
	scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

	selection, err := scheduler.SelectFixed(context.Background(), MediaFixedAccountRequest{
		AccountID: account.ID, GroupID: 3, SessionHash: "task_public_id", SlotID: "media-task:task_public_id",
	})
	require.NoError(t, err)
	require.False(t, selection.Acquired)
	require.Equal(t, account.ID, selection.WaitPlan.AccountID)
	require.Equal(t, "media-task:task_public_id", selection.WaitPlan.SlotID)
	lease, err := scheduler.WaitForSlot(context.Background(), selection)
	require.NoError(t, err)
	require.NotNil(t, lease.ReleaseFunc)
	require.NotNil(t, lease.RefreshFunc)
	owned, refreshErr := lease.RefreshFunc(context.Background())
	require.NoError(t, refreshErr)
	require.True(t, owned)
	lease.ReleaseFunc()
	require.Equal(t, int64(1), waitReleaseCalls.Load())
	require.Equal(t, 1, selector.waitCalls)
}

func TestMediaSchedulerSelectFixedIgnoresRealtimeEligibilityAndRejectsSelectorDrift(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Account)
	}{
		{name: "disabled account", mutate: func(account *Account) { account.Status = StatusDisabled }},
		{name: "cooling account", mutate: func(account *Account) {
			until := time.Now().Add(time.Minute)
			account.TempUnschedulableUntil = &until
		}},
		{name: "rate limited account", mutate: func(account *Account) {
			until := time.Now().Add(time.Minute)
			account.RateLimitResetAt = &until
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			account := task12Account(7, PlatformOpenAI, "image", "upstream", "adapter", NativeAsyncRequired)
			tt.mutate(&account)
			repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
			account.Concurrency = 0
			selector := NewAccountCandidateSelector(nil, nil, task12SchedulingConfig())
			scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

			selection, err := scheduler.SelectFixed(context.Background(), MediaFixedAccountRequest{AccountID: account.ID})
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, account.ID, selection.Account.ID)
			require.True(t, selection.Acquired)
		})
	}

	for _, concurrency := range []int{0, -1} {
		t.Run(fmt.Sprintf("unlimited concurrency %d", concurrency), func(t *testing.T) {
			account := task12Account(7, PlatformOpenAI, "image", "upstream", "adapter", NativeAsyncRequired)
			account.Concurrency = concurrency
			repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
			selector := &mediaSchedulerSelectorStub{selectedID: account.ID}
			scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

			selection, err := scheduler.SelectFixed(context.Background(), MediaFixedAccountRequest{AccountID: account.ID})
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.True(t, selection.Acquired)
			require.Nil(t, selection.WaitPlan)
			require.Equal(t, 1, selector.requests)
		})
	}

	t.Run("selector returns different account", func(t *testing.T) {
		account := task12Account(7, PlatformOpenAI, "image", "upstream", "adapter", NativeAsyncRequired)
		repo := &mediaSchedulerAccountRepoStub{accounts: []Account{account}}
		var releases atomic.Int64
		selector := &mediaSchedulerSelectorStub{result: &AccountSelectionResult{
			Account: &Account{ID: 99}, Acquired: true, ReleaseFunc: func() { releases.Add(1) },
		}}
		scheduler := newTestMediaScheduler(repo, selector, NewMediaAdapterRegistry())

		selection, err := scheduler.SelectFixed(context.Background(), MediaFixedAccountRequest{AccountID: account.ID})
		require.Nil(t, selection)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
		require.Equal(t, int64(1), releases.Load())
	})
}

func TestMediaSchedulerWaitForSlotRejectsUntrustedSelections(t *testing.T) {
	tests := []struct {
		name      string
		selection *MediaAccountSelection
	}{
		{name: "acquired", selection: &MediaAccountSelection{Account: &Account{ID: 1, Concurrency: 2}, Acquired: true, WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 1}}},
		{name: "missing account", selection: &MediaAccountSelection{WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 1}}},
		{name: "mismatched account", selection: &MediaAccountSelection{Account: &Account{ID: 1, Concurrency: 2}, WaitPlan: &AccountWaitPlan{AccountID: 2, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 1}}},
		{name: "mismatched concurrency", selection: &MediaAccountSelection{Account: &Account{ID: 1, Concurrency: 2}, WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 1, Timeout: time.Second, MaxWaiting: 1}}},
		{name: "zero account concurrency", selection: &MediaAccountSelection{Account: &Account{ID: 1, Concurrency: 0}, WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 0, Timeout: time.Second, MaxWaiting: 1}}},
		{name: "zero timeout", selection: &MediaAccountSelection{Account: &Account{ID: 1, Concurrency: 2}, WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 2, Timeout: 0, MaxWaiting: 1}}},
		{name: "zero max waiting", selection: &MediaAccountSelection{Account: &Account{ID: 1, Concurrency: 2}, WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 2, Timeout: time.Second, MaxWaiting: 0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := &mediaSchedulerSelectorStub{waitFunc: func() {}}
			scheduler := newTestMediaScheduler(&mediaSchedulerAccountRepoStub{}, selector, NewMediaAdapterRegistry())
			release, err := scheduler.WaitForSlot(context.Background(), tt.selection)
			require.Nil(t, release)
			require.ErrorIs(t, err, ErrAccountConcurrencySaturated)
			require.Zero(t, selector.waitCalls)
		})
	}

	t.Run("invalid selection release is idempotent", func(t *testing.T) {
		var releases atomic.Int64
		selector := &mediaSchedulerSelectorStub{}
		scheduler := newTestMediaScheduler(&mediaSchedulerAccountRepoStub{}, selector, NewMediaAdapterRegistry())
		selection := &MediaAccountSelection{
			Account: &Account{ID: 1, Concurrency: 0}, ReleaseFunc: func() { releases.Add(1) },
			WaitPlan: &AccountWaitPlan{AccountID: 1, MaxConcurrency: 0, Timeout: time.Second, MaxWaiting: 1},
		}
		_, err := scheduler.WaitForSlot(context.Background(), selection)
		require.ErrorIs(t, err, ErrAccountConcurrencySaturated)
		require.Equal(t, int64(1), releases.Load())
		require.Zero(t, selector.waitCalls)
	})
}
