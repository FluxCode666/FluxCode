package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type usageBillingRepository struct {
	db *sql.DB
}

func NewUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB) service.UsageBillingRepository {
	return &usageBillingRepository{db: sqlDB}
}

func (r *usageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (_ *service.UsageBillingApplyResult, err error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.UsageBillingApplyResult{Applied: false}, nil
	}

	result := &service.UsageBillingApplyResult{Applied: true}
	if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
		return nil, err
	}
	if cmd.UsageLog != nil {
		if err := execUsageLogInsertNoResult(ctx, tx, prepareUsageLogInsert(cmd.UsageLog)); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) claimUsageBillingKey(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id
	`, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existingFingerprint string
		if err := tx.QueryRowContext(ctx, `
			SELECT request_fingerprint
			FROM usage_billing_dedup
			WHERE request_id = $1 AND api_key_id = $2
		`, cmd.RequestID, cmd.APIKeyID).Scan(&existingFingerprint); err != nil {
			return false, err
		}
		if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(cmd.RequestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var archivedFingerprint string
	err = tx.QueryRowContext(ctx, `
		SELECT request_fingerprint
		FROM usage_billing_dedup_archive
		WHERE request_id = $1 AND api_key_id = $2
	`, cmd.RequestID, cmd.APIKeyID).Scan(&archivedFingerprint)
	if err == nil {
		if strings.TrimSpace(archivedFingerprint) != strings.TrimSpace(cmd.RequestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return true, nil
}

func (r *usageBillingRepository) applyUsageBillingEffects(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand, result *service.UsageBillingApplyResult) error {
	if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
		if err := incrementUsageBillingSubscription(ctx, tx, *cmd.SubscriptionID, cmd.BilledAt, cmd.SubscriptionCost); err != nil {
			return err
		}
	}

	if cmd.BalanceCost > 0 {
		newBalance, err := deductUsageBillingBalance(ctx, tx, cmd.UserID, cmd.BalanceCost)
		if err != nil {
			return err
		}
		result.NewBalance = &newBalance
	}

	if cmd.APIKeyQuotaCost > 0 {
		exhausted, err := incrementUsageBillingAPIKeyQuota(ctx, tx, cmd.APIKeyID, cmd.APIKeyQuotaCost)
		if err != nil {
			return err
		}
		result.APIKeyQuotaExhausted = exhausted
	}

	if cmd.APIKeyRateLimitCost > 0 {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, cmd.APIKeyID, cmd.APIKeyRateLimitCost); err != nil {
			return err
		}
	}

	if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) || strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
		quotaState, err := incrementUsageBillingAccountQuota(ctx, tx, cmd.AccountID, cmd.AccountQuotaCost)
		if err != nil {
			return err
		}
		result.QuotaState = quotaState
	}

	return nil
}

func incrementUsageBillingSubscription(ctx context.Context, tx *sql.Tx, subscriptionID int64, billedAt time.Time, costUSD float64) error {
	group, err := loadUsageBillingSubscriptionGroup(ctx, tx, subscriptionID)
	if err != nil {
		return err
	}
	if billedAt.IsZero() {
		billedAt = time.Now()
	}
	if err := allocateUsageBillingSubscriptionGrants(ctx, tx, subscriptionID, group, billedAt, costUSD); err != nil {
		return err
	}

	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
	`
	res, err := tx.ExecContext(ctx, updateSQL, costUSD, subscriptionID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	return service.ErrSubscriptionNotFound
}

func loadUsageBillingSubscriptionGroup(ctx context.Context, tx *sql.Tx, subscriptionID int64) (*service.Group, error) {
	var daily, weekly, monthly sql.NullFloat64
	err := tx.QueryRowContext(ctx, `
		SELECT g.daily_limit_usd, g.weekly_limit_usd, g.monthly_limit_usd
		FROM user_subscriptions us
		JOIN groups g ON g.id = us.group_id
		WHERE us.id = $1
			AND us.deleted_at IS NULL
			AND g.deleted_at IS NULL
	`, subscriptionID).Scan(&daily, &weekly, &monthly)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, err
	}

	group := &service.Group{}
	if daily.Valid {
		v := daily.Float64
		group.DailyLimitUSD = &v
	}
	if weekly.Valid {
		v := weekly.Float64
		group.WeeklyLimitUSD = &v
	}
	if monthly.Valid {
		v := monthly.Float64
		group.MonthlyLimitUSD = &v
	}
	return group, nil
}

type usageBillingGrantRow struct {
	id      int64
	daily   float64
	weekly  float64
	monthly float64
}

