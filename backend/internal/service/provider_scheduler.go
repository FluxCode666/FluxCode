package service

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
)

type ProviderConcurrency interface {
	GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error)
	AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*AcquireResult, error)
}

type ProviderScheduler struct {
	concurrency ProviderConcurrency
}

func NewProviderScheduler(concurrency ProviderConcurrency) *ProviderScheduler {
	return &ProviderScheduler{concurrency: concurrency}
}

type ProviderScheduleRequest struct {
	Tier              RouteTier
	Candidates        []RouteCandidate
	StickyRoute       *RouteIdentity
	ExcludedRoutes    map[RouteIdentity]struct{}
	ExcludedProviders map[int64]struct{}
}

type ProviderRouteSelection struct {
	Candidate   RouteCandidate
	Acquired    bool
	ReleaseFunc func()
}

func (s *ProviderScheduler) Select(ctx context.Context, request ProviderScheduleRequest) (*ProviderRouteSelection, error) {
	if s == nil {
		return nil, errors.New("provider scheduler is not initialized")
	}
	if !request.Tier.IsValid() {
		return nil, errors.New("invalid provider route tier")
	}
	candidates := FilterRouteCandidates(request.Candidates, request.ExcludedRoutes, request.ExcludedProviders)
	eligible := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Tier != request.Tier || providerRouteCapabilityIneligible(ProviderRouteCapability{
			Profile: candidate.Profile, Account: candidate.Account, Endpoint: candidate.Endpoint,
			LogicalModel: candidate.LogicalModel, Capability: candidate.Capability,
		}) != "" {
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) == 0 {
		return nil, ErrNoProviderRoute
	}

	var stickyTried *RouteIdentity
	if request.StickyRoute != nil {
		for _, candidate := range eligible {
			if candidate.Identity == *request.StickyRoute {
				selection, err := s.acquire(ctx, candidate)
				if err != nil {
					return nil, err
				}
				if selection != nil && selection.Acquired {
					return selection, nil
				}
				identity := candidate.Identity
				stickyTried = &identity
				break
			}
		}
	}

	loads := map[int64]*AccountLoadInfo{}
	if s.concurrency != nil {
		loadRequest := make([]AccountWithConcurrency, 0, len(eligible))
		seen := make(map[int64]struct{}, len(eligible))
		for _, candidate := range eligible {
			if _, ok := seen[candidate.Account.ID]; ok {
				continue
			}
			seen[candidate.Account.ID] = struct{}{}
			loadRequest = append(loadRequest, AccountWithConcurrency{
				ID: candidate.Account.ID, MaxConcurrency: candidate.Account.EffectiveLoadFactor(),
			})
		}
		loaded, err := s.concurrency.GetAccountsLoadBatch(ctx, loadRequest)
		if err != nil {
			return nil, err
		}
		if loaded != nil {
			loads = loaded
		}
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		return providerRouteCandidateLess(eligible[i], eligible[j], loads)
	})
	for _, candidate := range eligible {
		if stickyTried != nil && candidate.Identity == *stickyTried {
			continue
		}
		selection, err := s.acquire(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if selection != nil && selection.Acquired {
			return selection, nil
		}
	}
	return nil, ErrNoProviderRoute
}

func (s *ProviderScheduler) acquire(ctx context.Context, candidate RouteCandidate) (*ProviderRouteSelection, error) {
	if s.concurrency == nil {
		return &ProviderRouteSelection{Candidate: candidate, Acquired: true, ReleaseFunc: func() {}}, nil
	}
	result, err := s.concurrency.AcquireAccountSlot(ctx, candidate.Account.ID, candidate.Account.Concurrency)
	if err != nil || result == nil || !result.Acquired {
		return nil, err
	}
	return &ProviderRouteSelection{
		Candidate: candidate, Acquired: true, ReleaseFunc: result.ReleaseFunc,
	}, nil
}

func providerRouteCandidateLess(left, right RouteCandidate, loads map[int64]*AccountLoadInfo) bool {
	if left.GroupPriority != right.GroupPriority {
		return left.GroupPriority < right.GroupPriority
	}
	if left.Account.Priority != right.Account.Priority {
		return left.Account.Priority < right.Account.Priority
	}
	leftLoad, rightLoad := providerLoadInfo(loads, left.Account.ID), providerLoadInfo(loads, right.Account.ID)
	if leftLoad.LoadRate != rightLoad.LoadRate {
		return leftLoad.LoadRate < rightLoad.LoadRate
	}
	if leftLoad.WaitingCount != rightLoad.WaitingCount {
		return leftLoad.WaitingCount < rightLoad.WaitingCount
	}
	if left.Account.LastUsedAt == nil && right.Account.LastUsedAt != nil {
		return true
	}
	if left.Account.LastUsedAt != nil && right.Account.LastUsedAt == nil {
		return false
	}
	if left.Account.LastUsedAt != nil && right.Account.LastUsedAt != nil && !left.Account.LastUsedAt.Equal(*right.Account.LastUsedAt) {
		return left.Account.LastUsedAt.Before(*right.Account.LastUsedAt)
	}
	return left.Identity.String() < right.Identity.String()
}

func providerLoadInfo(loads map[int64]*AccountLoadInfo, accountID int64) AccountLoadInfo {
	if info := loads[accountID]; info != nil {
		return *info
	}
	return AccountLoadInfo{AccountID: accountID}
}

type RouteSwitchBudget struct {
	max  int64
	used atomic.Int64
}

func NewRouteSwitchBudget(max int) *RouteSwitchBudget {
	if max < 0 {
		max = 0
	}
	return &RouteSwitchBudget{max: int64(max)}
}

func (b *RouteSwitchBudget) TrySwitch(_ RouteTier) bool {
	if b == nil {
		return false
	}
	for {
		used := b.used.Load()
		if used >= b.max {
			return false
		}
		if b.used.CompareAndSwap(used, used+1) {
			return true
		}
	}
}

func (b *RouteSwitchBudget) Used() int {
	if b == nil {
		return 0
	}
	return int(b.used.Load())
}
