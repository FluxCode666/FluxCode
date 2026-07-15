package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type accountCandidateConcurrencyStub struct {
	mu sync.Mutex

	loads             map[int64]*AccountLoadInfo
	loadErr           error
	acquireResults    map[int64][]bool
	acquireErr        error
	acquireCalls      []int64
	releaseCalls      map[int64]int
	waitAllowed       bool
	waitErr           error
	incrementCalls    int
	decrementCalls    int
	accountWaitCounts map[int64]int
}

func (s *accountCandidateConcurrencyStub) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	result := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		if load := s.loads[account.ID]; load != nil {
			copy := *load
			result[account.ID] = &copy
		}
	}
	return result, nil
}

func (s *accountCandidateConcurrencyStub) AcquireAccountSlot(ctx context.Context, accountID int64, _ int) (*AcquireResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acquireCalls = append(s.acquireCalls, accountID)
	if s.acquireErr != nil {
		return nil, s.acquireErr
	}
	acquired := true
	if queue, exists := s.acquireResults[accountID]; exists {
		acquired = len(queue) > 0 && queue[0]
		if len(queue) > 0 {
			s.acquireResults[accountID] = queue[1:]
		}
	}
	if !acquired {
		return &AcquireResult{}, nil
	}
	return &AcquireResult{Acquired: true, ReleaseFunc: func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.releaseCalls == nil {
			s.releaseCalls = make(map[int64]int)
		}
		s.releaseCalls[accountID]++
	}}, nil
}

func (s *accountCandidateConcurrencyStub) IncrementAccountWaitCount(ctx context.Context, _ int64, _ int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incrementCalls++
	if s.waitErr != nil {
		return false, s.waitErr
	}
	return s.waitAllowed, nil
}

func (s *accountCandidateConcurrencyStub) DecrementAccountWaitCount(_ context.Context, _ int64) {
	s.mu.Lock()
	s.decrementCalls++
	s.mu.Unlock()
}

func (s *accountCandidateConcurrencyStub) GetAccountWaitingCount(_ context.Context, accountID int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accountWaitCounts[accountID], nil
}

func (s *accountCandidateConcurrencyStub) snapshot() (acquires []int64, releases map[int64]int, increments, decrements int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.acquireCalls...), cloneIntMap(s.releaseCalls), s.incrementCalls, s.decrementCalls
}

func cloneIntMap(input map[int64]int) map[int64]int {
	result := make(map[int64]int, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

type accountCandidateGatewayCacheStub struct {
	mu       sync.Mutex
	bindings map[string]int64
	deleted  map[string]int
	setTTL   map[string]time.Duration
}

func (s *accountCandidateGatewayCacheStub) GetSessionAccountID(_ context.Context, _ int64, sessionHash string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindings[sessionHash], nil
}

func (s *accountCandidateGatewayCacheStub) SetSessionAccountID(_ context.Context, _ int64, sessionHash string, accountID int64, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bindings == nil {
		s.bindings = make(map[string]int64)
	}
	if s.setTTL == nil {
		s.setTTL = make(map[string]time.Duration)
	}
	s.bindings[sessionHash] = accountID
	s.setTTL[sessionHash] = ttl
	return nil
}

func (s *accountCandidateGatewayCacheStub) RefreshSessionTTL(_ context.Context, _ int64, sessionHash string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setTTL == nil {
		s.setTTL = make(map[string]time.Duration)
	}
	s.setTTL[sessionHash] = ttl
	return nil
}

func (s *accountCandidateGatewayCacheStub) DeleteSessionAccountID(_ context.Context, _ int64, sessionHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bindings, sessionHash)
	if s.deleted == nil {
		s.deleted = make(map[string]int)
	}
	s.deleted[sessionHash]++
	return nil
}

func task12SchedulingConfig() config.GatewaySchedulingConfig {
	return config.GatewaySchedulingConfig{
		StickySessionMaxWaiting:  2,
		StickySessionWaitTimeout: 250 * time.Millisecond,
		FallbackWaitTimeout:      250 * time.Millisecond,
		FallbackMaxWaiting:       10,
		LoadBatchEnabled:         true,
	}
}

func TestAccountCandidateSelectorUsesPriorityThenLoadWaitingAndAcquiresSlot(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	concurrency := &accountCandidateConcurrencyStub{loads: map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 10, WaitingCount: 3},
		2: {AccountID: 2, LoadRate: 10, WaitingCount: 1},
		3: {AccountID: 3, LoadRate: 10, WaitingCount: 1},
	}}
	selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())
	input := []*Account{
		{ID: 1, Priority: 10, Concurrency: 2, LastUsedAt: &older},
		{ID: 2, Priority: 10, Concurrency: 2, LastUsedAt: &newer},
		{ID: 3, Priority: 10, Concurrency: 2, LastUsedAt: &older},
		{ID: 4, Priority: 20, Concurrency: 2},
	}

	result, err := selector.Select(context.Background(), AccountCandidateSelectionRequest{GroupID: 1, Candidates: input})
	require.NoError(t, err)
	require.Equal(t, int64(3), result.Account.ID)
	require.True(t, result.Acquired)
	require.NotNil(t, result.ReleaseFunc)
	require.Equal(t, []int64{1, 2, 3, 4}, []int64{input[0].ID, input[1].ID, input[2].ID, input[3].ID}, "不得修改调用方切片顺序")
}

