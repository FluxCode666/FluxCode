package service

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
)

type MediaScheduleRequest struct {
	GroupID            int64
	RequestedModel     string
	Operation          MediaOperation
	SessionHash        string
	ExcludedAccountIDs map[int64]struct{}
	CandidateSnapshot  []MediaAccountCandidateSnapshot
}

type MediaAccountCandidateSnapshot struct {
	AccountID     int64                     `json:"account_id"`
	Platform      string                    `json:"platform"`
	ResolvedModel ResolvedMediaAccountModel `json:"resolved_model"`
}

type MediaAccountSelection struct {
	Account       *Account
	ResolvedModel ResolvedMediaAccountModel
	Acquired      bool
	ReleaseFunc   func()
	WaitPlan      *AccountWaitPlan
}

type MediaSchedulerAccountRepository interface {
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error)
	GetByID(ctx context.Context, id int64) (*Account, error)
	UpdateLastUsed(ctx context.Context, id int64) error
}

type MediaScheduler struct {
	accountRepo MediaSchedulerAccountRepository
	selector    AccountCandidateSelector
	adapters    *MediaAdapterRegistry
}

func NewMediaScheduler(accountRepo MediaSchedulerAccountRepository, selector AccountCandidateSelector, adapters *MediaAdapterRegistry) *MediaScheduler {
	return &MediaScheduler{accountRepo: accountRepo, selector: selector, adapters: adapters}
}

func (s *MediaScheduler) SnapshotCandidates(ctx context.Context, groupID int64, requestedModel string) ([]MediaAccountCandidateSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if s == nil || s.accountRepo == nil || s.adapters == nil || requestedModel == "" {
		return nil, ErrNoAvailableAccounts
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list media candidate accounts: %w", err)
	}

	result := make([]MediaAccountCandidateSnapshot, 0, len(accounts))
	seen := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.ID <= 0 || strings.TrimSpace(account.Platform) == "" {
			continue
		}
		if _, duplicate := seen[account.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate media candidate account id %d", ErrNoAvailableAccounts, account.ID)
		}
		seen[account.ID] = struct{}{}
		if !account.IsSchedulable() || !account.IsModelSupported(requestedModel) {
			continue
		}
		resolved := account.ResolveMediaModel(requestedModel)
		if !validResolvedMediaAccountModel(resolved) {
			continue
		}
		adapter, adapterErr := s.adapters.Resolve(resolved.Adapter)
		if adapterErr != nil || !adapterSupportsNativeMode(adapter, resolved.NativeAsyncMode) {
			continue
		}
		result = append(result, MediaAccountCandidateSnapshot{
			AccountID: account.ID,
			Platform:  account.Platform,
			ResolvedModel: ResolvedMediaAccountModel{
				Adapter:         resolved.Adapter,
				UpstreamModel:   resolved.UpstreamModel,
				NativeAsyncMode: resolved.NativeAsyncMode,
			},
		})
	}
	if len(result) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	return result, nil
}

func validResolvedMediaAccountModel(model ResolvedMediaAccountModel) bool {
	if model.Adapter == "" || model.UpstreamModel == "" {
		return false
	}
	switch model.NativeAsyncMode {
	case NativeAsyncUnsupported, NativeAsyncRequired, NativeAsyncOptional:
		return true
	default:
		return false
	}
}

func adapterSupportsNativeMode(adapter MediaAdapter, mode NativeAsyncMode) bool {
	if isNilMediaAdapter(adapter) {
		return false
	}
	_, syncSupported := adapter.(MediaSyncGenerator)
	_, submitSupported := adapter.(MediaAsyncSubmitter)
	_, pollSupported := adapter.(MediaAsyncPoller)
	switch mode {
	case NativeAsyncUnsupported:
		return syncSupported
	case NativeAsyncRequired:
		return submitSupported && pollSupported
	case NativeAsyncOptional:
		return syncSupported && submitSupported && pollSupported
	default:
		return false
	}
}

