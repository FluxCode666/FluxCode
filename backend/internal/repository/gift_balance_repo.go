package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type giftBalanceRepository struct {
	db *sql.DB
}

func NewGiftBalanceRepository(sqlDB *sql.DB) service.GiftBalanceRepository {
	return &giftBalanceRepository{db: sqlDB}
}

func (r *giftBalanceRepository) Create(ctx context.Context, record *service.GiftBalanceRecord) error {
	query := `INSERT INTO gift_balance_records (user_id, amount, remaining, source, source_ref_id, note, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW()) RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		record.UserID, record.Amount, record.Remaining, record.Source, record.SourceRefID, record.Note, record.ExpiresAt,
	).Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt)
}

func (r *giftBalanceRepository) GetByUserID(ctx context.Context, userID int64, offset, limit int) ([]service.GiftBalanceRecord, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gift_balance_records WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, amount, remaining, source, source_ref_id, note, expires_at, created_at, updated_at
		 FROM gift_balance_records WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var records []service.GiftBalanceRecord
	for rows.Next() {
		var rec service.GiftBalanceRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Amount, &rec.Remaining, &rec.Source, &rec.SourceRefID, &rec.Note, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, rec)
	}
	return records, total, rows.Err()
}

func (r *giftBalanceRepository) GetAvailableByUserID(ctx context.Context, userID int64) ([]service.GiftBalanceRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, amount, remaining, source, source_ref_id, note, expires_at, created_at, updated_at
		 FROM gift_balance_records
		 WHERE user_id = $1 AND remaining > 0 AND (expires_at IS NULL OR expires_at > NOW())
		 ORDER BY COALESCE(expires_at, '2099-12-31'::timestamptz) ASC, id ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []service.GiftBalanceRecord
	for rows.Next() {
		var rec service.GiftBalanceRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Amount, &rec.Remaining, &rec.Source, &rec.SourceRefID, &rec.Note, &rec.ExpiresAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *giftBalanceRepository) GetSummaryByUserID(ctx context.Context, userID int64) (*service.GiftBalanceSummary, error) {
	var summary service.GiftBalanceSummary
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE(SUM(amount), 0),
			COALESCE(SUM(CASE WHEN remaining > 0 AND (expires_at IS NULL OR expires_at > NOW()) THEN remaining ELSE 0 END), 0),
			COALESCE(SUM(amount - remaining), 0),
			COALESCE(SUM(CASE WHEN expires_at IS NOT NULL AND expires_at <= NOW() AND remaining > 0 THEN remaining ELSE 0 END), 0)
		 FROM gift_balance_records WHERE user_id = $1`, userID,
	).Scan(&summary.TotalGranted, &summary.TotalRemaining, &summary.TotalUsed, &summary.TotalExpired)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *giftBalanceRepository) DeductFIFO(ctx context.Context, userID int64, amount float64) (float64, error) {
	if amount <= 0 {
		return 0, nil
	}
	// 获取有效记录（FIFO）
	records, err := r.GetAvailableByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	var totalDeducted float64
	remaining := amount
	for _, rec := range records {
		if remaining <= 0 {
			break
		}
		deduct := rec.Remaining
		if deduct > remaining {
			deduct = remaining
		}
		newRemaining := rec.Remaining - deduct
		_, err := r.db.ExecContext(ctx,
			`UPDATE gift_balance_records SET remaining = $1, updated_at = NOW() WHERE id = $2`,
			newRemaining, rec.ID)
		if err != nil {
			return totalDeducted, fmt.Errorf("deduct gift balance record %d: %w", rec.ID, err)
		}
		totalDeducted += deduct
		remaining -= deduct
	}
	return totalDeducted, nil
}

func (r *giftBalanceRepository) ExpireRecords(ctx context.Context) (int, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE gift_balance_records SET remaining = 0, updated_at = NOW()
		 WHERE expires_at IS NOT NULL AND expires_at <= NOW() AND remaining > 0`)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (r *giftBalanceRepository) GetTotalRemainingByUserID(ctx context.Context, userID int64) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(remaining), 0) FROM gift_balance_records
		 WHERE user_id = $1 AND remaining > 0 AND (expires_at IS NULL OR expires_at > NOW())`,
		userID).Scan(&total)
	return total, err
}

func (r *giftBalanceRepository) ExistsBySourceRef(ctx context.Context, source string, sourceRefID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM gift_balance_records WHERE source = $1 AND source_ref_id = $2)`,
		source, sourceRefID).Scan(&exists)
	return exists, err
}

func (r *giftBalanceRepository) GetAdminStats(ctx context.Context) (*service.AdminReferralStats, error) {
	var stats service.AdminReferralStats
	// 推广关系统计
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0)
		 FROM referrals`).Scan(&stats.TotalReferrals, &stats.CompletedReferrals)
	// 赠送余额统计
	_ = r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount), 0), COALESCE(SUM(amount - remaining), 0), COALESCE(SUM(CASE WHEN remaining > 0 AND (expires_at IS NULL OR expires_at > NOW()) THEN remaining ELSE 0 END), 0)
		 FROM gift_balance_records`).Scan(&stats.TotalGiftGranted, &stats.TotalGiftUsed, &stats.TotalGiftRemaining)
	// 活跃推广人数
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT referrer_id) FROM referrals`).Scan(&stats.ActiveReferrersCount)
	_ = time.Now() // avoid unused import
	return &stats, nil
}
