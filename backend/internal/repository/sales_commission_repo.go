package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type salesCommissionRepository struct {
	db *sql.DB
}

func NewSalesCommissionRepository(sqlDB *sql.DB) service.SalesCommissionRepository {
	return &salesCommissionRepository{db: sqlDB}
}

func salesPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func (r *salesCommissionRepository) CreateForOrder(ctx context.Context, input *service.SalesCommissionCreate) error {
	if input == nil {
		return errors.New("nil sales commission create input")
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sales_commission_records (
			sales_user_id, referee_user_id, referral_id, payment_order_id,
			order_pay_amount_cny, order_credited_amount, commission_rate, commission_total_cny,
			credited_used_amount, unlocked_cny, status, note, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			CASE WHEN $4 IS NULL THEN $6 ELSE 0 END,
			CASE WHEN $4 IS NULL THEN $8 ELSE 0 END,
			CASE WHEN $4 IS NULL THEN $10 ELSE $11 END,
			$9, NOW(), NOW()
		)
		ON CONFLICT DO NOTHING
	`,
		input.SalesUserID,
		input.RefereeUserID,
		input.ReferralID,
		input.PaymentOrderID,
		cny(input.OrderPayAmountCNY),
		decimal.NewFromFloat(input.OrderCreditedAmount),
		decimal.NewFromFloat(input.CommissionRate),
		cny(input.CommissionTotalCNY),
		input.Note,
		service.SalesCommissionStatusUnlocked,
		service.SalesCommissionStatusFrozen,
	)
	return err
}

func (r *salesCommissionRepository) ListSummaries(ctx context.Context, params service.SalesCommissionSummaryListParams) ([]service.SalesCommissionSummary, int, error) {
	page, pageSize := salesPage(params.Page, params.PageSize)
	offset := (page - 1) * pageSize

	where := "WHERE TRUE"
	args := []any{}
	if strings.TrimSpace(params.Search) != "" {
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(params.Search))+"%")
		where += fmt.Sprintf(" AND (LOWER(u.email) LIKE $%d OR LOWER(u.username) LIKE $%d)", len(args), len(args))
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM (
			SELECT scr.sales_user_id
			FROM sales_commission_records scr
			JOIN users u ON u.id = scr.sales_user_id
			%s
			GROUP BY scr.sales_user_id
		) grouped
	`, where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	completedStatusArg := len(queryArgs) + 1
	blockedStatusArg := len(queryArgs) + 2
	queryArgs = append(queryArgs, payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT
			scr.sales_user_id,
			COALESCE(u.email, ''),
			COALESCE(u.username, ''),
			COALESCE(SUM(scr.commission_total_cny), 0),
			COALESCE(SUM(scr.commission_total_cny - scr.unlocked_cny), 0),
			COALESCE(SUM(scr.unlocked_cny), 0),
			COALESCE(SUM(CASE WHEN (scr.payment_order_id IS NULL OR po.status = $%d) AND scr.status <> $%d THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END), 0),
			COALESCE(SUM(scr.settled_cny), 0),
			COUNT(*)
		FROM sales_commission_records scr
		JOIN users u ON u.id = scr.sales_user_id
		LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
		%s
		GROUP BY scr.sales_user_id, u.email, u.username
		ORDER BY scr.sales_user_id ASC
		LIMIT $%d OFFSET $%d
	`, completedStatusArg, blockedStatusArg, where, len(queryArgs)-1, len(queryArgs))

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	summaries := []service.SalesCommissionSummary{}
	for rows.Next() {
		var s service.SalesCommissionSummary
		var totalCNY, frozenCNY, unlockedCNY, settleableCNY, settledCNY decimal.Decimal
		if err := rows.Scan(&s.SalesUserID, &s.SalesEmail, &s.SalesUsername, &totalCNY, &frozenCNY, &unlockedCNY, &settleableCNY, &settledCNY, &s.RecordsCount); err != nil {
			return nil, 0, err
		}
		s.TotalCommissionCNY = decimalFloat(totalCNY)
		s.FrozenCNY = decimalFloat(frozenCNY)
		s.UnlockedCNY = decimalFloat(unlockedCNY)
		s.SettleableCNY = decimalFloat(settleableCNY)
		s.SettledCNY = decimalFloat(settledCNY)
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return summaries, total, nil
}

func (r *salesCommissionRepository) GetSummaryBySalesUser(ctx context.Context, salesUserID int64) (*service.SalesCommissionSummary, error) {
	var s service.SalesCommissionSummary
	var totalCNY, frozenCNY, unlockedCNY, settleableCNY, settledCNY decimal.Decimal
	err := r.db.QueryRowContext(ctx, `
		SELECT
			u.id,
			COALESCE(u.email, ''),
			COALESCE(u.username, ''),
			COALESCE(SUM(scr.commission_total_cny), 0),
			COALESCE(SUM(scr.commission_total_cny - scr.unlocked_cny), 0),
			COALESCE(SUM(scr.unlocked_cny), 0),
			COALESCE(SUM(CASE WHEN (scr.payment_order_id IS NULL OR po.status = $2) AND scr.status <> $3 THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END), 0),
			COALESCE(SUM(scr.settled_cny), 0),
			COUNT(scr.id)
		FROM users u
		LEFT JOIN sales_commission_records scr ON scr.sales_user_id = u.id
		LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
		WHERE u.id = $1
		GROUP BY u.id, u.email, u.username
	`, salesUserID, payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).Scan(&s.SalesUserID, &s.SalesEmail, &s.SalesUsername, &totalCNY, &frozenCNY, &unlockedCNY, &settleableCNY, &settledCNY, &s.RecordsCount)
	if err != nil {
		return nil, err
	}
	s.TotalCommissionCNY = decimalFloat(totalCNY)
	s.FrozenCNY = decimalFloat(frozenCNY)
	s.UnlockedCNY = decimalFloat(unlockedCNY)
	s.SettleableCNY = decimalFloat(settleableCNY)
	s.SettledCNY = decimalFloat(settledCNY)
	return &s, nil
}

func (r *salesCommissionRepository) ListRecords(ctx context.Context, params service.SalesCommissionRecordListParams) ([]service.SalesCommissionRecord, int, error) {
	page, pageSize := salesPage(params.Page, params.PageSize)
	offset := (page - 1) * pageSize

	where, args := salesRecordWhere(params)
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM sales_commission_records scr
		JOIN users su ON su.id = scr.sales_user_id
		JOIN users ru ON ru.id = scr.referee_user_id
		LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
		%s
	`, where)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	completedStatusArg := len(queryArgs) + 1
	blockedStatusArg := len(queryArgs) + 2
	queryArgs = append(queryArgs, payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked, pageSize, offset)
	query := fmt.Sprintf(`
		SELECT
			scr.id,
			scr.sales_user_id,
			COALESCE(su.email, ''),
			COALESCE(su.username, ''),
			scr.referee_user_id,
			COALESCE(ru.email, ''),
			COALESCE(ru.username, ''),
			scr.referral_id,
			scr.payment_order_id,
			po.status,
			scr.order_pay_amount_cny,
			scr.order_credited_amount,
			scr.commission_rate,
			scr.commission_total_cny,
			scr.credited_used_amount,
			scr.commission_total_cny - scr.unlocked_cny,
			scr.unlocked_cny,
			scr.settled_cny,
			CASE WHEN (scr.payment_order_id IS NULL OR po.status = $%d) AND scr.status <> $%d THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END,
			scr.status,
			scr.note,
			scr.created_at,
			scr.updated_at
		FROM sales_commission_records scr
		JOIN users su ON su.id = scr.sales_user_id
		JOIN users ru ON ru.id = scr.referee_user_id
		LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
		%s
		ORDER BY scr.id ASC
		LIMIT $%d OFFSET $%d
	`, completedStatusArg, blockedStatusArg, where, len(queryArgs)-1, len(queryArgs))

	rows, err := r.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	records := []service.SalesCommissionRecord{}
	for rows.Next() {
		record, err := scanSalesCommissionRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (r *salesCommissionRepository) CreateSettlement(ctx context.Context, input *service.SalesCommissionSettlementCreate) (*service.SalesCommissionSettlement, error) {
	if input == nil || input.AmountCNY <= 0 {
		return nil, service.ErrSalesCommissionInvalidAmount
	}
	requested := cny(input.AmountCNY)
	if !requested.IsPositive() {
		return nil, service.ErrSalesCommissionInvalidAmount
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT scr.id, scr.unlocked_cny, scr.settled_cny
		FROM sales_commission_records scr
		LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
		WHERE scr.sales_user_id = $1
		  AND (scr.payment_order_id IS NULL OR po.status = $2)
		  AND scr.status <> $3
		  AND scr.unlocked_cny > scr.settled_cny
		ORDER BY scr.id ASC
		FOR UPDATE
	`, input.SalesUserID, payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked)
	if err != nil {
		return nil, err
	}

	type settleableRecord struct {
		id        int64
		unlocked  decimal.Decimal
		settled   decimal.Decimal
		available decimal.Decimal
	}
	records := []settleableRecord{}
	total := decimal.Zero
	for rows.Next() {
		var record settleableRecord
		if err := rows.Scan(&record.id, &record.unlocked, &record.settled); err != nil {
			_ = rows.Close()
			return nil, err
		}
		record.available = record.unlocked.Sub(record.settled)
		total = total.Add(record.available)
		records = append(records, record)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if requested.GreaterThan(total) {
		return nil, service.ErrSalesCommissionSettleAmountExceeded
	}

	var settlement service.SalesCommissionSettlement
	var createdBy sql.NullInt64
	if input.CreatedBy != nil {
		createdBy = sql.NullInt64{Int64: *input.CreatedBy, Valid: true}
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO sales_commission_settlements (sales_user_id, amount_cny, note, created_by, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, sales_user_id, amount_cny, note, created_by, created_at
	`, input.SalesUserID, requested, input.Note, createdBy).Scan(
		&settlement.ID,
		&settlement.SalesUserID,
		(*decimalFloatScanner)(&settlement.AmountCNY),
		&settlement.Note,
		&createdBy,
		&settlement.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if createdBy.Valid {
		settlement.CreatedBy = &createdBy.Int64
	}

	remaining := requested
	for _, record := range records {
		if !remaining.IsPositive() {
			break
		}
		allocated := decimal.Min(remaining, record.available).Round(2)
		if !allocated.IsPositive() {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sales_commission_settlement_items (settlement_id, commission_record_id, amount_cny, created_at)
			VALUES ($1, $2, $3, NOW())
		`, settlement.ID, record.id, allocated); err != nil {
			return nil, err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE sales_commission_records
			SET settled_cny = settled_cny + $1,
			    status = CASE
			        WHEN settled_cny + $1 >= commission_total_cny THEN $2
			        WHEN unlocked_cny >= commission_total_cny THEN $3
			        WHEN unlocked_cny > 0 THEN $4
			        ELSE $5
			    END,
			    updated_at = NOW()
			WHERE id = $6
		`,
			allocated,
			service.SalesCommissionStatusSettled,
			service.SalesCommissionStatusUnlocked,
			service.SalesCommissionStatusPartialUnlocked,
			service.SalesCommissionStatusFrozen,
			record.id,
		); err != nil {
			return nil, err
		}
		remaining = remaining.Sub(allocated)
	}

	if remaining.GreaterThan(decimal.NewFromFloat(0.004)) {
		return nil, fmt.Errorf("settlement allocation remainder: %s", remaining.String())
	}

	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(email, '') FROM users WHERE id = $1
	`, settlement.SalesUserID).Scan(&settlement.SalesEmail); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &settlement, nil
}

func (r *salesCommissionRepository) ListSettlements(ctx context.Context, params service.SalesCommissionSettlementListParams) ([]service.SalesCommissionSettlement, int, error) {
	page, pageSize := salesPage(params.Page, params.PageSize)
	offset := (page - 1) * pageSize

	where := "WHERE TRUE"
	args := []any{}
	if params.SalesUserID > 0 {
		args = append(args, params.SalesUserID)
		where += fmt.Sprintf(" AND scs.sales_user_id = $%d", len(args))
	}

	var total int
	if err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM sales_commission_settlements scs
		JOIN users u ON u.id = scs.sales_user_id
		%s
	`, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT scs.id, scs.sales_user_id, COALESCE(u.email, ''), scs.amount_cny, scs.note, scs.created_by, scs.created_at
		FROM sales_commission_settlements scs
		JOIN users u ON u.id = scs.sales_user_id
		%s
		ORDER BY scs.id DESC
		LIMIT $%d OFFSET $%d
	`, where, len(queryArgs)-1, len(queryArgs)), queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	settlements := []service.SalesCommissionSettlement{}
	for rows.Next() {
		var settlement service.SalesCommissionSettlement
		var amount decimal.Decimal
		var createdBy sql.NullInt64
		if err := rows.Scan(&settlement.ID, &settlement.SalesUserID, &settlement.SalesEmail, &amount, &settlement.Note, &createdBy, &settlement.CreatedAt); err != nil {
			return nil, 0, err
		}
		settlement.AmountCNY = decimalFloat(amount)
		if createdBy.Valid {
			settlement.CreatedBy = &createdBy.Int64
		}
		settlements = append(settlements, settlement)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return settlements, total, nil
}

func salesRecordWhere(params service.SalesCommissionRecordListParams) (string, []any) {
	where := "WHERE TRUE"
	args := []any{}
	if params.SalesUserID > 0 {
		args = append(args, params.SalesUserID)
		where += fmt.Sprintf(" AND scr.sales_user_id = $%d", len(args))
	}
	if params.RefereeUserID > 0 {
		args = append(args, params.RefereeUserID)
		where += fmt.Sprintf(" AND scr.referee_user_id = $%d", len(args))
	}
	if params.PaymentOrderID > 0 {
		args = append(args, params.PaymentOrderID)
		where += fmt.Sprintf(" AND scr.payment_order_id = $%d", len(args))
	}
	if strings.TrimSpace(params.Status) != "" {
		args = append(args, strings.TrimSpace(params.Status))
		where += fmt.Sprintf(" AND scr.status = $%d", len(args))
	}
	return where, args
}

func scanSalesCommissionRecord(rows *sql.Rows) (service.SalesCommissionRecord, error) {
	var record service.SalesCommissionRecord
	var orderPay, orderCredited, rate, total, creditedUsed, frozen, unlocked, settled, settleable decimal.Decimal
	var paymentOrderID sql.NullInt64
	var paymentOrderStatus sql.NullString
	err := rows.Scan(
		&record.ID,
		&record.SalesUserID,
		&record.SalesEmail,
		&record.SalesUsername,
		&record.RefereeUserID,
		&record.RefereeEmail,
		&record.RefereeUsername,
		&record.ReferralID,
		&paymentOrderID,
		&paymentOrderStatus,
		&orderPay,
		&orderCredited,
		&rate,
		&total,
		&creditedUsed,
		&frozen,
		&unlocked,
		&settled,
		&settleable,
		&record.Status,
		&record.Note,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return record, err
	}
	if paymentOrderID.Valid {
		record.PaymentOrderID = &paymentOrderID.Int64
	}
	if paymentOrderStatus.Valid {
		record.PaymentOrderStatus = paymentOrderStatus.String
	}
	record.OrderPayAmountCNY = decimalFloat(orderPay)
	record.OrderCreditedAmount = decimalFloat(orderCredited)
	record.CommissionRate = decimalFloat(rate)
	record.CommissionTotalCNY = decimalFloat(total)
	record.CreditedUsedAmount = decimalFloat(creditedUsed)
	record.FrozenCNY = decimalFloat(frozen)
	record.UnlockedCNY = decimalFloat(unlocked)
	record.SettledCNY = decimalFloat(settled)
	record.SettleableCNY = decimalFloat(settleable)
	return record, nil
}

func cny(amount float64) decimal.Decimal {
	return decimal.NewFromFloat(amount).Round(2)
}

func decimalFloat(d decimal.Decimal) float64 {
	v, _ := d.Round(8).Float64()
	return v
}

type decimalFloatScanner float64

func (s *decimalFloatScanner) Scan(src any) error {
	var d decimal.Decimal
	if err := d.Scan(src); err != nil {
		return err
	}
	*s = decimalFloatScanner(decimalFloat(d))
	return nil
}