func (s *MediaScheduler) Select(ctx context.Context, req MediaScheduleRequest) (*MediaAccountSelection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.accountRepo == nil || s.selector == nil || s.adapters == nil {
		return nil, ErrNoAvailableAccounts
	}
	snapshots, err := validateMediaCandidateSnapshot(req.CandidateSnapshot)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, req.GroupID)
	if err != nil {
		return nil, fmt.Errorf("list current media candidate accounts: %w", err)
	}

	candidates := make([]*Account, 0, len(accounts))
	allowed := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		snapshot, snapshotted := snapshots[account.ID]
		if !snapshotted {
			continue
		}
		if _, duplicate := allowed[account.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate current media account id %d", ErrNoAvailableAccounts, account.ID)
		}
		if _, excluded := req.ExcludedAccountIDs[account.ID]; excluded || !account.IsSchedulable() {
			continue
		}
		if snapshot.Platform == "" || account.Platform != snapshot.Platform {
			continue
		}
		adapter, adapterErr := s.adapters.Resolve(snapshot.ResolvedModel.Adapter)
		if adapterErr != nil || !adapterSupportsNativeMode(adapter, snapshot.ResolvedModel.NativeAsyncMode) {
			continue
		}
		copy := *account
		candidates = append(candidates, &copy)
		allowed[account.ID] = &copy
	}
	if len(candidates) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	selected, err := s.selector.Select(ctx, AccountCandidateSelectionRequest{
		GroupID:            req.GroupID,
		SessionHash:        req.SessionHash,
		Candidates:         candidates,
		ExcludedAccountIDs: cloneAccountIDSet(req.ExcludedAccountIDs),
	})
	if err != nil {
		return nil, err
	}
	if selected == nil || selected.Account == nil {
		return nil, fmt.Errorf("%w: account candidate selector returned no account", ErrNoAvailableAccounts)
	}
	canonical, exists := allowed[selected.Account.ID]
	if !exists {
		if selected.ReleaseFunc != nil {
			selected.ReleaseFunc()
		}
		return nil, fmt.Errorf("%w: selector returned account %d outside candidate snapshot", ErrNoAvailableAccounts, selected.Account.ID)
	}
	if selected.Acquired {
		if selected.WaitPlan != nil || selected.ReleaseFunc == nil {
			if selected.ReleaseFunc != nil {
				selected.ReleaseFunc()
			}
			return nil, fmt.Errorf("%w: inconsistent acquired media account selection", ErrNoAvailableAccounts)
		}
	} else if selected.ReleaseFunc != nil || selected.WaitPlan == nil || selected.WaitPlan.AccountID != canonical.ID {
		if selected.ReleaseFunc != nil {
			selected.ReleaseFunc()
		}
		return nil, fmt.Errorf("%w: inconsistent waiting media account selection", ErrNoAvailableAccounts)
	}

	return &MediaAccountSelection{
		Account:       canonical,
		ResolvedModel: snapshots[canonical.ID].ResolvedModel,
		Acquired:      selected.Acquired,
		ReleaseFunc:   wrapOptionalIdempotentRelease(selected.ReleaseFunc),
		WaitPlan:      cloneAccountWaitPlan(selected.WaitPlan),
	}, nil
}

func validateMediaCandidateSnapshot(input []MediaAccountCandidateSnapshot) (map[int64]MediaAccountCandidateSnapshot, error) {
	if len(input) == 0 {
		return nil, ErrNoAvailableAccounts
	}
	result := make(map[int64]MediaAccountCandidateSnapshot, len(input))
	for _, candidate := range input {
		if candidate.AccountID <= 0 || candidate.Platform == "" || !validResolvedMediaAccountModel(candidate.ResolvedModel) {
			return nil, fmt.Errorf("%w: invalid media candidate snapshot", ErrNoAvailableAccounts)
		}
		if _, duplicate := result[candidate.AccountID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate media candidate account id %d", ErrNoAvailableAccounts, candidate.AccountID)
		}
		result[candidate.AccountID] = candidate
	}
	return result, nil
}

func cloneAccountIDSet(input map[int64]struct{}) map[int64]struct{} {
	if input == nil {
		return nil
	}
	result := make(map[int64]struct{}, len(input))
	for accountID := range input {
		result[accountID] = struct{}{}
	}
	return result
}

func cloneAccountWaitPlan(plan *AccountWaitPlan) *AccountWaitPlan {
	if plan == nil {
		return nil
	}
	copy := *plan
	return &copy
}

func wrapOptionalIdempotentRelease(release func()) func() {
	if release == nil {
		return nil
	}
	var released atomic.Bool
	return func() {
		if released.CompareAndSwap(false, true) {
			release()
		}
	}
}

func (s *MediaScheduler) WaitForSlot(ctx context.Context, selection *MediaAccountSelection) (func(), error) {
	if s == nil || s.selector == nil || selection == nil {
		return nil, ErrAccountConcurrencySaturated
	}
	return s.selector.Wait(ctx, cloneAccountWaitPlan(selection.WaitPlan))
}

func (s *MediaScheduler) MarkUsed(ctx context.Context, accountID int64) error {
	if s == nil || s.accountRepo == nil {
		return fmt.Errorf("mark media account %d used: %w", accountID, ErrNoAvailableAccounts)
	}
	if err := s.accountRepo.UpdateLastUsed(ctx, accountID); err != nil {
		return fmt.Errorf("mark media account %d used: %w", accountID, err)
	}
	return nil
}

func (s *MediaScheduler) GetFixedAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("get fixed media account %d: %w", accountID, ErrNoAvailableAccounts)
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get fixed media account %d: %w", accountID, err)
	}
	if account == nil {
		return nil, fmt.Errorf("get fixed media account %d: %w", accountID, ErrNoAvailableAccounts)
	}
	return account, nil
}
