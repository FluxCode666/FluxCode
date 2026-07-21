package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type mediaBillingRepository struct {
	db *sql.DB
}

func NewMediaBillingRepository(db *sql.DB) service.MediaBillingLedgerRepository {
	return &mediaBillingRepository{db: db}
}

type mediaBillingOperationRow struct {
	ID                      int64
	TaskID                  int64
	Operation               service.MediaBillingOperation
	RequestFingerprint      string
	PrechargedAmount        float64
	FinalAmount             float64
	RefundedAmount          float64
	AdditionalChargedAmount float64
	Allocation              service.MediaBillingAllocation
}

type mediaBillingGroupState struct {
	SubscriptionType string
	DailyLimit       *float64
	WeeklyLimit      *float64
	MonthlyLimit     *float64
}

type mediaBillingAPIKeyState struct {
	Quota         float64
	QuotaUsed     float64
	RateLimit5h   float64
	RateLimit1d   float64
	RateLimit7d   float64
	Window5hStart *time.Time
	Window1dStart *time.Time
	Window7dStart *time.Time
	Status        string
}

type mediaBillingSubscriptionState struct {
	ID           int64
	DailyStart   *time.Time
	WeeklyStart  *time.Time
	MonthlyStart *time.Time
}

type mediaBillingFundingMutation struct {
	Allocation      service.MediaBillingAllocation
	OrdinaryCharged decimal.Decimal
}

func (r *mediaBillingRepository) Precharge(ctx context.Context, cmd service.MediaBillingPrechargeCommand) (_ service.MediaBillingLedgerResult, err error) {
	if r == nil || r.db == nil {
		return service.MediaBillingLedgerResult{}, errors.New("media billing repository db is nil")
	}
	amount, err := normalizeMediaBillingRepoAmount(cmd.Amount)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if cmd.BilledAt.IsZero() {
		cmd.BilledAt = time.Now().UTC()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := validateMediaBillingTask(ctx, tx, cmd.TaskID, cmd.TaskPublicID, cmd.UserID, cmd.APIKeyID, cmd.GroupID); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	operationID, claimed, err := claimMediaBillingOperation(ctx, tx, mediaBillingClaim{
		TaskID: cmd.TaskID, TaskPublicID: cmd.TaskPublicID, UserID: cmd.UserID,
		APIKeyID: cmd.APIKeyID, GroupID: cmd.GroupID, IdempotencyKey: cmd.IdempotencyKey,
		Operation: service.MediaBillingOperationPrecharge, RequestFingerprint: cmd.RequestFingerprint,
	})
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if !claimed {
		existing, loadErr := loadMediaBillingOperation(ctx, tx, cmd.IdempotencyKey)
		if loadErr != nil {
			return service.MediaBillingLedgerResult{}, loadErr
		}
		if err := validateExistingMediaBillingOperation(existing, cmd.TaskID, service.MediaBillingOperationPrecharge, cmd.RequestFingerprint); err != nil {
			return service.MediaBillingLedgerResult{}, err
		}
		return mediaBillingLedgerResult(existing, false), nil
	}

	groupState, err := loadMediaBillingGroupState(ctx, tx, cmd.GroupID)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	mutation, err := applyMediaBillingInitialFunding(ctx, tx, cmd, groupState, amount)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	// Keep the shared billing lock order aligned with Usage Billing: funding
	// rows (and any related commission rows) must always precede api_keys.
	apiKeyState, err := loadAndLockMediaBillingAPIKey(ctx, tx, cmd.APIKeyID, cmd.UserID, cmd.GroupID)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if err := applyMediaBillingAPIKeyDelta(ctx, tx, cmd.APIKeyID, apiKeyState, amount, &mutation.Allocation); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if err := completeMediaBillingOperation(ctx, tx, operationID, amount, decimal.Zero, decimal.Zero, mutation.Allocation); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return service.MediaBillingLedgerResult{}, errors.Join(
			service.ErrMediaPrechargeResultUnknown,
			fmt.Errorf("commit media precharge: %w", err),
		)
	}
	tx = nil
	return service.MediaBillingLedgerResult{
		Applied: true, PrechargedAmount: amount.InexactFloat64(), Allocation: mutation.Allocation,
		APIKeyStatusChanged: mediaBillingAPIKeyStatusCrossed(apiKeyState, amount),
	}, nil
}

