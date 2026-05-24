package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type salesCommissionRepository struct {
	db *sql.DB
}

type salesCommissionMonthlySnapshot struct {
	ID                  int64
	CommissionMode      string
	FixedCommissionRate float64
	MinMonthlySalesCNY  float64
	Tiers               []service.SalesCommissionTier
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
	if input.CommissionEventAt.IsZero() {
		return errors.New("sales commission create input missing commission_event_at")
	}
	if input.CommissionMonth.IsZero() {
		return errors.New("sales commission create input missing commission_month")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var lockedUserID int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM users
		WHERE id = $1
		FOR UPDATE
	`, input.SalesUserID).Scan(&lockedUserID); err != nil {
		return err
	}

	snapshot, err := ensureSalesCommissionMonthlySnapshot(ctx, tx, input)
	if err != nil {
		return err
	}

	// spec §6.4：插入当前事件的基础字段，金额/比例/解锁先置 0；后续整月重算会回写。
	// 手动完成（无 payment_order_id）按照 spec §6.8 把 credited_used_amount 初始化为
	// order_credited_amount，使其在重算时直接 unlocked = commission_total。
	// ON CONFLICT 显式针对 payment_order_id 的部分唯一索引（migration 113），
	// 避免静默吞掉其它约束冲突。
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sales_commission_records (
			sales_user_id, referee_user_id, referral_id, payment_order_id,
			order_pay_amount_cny, order_credited_amount, commission_mode, commission_rate, commission_total_cny,
			commission_month, commission_event_at, snapshot_id, monthly_sales_before_cny, monthly_sales_after_cny,
			credited_used_amount, unlocked_cny, status, note, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4::bigint,
			$5, $6, $7, 0, 0,
			$8, $9, $10, 0, 0,
			CASE WHEN $4::bigint IS NULL THEN $6 ELSE 0::numeric END,
			0,
			$11, $12, NOW(), NOW()
		)
		ON CONFLICT (payment_order_id) WHERE payment_order_id IS NOT NULL DO NOTHING
	`,
		input.SalesUserID,
		input.RefereeUserID,
		input.ReferralID,
		input.PaymentOrderID,
		cny(input.OrderPayAmountCNY),
		decimal.NewFromFloat(input.OrderCreditedAmount),
		snapshot.CommissionMode,
		input.CommissionMonth,
		input.CommissionEventAt,
		snapshot.ID,
		service.SalesCommissionStatusFrozen,
		input.Note,
	); err != nil {
		return err
	}

	// 始终触发整月重算：即便 INSERT 因 ON CONFLICT 静默跳过（重复 payment_order_id），
	// reprice 仍是幂等的——这能修复 "首次 INSERT 成功但 reprice 失败回滚后再重试时
	// 只走 ON CONFLICT 跳过路径，导致整月数据停留在 0" 的场景。
	if err := repriceMonthlyCommissionRecordsTx(ctx, tx, snapshot, input.SalesUserID, input.CommissionMonth); err != nil {
		return err
	}

	return tx.Commit()
}