func allocateUsageBillingSubscriptionGrants(ctx context.Context, tx *sql.Tx, subscriptionID int64, group *service.Group, at time.Time, costUSD float64) error {
	if costUSD <= 0 {
		return nil
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM subscription_grants
		WHERE subscription_id = $1
			AND deleted_at IS NULL
			AND starts_at <= $2
			AND expires_at > $2
		ORDER BY expires_at ASC, id ASC
		FOR UPDATE
	`, subscriptionID, at)
	if err != nil {
		return err
	}
	defer rows.Close() //nolint:errcheck

	grants := make([]usageBillingGrantRow, 0, 4)
	for rows.Next() {
		var grant usageBillingGrantRow
		if err := rows.Scan(&grant.id, &grant.daily, &grant.weekly, &grant.monthly); err != nil {
			return err
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(grants) == 0 {
		var tailID int64
		err := tx.QueryRowContext(ctx, `
			SELECT id
			FROM subscription_grants
			WHERE subscription_id = $1
				AND deleted_at IS NULL
			ORDER BY expires_at DESC, id DESC
			LIMIT 1
			FOR UPDATE
		`, subscriptionID).Scan(&tailID)
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrSubscriptionGrantNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE subscription_grants
			SET
				daily_usage_usd = daily_usage_usd + $1,
				weekly_usage_usd = weekly_usage_usd + $1,
				monthly_usage_usd = monthly_usage_usd + $1,
				updated_at = NOW()
			WHERE id = $2
		`, costUSD, tailID)
		return err
	}

	remaining := costUSD
	for i, grant := range grants {
		if remaining <= 0 {
			break
		}

		cap := math.Inf(1)
		if group != nil {
			if group.HasDailyLimit() {
				cap = math.Min(cap, *group.DailyLimitUSD-grant.daily)
			}
			if group.HasWeeklyLimit() {
				cap = math.Min(cap, *group.WeeklyLimitUSD-grant.weekly)
			}
			if group.HasMonthlyLimit() {
				cap = math.Min(cap, *group.MonthlyLimitUSD-grant.monthly)
			}
		}
		if cap < 0 {
			cap = 0
		}

		allocation := remaining
		if !math.IsInf(cap, 1) {
			allocation = math.Min(remaining, cap)
		}
		if allocation <= 0 && i == len(grants)-1 {
			allocation = remaining
		}
		if allocation <= 0 {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE subscription_grants
			SET
				daily_usage_usd = daily_usage_usd + $1,
				weekly_usage_usd = weekly_usage_usd + $1,
				monthly_usage_usd = monthly_usage_usd + $1,
				updated_at = NOW()
			WHERE id = $2
		`, allocation, grant.id); err != nil {
			return err
		}

		remaining -= allocation
	}

	return nil
}

func deductUsageBillingBalance(ctx context.Context, tx *sql.Tx, userID int64, amount float64) (float64, error) {
	// 优先从赠送余额扣减（FIFO：按创建时间升序）
	giftDeducted := deductGiftBalanceFIFO(ctx, tx, userID, amount)
	remainingCost := amount - giftDeducted

	// 不足部分从正常余额扣减
	if remainingCost > 0.0001 { // 避免浮点精度问题
		var newBalance float64
		err := tx.QueryRowContext(ctx, `
			UPDATE users
			SET balance = balance - $1,
				updated_at = NOW()
			WHERE id = $2 AND deleted_at IS NULL
			RETURNING balance
		`, remainingCost, userID).Scan(&newBalance)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, service.ErrUserNotFound
		}
		if err != nil {
			return 0, err
		}
		if err := unlockSalesCommissionFIFO(ctx, tx, userID, remainingCost); err != nil {
			return 0, err
		}
		return newBalance, nil
	}

	// 全部从赠送余额扣减，查询当前正常余额
	var newBalance float64
	err := tx.QueryRowContext(ctx, `
		SELECT balance FROM users WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrUserNotFound
	}
	if err != nil {
		return 0, err
	}
	return newBalance, nil
}

