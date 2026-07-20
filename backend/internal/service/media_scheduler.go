package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
)

type MediaScheduleRequest struct {
	GroupID            int64
	RequestedModel     string
	Operation          MediaOperation
	SessionHash        string
	SlotID             string
	ExcludedAccountIDs map[int64]struct{}
	CandidateSnapshot  []MediaAccountCandidateSnapshot
}

type MediaFixedAccountRequest struct {
	AccountID   int64
	GroupID     int64
	SessionHash string
	SlotID      string
}

type MediaAccountCandidateSnapshot struct {
	AccountID       int64                     `json:"account_id"`
	Platform        string                    `json:"platform"`
	ResolvedModel   ResolvedMediaAccountModel `json:"resolved_model"`
	ResolvedRequest json.RawMessage           `json:"resolved_request,omitempty"`
}

type MediaAccountSelection struct {
	Account         *Account
	ResolvedModel   ResolvedMediaAccountModel
	ResolvedRequest json.RawMessage
	Acquired        bool
	ReleaseFunc     func()
	RefreshFunc     func(context.Context) (bool, error)
	WaitPlan        *AccountWaitPlan
}

type MediaSchedulerAccountRepository interface {
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error)
	GetByID(ctx context.Context, id int64) (*Account, error)
	UpdateLastUsed(ctx context.Context, id int64) error
}

type MediaSchedulerGroupRepository interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
}

type mediaSchedulerModelScopeRepository interface {
	ListEnabledMediaModelIDs(ctx context.Context, groupID int64) ([]string, error)
}

type MediaScheduler struct {
	accountRepo MediaSchedulerAccountRepository
	groupRepo   MediaSchedulerGroupRepository
	selector    AccountCandidateSelector
	adapters    *MediaAdapterRegistry
	registry    *MediaModelRegistry
	scopes      mediaSchedulerModelScopeRepository
}

func NewMediaScheduler(accountRepo MediaSchedulerAccountRepository, selector AccountCandidateSelector, adapters *MediaAdapterRegistry, groupRepo MediaSchedulerGroupRepository, dependencies ...any) *MediaScheduler {
	scheduler := &MediaScheduler{accountRepo: accountRepo, groupRepo: groupRepo, selector: selector, adapters: adapters}
	for _, dependency := range dependencies {
		switch value := dependency.(type) {
		case *MediaModelRegistry:
			scheduler.registry = value
		case mediaSchedulerModelScopeRepository:
			scheduler.scopes = value
		}
	}
	return scheduler
}

func (s *MediaScheduler) SnapshotCandidates(ctx context.Context, groupID int64, requestedModel string) ([]MediaAccountCandidateSnapshot, error) {
	return s.snapshotCandidates(ctx, groupID, requestedModel, "")
}

func (s *MediaScheduler) SnapshotCandidatesForOperation(ctx context.Context, groupID int64, requestedModel string, operation MediaOperation) ([]MediaAccountCandidateSnapshot, error) {
	return s.snapshotCandidates(ctx, groupID, requestedModel, operation)
}