func TestAccountCandidateSelectorStickyIsScopedAndInvalidBindingIsCleared(t *testing.T) {
	cache := &accountCandidateGatewayCacheStub{bindings: map[string]int64{"hit": 2, "outside": 99, "excluded": 1}}
	concurrency := &accountCandidateConcurrencyStub{loads: map[int64]*AccountLoadInfo{
		1: {AccountID: 1, LoadRate: 0},
		2: {AccountID: 2, LoadRate: 90},
	}}
	selector := NewAccountCandidateSelector(concurrency, cache, task12SchedulingConfig())
	candidates := []*Account{{ID: 1, Priority: 1, Concurrency: 1}, {ID: 2, Priority: 10, Concurrency: 1}}

	result, err := selector.Select(context.Background(), AccountCandidateSelectionRequest{GroupID: 7, SessionHash: "hit", Candidates: candidates})
	require.NoError(t, err)
	require.Equal(t, int64(2), result.Account.ID, "候选集内 sticky 必须先于优先级/负载")
	require.Equal(t, stickySessionTTL, cache.setTTL["hit"])

	result, err = selector.Select(context.Background(), AccountCandidateSelectionRequest{GroupID: 7, SessionHash: "outside", Candidates: candidates})
	require.NoError(t, err)
	require.Contains(t, []int64{1, 2}, result.Account.ID)
	require.Equal(t, 1, cache.deleted["outside"])

	result, err = selector.Select(context.Background(), AccountCandidateSelectionRequest{
		GroupID: 7, SessionHash: "excluded", Candidates: candidates, ExcludedAccountIDs: map[int64]struct{}{1: {}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), result.Account.ID)
	require.Equal(t, 1, cache.deleted["excluded"])
}

func TestAccountCandidateSelectorStickyFullReturnsStickyWaitPlan(t *testing.T) {
	cache := &accountCandidateGatewayCacheStub{bindings: map[string]int64{"sticky": 2}}
	concurrency := &accountCandidateConcurrencyStub{
		loads:             map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2, LoadRate: 100}},
		acquireResults:    map[int64][]bool{2: {false}},
		accountWaitCounts: map[int64]int{2: 0},
	}
	selector := NewAccountCandidateSelector(concurrency, cache, task12SchedulingConfig())

	result, err := selector.Select(context.Background(), AccountCandidateSelectionRequest{
		GroupID: 3, SessionHash: "sticky",
		Candidates: []*Account{{ID: 1, Priority: 1, Concurrency: 1}, {ID: 2, Priority: 10, Concurrency: 4}},
	})
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Equal(t, int64(2), result.Account.ID)
	require.Equal(t, &AccountWaitPlan{
		AccountID: 2, MaxConcurrency: 4, Timeout: 250 * time.Millisecond, MaxWaiting: 2,
	}, result.WaitPlan)
}

func TestAccountCandidateSelectorLoadFailureFallsBackToPriorityLRU(t *testing.T) {
	oldest := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-time.Hour)
	concurrency := &accountCandidateConcurrencyStub{loadErr: errors.New("redis unavailable")}
	selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())

	result, err := selector.Select(context.Background(), AccountCandidateSelectionRequest{Candidates: []*Account{
		{ID: 1, Priority: 5, Concurrency: 1, LastUsedAt: &newer},
		{ID: 2, Priority: 5, Concurrency: 1, LastUsedAt: &oldest},
		{ID: 3, Priority: 10, Concurrency: 1},
	}})
	require.NoError(t, err)
	require.Equal(t, int64(2), result.Account.ID)
}

func TestAccountCandidateSelectorAcquireRaceAndReleaseAreAtomicAndIdempotent(t *testing.T) {
	concurrency := &atomicAccountCandidateConcurrency{}
	selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())
	req := AccountCandidateSelectionRequest{Candidates: []*Account{{ID: 1, Priority: 1, Concurrency: 1}}}

	var wg sync.WaitGroup
	results := make(chan *AccountSelectionResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := selector.Select(context.Background(), req)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var acquired, waiting *AccountSelectionResult
	for result := range results {
		if result.Acquired {
			acquired = result
		} else {
			waiting = result
		}
	}
	require.NotNil(t, acquired)
	require.NotNil(t, waiting)
	require.NotNil(t, waiting.WaitPlan)

	var releaseWG sync.WaitGroup
	for range 8 {
		releaseWG.Add(1)
		go func() {
			defer releaseWG.Done()
			acquired.ReleaseFunc()
		}()
	}
	releaseWG.Wait()
	require.Equal(t, 1, concurrency.releaseCount())
}