// repriceMonthlyCommissionRecordsTx 在事务内对 (salesUserID, commissionMonth) 当月所有
// sales_commission_records 执行 spec §6.5 / §6.6 的整月累进重算。
//
// 行为：
//   - 锁住当月所有记录（FOR UPDATE）按 (commission_event_at ASC, id ASC) 顺序读取。
//   - 调用 service.RecomputeMonthlyCommissionRecords 算出每条记录新的金额。
//   - 批量 UPDATE：commission_total_cny / commission_rate /
//     monthly_sales_before_cny / monthly_sales_after_cny / unlocked_cny / status。
//   - settlement_blocked 状态保留不变；其他 status 按 commission_total / unlocked / settled
//     关系重新推断，与现有 settlement 路径的 CASE 表达保持一致。
func repriceMonthlyCommissionRecordsTx(ctx context.Context, tx *sql.Tx, snapshot *salesCommissionMonthlySnapshot, salesUserID int64, commissionMonth time.Time) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, order_pay_amount_cny, order_credited_amount, credited_used_amount, payment_order_id
		FROM sales_commission_records
		WHERE sales_user_id = $1
		  AND commission_month = $2
		ORDER BY commission_event_at ASC, id ASC
		FOR UPDATE
	`, salesUserID, commissionMonth)
	if err != nil {
		return err
	}

	type repriceRow struct {
		id              int64
		hasPaymentOrder bool
	}

	var ordered []repriceRow
	var inputs []service.SalesCommissionMonthlyRecordInput
	for rows.Next() {
		var (
			id             int64
			payAmount      decimal.Decimal
			creditedAmount decimal.Decimal
			creditedUsed   decimal.Decimal
			paymentOrderID sql.NullInt64
		)
		if err := rows.Scan(&id, &payAmount, &creditedAmount, &creditedUsed, &paymentOrderID); err != nil {
			_ = rows.Close()
			return err
		}
		ordered = append(ordered, repriceRow{
			id:              id,
			hasPaymentOrder: paymentOrderID.Valid,
		})
		inputs = append(inputs, service.SalesCommissionMonthlyRecordInput{
			OrderPayAmountCNY:   decimalFloat(payAmount),
			OrderCreditedAmount: decimalFloat(creditedAmount),
			CreditedUsedAmount:  decimalFloat(creditedUsed),
			HasPaymentOrder:     paymentOrderID.Valid,
		})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(inputs) == 0 {
		return nil
	}

	results, err := service.RecomputeMonthlyCommissionRecords(inputs, service.SalesCommissionSnapshot{
		CommissionMode:      snapshot.CommissionMode,
		FixedCommissionRate: snapshot.FixedCommissionRate,
		MinMonthlySalesCNY:  snapshot.MinMonthlySalesCNY,
		Tiers:               snapshot.Tiers,
	})
	if err != nil {
		return err
	}

	// status 推断与 settlement 路径保持一致；额外保留 settlement_blocked，
	// 并在 commission_total <= 0 时强制 frozen，避免被 settled_cny=0 视作 settled。
	const updateSQL = `
		UPDATE sales_commission_records
		SET commission_total_cny = $1,
		    commission_rate = $2,
		    monthly_sales_before_cny = $3,
		    monthly_sales_after_cny = $4,
		    unlocked_cny = $5,
		    status = CASE
		        WHEN status = $6 THEN $6
		        WHEN $1 <= 0::numeric THEN $7
		        WHEN settled_cny >= $1 THEN $8
		        WHEN $5 >= $1 THEN $9
		        WHEN $5 > 0::numeric THEN $10
		        ELSE $7
		    END,
		    updated_at = NOW()
		WHERE id = $11
	`
	for i, row := range ordered {
		result := results[i]
		if _, err := tx.ExecContext(ctx, updateSQL,
			cny(result.CommissionTotalCNY),
			decimal.NewFromFloat(result.CommissionRate).Round(4),
			cny(result.MonthlySalesBeforeCNY),
			cny(result.MonthlySalesAfterCNY),
			cny(result.UnlockedCNY),
			service.SalesCommissionStatusSettlementBlocked,
			service.SalesCommissionStatusFrozen,
			service.SalesCommissionStatusSettled,
			service.SalesCommissionStatusUnlocked,
			service.SalesCommissionStatusPartialUnlocked,
			row.id,
		); err != nil {
			return err
		}
	}
	return nil
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

	// 排序方向由 service.NormalizeSalesCommissionRecordSortOrder 兜底成合法常量；这里直接
	// 拼接 SQL 关键字而不是占位符（PostgreSQL 不接受 ORDER BY 用 $N 占位）。
	// 第二排序键用同方向的 scr.id：当 created_at 在毫秒粒度内冲突时仍能给出稳定顺序，
	// 避免分页 OFFSET 时同一行漏出或重复。
	sortDir := "DESC"
	if service.NormalizeSalesCommissionRecordSortOrder(params.SortOrder) == service.SalesCommissionRecordSortAsc {
		sortDir = "ASC"
	}

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
			scr.commission_event_at,
			scr.commission_month,
			scr.snapshot_id,
			scr.commission_mode,
			scr.monthly_sales_before_cny,
			scr.monthly_sales_after_cny,
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
		ORDER BY scr.created_at %s, scr.id %s
		LIMIT $%d OFFSET $%d
	`, completedStatusArg, blockedStatusArg, where, sortDir, sortDir, len(queryArgs)-1, len(queryArgs))

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