func (s *MediaScheduler) snapshotCandidates(ctx context.Context, groupID int64, requestedModel string, operation MediaOperation) ([]MediaAccountCandidateSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if s == nil || s.accountRepo == nil || s.groupRepo == nil || s.adapters == nil || requestedModel == "" {
		return nil, ErrNoAvailableAccounts
	}
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get current media scheduling group: %w", err)
	}
	if group == nil {
		return nil, fmt.Errorf("get current media scheduling group: %w", ErrNoAvailableAccounts)
	}
	canonicalModel := requestedModel
	var registryDefinition *MediaModelDefinition
	if s.registry != nil {
		resolvedModel, resolveErr := s.registry.CanonicalModelID(requestedModel)
		if resolveErr != nil {
			return nil, ErrNoAvailableAccounts
		}
		canonicalModel = resolvedModel
		if s.scopes == nil {
			return nil, fmt.Errorf("%w: media model scopes are not configured", ErrNoAvailableAccounts)
		}
		allowedModels, scopeErr := s.scopes.ListEnabledMediaModelIDs(ctx, groupID)
		if scopeErr != nil {
			return nil, fmt.Errorf("list group media model scopes: %w", scopeErr)
		}
		allowed := make(map[string]struct{}, len(allowedModels))
		for _, modelID := range allowedModels {
			allowed[normalizeMediaModelID(modelID)] = struct{}{}
		}
		if _, ok := allowed[canonicalModel]; !ok {
			return nil, ErrNoAvailableAccounts
		}
		if group.Platform != PlatformMedia {
			return nil, ErrNoAvailableAccounts
		}
		var definitionErr error
		if operation != "" {
			registryDefinition, definitionErr = s.registry.Resolve(requestedModel, operation)
		} else {
			registryDefinition, definitionErr = s.registry.definitionByID(canonicalModel)
		}
		if definitionErr != nil {
			return nil, definitionErr
		}
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
		if s.registry != nil && account.Platform != PlatformMedia {
			continue
		}
		if !group.MediaCrossPlatformEnabled && account.Platform != group.Platform {
			continue
		}
		modelForAccount := requestedModel
		if s.registry != nil {
			modelForAccount = canonicalModel
		}
		if !account.IsSchedulable() || !account.HasMediaModel(modelForAccount) {
			continue
		}
		resolved := account.ResolveMediaModel(modelForAccount)
		if registryDefinition != nil {
			resolved.Adapter = registryDefinition.DefaultAdapter
		}
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
				Provider:        resolved.Provider,
				Adapter:         resolved.Adapter,
				UpstreamModel:   resolved.UpstreamModel,
				NativeAsyncMode: resolved.NativeAsyncMode,
				RequestMapping:  resolved.RequestMapping,
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
	if s.registry != nil {
		if _, resolveErr := s.registry.Resolve(req.RequestedModel, req.Operation); resolveErr != nil {
			return nil, resolveErr
		}
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
		if s.registry != nil && account.Platform != PlatformMedia {
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
		SlotID:             req.SlotID,
		Candidates:         candidates,
		ExcludedAccountIDs: cloneAccountIDSet(req.ExcludedAccountIDs),
	})
	if err != nil {
		releaseAccountSelectionResult(selected)
		return nil, err
	}
	if selected == nil || selected.Account == nil {
		releaseAccountSelectionResult(selected)
		return nil, fmt.Errorf("%w: account candidate selector returned no account", ErrNoAvailableAccounts)
	}
	canonical, exists := allowed[selected.Account.ID]
	if !exists {
		releaseAccountSelectionResult(selected)
		return nil, fmt.Errorf("%w: selector returned account %d outside candidate snapshot", ErrNoAvailableAccounts, selected.Account.ID)
	}
	if selected.Acquired {
		if selected.WaitPlan != nil || selected.ReleaseFunc == nil || (req.SlotID != "" && selected.RefreshFunc == nil) {
			releaseAccountSelectionResult(selected)
			return nil, fmt.Errorf("%w: inconsistent acquired media account selection", ErrNoAvailableAccounts)
		}
	} else if !validMediaAccountWaitPlan(canonical, selected.WaitPlan, req.SlotID) || selected.ReleaseFunc != nil || selected.RefreshFunc != nil {
		releaseAccountSelectionResult(selected)
		return nil, fmt.Errorf("%w: inconsistent waiting media account selection", ErrNoAvailableAccounts)
	}

	return &MediaAccountSelection{
		Account:         canonical,
		ResolvedModel:   snapshots[canonical.ID].ResolvedModel,
		ResolvedRequest: append(json.RawMessage(nil), snapshots[canonical.ID].ResolvedRequest...),
		Acquired:        selected.Acquired,
		ReleaseFunc:     wrapOptionalIdempotentRelease(selected.ReleaseFunc),
		RefreshFunc:     selected.RefreshFunc,
		WaitPlan:        cloneAccountWaitPlan(selected.WaitPlan),
	}, nil
}

