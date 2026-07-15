package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var ErrAccountConcurrencySaturated = errors.New("account concurrency saturated")

const accountCandidateWaitPollInterval = 100 * time.Millisecond

type AccountCandidateSelectionRequest struct {
	GroupID            int64
	SessionHash        string
	Candidates         []*Account
	ExcludedAccountIDs map[int64]struct{}
}

type AccountCandidateSelector interface {
	Select(ctx context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error)
	Wait(ctx context.Context, plan *AccountWaitPlan) (release func(), err error)
}

// AccountCandidateConcurrency contains only the existing concurrency primitives
// needed to schedule within a caller-owned candidate set.
type AccountCandidateConcurrency interface {
	GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
	AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error)
	IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error)
	DecrementAccountWaitCount(ctx context.Context, accountID int64)
	GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error)
}

type accountCandidateSelector struct {
	concurrency AccountCandidateConcurrency
	cache       GatewayCache
	config      config.GatewaySchedulingConfig
}

func NewAccountCandidateSelector(concurrency AccountCandidateConcurrency, cache GatewayCache, cfg config.GatewaySchedulingConfig) AccountCandidateSelector {
	return &accountCandidateSelector{
		concurrency: concurrency,
		cache:       cache,
		config:      normalizeAccountCandidateSchedulingConfig(cfg),
	}
}

func normalizeAccountCandidateSchedulingConfig(cfg config.GatewaySchedulingConfig) config.GatewaySchedulingConfig {
	if cfg.StickySessionMaxWaiting <= 0 {
		cfg.StickySessionMaxWaiting = 3
	}
	if cfg.StickySessionWaitTimeout <= 0 {
		cfg.StickySessionWaitTimeout = 45 * time.Second
	}
	if cfg.FallbackMaxWaiting <= 0 {
		cfg.FallbackMaxWaiting = 100
	}
	if cfg.FallbackWaitTimeout <= 0 {
		cfg.FallbackWaitTimeout = 30 * time.Second
	}
	return cfg
}

func (s *accountCandidateSelector) Select(ctx context.Context, req AccountCandidateSelectionRequest) (*AccountSelectionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	candidates := candidateAccounts(req.Candidates, req.ExcludedAccountIDs)
	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	stickyID := s.stickyAccountID(ctx, req.GroupID, req.SessionHash)
	if stickyID > 0 {
		sticky := findCandidateAccount(candidates, stickyID)
		if sticky == nil {
			s.clearSticky(ctx, req.GroupID, req.SessionHash)
		} else if result, decided, err := s.trySticky(ctx, req, sticky); decided || err != nil {
			return result, err
		}
	}

	ordered := s.orderCandidates(ctx, candidates)
	for _, candidate := range ordered {
		result, err := s.acquire(ctx, candidate)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			continue
		}
		if !result.Acquired {
			continue
		}
		s.setSticky(ctx, req.GroupID, req.SessionHash, candidate.ID)
		return result, nil
	}

	best := ordered[0]
	return &AccountSelectionResult{
		Account: best,
		WaitPlan: &AccountWaitPlan{
			AccountID:      best.ID,
			MaxConcurrency: best.Concurrency,
			Timeout:        s.config.FallbackWaitTimeout,
			MaxWaiting:     s.config.FallbackMaxWaiting,
		},
	}, nil
}

func candidateAccounts(input []*Account, excluded map[int64]struct{}) []*Account {
	result := make([]*Account, 0, len(input))
	seen := make(map[int64]struct{}, len(input))
	for _, candidate := range input {
		if candidate == nil || candidate.ID <= 0 {
			continue
		}
		if _, skip := excluded[candidate.ID]; skip {
			continue
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			continue
		}
		seen[candidate.ID] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func findCandidateAccount(candidates []*Account, accountID int64) *Account {
	for _, candidate := range candidates {
		if candidate.ID == accountID {
			return candidate
		}
	}
	return nil
}

func (s *accountCandidateSelector) stickyAccountID(ctx context.Context, groupID int64, sessionHash string) int64 {
	if s == nil || s.cache == nil || sessionHash == "" {
		return 0
	}
	accountID, err := s.cache.GetSessionAccountID(ctx, groupID, sessionHash)
	if err != nil {
		return 0
	}
	return accountID
}

func (s *accountCandidateSelector) clearSticky(ctx context.Context, groupID int64, sessionHash string) {
	if s == nil || s.cache == nil || sessionHash == "" {
		return
	}
	_ = s.cache.DeleteSessionAccountID(ctx, groupID, sessionHash)
}

func (s *accountCandidateSelector) setSticky(ctx context.Context, groupID int64, sessionHash string, accountID int64) {
	if s == nil || s.cache == nil || sessionHash == "" {
		return
	}
	_ = s.cache.SetSessionAccountID(ctx, groupID, sessionHash, accountID, stickySessionTTL)
}

func (s *accountCandidateSelector) trySticky(ctx context.Context, req AccountCandidateSelectionRequest, account *Account) (*AccountSelectionResult, bool, error) {
	result, err := s.acquire(ctx, account)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, ctxErr
		}
		return nil, false, nil
	}
	if result.Acquired {
		s.setSticky(ctx, req.GroupID, req.SessionHash, account.ID)
		return result, true, nil
	}
	if s.concurrency == nil {
		return nil, false, nil
	}
	waiting, waitErr := s.concurrency.GetAccountWaitingCount(ctx, account.ID)
	if waitErr == nil && waiting >= s.config.StickySessionMaxWaiting {
		return nil, false, nil
	}
	return &AccountSelectionResult{
		Account: account,
		WaitPlan: &AccountWaitPlan{
			AccountID:      account.ID,
			MaxConcurrency: account.Concurrency,
			Timeout:        s.config.StickySessionWaitTimeout,
			MaxWaiting:     s.config.StickySessionMaxWaiting,
		},
	}, true, nil
}