func (r *mediaBillingRepository) Settle(ctx context.Context, cmd service.MediaBillingSettlementCommand) (_ service.MediaBillingLedgerResult, err error) {
	if r == nil || r.db == nil {
		return service.MediaBillingLedgerResult{}, errors.New("media billing repository db is nil")
	}
	if cmd.Operation != service.MediaBillingOperationSuccess && cmd.Operation != service.MediaBillingOperationFailure {
		return service.MediaBillingLedgerResult{}, fmt.Errorf("%w: invalid terminal operation %q", service.ErrMediaBillingOperationConflict, cmd.Operation)
	}
	finalAmount, err := normalizeMediaBillingRepoAmount(cmd.FinalAmount)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	accountBaseAmount, err := normalizeMediaBillingRepoAmount(cmd.AccountBaseAmount)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if cmd.Operation == service.MediaBillingOperationFailure && accountBaseAmount.IsPositive() {
		return service.MediaBillingLedgerResult{}, fmt.Errorf("%w: failed settlement cannot charge account quota", service.ErrMediaBillingOperationConflict)
	}
	if cmd.BilledAt.IsZero() {
		cmd.BilledAt = time.Now().UTC()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := validateMediaBillingTask(ctx, tx, cmd.TaskID, cmd.TaskPublicID, cmd.UserID, cmd.APIKeyID, cmd.GroupID); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	operationID, claimed, err := claimMediaBillingOperation(ctx, tx, mediaBillingClaim{
		TaskID: cmd.TaskID, TaskPublicID: cmd.TaskPublicID, UserID: cmd.UserID,
		APIKeyID: cmd.APIKeyID, GroupID: cmd.GroupID, AccountID: cmd.AccountID,
		IdempotencyKey: cmd.IdempotencyKey, Operation: cmd.Operation, RequestFingerprint: cmd.RequestFingerprint,
	})
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if !claimed {
		existing, loadErr := loadMediaBillingOperation(ctx, tx, cmd.IdempotencyKey)
		if loadErr != nil {
			return service.MediaBillingLedgerResult{}, loadErr
		}
		if err := validateExistingMediaBillingOperation(existing, cmd.TaskID, cmd.Operation, cmd.RequestFingerprint); err != nil {
			return service.MediaBillingLedgerResult{}, err
		}
		return mediaBillingLedgerResult(existing, false), nil
	}

	precharge, err := loadMediaBillingPrechargeForUpdate(ctx, tx, cmd.TaskID)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	precharged, err := normalizeMediaBillingRepoAmount(precharge.PrechargedAmount)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}

	allocation := precharge.Allocation
	refunded := decimal.Zero
	additional := decimal.Zero
	ordinaryFinal := decimal.Zero
	switch finalAmount.Cmp(precharged) {
	case -1:
		refunded = precharged.Sub(finalAmount)
		ordinaryFinal, err = refundMediaBillingFunding(ctx, tx, cmd.UserID, cmd.GroupID, refunded, precharged, allocation)
	case 0:
		ordinaryFinal, err = normalizeMediaBillingRepoAmount(allocation.OrdinaryBalance)
	case 1:
		additional = finalAmount.Sub(precharged)
		var extra mediaBillingFundingMutation
		extra, err = applyMediaBillingAdditionalFunding(ctx, tx, cmd, additional, allocation)
		if err == nil {
			allocation = mergeMediaBillingAllocations(allocation, extra.Allocation)
			ordinaryInitial, normalizeErr := normalizeMediaBillingRepoAmount(precharge.Allocation.OrdinaryBalance)
			if normalizeErr != nil {
				err = normalizeErr
			} else {
				ordinaryFinal = ordinaryInitial.Add(extra.OrdinaryCharged)
			}
		}
	}
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}

	if ordinaryFinal.IsPositive() {
		if err := unlockSalesCommissionFIFO(ctx, tx, cmd.UserID, ordinaryFinal.InexactFloat64()); err != nil {
			return service.MediaBillingLedgerResult{}, err
		}
	}

	// Usage Billing locks commission rows before api_keys. Preserve the same
	// order here; moving this lock only after the balance mutation is not enough
	// because commission -> api_key would still be inverted.
	apiKeyState, err := loadAndLockMediaBillingAPIKeyForSettlement(ctx, tx, cmd.APIKeyID, cmd.UserID, cmd.GroupID)
	if err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	delta := finalAmount.Sub(precharged)
	if err := applyMediaBillingAPIKeySettlementDelta(ctx, tx, cmd.APIKeyID, apiKeyState, delta, precharge.Allocation, &allocation); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}

	accountQuotaChanged := false
	if cmd.Operation == service.MediaBillingOperationSuccess && cmd.AccountID != nil && *cmd.AccountID > 0 && accountBaseAmount.IsPositive() {
		accountCharge, changed, accountErr := applyMediaBillingAccountQuota(ctx, tx, *cmd.AccountID, accountBaseAmount)
		if accountErr != nil {
			return service.MediaBillingLedgerResult{}, accountErr
		}
		allocation.AccountQuota = accountCharge.InexactFloat64()
		accountQuotaChanged = changed
	}

	if err := completeMediaBillingOperation(ctx, tx, operationID, precharged, finalAmount, refunded, allocation, additional); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return service.MediaBillingLedgerResult{}, err
	}
	tx = nil
	return service.MediaBillingLedgerResult{
		Applied: true, PrechargedAmount: precharged.InexactFloat64(), FinalAmount: finalAmount.InexactFloat64(),
		RefundedAmount: refunded.InexactFloat64(), AdditionalChargedAmount: additional.InexactFloat64(),
		Allocation: allocation, APIKeyStatusChanged: mediaBillingAPIKeyStatusChangedByDelta(apiKeyState, delta),
		AccountQuotaChanged: accountQuotaChanged,
	}, nil
}

type mediaBillingClaim struct {
	TaskID             int64
	TaskPublicID       string
	UserID             int64
	APIKeyID           int64
	GroupID            int64
	AccountID          *int64
	IdempotencyKey     string
	Operation          service.MediaBillingOperation
	RequestFingerprint string
}

func validateMediaBillingTask(
	ctx context.Context,
	tx *sql.Tx,
	taskID int64,
	publicID string,
	userID, apiKeyID, groupID int64,
) error {
	if taskID <= 0 || userID <= 0 || apiKeyID <= 0 || groupID <= 0 || strings.TrimSpace(publicID) == "" {
		return fmt.Errorf("%w: media task identity is incomplete", service.ErrMediaBillingOperationConflict)
	}
	var found int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM media_tasks
		WHERE id = $1
			AND public_id = $2
			AND user_id = $3
			AND api_key_id = $4
			AND group_id = $5
		FOR KEY SHARE
	`, taskID, publicID, userID, apiKeyID, groupID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: media task identity does not match", service.ErrMediaBillingOperationConflict)
	}
	return err
}

func claimMediaBillingOperation(ctx context.Context, tx *sql.Tx, claim mediaBillingClaim) (int64, bool, error) {
	if strings.TrimSpace(claim.IdempotencyKey) == "" || len(claim.IdempotencyKey) > 96 ||
		strings.TrimSpace(claim.RequestFingerprint) == "" || len(claim.RequestFingerprint) > 64 {
		return 0, false, fmt.Errorf("%w: invalid media billing idempotency metadata", service.ErrMediaBillingOperationConflict)
	}
	var operationID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO media_billing_operations (
			task_id, task_public_id, user_id, api_key_id, group_id, account_id,
			idempotency_key, operation, request_fingerprint
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
		RETURNING id
	`,
		claim.TaskID, claim.TaskPublicID, claim.UserID, claim.APIKeyID, claim.GroupID, claim.AccountID,
		claim.IdempotencyKey, string(claim.Operation), claim.RequestFingerprint,
	).Scan(&operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return operationID, true, nil
}

func loadMediaBillingOperation(ctx context.Context, tx *sql.Tx, idempotencyKey string) (mediaBillingOperationRow, error) {
	var row mediaBillingOperationRow
	var operation string
	var allocationJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT id, task_id, operation, request_fingerprint,
			precharged_amount, final_amount, refunded_amount, additional_charged_amount, allocation
		FROM media_billing_operations
		WHERE idempotency_key = $1
	`, idempotencyKey).Scan(
		&row.ID, &row.TaskID, &operation, &row.RequestFingerprint,
		&row.PrechargedAmount, &row.FinalAmount, &row.RefundedAmount, &row.AdditionalChargedAmount, &allocationJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaBillingOperationRow{}, service.ErrMediaBillingOperationConflict
	}
	if err != nil {
		return mediaBillingOperationRow{}, err
	}
	row.Operation = service.MediaBillingOperation(operation)
	if len(allocationJSON) > 0 {
		if err := json.Unmarshal(allocationJSON, &row.Allocation); err != nil {
			return mediaBillingOperationRow{}, fmt.Errorf("decode media billing allocation: %w", err)
		}
	}
	return row, nil
}

func loadMediaBillingPrechargeForUpdate(ctx context.Context, tx *sql.Tx, taskID int64) (mediaBillingOperationRow, error) {
	var row mediaBillingOperationRow
	var operation string
	var allocationJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT id, task_id, operation, request_fingerprint,
			precharged_amount, final_amount, refunded_amount, additional_charged_amount, allocation
		FROM media_billing_operations
		WHERE task_id = $1 AND operation = 'precharge'
		FOR UPDATE
	`, taskID).Scan(
		&row.ID, &row.TaskID, &operation, &row.RequestFingerprint,
		&row.PrechargedAmount, &row.FinalAmount, &row.RefundedAmount, &row.AdditionalChargedAmount, &allocationJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaBillingOperationRow{}, service.ErrMediaBillingPrechargeMissing
	}
	if err != nil {
		return mediaBillingOperationRow{}, err
	}
	row.Operation = service.MediaBillingOperation(operation)
	if err := json.Unmarshal(allocationJSON, &row.Allocation); err != nil {
		return mediaBillingOperationRow{}, fmt.Errorf("decode media billing precharge allocation: %w", err)
	}
	return row, nil
}