func (s *MediaScheduler) SelectFixed(ctx context.Context, req MediaFixedAccountRequest) (*MediaAccountSelection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.accountRepo == nil || s.selector == nil || req.AccountID <= 0 {
		return nil, ErrNoAvailableAccounts
	}
	account, err := s.loadFixedAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.ID != req.AccountID {
		return nil, ErrNoAvailableAccounts
	}
	canonical := *account
	fixedSelector, ok := s.selector.(fixedAccountCandidateSelector)
	if !ok {
		return nil, ErrStableAccountSlotUnsupported
	}
	selected, err := fixedSelector.SelectFixed(ctx, AccountCandidateSelectionRequest{
		GroupID: req.GroupID, SessionHash: req.SessionHash, SlotID: req.SlotID, Candidates: []*Account{&canonical},
	})
	if err != nil {
		releaseAccountSelectionResult(selected)
		return nil, err
	}
	if selected == nil || selected.Account == nil || selected.Account.ID != canonical.ID {
		releaseAccountSelectionResult(selected)
		return nil, fmt.Errorf("%w: fixed account selector changed account %d", ErrNoAvailableAccounts, req.AccountID)
	}
	if selected.Acquired {
		if selected.WaitPlan != nil || selected.ReleaseFunc == nil || (req.SlotID != "" && selected.RefreshFunc == nil) {
			releaseAccountSelectionResult(selected)
			return nil, fmt.Errorf("%w: inconsistent acquired fixed media account selection", ErrNoAvailableAccounts)
		}
	} else if !validMediaAccountWaitPlan(&canonical, selected.WaitPlan, req.SlotID) || selected.ReleaseFunc != nil || selected.RefreshFunc != nil {
		releaseAccountSelectionResult(selected)
		return nil, fmt.Errorf("%w: inconsistent waiting fixed media account selection", ErrNoAvailableAccounts)
	}
	return &MediaAccountSelection{
		Account:     &canonical,
		Acquired:    selected.Acquired,
		ReleaseFunc: wrapOptionalIdempotentRelease(selected.ReleaseFunc),
		RefreshFunc: selected.RefreshFunc,
		WaitPlan:    cloneAccountWaitPlan(selected.WaitPlan),
	}, nil
}

func releaseAccountSelectionResult(selection *AccountSelectionResult) {
	if selection != nil && selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func validMediaAccountWaitPlan(account *Account, plan *AccountWaitPlan, slotID string) bool {
	return account != nil &&
		account.ID > 0 &&
		account.Concurrency > 0 &&
		plan != nil &&
		plan.AccountID == account.ID &&
		plan.MaxConcurrency == account.Concurrency &&
		plan.Timeout > 0 &&
		plan.MaxWaiting > 0 &&
		plan.SlotID == slotID
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
		if err := candidate.ResolvedModel.RequestMapping.Validate(); err != nil {
			return nil, fmt.Errorf("%w: invalid media request mapping snapshot: %w", ErrNoAvailableAccounts, err)
		}
		if len(candidate.ResolvedRequest) > 0 && !json.Valid(candidate.ResolvedRequest) {
			return nil, fmt.Errorf("%w: invalid resolved media request snapshot", ErrNoAvailableAccounts)
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

func (s *MediaScheduler) WaitForSlot(ctx context.Context, selection *MediaAccountSelection) (*AcquireResult, error) {
	if s == nil || s.selector == nil || !validMediaWaitSelection(selection) {
		if selection != nil && selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		return nil, ErrAccountConcurrencySaturated
	}
	plan := cloneAccountWaitPlan(selection.WaitPlan)
	if plan.SlotID == "" {
		release, err := s.selector.Wait(ctx, plan)
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, ErrAccountConcurrencySaturated
		}
		return &AcquireResult{Acquired: true, ReleaseFunc: wrapOptionalIdempotentRelease(release)}, nil
	}
	waiter, ok := s.selector.(stableAccountCandidateWaiter)
	if !ok {
		return nil, ErrStableAccountSlotUnsupported
	}
	lease, err := waiter.WaitStable(ctx, plan)
	if err != nil {
		if lease != nil && lease.ReleaseFunc != nil {
			lease.ReleaseFunc()
		}
		return nil, err
	}
	if lease == nil || !lease.Acquired || lease.ReleaseFunc == nil || lease.RefreshFunc == nil {
		if lease != nil && lease.ReleaseFunc != nil {
			lease.ReleaseFunc()
		}
		return nil, ErrStableAccountSlotUnsupported
	}
	return &AcquireResult{
		Acquired:    true,
		ReleaseFunc: wrapOptionalIdempotentRelease(lease.ReleaseFunc),
		RefreshFunc: lease.RefreshFunc,
	}, nil
}

func validMediaWaitSelection(selection *MediaAccountSelection) bool {
	return selection != nil &&
		!selection.Acquired &&
		selection.ReleaseFunc == nil &&
		selection.WaitPlan != nil &&
		validMediaAccountWaitPlan(selection.Account, selection.WaitPlan, selection.WaitPlan.SlotID)
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
	return s.loadFixedAccount(ctx, accountID)
}

func (s *MediaScheduler) loadFixedAccount(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			err = errors.Join(ErrNoAvailableAccounts, err)
		}
		return nil, fmt.Errorf("get fixed media account %d: %w", accountID, err)
	}
	if account == nil {
		return nil, fmt.Errorf("get fixed media account %d: %w", accountID, ErrNoAvailableAccounts)
	}
	return account, nil
}
