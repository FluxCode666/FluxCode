package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const mediaBillingPostCommitTimeout = 10 * time.Second

// ProductionMediaBilling applies media charges through the durable local
// ledger, then refreshes every cache or scheduler view affected by the commit.
// The ledger is the source of truth; post-commit effects are deliberately
// repeatable so a coordinator retry can repair a partial notification failure.
type ProductionMediaBilling struct {
	ledger      MediaBillingLedgerRepository
	cache       BillingCache
	authCache   APIKeyAuthCacheInvalidator
	outboxQueue SchedulerOutboxQueue
}

func NewProductionMediaBilling(
	ledger MediaBillingLedgerRepository,
	cache BillingCache,
	authCache APIKeyAuthCacheInvalidator,
	outboxQueue SchedulerOutboxQueue,
) *ProductionMediaBilling {
	return &ProductionMediaBilling{
		ledger: ledger, cache: cache, authCache: authCache, outboxQueue: outboxQueue,
	}
}

func (b *ProductionMediaBilling) Precharge(
	ctx context.Context,
	task *MediaTask,
	snapshot MediaBillingSnapshot,
) (MediaPrechargeResult, error) {
	if err := b.validate(); err != nil {
		return MediaPrechargeResult{}, err
	}
	if err := validateProductionMediaBillingTask(task); err != nil {
		return MediaPrechargeResult{}, err
	}
	persistedSnapshot, err := decodeTaskMediaBillingSnapshot(task)
	if err != nil {
		return MediaPrechargeResult{}, err
	}
	snapshot, err = normalizeMediaBillingSnapshot(snapshot)
	if err != nil {
		return MediaPrechargeResult{}, err
	}
	if !equalMediaBillingSnapshots(snapshot, persistedSnapshot) {
		return MediaPrechargeResult{}, fmt.Errorf("%w: precharge snapshot differs from the persisted task snapshot", ErrInvalidMediaBillingSnapshot)
	}
	idempotencyKey, err := MediaBillingIdempotencyKey(task, MediaBillingOperationPrecharge)
	if err != nil {
		return MediaPrechargeResult{}, err
	}
	result, err := b.ledger.Precharge(ctx, MediaBillingPrechargeCommand{
		IdempotencyKey: idempotencyKey, RequestFingerprint: task.RequestFingerprint,
		TaskID: task.ID, TaskPublicID: task.PublicID, UserID: task.UserID,
		APIKeyID: task.APIKeyID, GroupID: task.GroupID,
		Amount: snapshot.EstimatedAmount, BilledAt: productionMediaBillingTime(task),
	})
	if err != nil {
		return MediaPrechargeResult{}, err
	}
	precharged, err := normalizeMediaPrechargeResult(MediaPrechargeResult{PrechargedAmount: result.PrechargedAmount})
	if err != nil {
		return MediaPrechargeResult{}, err
	}
	if err := b.refreshCommittedState(ctx, task, MediaBillingOperationPrecharge, result); err != nil {
		return precharged, errors.Join(
			ErrMediaPrechargeResultUnknown,
			fmt.Errorf("refresh committed media precharge state: %w", err),
		)
	}
	return precharged, nil
}

func (b *ProductionMediaBilling) SettleSuccess(
	ctx context.Context,
	task *MediaTask,
	usage MediaUsage,
) (MediaSettlementResult, error) {
	if err := b.validate(); err != nil {
		return MediaSettlementResult{}, err
	}
	if err := validateProductionMediaBillingTask(task); err != nil {
		return MediaSettlementResult{}, err
	}
	snapshot, err := decodeTaskMediaBillingSnapshot(task)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	pricing, err := CalculateMediaPricingSettlement(snapshot, usage)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	return b.settle(ctx, task, MediaBillingOperationSuccess, pricing.UserAmount, pricing.AccountBaseAmount)
}

func (b *ProductionMediaBilling) SettleFailure(
	ctx context.Context,
	task *MediaTask,
	settlement MediaFailureSettlement,
) (MediaSettlementResult, error) {
	if err := b.validate(); err != nil {
		return MediaSettlementResult{}, err
	}
	if err := validateProductionMediaBillingTask(task); err != nil {
		return MediaSettlementResult{}, err
	}
	if err := validateMediaFailureSettlement(settlement); err != nil {
		return MediaSettlementResult{}, err
	}
	precharged, err := normalizeMediaAmount(task.PrechargedAmount)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	finalAmount, err := normalizeMediaDecimalAmount(
		precharged.Mul(decimal.NewFromFloat(settlement.PenaltyRatio)),
	)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	return b.settle(ctx, task, MediaBillingOperationFailure, finalAmount.InexactFloat64(), 0)
}