func (s *accountCandidateSelector) orderCandidates(ctx context.Context, candidates []*Account) []*Account {
	ordered := append([]*Account(nil), candidates...)
	if s == nil || s.concurrency == nil || !s.config.LoadBatchEnabled {
		sortAccountsByPriorityAndLastUsed(ordered, false)
		return ordered
	}

	loadRequest := make([]AccountWithConcurrency, 0, len(ordered))
	for _, candidate := range ordered {
		loadRequest = append(loadRequest, AccountWithConcurrency{
			ID:             candidate.ID,
			MaxConcurrency: candidate.EffectiveLoadFactor(),
		})
	}
	loads, err := s.concurrency.GetAccountsLoadBatch(ctx, loadRequest)
	if err != nil {
		sortAccountsByPriorityAndLastUsed(ordered, false)
		return ordered
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		leftLoad := accountCandidateLoad(loads, left.ID)
		rightLoad := accountCandidateLoad(loads, right.ID)
		if leftLoad.LoadRate != rightLoad.LoadRate {
			return leftLoad.LoadRate < rightLoad.LoadRate
		}
		if leftLoad.WaitingCount != rightLoad.WaitingCount {
			return leftLoad.WaitingCount < rightLoad.WaitingCount
		}
		switch {
		case left.LastUsedAt == nil && right.LastUsedAt != nil:
			return true
		case left.LastUsedAt != nil && right.LastUsedAt == nil:
			return false
		case left.LastUsedAt != nil && right.LastUsedAt != nil && !left.LastUsedAt.Equal(*right.LastUsedAt):
			return left.LastUsedAt.Before(*right.LastUsedAt)
		default:
			return left.ID < right.ID
		}
	})
	return ordered
}

func accountCandidateLoad(loads map[int64]*AccountLoadInfo, accountID int64) AccountLoadInfo {
	if load := loads[accountID]; load != nil {
		return *load
	}
	return AccountLoadInfo{AccountID: accountID}
}

func (s *accountCandidateSelector) acquire(ctx context.Context, account *Account) (*AccountSelectionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.concurrency == nil {
		return &AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	result, err := s.concurrency.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("acquire account %d concurrency slot: %w", account.ID, err)
	}
	if result == nil || !result.Acquired {
		return &AccountSelectionResult{Account: account}, nil
	}
	return &AccountSelectionResult{
		Account:     account,
		Acquired:    true,
		ReleaseFunc: idempotentRelease(result.ReleaseFunc),
	}, nil
}

func idempotentRelease(release func()) func() {
	if release == nil {
		return func() {}
	}
	var released atomic.Bool
	return func() {
		if released.CompareAndSwap(false, true) {
			release()
		}
	}
}

func (s *accountCandidateSelector) Wait(ctx context.Context, plan *AccountWaitPlan) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.concurrency == nil || plan == nil || plan.AccountID <= 0 || plan.Timeout <= 0 {
		return nil, ErrAccountConcurrencySaturated
	}

	allowed, err := s.concurrency.IncrementAccountWaitCount(ctx, plan.AccountID, plan.MaxWaiting)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("increment account %d wait count: %w", plan.AccountID, err)
	}
	if !allowed {
		return nil, ErrAccountConcurrencySaturated
	}
	defer s.concurrency.DecrementAccountWaitCount(ctx, plan.AccountID)

	tryAcquire := func() (func(), bool, error) {
		result, acquireErr := s.concurrency.AcquireAccountSlot(ctx, plan.AccountID, plan.MaxConcurrency)
		if acquireErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, false, ctxErr
			}
			return nil, false, fmt.Errorf("acquire waiting account %d concurrency slot: %w", plan.AccountID, acquireErr)
		}
		if result == nil || !result.Acquired {
			return nil, false, nil
		}
		return idempotentRelease(result.ReleaseFunc), true, nil
	}

	if release, acquired, acquireErr := tryAcquire(); acquired || acquireErr != nil {
		return release, acquireErr
	}

	timeout := time.NewTimer(plan.Timeout)
	defer timeout.Stop()
	ticker := time.NewTicker(accountCandidateWaitPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, ErrAccountConcurrencySaturated
		case <-ticker.C:
			release, acquired, acquireErr := tryAcquire()
			if acquireErr != nil {
				return nil, acquireErr
			}
			if acquired {
				return release, nil
			}
		}
	}
}