type atomicAccountCandidateConcurrency struct {
	mu       sync.Mutex
	active   bool
	releases int
}

func (s *atomicAccountCandidateConcurrency) GetAccountsLoadBatch(context.Context, []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return map[int64]*AccountLoadInfo{1: {AccountID: 1}}, nil
}

func (s *atomicAccountCandidateConcurrency) AcquireAccountSlot(context.Context, int64, int) (*AcquireResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return &AcquireResult{}, nil
	}
	s.active = true
	return &AcquireResult{Acquired: true, ReleaseFunc: func() {
		s.mu.Lock()
		s.active = false
		s.releases++
		s.mu.Unlock()
	}}, nil
}

func (s *atomicAccountCandidateConcurrency) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	return true, nil
}

func (s *atomicAccountCandidateConcurrency) DecrementAccountWaitCount(context.Context, int64) {}

func (s *atomicAccountCandidateConcurrency) GetAccountWaitingCount(context.Context, int64) (int, error) {
	return 0, nil
}

func (s *atomicAccountCandidateConcurrency) releaseCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.releases
}

func TestAccountCandidateSelectorReturnsBestWaitPlanWhenAllSlotsAreFull(t *testing.T) {
	concurrency := &accountCandidateConcurrencyStub{
		loads: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, LoadRate: 40},
			2: {AccountID: 2, LoadRate: 10},
		},
		acquireResults: map[int64][]bool{1: {false}, 2: {false}},
	}
	selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())

	result, err := selector.Select(context.Background(), AccountCandidateSelectionRequest{Candidates: []*Account{
		{ID: 1, Priority: 1, Concurrency: 3},
		{ID: 2, Priority: 1, Concurrency: 7},
	}})
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Nil(t, result.ReleaseFunc)
	require.Equal(t, int64(2), result.Account.ID)
	require.Equal(t, &AccountWaitPlan{AccountID: 2, MaxConcurrency: 7, Timeout: 250 * time.Millisecond, MaxWaiting: 10}, result.WaitPlan)
}

func TestAccountCandidateSelectorWaitCleansUpAndReturnsIdempotentRelease(t *testing.T) {
	concurrency := &accountCandidateConcurrencyStub{
		waitAllowed:    true,
		acquireResults: map[int64][]bool{5: {false, true}},
	}
	selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())

	release, err := selector.Wait(context.Background(), &AccountWaitPlan{
		AccountID: 5, MaxConcurrency: 1, MaxWaiting: 2, Timeout: 500 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NotNil(t, release)
	_, _, increments, decrements := concurrency.snapshot()
	require.Equal(t, 1, increments)
	require.Equal(t, 1, decrements)

	release()
	release()
	_, releases, _, _ := concurrency.snapshot()
	require.Equal(t, 1, releases[5])
}

func TestAccountCandidateSelectorWaitTimeoutCancelAndQueueFull(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		concurrency := &accountCandidateConcurrencyStub{waitAllowed: true, acquireResults: map[int64][]bool{1: {false, false, false}}}
		selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())
		release, err := selector.Wait(context.Background(), &AccountWaitPlan{AccountID: 1, MaxConcurrency: 1, MaxWaiting: 1, Timeout: 30 * time.Millisecond})
		require.Nil(t, release)
		require.ErrorIs(t, err, ErrAccountConcurrencySaturated)
		_, _, _, decrements := concurrency.snapshot()
		require.Equal(t, 1, decrements)
	})

	t.Run("cancel", func(t *testing.T) {
		concurrency := &accountCandidateConcurrencyStub{waitAllowed: true, acquireResults: map[int64][]bool{1: {false, false, false}}}
		selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())
		ctx, cancel := context.WithCancel(context.Background())
		cancelTimer := time.AfterFunc(10*time.Millisecond, cancel)
		defer cancelTimer.Stop()
		release, err := selector.Wait(ctx, &AccountWaitPlan{AccountID: 1, MaxConcurrency: 1, MaxWaiting: 1, Timeout: time.Second})
		require.Nil(t, release)
		require.ErrorIs(t, err, context.Canceled)
		_, _, increments, decrements := concurrency.snapshot()
		require.Equal(t, 1, increments)
		require.Equal(t, 1, decrements)
	})

	t.Run("queue full", func(t *testing.T) {
		concurrency := &accountCandidateConcurrencyStub{waitAllowed: false}
		selector := NewAccountCandidateSelector(concurrency, nil, task12SchedulingConfig())
		release, err := selector.Wait(context.Background(), &AccountWaitPlan{AccountID: 1, MaxConcurrency: 1, MaxWaiting: 1, Timeout: time.Second})
		require.Nil(t, release)
		require.ErrorIs(t, err, ErrAccountConcurrencySaturated)
		_, _, _, decrements := concurrency.snapshot()
		require.Zero(t, decrements, "未进入队列时不得递减")
	})
}