func (r *salesCommissionRepository) CreateSettlement(ctx context.Context, input *service.SalesCommissionSettlementCreate) (*service.SalesCommissionSettlement, error) {
	if input == nil {
		return nil, errors.New("nil settlement create input")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. 事务内锁定并查出实际可结算总额（FOR UPDATE 防止并发结算）
	//    注意：PostgreSQL 不允许 FOR UPDATE 与聚合函数并用，
	//    所以先在子查询中锁行，外层再做 SUM。
	var actualSettleable decimal.Decimal
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
			CASE WHEN (locked.payment_order_id IS NULL OR po.status = $2) AND locked.status <> $3
			     THEN locked.unlocked_cny - locked.settled_cny
			     ELSE 0
			END
		), 0)
		FROM (
			SELECT id, payment_order_id, status, unlocked_cny, settled_cny
			FROM sales_commission_records
			WHERE sales_user_id = $1
			FOR UPDATE
		) locked
		LEFT JOIN payment_orders po ON po.id = locked.payment_order_id
	`, input.SalesUserID, payment.OrderStatusCompleted, service.SalesCommissionStatusSettlementBlocked).Scan(&actualSettleable)
	if err != nil {
		return nil, err
	}

	actualSettleableF := decimalFloat(actualSettleable)
	if actualSettleableF <= 0 {
		return nil, service.ErrSalesCommissionNoSettleable
	}
	if input.AmountCNY > actualSettleableF {
		return nil, service.ErrSalesCommissionSettleAmountExceeds
	}

	// 2. 按入参金额分配结算到各条记录（按创建时间从早到晚逐条消耗）
	//    settleable = unlocked_cny - settled_cny（计算值，非物理列）
	//    frozen = commission_total_cny - unlocked_cny（计算值，非物理列）
	_, err = tx.ExecContext(ctx, `
		WITH ordered AS (
			SELECT scr.id,
			       scr.unlocked_cny - scr.settled_cny AS available,
			       SUM(scr.unlocked_cny - scr.settled_cny) OVER (ORDER BY scr.created_at, scr.id) AS cumulative
			FROM sales_commission_records scr
			LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
			WHERE scr.sales_user_id = $1
			  AND scr.status <> $2
			  AND (scr.payment_order_id IS NULL OR po.status = $3)
			  AND scr.unlocked_cny > scr.settled_cny
		),
		allocation AS (
			SELECT id,
			       LEAST(available, GREATEST($4 - (cumulative - available), 0)) AS to_settle
			FROM ordered
			WHERE cumulative - available < $4
		)
		UPDATE sales_commission_records scr
		SET settled_cny = scr.settled_cny + a.to_settle,
		    status = CASE
		        WHEN (scr.settled_cny + a.to_settle) >= scr.unlocked_cny
		             AND scr.commission_total_cny = scr.unlocked_cny THEN 'settled'
		        ELSE scr.status
		    END,
		    updated_at = NOW()
		FROM allocation a
		WHERE scr.id = a.id
	`, input.SalesUserID, service.SalesCommissionStatusSettlementBlocked, payment.OrderStatusCompleted, input.AmountCNY)
	if err != nil {
		return nil, err
	}

	// 3. 创建结算记录（金额使用入参，已通过上面的校验）
	var settlement service.SalesCommissionSettlement
	err = tx.QueryRowContext(ctx, `
		INSERT INTO sales_commission_settlements (sales_user_id, amount_cny, note, created_by, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, sales_user_id, amount_cny, note, created_by, created_at
	`, input.SalesUserID, input.AmountCNY, input.Note, input.CreatedBy).Scan(
		&settlement.ID, &settlement.SalesUserID, &settlement.AmountCNY, &settlement.Note, &settlement.CreatedBy, &settlement.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &settlement, nil
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
	var orderPay, orderCredited, rate, monthlyBefore, monthlyAfter, total, creditedUsed, frozen, unlocked, settled, settleable decimal.Decimal
	var paymentOrderID sql.NullInt64
	var paymentOrderStatus sql.NullString
	var snapshotID sql.NullInt64
	var commissionEventAt sql.NullTime
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
		&commissionEventAt,
		&record.CommissionMonth,
		&snapshotID,
		&record.CommissionMode,
		&monthlyBefore,
		&monthlyAfter,
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
	if commissionEventAt.Valid {
		record.CommissionEventAt = commissionEventAt.Time
	}
	if snapshotID.Valid {
		record.SnapshotID = &snapshotID.Int64
	}
	record.OrderPayAmountCNY = decimalFloat(orderPay)
	record.OrderCreditedAmount = decimalFloat(orderCredited)
	record.CommissionRate = decimalFloat(rate)
	record.MonthlySalesBeforeCNY = decimalFloat(monthlyBefore)
	record.MonthlySalesAfterCNY = decimalFloat(monthlyAfter)
	record.CommissionTotalCNY = decimalFloat(total)
	record.CreditedUsedAmount = decimalFloat(creditedUsed)
	record.FrozenCNY = decimalFloat(frozen)
	record.UnlockedCNY = decimalFloat(unlocked)
	record.SettledCNY = decimalFloat(settled)
	record.SettleableCNY = decimalFloat(settleable)
	return record, nil
}

func ensureSalesCommissionMonthlySnapshot(ctx context.Context, tx *sql.Tx, input *service.SalesCommissionCreate) (*salesCommissionMonthlySnapshot, error) {
	snapshot, err := loadSalesCommissionMonthlySnapshot(ctx, tx, input.SalesUserID, input.CommissionMonth)
	if err == nil {
		return snapshot, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	tiers := service.CloneSalesCommissionTiers(input.CommissionTiers)
	tiersJSON, err := json.Marshal(tiers)
	if err != nil {
		return nil, err
	}
	mode := service.NormalizeSalesCommissionMode(input.CommissionMode)
	// spec §5.1：tiered 模式允许 user.sales_commission_rate=0，且该字段不参与梯度计算。
	// 写 snapshot 时强制 0，避免用户切换到梯度模式后仍把旧 fixed rate 留在快照里被误读。
	fixedRate := input.CommissionRate
	if mode == service.SalesCommissionModeTiered {
		fixedRate = 0
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO sales_commission_monthly_snapshots (
			sales_user_id, commission_month, timezone, commission_mode,
			fixed_commission_rate, min_monthly_sales_cny, tiers_json, created_at, updated_at
		)
		VALUES ($1::bigint, $2::date, $3::varchar, $4::varchar, $5::numeric, $6::numeric, $7::jsonb, NOW(), NOW())
		ON CONFLICT (sales_user_id, commission_month) DO NOTHING
	`,
		input.SalesUserID,
		input.CommissionMonth,
		"Asia/Shanghai",
		mode,
		decimal.NewFromFloat(fixedRate).Round(4),
		cny(input.CommissionMinMonthlySales),
		string(tiersJSON),
	)
	if err != nil {
		return nil, err
	}
	return loadSalesCommissionMonthlySnapshot(ctx, tx, input.SalesUserID, input.CommissionMonth)
}