func unlockSalesCommissionFIFO(ctx context.Context, tx *sql.Tx, refereeUserID int64, ordinaryUsageAmount float64) error {
	if ordinaryUsageAmount <= 0.0001 {
		return nil
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT scr.id, scr.order_credited_amount, scr.credited_used_amount, scr.commission_total_cny, scr.unlocked_cny
		FROM sales_commission_records scr
		JOIN payment_orders po ON po.id = scr.payment_order_id
		WHERE scr.referee_user_id = $1
			AND po.status = $2
			AND scr.status <> $3
			AND scr.credited_used_amount < scr.order_credited_amount
		ORDER BY scr.id ASC
		FOR UPDATE
	`, refereeUserID, service.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked)
	if err != nil {
		return err
	}
	defer rows.Close()

	type commissionRow struct {
		id                  int64
		orderCreditedAmount float64
		creditedUsedAmount  float64
		commissionTotalCNY  float64
		unlockedCNY         float64
	}
	var records []commissionRow
	for rows.Next() {
		var r commissionRow
		if err := rows.Scan(&r.id, &r.orderCreditedAmount, &r.creditedUsedAmount, &r.commissionTotalCNY, &r.unlockedCNY); err != nil {
			return err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	remaining := decimal.NewFromFloat(ordinaryUsageAmount)
	for _, rec := range records {
		if remaining.LessThanOrEqual(decimal.Zero) {
			break
		}

		orderCredited := decimal.NewFromFloat(rec.orderCreditedAmount)
		alreadyUsed := decimal.NewFromFloat(rec.creditedUsedAmount)
		available := orderCredited.Sub(alreadyUsed)
		if available.LessThanOrEqual(decimal.Zero) || orderCredited.LessThanOrEqual(decimal.Zero) {
			continue
		}

		allocated := remaining
		if available.LessThan(allocated) {
			allocated = available
		}
		newUsed := alreadyUsed.Add(allocated).Round(8)
		totalCommission := decimal.NewFromFloat(rec.commissionTotalCNY)

		// spec §6.5：commission_total=0 表示该记录所在月还未达门槛。
		// 此时只推进 credited_used_amount（保留比例信息以便跨门槛 reprice 时
		// 按已用比例补算 unlocked），不动 unlocked / status。
		if !totalCommission.IsPositive() {
			if _, err := tx.ExecContext(ctx, `
				UPDATE sales_commission_records
				SET credited_used_amount = $1, updated_at = NOW()
				WHERE id = $2
			`, newUsed.InexactFloat64(), rec.id); err != nil {
				return err
			}
			remaining = remaining.Sub(allocated)
			continue
		}

		unlockDelta := allocated.Div(orderCredited).Mul(totalCommission).Round(2)
		newUnlocked := decimal.NewFromFloat(rec.unlockedCNY).Add(unlockDelta).Round(2)
		if newUsed.GreaterThanOrEqual(orderCredited) {
			newUsed = orderCredited.Round(8)
			newUnlocked = totalCommission.Round(2)
		}

		status := service.SalesCommissionStatusPartialUnlocked
		if newUnlocked.GreaterThanOrEqual(totalCommission) {
			status = service.SalesCommissionStatusUnlocked
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE sales_commission_records
			SET credited_used_amount = $1, unlocked_cny = $2, status = $3, updated_at = NOW()
			WHERE id = $4
		`, newUsed.InexactFloat64(), newUnlocked.InexactFloat64(), status, rec.id); err != nil {
			return err
		}
		remaining = remaining.Sub(allocated)
	}
	return nil
}

// deductGiftBalanceFIFO 在事务内按 FIFO 扣减赠送余额，返回实际扣减金额
func deductGiftBalanceFIFO(ctx context.Context, tx *sql.Tx, userID int64, amount float64) float64 {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, remaining FROM gift_balance_records
		 WHERE user_id = $1 AND remaining > 0 AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY COALESCE(expires_at, '2099-12-31'::timestamptz) ASC, id ASC FOR UPDATE`, userID)
	if err != nil {
		return 0
	}
	defer rows.Close()

	var totalDeducted float64
	remaining := amount
	type record struct {
		id        int64
		remaining float64
	}
	var records []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.remaining); err != nil {
			return totalDeducted
		}
		records = append(records, r)
	}
	rows.Close()

	for _, rec := range records {
		if remaining <= 0 {
			break
		}
		deduct := rec.remaining
		if deduct > remaining {
			deduct = remaining
		}
		newRemaining := rec.remaining - deduct
		if _, err := tx.ExecContext(ctx,
			`UPDATE gift_balance_records SET remaining = $1, updated_at = NOW() WHERE id = $2`,
			newRemaining, rec.id); err != nil {
			return totalDeducted
		}
		totalDeducted += deduct
		remaining -= deduct
	}
	return totalDeducted
}

func incrementUsageBillingAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, amount float64) (bool, error) {
	var exhausted bool
	err := tx.QueryRowContext(ctx, `
		UPDATE api_keys
		SET quota_used = quota_used + $1,
			status = CASE
				WHEN quota > 0
					AND status = $3
					AND quota_used < quota
					AND quota_used + $1 >= quota
				THEN $4
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING quota > 0 AND quota_used >= quota AND quota_used - $1 < quota
	`, amount, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).Scan(&exhausted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return false, err
	}
	return exhausted, nil
}

func incrementUsageBillingAPIKeyRateLimit(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN $1 ELSE usage_5h + $1 END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN $1 ELSE usage_1d + $1 END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN $1 ELSE usage_7d + $1 END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, cost, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func incrementUsageBillingAccountQuota(ctx context.Context, tx *sql.Tx, accountID int64, amount float64) (*service.AccountQuotaState, error) {
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
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING
			COALESCE((extra->>'quota_used')::numeric, 0),
			COALESCE((extra->>'quota_limit')::numeric, 0),
			COALESCE((extra->>'quota_daily_used')::numeric, 0),
			COALESCE((extra->>'quota_daily_limit')::numeric, 0),
			COALESCE((extra->>'quota_weekly_used')::numeric, 0),
			COALESCE((extra->>'quota_weekly_limit')::numeric, 0)`,
		amount, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var state service.AccountQuotaState
	if rows.Next() {
		if err := rows.Scan(
			&state.TotalUsed, &state.TotalLimit,
			&state.DailyUsed, &state.DailyLimit,
			&state.WeeklyUsed, &state.WeeklyLimit,
		); err != nil {
			return nil, err
		}
	} else {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrAccountNotFound
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if state.TotalLimit > 0 && state.TotalUsed >= state.TotalLimit && (state.TotalUsed-amount) < state.TotalLimit {
		if err := enqueueSchedulerOutbox(ctx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
			return nil, err
		}
	}
	return &state, nil
}