func (b *ProductionMediaBilling) settle(
	ctx context.Context,
	task *MediaTask,
	operation MediaBillingOperation,
	finalAmount float64,
	accountBaseAmount float64,
) (MediaSettlementResult, error) {
	idempotencyKey, err := MediaBillingIdempotencyKey(task, operation)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	ledgerResult, err := b.ledger.Settle(ctx, MediaBillingSettlementCommand{
		IdempotencyKey: idempotencyKey, RequestFingerprint: task.RequestFingerprint,
		Operation: operation, TaskID: task.ID, TaskPublicID: task.PublicID,
		UserID: task.UserID, APIKeyID: task.APIKeyID, GroupID: task.GroupID,
		AccountID: task.AccountID, FinalAmount: finalAmount,
		AccountBaseAmount: accountBaseAmount, BilledAt: productionMediaBillingTime(task),
	})
	if err != nil {
		return MediaSettlementResult{}, err
	}
	result := MediaSettlementResult{
		FinalAmount: ledgerResult.FinalAmount, RefundedAmount: ledgerResult.RefundedAmount,
		AdditionalChargedAmount: ledgerResult.AdditionalChargedAmount,
	}
	result, err = normalizeMediaSettlementResult(ledgerResult.PrechargedAmount, result)
	if err != nil {
		return MediaSettlementResult{}, err
	}
	if err := b.refreshCommittedState(ctx, task, operation, ledgerResult); err != nil {
		return result, fmt.Errorf("refresh committed media settlement state: %w", err)
	}
	return result, nil
}

func (b *ProductionMediaBilling) validate() error {
	if b == nil || b.ledger == nil || b.cache == nil || b.authCache == nil || b.outboxQueue == nil {
		return errors.New("production media billing dependencies are incomplete")
	}
	return nil
}

func validateProductionMediaBillingTask(task *MediaTask) error {
	if task == nil || task.ID <= 0 || strings.TrimSpace(task.PublicID) == "" ||
		task.UserID <= 0 || task.APIKeyID <= 0 || task.GroupID <= 0 ||
		strings.TrimSpace(task.RequestFingerprint) == "" {
		return errors.New("production media billing task identity is invalid")
	}
	return nil
}

func decodeTaskMediaBillingSnapshot(task *MediaTask) (MediaBillingSnapshot, error) {
	if task == nil || len(task.BillingSnapshot) == 0 || bytes.Equal(bytes.TrimSpace(task.BillingSnapshot), []byte("null")) {
		return MediaBillingSnapshot{}, fmt.Errorf("%w: task billing snapshot is empty", ErrInvalidMediaBillingSnapshot)
	}
	var snapshot MediaBillingSnapshot
	if err := json.Unmarshal(task.BillingSnapshot, &snapshot); err != nil {
		return MediaBillingSnapshot{}, fmt.Errorf("decode task media billing snapshot: %w", err)
	}
	snapshot, err := normalizeMediaBillingSnapshot(snapshot)
	if err != nil {
		return MediaBillingSnapshot{}, err
	}
	return snapshot, nil
}

func equalMediaBillingSnapshots(left, right MediaBillingSnapshot) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func productionMediaBillingTime(task *MediaTask) time.Time {
	if task != nil && !task.CreatedAt.IsZero() {
		return task.CreatedAt.UTC()
	}
	return time.Now().UTC()
}

func (b *ProductionMediaBilling) refreshCommittedState(
	ctx context.Context,
	task *MediaTask,
	operation MediaBillingOperation,
	result MediaBillingLedgerResult,
) error {
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaBillingPostCommitTimeout)
	defer cancel()

	var refreshErrors []error
	switch result.Allocation.FundingSource {
	case MediaBillingFundingBalance:
		if err := b.cache.InvalidateUserBalance(refreshCtx, task.UserID); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("invalidate media user balance cache: %w", err))
		}
	case MediaBillingFundingSubscription:
		if err := b.cache.InvalidateSubscriptionCache(refreshCtx, task.UserID, task.GroupID); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("invalidate media subscription cache: %w", err))
		}
	case MediaBillingFundingFree:
	default:
		refreshErrors = append(refreshErrors, fmt.Errorf("%w: unknown funding source %q", ErrMediaBillingFundingInvalid, result.Allocation.FundingSource))
	}
	if result.Allocation.APIKeyRateLimitEnabled {
		if err := b.cache.InvalidateAPIKeyRateLimit(refreshCtx, task.APIKeyID); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("invalidate media API key rate-limit cache: %w", err))
		}
	}
	// A replay may be repairing an invalidation that failed after the original
	// commit, so repeat the auth-cache eviction even though the ledger no longer
	// reports the one-time status transition.
	if result.APIKeyStatusChanged || !result.Applied {
		b.authCache.InvalidateAuthCacheByUserID(refreshCtx, task.UserID)
	}
	accountQuotaChanged := result.AccountQuotaChanged ||
		(!result.Applied && result.Allocation.AccountQuota > 0)
	if operation == MediaBillingOperationSuccess && accountQuotaChanged {
		if task.AccountID == nil || *task.AccountID <= 0 {
			refreshErrors = append(refreshErrors, errors.New("media account quota changed without an account id"))
		} else if err := b.outboxQueue.Publish(
			refreshCtx,
			SchedulerOutboxEventAccountChanged,
			task.AccountID,
			nil,
			nil,
		); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("publish media account quota change: %w", err))
		}
	}
	return errors.Join(refreshErrors...)
}

var _ MediaBillingPort = (*ProductionMediaBilling)(nil)