// GetMonthlyProgress 拉取销售用户当月梯度进度的原始数据，由 service 层做派生。
//
//   - snapshot 不存在 → Snapshot=nil，service 层用 user 当前规则做 "预期" 展示。
//   - records 当月聚合用 SUM(order_pay_amount_cny) / SUM(commission_total_cny)，
//     与 ListSummaries / GetSummaryBySalesUser 的口径一致。
func (r *salesCommissionRepository) GetMonthlyProgress(ctx context.Context, salesUserID int64, commissionMonth time.Time) (*service.SalesCommissionMonthlyProgressData, error) {
	data := &service.SalesCommissionMonthlyProgressData{}

	var (
		snapshotID                 int64
		commissionMode             string
		fixedRate, minMonthlySales decimal.Decimal
		tiersJSON                  []byte
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, commission_mode, fixed_commission_rate, min_monthly_sales_cny, tiers_json
		FROM sales_commission_monthly_snapshots
		WHERE sales_user_id = $1
		  AND commission_month = $2
	`, salesUserID, commissionMonth).Scan(&snapshotID, &commissionMode, &fixedRate, &minMonthlySales, &tiersJSON)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// 当月还没有 snapshot —— 由 service 层 fallback 到 user 现行规则。
	case err != nil:
		return nil, err
	default:
		snap := &service.SalesCommissionSnapshot{
			CommissionMode:      service.NormalizeSalesCommissionMode(commissionMode),
			FixedCommissionRate: decimalFloat(fixedRate),
			MinMonthlySalesCNY:  decimalFloat(minMonthlySales),
		}
		if len(tiersJSON) > 0 {
			if err := json.Unmarshal(tiersJSON, &snap.Tiers); err != nil {
				return nil, err
			}
		}
		data.Snapshot = snap
	}

	var monthlySales, monthlyCommission decimal.Decimal
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(order_pay_amount_cny), 0),
			COALESCE(SUM(commission_total_cny), 0)
		FROM sales_commission_records
		WHERE sales_user_id = $1
		  AND commission_month = $2
	`, salesUserID, commissionMonth).Scan(&monthlySales, &monthlyCommission); err != nil {
		return nil, err
	}
	data.MonthlySalesCNY = decimalFloat(monthlySales)
	data.MonthlyCommissionCNY = decimalFloat(monthlyCommission)
	return data, nil
}

