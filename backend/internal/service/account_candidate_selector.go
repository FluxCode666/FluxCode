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

var (
	ErrAccountConcurrencySaturated  = errors.New("account concurrency saturated")
	ErrStableAccountSlotUnsupported = errors.New("stable account concurrency slots are unsupported")
)

const accountCandidateWaitPollInterval = 100 * time.Millisecond

type AccountCandidateSelectionRequest struct {
	GroupID            int64
	SessionHash        string
	SlotID             string
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

type accountSlotIDAcquirer interface {
	AcquireAccountSlotWithID(ctx context.Context, accountID int64, maxConcurrency int, slotID string) (*AcquireResult, error)
}

type stableAccountCandidateWaiter interface {
	WaitStable(ctx context.Context, plan *AccountWaitPlan) (*AcquireResult, error)
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
		s.clearSticky(ctx, req.GroupID, req.SessionHash)
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
	var firstBusy *Account
	var acquireErrors []error
	for _, candidate := range ordered {
		result, err := s.acquire(ctx, candidate, req.SlotID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			acquireErrors = append(acquireErrors, err)
			continue
		}
		if !result.Acquired {
			if firstBusy == nil {
				firstBusy = candidate
			}
			continue
		}
		s.setSticky(ctx, req.GroupID, req.SessionHash, candidate.ID)
		return result, nil
	}

	if firstBusy == nil {
		if len(acquireErrors) > 0 {
			return nil, errors.Join(acquireErrors...)
		}
		return nil, ErrNoAvailableAccounts
	}
	return &AccountSelectionResult{
		Account: firstBusy,
		WaitPlan: &AccountWaitPlan{
			AccountID:      firstBusy.ID,
			MaxConcurrency: firstBusy.Concurrency,
			Timeout:        s.config.FallbackWaitTimeout,
			MaxWaiting:     s.config.FallbackMaxWaiting,
			SlotID:         req.SlotID,
		},
	}, nil
}

func candidateAccounts(input []*Account, excluded map[int64]struct{}) []*Account {
	result := make([]*Account, 0, len(input))
	seen := make(map[int64]struct{}, len(input))
	for _, candidate := range input {
		if candidate == nil || candidate.ID <= 0 || !candidate.IsSchedulable() {
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
	result, err := s.acquire(ctx, account, req.SlotID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, ctxErr
		}
		return nil, true, err
	}
	if result.Acquired {
		s.setSticky(ctx, req.GroupID, req.SessionHash, account.ID)
		return result, true, nil
	}
	if s.concurrency == nil {
		return nil, false, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, true, ctxErr
	}
	waiting, waitErr := s.concurrency.GetAccountWaitingCount(ctx, account.ID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, true, ctxErr
	}
	if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) {
		return nil, true, waitErr
	}
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
			SlotID:         req.SlotID,
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
		if candidate.Concurrency <= 0 {
			continue
		}
		loadRequest = append(loadRequest, AccountWithConcurrency{
			ID:             candidate.ID,
			MaxConcurrency: candidate.EffectiveLoadFactor(),
		})
	}
	if len(loadRequest) == 0 {
		sortAccountsByPriorityAndLastUsed(ordered, false)
		return ordered
	}
	if cap := s.config.LoadBatchQueryCap; cap > 0 && len(loadRequest) > cap {
		loadRequest = loadRequest[:cap]
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

func (s *accountCandidateSelector) acquire(ctx context.Context, account *Account, slotID string) (*AccountSelectionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if account.Concurrency <= 0 {
		result := &AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}
		if slotID != "" {
			result.RefreshFunc = func(context.Context) (bool, error) { return true, nil }
		}
		return result, nil
	}
	if s == nil || s.concurrency == nil {
		if slotID != "" {
			return nil, ErrStableAccountSlotUnsupported
		}
		result := &AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() {}}
		return result, nil
	}
	var (
		result *AcquireResult
		err    error
	)
	if slotID == "" {
		result, err = s.concurrency.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
	} else if stable, ok := s.concurrency.(accountSlotIDAcquirer); ok {
		result, err = stable.AcquireAccountSlotWithID(ctx, account.ID, account.Concurrency, slotID)
	} else {
		return nil, ErrStableAccountSlotUnsupported
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("acquire account %d concurrency slot: %w", account.ID, err)
	}
	if result == nil || !result.Acquired {
		return &AccountSelectionResult{Account: account}, nil
	}
	if result.ReleaseFunc == nil || (slotID != "" && result.RefreshFunc == nil) {
		if result.ReleaseFunc != nil {
			result.ReleaseFunc()
		}
		return nil, ErrStableAccountSlotUnsupported
	}
	return &AccountSelectionResult{
		Account:     account,
		Acquired:    true,
		ReleaseFunc: idempotentRelease(result.ReleaseFunc),
		RefreshFunc: result.RefreshFunc,
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
	lease, err := s.waitForSlot(ctx, plan, false)
	if err != nil {
		return nil, err
	}
	return lease.ReleaseFunc, nil
}

func (s *accountCandidateSelector) WaitStable(ctx context.Context, plan *AccountWaitPlan) (*AcquireResult, error) {
	return s.waitForSlot(ctx, plan, true)
}

func (s *accountCandidateSelector) waitForSlot(ctx context.Context, plan *AccountWaitPlan, stable bool) (*AcquireResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.concurrency == nil || plan == nil || plan.AccountID <= 0 || plan.MaxConcurrency <= 0 || plan.MaxWaiting <= 0 || plan.Timeout <= 0 {
		return nil, ErrAccountConcurrencySaturated
	}
	if stable != (plan.SlotID != "") {
		return nil, ErrStableAccountSlotUnsupported
	}
	planCtx, cancel := context.WithTimeout(ctx, plan.Timeout)
	defer cancel()

	allowed, err := s.concurrency.IncrementAccountWaitCount(planCtx, plan.AccountID, plan.MaxWaiting)
	if err != nil {
		if waitErr := accountWaitContextError(ctx, planCtx); waitErr != nil {
			return nil, waitErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("increment account %d wait count: %w", plan.AccountID, err)
	}
	if !allowed {
		return nil, ErrAccountConcurrencySaturated
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cleanupCancel()
		s.concurrency.DecrementAccountWaitCount(cleanupCtx, plan.AccountID)
	}()
	if waitErr := accountWaitContextError(ctx, planCtx); waitErr != nil {
		return nil, waitErr
	}

	tryAcquire := func() (*AcquireResult, bool, error) {
		var (
			result     *AcquireResult
			acquireErr error
		)
		if !stable {
			result, acquireErr = s.concurrency.AcquireAccountSlot(planCtx, plan.AccountID, plan.MaxConcurrency)
		} else if stable, ok := s.concurrency.(accountSlotIDAcquirer); ok {
			result, acquireErr = stable.AcquireAccountSlotWithID(planCtx, plan.AccountID, plan.MaxConcurrency, plan.SlotID)
		} else {
			return nil, false, ErrStableAccountSlotUnsupported
		}
		if acquireErr != nil {
			if waitErr := accountWaitContextError(ctx, planCtx); waitErr != nil {
				return nil, false, waitErr
			}
			if errors.Is(acquireErr, context.Canceled) || errors.Is(acquireErr, context.DeadlineExceeded) {
				return nil, false, acquireErr
			}
			return nil, false, fmt.Errorf("acquire waiting account %d concurrency slot: %w", plan.AccountID, acquireErr)
		}
		if result == nil || !result.Acquired {
			return nil, false, nil
		}
		if result.ReleaseFunc == nil || (stable && result.RefreshFunc == nil) {
			if result.ReleaseFunc != nil {
				result.ReleaseFunc()
			}
			return nil, false, ErrStableAccountSlotUnsupported
		}
		return &AcquireResult{
			Acquired:    true,
			ReleaseFunc: idempotentRelease(result.ReleaseFunc),
			RefreshFunc: result.RefreshFunc,
		}, true, nil
	}

	if lease, acquired, acquireErr := tryAcquire(); acquired || acquireErr != nil {
		return lease, acquireErr
	}

	ticker := time.NewTicker(accountCandidateWaitPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-planCtx.Done():
			return nil, accountWaitContextError(ctx, planCtx)
		case <-ticker.C:
			lease, acquired, acquireErr := tryAcquire()
			if acquireErr != nil {
				return nil, acquireErr
			}
			if acquired {
				return lease, nil
			}
		}
	}
}

func accountWaitContextError(parentCtx, planCtx context.Context) error {
	if err := parentCtx.Err(); err != nil {
		return err
	}
	if err := planCtx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrAccountConcurrencySaturated
		}
		return err
	}
	return nil
}