func validateExistingMediaBillingOperation(
	row mediaBillingOperationRow,
	taskID int64,
	operation service.MediaBillingOperation,
	fingerprint string,
) error {
	if row.TaskID != taskID || row.Operation != operation ||
		strings.TrimSpace(row.RequestFingerprint) != strings.TrimSpace(fingerprint) {
		return service.ErrMediaBillingOperationConflict
	}
	return nil
}

func mediaBillingLedgerResult(row mediaBillingOperationRow, applied bool) service.MediaBillingLedgerResult {
	return service.MediaBillingLedgerResult{
		Applied: applied, PrechargedAmount: row.PrechargedAmount, FinalAmount: row.FinalAmount,
		RefundedAmount: row.RefundedAmount, AdditionalChargedAmount: row.AdditionalChargedAmount,
		Allocation: row.Allocation,
	}
}

func completeMediaBillingOperation(
	ctx context.Context,
	tx *sql.Tx,
	operationID int64,
	precharged, final, refunded decimal.Decimal,
	allocation service.MediaBillingAllocation,
	additionalValues ...decimal.Decimal,
) error {
	additional := decimal.Zero
	if len(additionalValues) > 0 {
		additional = additionalValues[0]
	}
	encoded, err := json.Marshal(allocation)
	if err != nil {
		return fmt.Errorf("encode media billing allocation: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE media_billing_operations
		SET precharged_amount = $1,
			final_amount = $2,
			refunded_amount = $3,
			additional_charged_amount = $4,
			allocation = $5,
			updated_at = NOW()
		WHERE id = $6
	`, precharged, final, refunded, additional, encoded, operationID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("%w: complete media billing operation affected %d rows", service.ErrMediaBillingOperationConflict, affected)
	}
	return nil
}

func normalizeMediaBillingRepoAmount(value float64) (decimal.Decimal, error) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero, service.ErrMediaBillingFundingInvalid
	}
	amount := decimal.NewFromFloat(value).Round(8)
	if amount.GreaterThan(decimal.RequireFromString("999999999999.99999999")) {
		return decimal.Zero, service.ErrMediaBillingFundingInvalid
	}
	return amount, nil
}

func loadMediaBillingGroupState(ctx context.Context, tx *sql.Tx, groupID int64) (mediaBillingGroupState, error) {
	return loadMediaBillingGroupStateWithDeleted(ctx, tx, groupID, false)
}

func loadMediaBillingGroupStateForSettlement(ctx context.Context, tx *sql.Tx, groupID int64) (mediaBillingGroupState, error) {
	return loadMediaBillingGroupStateWithDeleted(ctx, tx, groupID, true)
}

func loadMediaBillingGroupStateWithDeleted(
	ctx context.Context,
	tx *sql.Tx,
	groupID int64,
	includeDeleted bool,
) (mediaBillingGroupState, error) {
	var state mediaBillingGroupState
	var daily, weekly, monthly sql.NullFloat64
	query := `
		SELECT subscription_type, daily_limit_usd, weekly_limit_usd, monthly_limit_usd
		FROM groups
		WHERE id = $1`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	err := tx.QueryRowContext(ctx, query, groupID).Scan(&state.SubscriptionType, &daily, &weekly, &monthly)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaBillingGroupState{}, service.ErrGroupNotFound
	}
	if err != nil {
		return mediaBillingGroupState{}, err
	}
	if daily.Valid {
		value := daily.Float64
		state.DailyLimit = &value
	}
	if weekly.Valid {
		value := weekly.Float64
		state.WeeklyLimit = &value
	}
	if monthly.Valid {
		value := monthly.Float64
		state.MonthlyLimit = &value
	}
	return state, nil
}

func loadAndLockMediaBillingAPIKey(
	ctx context.Context,
	tx *sql.Tx,
	apiKeyID, userID, groupID int64,
) (mediaBillingAPIKeyState, error) {
	return loadAndLockMediaBillingAPIKeyWithDeleted(ctx, tx, apiKeyID, userID, groupID, false)
}

func loadAndLockMediaBillingAPIKeyForSettlement(
	ctx context.Context,
	tx *sql.Tx,
	apiKeyID, userID, groupID int64,
) (mediaBillingAPIKeyState, error) {
	// A group cascade can clear api_keys.group_id after the task has frozen its
	// identity. Settlement therefore authenticates the immutable key + owner IDs.
	return loadAndLockMediaBillingAPIKeyWithDeleted(ctx, tx, apiKeyID, userID, groupID, true)
}

func loadAndLockMediaBillingAPIKeyWithDeleted(
	ctx context.Context,
	tx *sql.Tx,
	apiKeyID, userID, groupID int64,
	includeDeleted bool,
) (mediaBillingAPIKeyState, error) {
	var state mediaBillingAPIKeyState
	var window5h, window1d, window7d sql.NullTime
	query := `
		SELECT quota, quota_used, rate_limit_5h, rate_limit_1d, rate_limit_7d,
			window_5h_start, window_1d_start, window_7d_start, status
		FROM api_keys
		WHERE id = $1 AND user_id = $2`
	args := []any{apiKeyID, userID}
	if !includeDeleted {
		query += ` AND group_id = $3 AND deleted_at IS NULL`
		args = append(args, groupID)
	}
	query += ` FOR UPDATE`
	err := tx.QueryRowContext(ctx, query, args...).Scan(
		&state.Quota, &state.QuotaUsed, &state.RateLimit5h, &state.RateLimit1d, &state.RateLimit7d,
		&window5h, &window1d, &window7d, &state.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaBillingAPIKeyState{}, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return mediaBillingAPIKeyState{}, err
	}
	state.Window5hStart = nullableTimePointer(window5h)
	state.Window1dStart = nullableTimePointer(window1d)
	state.Window7dStart = nullableTimePointer(window7d)
	return state, nil
}

func nullableTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}

func applyMediaBillingInitialFunding(
	ctx context.Context,
	tx *sql.Tx,
	cmd service.MediaBillingPrechargeCommand,
	group mediaBillingGroupState,
	amount decimal.Decimal,
) (mediaBillingFundingMutation, error) {
	if group.SubscriptionType == service.SubscriptionTypeSubscription {
		subscription, err := loadAndLockMediaBillingSubscription(ctx, tx, cmd.UserID, cmd.GroupID, cmd.BilledAt)
		if err != nil {
			return mediaBillingFundingMutation{}, err
		}
		allocation := service.MediaBillingAllocation{
			FundingSource:            service.MediaBillingFundingSubscription,
			SubscriptionID:           &subscription.ID,
			SubscriptionDailyStart:   cloneMediaBillingTime(subscription.DailyStart),
			SubscriptionWeeklyStart:  cloneMediaBillingTime(subscription.WeeklyStart),
			SubscriptionMonthlyStart: cloneMediaBillingTime(subscription.MonthlyStart),
		}
		if amount.IsZero() {
			if err := lockActiveMediaBillingUser(ctx, tx, cmd.UserID); err != nil {
				return mediaBillingFundingMutation{}, err
			}
			return mediaBillingFundingMutation{Allocation: allocation}, nil
		}
		grants, err := allocateMediaBillingSubscription(ctx, tx, subscription.ID, group, cmd.BilledAt, amount)
		if err != nil {
			return mediaBillingFundingMutation{}, err
		}
		if err := lockActiveMediaBillingUser(ctx, tx, cmd.UserID); err != nil {
			return mediaBillingFundingMutation{}, err
		}
		allocation.SubscriptionGrant = grants
		return mediaBillingFundingMutation{Allocation: allocation}, nil
	}
	if amount.IsZero() {
		if err := lockActiveMediaBillingUser(ctx, tx, cmd.UserID); err != nil {
			return mediaBillingFundingMutation{}, err
		}
		return mediaBillingFundingMutation{Allocation: service.MediaBillingAllocation{FundingSource: service.MediaBillingFundingBalance}}, nil
	}
	return deductMediaBillingBalance(ctx, tx, cmd.UserID, amount, false)
}

func lockActiveMediaBillingUser(ctx context.Context, tx *sql.Tx, userID int64) error {
	var found int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, userID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrUserNotFound
	}
	return err
}

func loadAndLockMediaBillingSubscription(
	ctx context.Context,
	tx *sql.Tx,
	userID, groupID int64,
	at time.Time,
) (mediaBillingSubscriptionState, error) {
	var state mediaBillingSubscriptionState
	var daily, weekly, monthly sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT id, daily_window_start, weekly_window_start, monthly_window_start
		FROM user_subscriptions
		WHERE user_id = $1
			AND group_id = $2
			AND status = $3
			AND starts_at <= $4
			AND expires_at > $4
			AND deleted_at IS NULL
		ORDER BY expires_at DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`, userID, groupID, service.SubscriptionStatusActive, at).Scan(
		&state.ID, &daily, &weekly, &monthly,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaBillingSubscriptionState{}, service.ErrSubscriptionNotFound
	}
	if err != nil {
		return mediaBillingSubscriptionState{}, err
	}
	state.DailyStart = nullableTimePointer(daily)
	state.WeeklyStart = nullableTimePointer(weekly)
	state.MonthlyStart = nullableTimePointer(monthly)
	return state, nil
}

func cloneMediaBillingTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type mediaBillingGrantRow struct {
	ID      int64
	Daily   decimal.Decimal
	Weekly  decimal.Decimal
	Monthly decimal.Decimal
}

func allocateMediaBillingSubscription(
	ctx context.Context,
	tx *sql.Tx,
	subscriptionID int64,
	group mediaBillingGroupState,
	at time.Time,
	amount decimal.Decimal,
) ([]service.MediaBillingGrantAllocation, error) {
	return allocateMediaBillingSubscriptionWithDeleted(ctx, tx, subscriptionID, group, at, amount, false)
}

func allocateMediaBillingSubscriptionForSettlement(
	ctx context.Context,
	tx *sql.Tx,
	subscriptionID int64,
	group mediaBillingGroupState,
	at time.Time,
	amount decimal.Decimal,
) ([]service.MediaBillingGrantAllocation, error) {
	return allocateMediaBillingSubscriptionWithDeleted(ctx, tx, subscriptionID, group, at, amount, true)
}

func allocateMediaBillingSubscriptionWithDeleted(
	ctx context.Context,
	tx *sql.Tx,
	subscriptionID int64,
	group mediaBillingGroupState,
	at time.Time,
	amount decimal.Decimal,
	includeDeleted bool,
) ([]service.MediaBillingGrantAllocation, error) {
	if !amount.IsPositive() {
		return nil, nil
	}
	// Settlement reconstructs the grant set visible when the task was billed:
	// grants created later or already deleted then must never be adopted.
	selectionPredicate := " AND created_at <= $2 AND (deleted_at IS NULL OR deleted_at > $2)"
	mutationPredicate := ""
	if !includeDeleted {
		selectionPredicate = " AND deleted_at IS NULL"
		mutationPredicate = " AND deleted_at IS NULL"
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM subscription_grants
		WHERE subscription_id = $1`+selectionPredicate+`
			AND starts_at <= $2
			AND expires_at > $2
		ORDER BY expires_at ASC, id ASC
		FOR UPDATE
	`, subscriptionID, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []mediaBillingGrantRow
	for rows.Next() {
		var grant mediaBillingGrantRow
		if err := rows.Scan(&grant.ID, &grant.Daily, &grant.Weekly, &grant.Monthly); err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	if len(grants) == 0 {
		var tail mediaBillingGrantRow
		tailQuery := `
			SELECT id, daily_usage_usd, weekly_usage_usd, monthly_usage_usd
			FROM subscription_grants
			WHERE subscription_id = $1` + selectionPredicate + `
			ORDER BY expires_at DESC, id DESC
			LIMIT 1
			FOR UPDATE`
		tailArgs := []any{subscriptionID}
		if includeDeleted {
			tailArgs = append(tailArgs, at)
		}
		err := tx.QueryRowContext(ctx, tailQuery, tailArgs...).Scan(&tail.ID, &tail.Daily, &tail.Weekly, &tail.Monthly)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrSubscriptionGrantNotFound
		}
		if err != nil {
			return nil, err
		}
		grants = []mediaBillingGrantRow{tail}
	}

	remaining := amount
	allocations := make([]service.MediaBillingGrantAllocation, 0, len(grants))
	for index, grant := range grants {
		if !remaining.IsPositive() {
			break
		}
		allocation := remaining
		if capacity, limited := mediaBillingGrantCapacity(group, grant); limited && capacity.LessThan(allocation) {
			allocation = capacity
		}
		if !allocation.IsPositive() && index == len(grants)-1 {
			allocation = remaining
		}
		if !allocation.IsPositive() {
			continue
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE subscription_grants
			SET daily_usage_usd = daily_usage_usd + $1,
				weekly_usage_usd = weekly_usage_usd + $1,
				monthly_usage_usd = monthly_usage_usd + $1,
				updated_at = NOW()
			WHERE id = $2 AND subscription_id = $3`+mutationPredicate+`
		`, allocation, grant.ID, subscriptionID)
		if err != nil {
			return nil, err
		}
		if err := requireMediaBillingRowsAffected(result, 1, "allocate subscription grant"); err != nil {
			return nil, err
		}
		allocations = append(allocations, service.MediaBillingGrantAllocation{GrantID: grant.ID, Amount: allocation.InexactFloat64()})
		remaining = remaining.Sub(allocation)
	}
	if remaining.IsPositive() {
		return nil, service.ErrMediaBillingFundingInvalid
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET daily_usage_usd = daily_usage_usd + $1,
			weekly_usage_usd = weekly_usage_usd + $1,
			monthly_usage_usd = monthly_usage_usd + $1,
			updated_at = NOW()
		WHERE id = $2`+mutationPredicate+`
	`, amount, subscriptionID)
	if err != nil {
		return nil, err
	}
	if err := requireMediaBillingRowsAffected(result, 1, "increment media subscription usage"); err != nil {
		return nil, err
	}
	return allocations, nil
}

func mediaBillingGrantCapacity(group mediaBillingGroupState, grant mediaBillingGrantRow) (decimal.Decimal, bool) {
	var capacity decimal.Decimal
	limited := false
	for _, item := range []struct {
		limit *float64
		used  decimal.Decimal
	}{
		{group.DailyLimit, grant.Daily},
		{group.WeeklyLimit, grant.Weekly},
		{group.MonthlyLimit, grant.Monthly},
	} {
		if item.limit == nil || *item.limit <= 0 {
			continue
		}
		candidate := decimal.NewFromFloat(*item.limit).Sub(item.used)
		if candidate.IsNegative() {
			candidate = decimal.Zero
		}
		if !limited || candidate.LessThan(capacity) {
			capacity = candidate
			limited = true
		}
	}
	return capacity, limited
}

func deductMediaBillingBalance(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
	amount decimal.Decimal,
	includeDeletedUser bool,
) (mediaBillingFundingMutation, error) {
	allocation := service.MediaBillingAllocation{FundingSource: service.MediaBillingFundingBalance}
	remaining := amount
	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining
		FROM gift_balance_records
		WHERE user_id = $1
			AND remaining > 0
			AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY COALESCE(expires_at, '2099-12-31'::timestamptz) ASC, id ASC
		FOR UPDATE
	`, userID)
	if err != nil {
		return mediaBillingFundingMutation{}, err
	}
	type giftRow struct {
		id        int64
		remaining decimal.Decimal
	}
	var gifts []giftRow
	for rows.Next() {
		var gift giftRow
		if err := rows.Scan(&gift.id, &gift.remaining); err != nil {
			_ = rows.Close()
			return mediaBillingFundingMutation{}, err
		}
		gifts = append(gifts, gift)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return mediaBillingFundingMutation{}, err
	}
	if err := rows.Close(); err != nil {
		return mediaBillingFundingMutation{}, err
	}
	for _, gift := range gifts {
		if !remaining.IsPositive() {
			break
		}
		deduct := gift.remaining
		if remaining.LessThan(deduct) {
			deduct = remaining
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE gift_balance_records
			SET remaining = remaining - $1, updated_at = NOW()
			WHERE id = $2 AND user_id = $3 AND remaining >= $1
		`, deduct, gift.id, userID)
		if err != nil {
			return mediaBillingFundingMutation{}, err
		}
		if err := requireMediaBillingRowsAffected(result, 1, "deduct gift balance"); err != nil {
			return mediaBillingFundingMutation{}, err
		}
		allocation.GiftBalances = append(allocation.GiftBalances, service.MediaBillingGiftAllocation{
			RecordID: gift.id, Amount: deduct.InexactFloat64(),
		})
		remaining = remaining.Sub(deduct)
	}
	if remaining.IsPositive() {
		query := `
			UPDATE users
			SET balance = balance - $1, updated_at = NOW()
			WHERE id = $2`
		if !includeDeletedUser {
			query += ` AND deleted_at IS NULL`
		}
		result, err := tx.ExecContext(ctx, query, remaining, userID)
		if err != nil {
			return mediaBillingFundingMutation{}, err
		}
		if err := requireMediaBillingRowsAffected(result, 1, "deduct ordinary balance"); err != nil {
			return mediaBillingFundingMutation{}, err
		}
		allocation.OrdinaryBalance = remaining.InexactFloat64()
	} else if !includeDeletedUser {
		// A gift-only precharge still has to prove that its owning user is active.
		// This lock follows the gift rows, preserving the shared gift -> user order.
		if err := lockActiveMediaBillingUser(ctx, tx, userID); err != nil {
			return mediaBillingFundingMutation{}, err
		}
	}
	return mediaBillingFundingMutation{Allocation: allocation, OrdinaryCharged: remaining}, nil
}

func requireMediaBillingRowsAffected(result sql.Result, expected int64, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != expected {
		return fmt.Errorf("%w: %s affected %d rows", service.ErrMediaBillingFundingInvalid, operation, affected)
	}
	return nil
}

func applyMediaBillingAdditionalFunding(
	ctx context.Context,
	tx *sql.Tx,
	cmd service.MediaBillingSettlementCommand,
	amount decimal.Decimal,
	precharge service.MediaBillingAllocation,
) (mediaBillingFundingMutation, error) {
	if !amount.IsPositive() {
		return mediaBillingFundingMutation{Allocation: precharge}, nil
	}
	switch precharge.FundingSource {
	case service.MediaBillingFundingBalance:
		return deductMediaBillingBalance(ctx, tx, cmd.UserID, amount, true)
	case service.MediaBillingFundingSubscription:
		if precharge.SubscriptionID == nil || *precharge.SubscriptionID <= 0 {
			return mediaBillingFundingMutation{}, service.ErrMediaBillingFundingInvalid
		}
		group, err := loadMediaBillingGroupStateForSettlement(ctx, tx, cmd.GroupID)
		if err != nil {
			return mediaBillingFundingMutation{}, err
		}
		if err := lockMediaBillingSubscriptionByID(ctx, tx, *precharge.SubscriptionID, cmd.UserID, cmd.GroupID); err != nil {
			return mediaBillingFundingMutation{}, err
		}
		grants, err := allocateMediaBillingSubscriptionForSettlement(ctx, tx, *precharge.SubscriptionID, group, cmd.BilledAt, amount)
		if err != nil {
			return mediaBillingFundingMutation{}, err
		}
		return mediaBillingFundingMutation{Allocation: service.MediaBillingAllocation{
			FundingSource:     service.MediaBillingFundingSubscription,
			SubscriptionID:    precharge.SubscriptionID,
			SubscriptionGrant: grants,
		}}, nil
	case service.MediaBillingFundingFree:
		return mediaBillingFundingMutation{}, fmt.Errorf("%w: free precharge cannot receive an additional charge", service.ErrMediaBillingFundingInvalid)
	default:
		return mediaBillingFundingMutation{}, service.ErrMediaBillingFundingInvalid
	}
}

func lockMediaBillingSubscriptionByID(
	ctx context.Context,
	tx *sql.Tx,
	subscriptionID, userID, groupID int64,
) error {
	var found int
	// The precharge allocation froze this exact subscription identity. A later
	// soft delete must not prevent its usage from being restored or finalized.
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM user_subscriptions
		WHERE id = $1 AND user_id = $2 AND group_id = $3
		FOR UPDATE
	`, subscriptionID, userID, groupID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrSubscriptionNotFound
	}
	return err
}

func mergeMediaBillingAllocations(
	base service.MediaBillingAllocation,
	extra service.MediaBillingAllocation,
) service.MediaBillingAllocation {
	base.GiftBalances = append(base.GiftBalances, extra.GiftBalances...)
	base.SubscriptionGrant = append(base.SubscriptionGrant, extra.SubscriptionGrant...)
	base.OrdinaryBalance = decimal.NewFromFloat(base.OrdinaryBalance).
		Add(decimal.NewFromFloat(extra.OrdinaryBalance)).Round(8).InexactFloat64()
	return base
}

// refundMediaBillingFunding returns the ordinary-balance portion that remains
// charged after the proportional refund. That retained portion is the only
// amount eligible to unlock sales commission.
func refundMediaBillingFunding(
	ctx context.Context,
	tx *sql.Tx,
	userID, groupID int64,
	refund, precharged decimal.Decimal,
	allocation service.MediaBillingAllocation,
) (decimal.Decimal, error) {
	if !refund.IsPositive() {
		return normalizeMediaBillingRepoAmount(allocation.OrdinaryBalance)
	}
	if !precharged.IsPositive() || refund.GreaterThan(precharged) {
		return decimal.Zero, service.ErrMediaBillingFundingInvalid
	}
	switch allocation.FundingSource {
	case service.MediaBillingFundingBalance:
		ordinary, err := refundMediaBillingBalance(ctx, tx, userID, refund, precharged, allocation)
		return ordinary, err
	case service.MediaBillingFundingSubscription:
		if err := refundMediaBillingSubscription(ctx, tx, userID, groupID, refund, precharged, allocation); err != nil {
			return decimal.Zero, err
		}
		return decimal.Zero, nil
	case service.MediaBillingFundingFree:
		return decimal.Zero, service.ErrMediaBillingFundingInvalid
	default:
		return decimal.Zero, service.ErrMediaBillingFundingInvalid
	}
}

type mediaBillingRefundBucket struct {
	amount decimal.Decimal
	apply  func(decimal.Decimal) error
}

func distributeMediaBillingRefund(
	refund, precharged decimal.Decimal,
	buckets []mediaBillingRefundBucket,
) ([]decimal.Decimal, error) {
	total := decimal.Zero
	for _, bucket := range buckets {
		total = total.Add(bucket.amount)
	}
	if !total.Round(8).Equal(precharged.Round(8)) {
		return nil, service.ErrMediaBillingFundingInvalid
	}
	remainingRefund := refund
	remainingBase := precharged
	shares := make([]decimal.Decimal, len(buckets))
	for index, bucket := range buckets {
		if bucket.amount.IsNegative() {
			return nil, service.ErrMediaBillingFundingInvalid
		}
		if !bucket.amount.IsPositive() {
			continue
		}
		share := remainingRefund
		if index < len(buckets)-1 && remainingBase.IsPositive() {
			share = remainingRefund.Mul(bucket.amount).Div(remainingBase).Round(8)
		}
		if share.GreaterThan(bucket.amount) {
			share = bucket.amount
		}
		if share.GreaterThan(remainingRefund) {
			share = remainingRefund
		}
		shares[index] = share
		remainingRefund = remainingRefund.Sub(share)
		remainingBase = remainingBase.Sub(bucket.amount)
	}
	if !remainingRefund.IsZero() {
		for index := len(buckets) - 1; index >= 0; index-- {
			capacity := buckets[index].amount.Sub(shares[index])
			if !capacity.IsPositive() {
				continue
			}
			add := remainingRefund
			if capacity.LessThan(add) {
				add = capacity
			}
			shares[index] = shares[index].Add(add)
			remainingRefund = remainingRefund.Sub(add)
			if remainingRefund.IsZero() {
				break
			}
		}
	}
	if !remainingRefund.IsZero() {
		return nil, service.ErrMediaBillingFundingInvalid
	}
	for index, share := range shares {
		if share.IsPositive() && buckets[index].apply != nil {
			if err := buckets[index].apply(share); err != nil {
				return nil, err
			}
		}
	}
	return shares, nil
}

func refundMediaBillingBalance(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
	refund, precharged decimal.Decimal,
	allocation service.MediaBillingAllocation,
) (decimal.Decimal, error) {
	buckets := make([]mediaBillingRefundBucket, 0, len(allocation.GiftBalances)+1)
	for _, gift := range allocation.GiftBalances {
		gift := gift
		amount, err := normalizeMediaBillingRepoAmount(gift.Amount)
		if err != nil || gift.RecordID <= 0 {
			return decimal.Zero, service.ErrMediaBillingFundingInvalid
		}
		buckets = append(buckets, mediaBillingRefundBucket{amount: amount, apply: func(share decimal.Decimal) error {
			result, err := tx.ExecContext(ctx, `
				UPDATE gift_balance_records
				SET remaining = remaining + $1, updated_at = NOW()
				WHERE id = $2 AND user_id = $3
			`, share, gift.RecordID, userID)
			if err != nil {
				return err
			}
			return requireMediaBillingRowsAffected(result, 1, "refund gift balance")
		}})
	}
	ordinary, err := normalizeMediaBillingRepoAmount(allocation.OrdinaryBalance)
	if err != nil {
		return decimal.Zero, err
	}
	ordinaryIndex := len(buckets)
	buckets = append(buckets, mediaBillingRefundBucket{amount: ordinary, apply: func(share decimal.Decimal) error {
		// The task owns this historical balance mutation even if the user was
		// soft-deleted while its asynchronous provider call was still running.
		result, err := tx.ExecContext(ctx, `
			UPDATE users
			SET balance = balance + $1, updated_at = NOW()
			WHERE id = $2
		`, share, userID)
		if err != nil {
			return err
		}
		return requireMediaBillingRowsAffected(result, 1, "refund ordinary balance")
	}})
	shares, err := distributeMediaBillingRefund(refund, precharged, buckets)
	if err != nil {
		return decimal.Zero, err
	}
	return ordinary.Sub(shares[ordinaryIndex]), nil
}

func refundMediaBillingSubscription(
	ctx context.Context,
	tx *sql.Tx,
	userID, groupID int64,
	refund, precharged decimal.Decimal,
	allocation service.MediaBillingAllocation,
) error {
	if allocation.SubscriptionID == nil || *allocation.SubscriptionID <= 0 || len(allocation.SubscriptionGrant) == 0 {
		return service.ErrMediaBillingFundingInvalid
	}
	// All subscription billing paths lock the parent before grant rows. Usage
	// Billing takes this lock in loadUsageBillingSubscriptionGroup.
	if err := lockMediaBillingSubscriptionByID(ctx, tx, *allocation.SubscriptionID, userID, groupID); err != nil {
		return err
	}
	buckets := make([]mediaBillingRefundBucket, 0, len(allocation.SubscriptionGrant))
	for _, grant := range allocation.SubscriptionGrant {
		grant := grant
		amount, err := normalizeMediaBillingRepoAmount(grant.Amount)
		if err != nil || grant.GrantID <= 0 {
			return service.ErrMediaBillingFundingInvalid
		}
		buckets = append(buckets, mediaBillingRefundBucket{amount: amount, apply: func(share decimal.Decimal) error {
			result, err := tx.ExecContext(ctx, `
				UPDATE subscription_grants sg
				SET daily_usage_usd = CASE
						WHEN us.daily_window_start IS NOT DISTINCT FROM $1 THEN GREATEST(0, sg.daily_usage_usd - $4)
						ELSE sg.daily_usage_usd END,
					weekly_usage_usd = CASE
						WHEN us.weekly_window_start IS NOT DISTINCT FROM $2 THEN GREATEST(0, sg.weekly_usage_usd - $4)
						ELSE sg.weekly_usage_usd END,
					monthly_usage_usd = CASE
						WHEN us.monthly_window_start IS NOT DISTINCT FROM $3 THEN GREATEST(0, sg.monthly_usage_usd - $4)
						ELSE sg.monthly_usage_usd END,
					updated_at = NOW()
				FROM user_subscriptions us
				WHERE sg.id = $5 AND sg.subscription_id = $6
					AND us.id = $6
			`, allocation.SubscriptionDailyStart, allocation.SubscriptionWeeklyStart,
				allocation.SubscriptionMonthlyStart, share, grant.GrantID, *allocation.SubscriptionID)
			if err != nil {
				return err
			}
			return requireMediaBillingRowsAffected(result, 1, "refund subscription grant")
		}})
	}
	if _, err := distributeMediaBillingRefund(refund, precharged, buckets); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET daily_usage_usd = CASE
				WHEN daily_window_start IS NOT DISTINCT FROM $1 THEN GREATEST(0, daily_usage_usd - $4)
				ELSE daily_usage_usd END,
			weekly_usage_usd = CASE
				WHEN weekly_window_start IS NOT DISTINCT FROM $2 THEN GREATEST(0, weekly_usage_usd - $4)
				ELSE weekly_usage_usd END,
			monthly_usage_usd = CASE
				WHEN monthly_window_start IS NOT DISTINCT FROM $3 THEN GREATEST(0, monthly_usage_usd - $4)
				ELSE monthly_usage_usd END,
			updated_at = NOW()
		WHERE id = $5
	`, allocation.SubscriptionDailyStart, allocation.SubscriptionWeeklyStart,
		allocation.SubscriptionMonthlyStart, refund, *allocation.SubscriptionID)
	if err != nil {
		return err
	}
	return requireMediaBillingRowsAffected(result, 1, "refund media subscription usage")
}

func applyMediaBillingAPIKeyDelta(
	ctx context.Context,
	tx *sql.Tx,
	apiKeyID int64,
	state mediaBillingAPIKeyState,
	amount decimal.Decimal,
	allocation *service.MediaBillingAllocation,
) error {
	if allocation == nil {
		return service.ErrMediaBillingFundingInvalid
	}
	allocation.APIKeyQuotaEnabled = state.Quota > 0
	allocation.APIKeyRateLimitEnabled = state.RateLimit5h > 0 || state.RateLimit1d > 0 || state.RateLimit7d > 0
	allocation.APIKeyWindow5hStart = cloneMediaBillingTime(state.Window5hStart)
	allocation.APIKeyWindow1dStart = cloneMediaBillingTime(state.Window1dStart)
	allocation.APIKeyWindow7dStart = cloneMediaBillingTime(state.Window7dStart)
	if !amount.IsPositive() {
		return nil
	}
	if allocation.APIKeyQuotaEnabled {
		result, err := tx.ExecContext(ctx, `
			UPDATE api_keys
			SET quota_used = quota_used + $1,
				status = CASE
					WHEN quota > 0 AND status = $3 AND quota_used < quota AND quota_used + $1 >= quota
					THEN $4
					ELSE status
				END,
				updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
		`, amount, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted)
		if err != nil {
			return err
		}
		if err := requireMediaBillingRowsAffected(result, 1, "precharge API key quota"); err != nil {
			return err
		}
		allocation.APIKeyQuota = amount.InexactFloat64()
	}
	if allocation.APIKeyRateLimitEnabled {
		window5h, window1d, window7d, err := incrementMediaBillingAPIKeyRateLimit(ctx, tx, apiKeyID, amount, false)
		if err != nil {
			return err
		}
		allocation.APIKeyRateLimit = amount.InexactFloat64()
		allocation.APIKeyWindow5hStart = window5h
		allocation.APIKeyWindow1dStart = window1d
		allocation.APIKeyWindow7dStart = window7d
	}
	return nil
}

func incrementMediaBillingAPIKeyRateLimit(
	ctx context.Context,
	tx *sql.Tx,
	apiKeyID int64,
	amount decimal.Decimal,
	includeDeleted bool,
) (*time.Time, *time.Time, *time.Time, error) {
	var window5h, window1d, window7d sql.NullTime
	query := `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN $1 ELSE usage_5h + $1 END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN $1 ELSE usage_1d + $1 END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN $1 ELSE usage_7d + $1 END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $2`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	query += ` RETURNING window_5h_start, window_1d_start, window_7d_start`
	err := tx.QueryRowContext(ctx, query, amount, apiKeyID).Scan(&window5h, &window1d, &window7d)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return nullableTimePointer(window5h), nullableTimePointer(window1d), nullableTimePointer(window7d), nil
}

func applyMediaBillingAPIKeySettlementDelta(
	ctx context.Context,
	tx *sql.Tx,
	apiKeyID int64,
	state mediaBillingAPIKeyState,
	delta decimal.Decimal,
	precharge service.MediaBillingAllocation,
	allocation *service.MediaBillingAllocation,
) error {
	if allocation == nil || delta.IsZero() {
		return nil
	}
	if precharge.APIKeyQuotaEnabled {
		result, err := tx.ExecContext(ctx, `
			UPDATE api_keys
			SET quota_used = GREATEST(0, quota_used + $1),
				status = CASE
					WHEN quota > 0 AND status = $3::varchar AND quota_used < quota AND GREATEST(0, quota_used + $1) >= quota
					THEN $4::varchar
					WHEN status = $4::varchar AND (quota <= 0 OR GREATEST(0, quota_used + $1) < quota)
					THEN $3::varchar
					ELSE status
				END,
				updated_at = NOW()
			WHERE id = $2
		`, delta, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted)
		if err != nil {
			return err
		}
		if err := requireMediaBillingRowsAffected(result, 1, "settle API key quota"); err != nil {
			return err
		}
		allocation.APIKeyQuota = nonNegativeMediaBillingDecimal(decimal.NewFromFloat(precharge.APIKeyQuota).Add(delta)).Round(8).InexactFloat64()
	}
	if !precharge.APIKeyRateLimitEnabled {
		return nil
	}
	if delta.IsPositive() {
		window5h, window1d, window7d, err := incrementMediaBillingAPIKeyRateLimit(ctx, tx, apiKeyID, delta, true)
		if err != nil {
			return err
		}
		allocation.APIKeyWindow5hStart = window5h
		allocation.APIKeyWindow1dStart = window1d
		allocation.APIKeyWindow7dStart = window7d
	} else {
		refund := delta.Neg()
		result, err := tx.ExecContext(ctx, `
			UPDATE api_keys
			SET usage_5h = CASE
					WHEN window_5h_start IS NOT DISTINCT FROM $1 THEN GREATEST(0, usage_5h - $4)
					ELSE usage_5h END,
				usage_1d = CASE
					WHEN window_1d_start IS NOT DISTINCT FROM $2 THEN GREATEST(0, usage_1d - $4)
					ELSE usage_1d END,
				usage_7d = CASE
					WHEN window_7d_start IS NOT DISTINCT FROM $3 THEN GREATEST(0, usage_7d - $4)
					ELSE usage_7d END,
				updated_at = NOW()
			WHERE id = $5
		`, precharge.APIKeyWindow5hStart, precharge.APIKeyWindow1dStart,
			precharge.APIKeyWindow7dStart, refund, apiKeyID)
		if err != nil {
			return err
		}
		if err := requireMediaBillingRowsAffected(result, 1, "refund API key rate limit"); err != nil {
			return err
		}
	}
	allocation.APIKeyRateLimit = nonNegativeMediaBillingDecimal(decimal.NewFromFloat(precharge.APIKeyRateLimit).Add(delta)).Round(8).InexactFloat64()
	_ = state
	return nil
}

func mediaBillingAPIKeyStatusCrossed(state mediaBillingAPIKeyState, amount decimal.Decimal) bool {
	return state.Quota > 0 && state.Status == service.StatusAPIKeyActive &&
		decimal.NewFromFloat(state.QuotaUsed).LessThan(decimal.NewFromFloat(state.Quota)) &&
		decimal.NewFromFloat(state.QuotaUsed).Add(amount).GreaterThanOrEqual(decimal.NewFromFloat(state.Quota))
}

func mediaBillingAPIKeyStatusChangedByDelta(state mediaBillingAPIKeyState, delta decimal.Decimal) bool {
	if state.Quota <= 0 || delta.IsZero() {
		return false
	}
	quota := decimal.NewFromFloat(state.Quota)
	used := decimal.NewFromFloat(state.QuotaUsed)
	updated := nonNegativeMediaBillingDecimal(used.Add(delta))
	return (state.Status == service.StatusAPIKeyActive && used.LessThan(quota) && updated.GreaterThanOrEqual(quota)) ||
		(state.Status == service.StatusAPIKeyQuotaExhausted && updated.LessThan(quota))
}

func nonNegativeMediaBillingDecimal(value decimal.Decimal) decimal.Decimal {
	if value.IsNegative() {
		return decimal.Zero
	}
	return value
}

func applyMediaBillingAccountQuota(
	ctx context.Context,
	tx *sql.Tx,
	accountID int64,
	baseAmount decimal.Decimal,
) (decimal.Decimal, bool, error) {
	var accountType string
	var rateMultiplier float64
	var extraJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT type, rate_multiplier, extra
		FROM accounts
		WHERE id = $1
		FOR UPDATE
	`, accountID).Scan(&accountType, &rateMultiplier, &extraJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return decimal.Zero, false, service.ErrAccountNotFound
	}
	if err != nil {
		return decimal.Zero, false, err
	}
	if !strings.EqualFold(accountType, service.AccountTypeAPIKey) && !strings.EqualFold(accountType, service.AccountTypeBedrock) {
		return decimal.Zero, false, nil
	}
	extra := map[string]any{}
	if len(extraJSON) > 0 {
		if err := json.Unmarshal(extraJSON, &extra); err != nil {
			return decimal.Zero, false, fmt.Errorf("decode media billing account quota: %w", err)
		}
	}
	account := &service.Account{Type: accountType, Extra: extra, RateMultiplier: &rateMultiplier}
	if !account.HasAnyQuotaLimit() {
		return decimal.Zero, false, nil
	}
	charge := baseAmount.Mul(decimal.NewFromFloat(account.BillingRateMultiplier())).Round(8)
	if !charge.IsPositive() {
		return decimal.Zero, false, nil
	}

	rows, err := tx.QueryContext(ctx,
		`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			|| jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $1)
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_daily_used',
					CASE WHEN `+dailyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $1 END,
					'quota_daily_start',
					CASE WHEN `+dailyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_daily_start', `+nowUTC+`) END
				)
				|| CASE WHEN `+dailyExpiredExpr+` AND `+nextDailyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_daily_reset_at', `+nextDailyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_weekly_used',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $1 END,
					'quota_weekly_start',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_weekly_start', `+nowUTC+`) END
				)
				|| CASE WHEN `+weeklyExpiredExpr+` AND `+nextWeeklyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_weekly_reset_at', `+nextWeeklyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
		), updated_at = NOW()
		WHERE id = $2
		RETURNING id`,
		charge, accountID)
	if err != nil {
		return decimal.Zero, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return decimal.Zero, false, err
		}
		return decimal.Zero, false, service.ErrAccountNotFound
	}
	var returnedID int64
	if err := rows.Scan(&returnedID); err != nil {
		return decimal.Zero, false, err
	}
	if returnedID != accountID {
		return decimal.Zero, false, service.ErrMediaBillingFundingInvalid
	}
	if err := rows.Close(); err != nil {
		return decimal.Zero, false, err
	}
	return charge, true, nil
}

var _ service.MediaBillingLedgerRepository = (*mediaBillingRepository)(nil)