func loadSalesCommissionMonthlySnapshot(ctx context.Context, tx *sql.Tx, salesUserID int64, commissionMonth any) (*salesCommissionMonthlySnapshot, error) {
	var snapshot salesCommissionMonthlySnapshot
	var fixedRate, minMonthlySales decimal.Decimal
	var tiersJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT id, commission_mode, fixed_commission_rate, min_monthly_sales_cny, tiers_json
		FROM sales_commission_monthly_snapshots
		WHERE sales_user_id = $1
		  AND commission_month = $2
	`, salesUserID, commissionMonth).Scan(
		&snapshot.ID,
		&snapshot.CommissionMode,
		&fixedRate,
		&minMonthlySales,
		&tiersJSON,
	)
	if err != nil {
		return nil, err
	}
	snapshot.CommissionMode = service.NormalizeSalesCommissionMode(snapshot.CommissionMode)
	snapshot.FixedCommissionRate = decimalFloat(fixedRate)
	snapshot.MinMonthlySalesCNY = decimalFloat(minMonthlySales)
	if len(tiersJSON) > 0 {
		if err := json.Unmarshal(tiersJSON, &snapshot.Tiers); err != nil {
			return nil, err
		}
	}
	return &snapshot, nil
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

// GetOverview 拉取 admin 端数据看板所需的全部数据（spec §15.3）。
//
//   - KPI / status_breakdown / mode_breakdown 通过单条聚合 SQL 一次返回。
//   - threshold_met_users 只统计 tiered 模式下且 min_monthly_sales > 0 的销售用户，
//     fixed 模式没有 "门槛" 概念，避免被误算成 "全部活跃销售都达门槛"。
//   - monthly_trend 按 commission_month 聚合 [MonthlyTrendStart, MonthlyTrendEnd]，
//     未命中的月份由 service 层补零。
//   - top_sales 按区间内 commission_total_cny 倒序 Top10。
func (r *salesCommissionRepository) GetOverview(ctx context.Context, query service.SalesCommissionOverviewQuery) (*service.SalesCommissionOverviewData, error) {
	data := &service.SalesCommissionOverviewData{}

	// 1) KPI + status_breakdown + mode_breakdown 单次聚合
	var (
		relatedOrder, commissionTotal, frozen, settleable, settled decimal.Decimal
		fixedCommission, tieredCommission                          decimal.Decimal
		activeUsers, fixedRecords, tieredRecords                   int
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(scr.order_pay_amount_cny), 0),
			COALESCE(SUM(scr.commission_total_cny), 0),
			COALESCE(SUM(scr.commission_total_cny - scr.unlocked_cny), 0),
			COALESCE(SUM(CASE WHEN (scr.payment_order_id IS NULL OR po.status = $3)
			                       AND scr.status <> $4
			                  THEN scr.unlocked_cny - scr.settled_cny ELSE 0 END), 0),
			COALESCE(SUM(scr.settled_cny), 0),
			COUNT(DISTINCT scr.sales_user_id),
			COALESCE(SUM(CASE WHEN scr.commission_mode = $5 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN scr.commission_mode = $6 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN scr.commission_mode = $5 THEN scr.commission_total_cny ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN scr.commission_mode = $6 THEN scr.commission_total_cny ELSE 0 END), 0)
		FROM sales_commission_records scr
		LEFT JOIN payment_orders po ON po.id = scr.payment_order_id
		WHERE scr.commission_event_at >= $1 AND scr.commission_event_at <= $2
	`,
		query.Start, query.End,
		payment.OrderStatusCompleted,
		service.SalesCommissionStatusSettlementBlocked,
		service.SalesCommissionModeFixed,
		service.SalesCommissionModeTiered,
	).Scan(
		&relatedOrder, &commissionTotal, &frozen, &settleable, &settled,
		&activeUsers, &fixedRecords, &tieredRecords, &fixedCommission, &tieredCommission,
	)
	if err != nil {
		return nil, err
	}
	data.KPI.RelatedOrderAmountCNY = decimalFloat(relatedOrder)
	data.KPI.CommissionTotalCNY = decimalFloat(commissionTotal)
	data.KPI.FrozenCNY = decimalFloat(frozen)
	data.KPI.SettleableCNY = decimalFloat(settleable)
	data.KPI.SettledCNY = decimalFloat(settled)
	data.KPI.ActiveSalesUsers = activeUsers
	data.StatusBreakdown.FrozenCNY = data.KPI.FrozenCNY
	data.StatusBreakdown.SettleableCNY = data.KPI.SettleableCNY
	data.StatusBreakdown.SettledCNY = data.KPI.SettledCNY
	data.ModeBreakdown.FixedRecords = fixedRecords
	data.ModeBreakdown.TieredRecords = tieredRecords
	data.ModeBreakdown.FixedCommissionCNY = decimalFloat(fixedCommission)
	data.ModeBreakdown.TieredCommissionCNY = decimalFloat(tieredCommission)

	// 2) threshold_met_users
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT scr.sales_user_id)
		FROM sales_commission_records scr
		JOIN sales_commission_monthly_snapshots s
		  ON s.sales_user_id = scr.sales_user_id AND s.commission_month = scr.commission_month
		WHERE scr.commission_event_at >= $1 AND scr.commission_event_at <= $2
		  AND s.commission_mode = $3
		  AND s.min_monthly_sales_cny > 0
		  AND scr.monthly_sales_after_cny >= s.min_monthly_sales_cny
	`, query.Start, query.End, service.SalesCommissionModeTiered).Scan(&data.KPI.ThresholdMetUsers); err != nil {
		return nil, err
	}

	// avg_commission_rate（在 SQL 之外计算，避免除零）
	if data.KPI.RelatedOrderAmountCNY > 0 {
		data.KPI.AvgCommissionRate = data.KPI.CommissionTotalCNY / data.KPI.RelatedOrderAmountCNY * 100
	}

	// 3) monthly_trend：按 commission_month 桶聚合（窗口由 service 决定）
	monthlyRows, err := r.db.QueryContext(ctx, `
		SELECT scr.commission_month,
		       COALESCE(SUM(scr.order_pay_amount_cny), 0),
		       COALESCE(SUM(scr.commission_total_cny), 0)
		FROM sales_commission_records scr
		WHERE scr.commission_month >= $1 AND scr.commission_month <= $2
		GROUP BY scr.commission_month
		ORDER BY scr.commission_month ASC
	`, query.MonthlyTrendStart, query.MonthlyTrendEnd)
	if err != nil {
		return nil, err
	}
	defer monthlyRows.Close()
	for monthlyRows.Next() {
		var (
			month        time.Time
			relatedMonth decimal.Decimal
			commMonth    decimal.Decimal
		)
		if err := monthlyRows.Scan(&month, &relatedMonth, &commMonth); err != nil {
			return nil, err
		}
		data.MonthlyTrend = append(data.MonthlyTrend, service.SalesCommissionMonthlyTrend{
			Month:                 month,
			RelatedOrderAmountCNY: decimalFloat(relatedMonth),
			CommissionTotalCNY:    decimalFloat(commMonth),
		})
	}
	if err := monthlyRows.Err(); err != nil {
		return nil, err
	}

	// 4) top_sales Top 10
	topRows, err := r.db.QueryContext(ctx, `
		SELECT scr.sales_user_id,
		       COALESCE(u.email, ''),
		       COALESCE(u.username, ''),
		       COALESCE(SUM(scr.order_pay_amount_cny), 0),
		       COALESCE(SUM(scr.commission_total_cny), 0)
		FROM sales_commission_records scr
		JOIN users u ON u.id = scr.sales_user_id
		WHERE scr.commission_event_at >= $1 AND scr.commission_event_at <= $2
		GROUP BY scr.sales_user_id, u.email, u.username
		ORDER BY SUM(scr.commission_total_cny) DESC, scr.sales_user_id ASC
		LIMIT 10
	`, query.Start, query.End)
	if err != nil {
		return nil, err
	}
	defer topRows.Close()
	for topRows.Next() {
		var (
			item     service.SalesCommissionTopSales
			ordersum decimal.Decimal
			commsum  decimal.Decimal
		)
		if err := topRows.Scan(&item.SalesUserID, &item.SalesEmail, &item.SalesUsername, &ordersum, &commsum); err != nil {
			return nil, err
		}
		item.RelatedOrderAmountCNY = decimalFloat(ordersum)
		item.CommissionTotalCNY = decimalFloat(commsum)
		data.TopSales = append(data.TopSales, item)
	}
	if err := topRows.Err(); err != nil {
		return nil, err
	}

	return data, nil
}

// ListMissingCommissionPaymentOrders 实现 SalesCommissionRepository 同名接口。
//
// 仅选取 service.HandleBalanceRechargeCompleted 入参所需字段，避免拉无关列。
// 排序按 po.id ASC，确保多次调用结果稳定且 limit 截断有意义。
func (r *salesCommissionRepository) ListMissingCommissionPaymentOrders(ctx context.Context, limit int) ([]*dbent.PaymentOrder, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			po.id,
			po.user_id,
			po.order_type,
			po.status,
			po.pay_amount,
			po.amount,
			po.paid_at,
			po.completed_at,
			po.created_at
		FROM payment_orders po
		WHERE po.status = $1
		  AND po.order_type = $2
		  AND po.pay_amount > 0
		  AND po.amount > 0
		  AND NOT EXISTS (
		    SELECT 1
		    FROM sales_commission_records r
		    WHERE r.payment_order_id = po.id
		  )
		ORDER BY po.id ASC
		LIMIT $3
	`, payment.OrderStatusCompleted, payment.OrderTypeBalance, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*dbent.PaymentOrder
	for rows.Next() {
		var (
			po          dbent.PaymentOrder
			paidAt      sql.NullTime
			completedAt sql.NullTime
		)
		if err := rows.Scan(
			&po.ID,
			&po.UserID,
			&po.OrderType,
			&po.Status,
			&po.PayAmount,
			&po.Amount,
			&paidAt,
			&completedAt,
			&po.CreatedAt,
		); err != nil {
			return nil, err
		}
		if paidAt.Valid {
			t := paidAt.Time
			po.PaidAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			po.CompletedAt = &t
		}
		orders = append(orders, &po)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orders, nil
}
