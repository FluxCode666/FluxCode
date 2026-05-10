package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type referralRepository struct {
	db *sql.DB
}

func NewReferralRepository(sqlDB *sql.DB) service.ReferralRepository {
	return &referralRepository{db: sqlDB}
}

func (r *referralRepository) Create(ctx context.Context, ref *service.Referral) error {
	query := `INSERT INTO referrals (referrer_id, referee_id, referral_code, status, invitee_reward_amount, inviter_reward_amount, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW()) RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		ref.ReferrerID, ref.RefereeID, ref.ReferralCode, ref.Status, ref.InviteeRewardAmount, ref.InviterRewardAmount,
	).Scan(&ref.ID, &ref.CreatedAt, &ref.UpdatedAt)
}

func (r *referralRepository) GetByRefereeID(ctx context.Context, refereeID int64) (*service.Referral, error) {
	var ref service.Referral
	err := r.db.QueryRowContext(ctx,
		`SELECT id, referrer_id, referee_id, referral_code, status, invitee_reward_amount, inviter_reward_amount,
		        invitee_rewarded_at, inviter_rewarded_at, ongoing_reward_count, ongoing_reward_total, created_at, updated_at
		 FROM referrals WHERE referee_id = $1`, refereeID,
	).Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status,
		&ref.InviteeRewardAmount, &ref.InviterRewardAmount,
		&ref.InviteeRewardedAt, &ref.InviterRewardedAt,
		&ref.OngoingRewardCount, &ref.OngoingRewardTotal, &ref.CreatedAt, &ref.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (r *referralRepository) GetByReferrerID(ctx context.Context, referrerID int64, offset, limit int) ([]service.Referral, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM referrals WHERE referrer_id = $1`, referrerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.id, r.referrer_id, r.referee_id, r.referral_code, r.status,
		        r.invitee_reward_amount, r.inviter_reward_amount,
		        r.invitee_rewarded_at, r.inviter_rewarded_at,
		        r.ongoing_reward_count, r.ongoing_reward_total, r.created_at, r.updated_at,
		        COALESCE(u.email, ''), COALESCE(u.username, '')
		 FROM referrals r LEFT JOIN users u ON r.referee_id = u.id
		 WHERE r.referrer_id = $1 ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`,
		referrerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var refs []service.Referral
	for rows.Next() {
		var ref service.Referral
		if err := rows.Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status,
			&ref.InviteeRewardAmount, &ref.InviterRewardAmount,
			&ref.InviteeRewardedAt, &ref.InviterRewardedAt,
			&ref.OngoingRewardCount, &ref.OngoingRewardTotal, &ref.CreatedAt, &ref.UpdatedAt,
			&ref.RefereeEmail, &ref.RefereeUsername); err != nil {
			return nil, 0, err
		}
		refs = append(refs, ref)
	}
	return refs, total, rows.Err()
}

func (r *referralRepository) CountByReferrerID(ctx context.Context, referrerID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM referrals WHERE referrer_id = $1`, referrerID).Scan(&count)
	return count, err
}

func (r *referralRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE referrals SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func (r *referralRepository) SetInviteeRewarded(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE referrals SET invitee_rewarded_at = NOW(), updated_at = NOW() WHERE id = $1`,
		id)
	return err
}

func (r *referralRepository) SetInviterRewarded(ctx context.Context, id int64, rewardAmount float64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE referrals SET status = 'completed', inviter_reward_amount = $1, inviter_rewarded_at = NOW(), updated_at = NOW() WHERE id = $2`,
		rewardAmount, id)
	return err
}

func (r *referralRepository) IncrementOngoingReward(ctx context.Context, id int64, amount float64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE referrals SET ongoing_reward_count = ongoing_reward_count + 1, ongoing_reward_total = ongoing_reward_total + $1, updated_at = NOW() WHERE id = $2`,
		amount, id)
	return err
}

func (r *referralRepository) GetStatsByReferrerID(ctx context.Context, referrerID int64) (*service.ReferralStats, error) {
	var stats service.ReferralStats
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(inviter_reward_amount), 0),
			COALESCE(SUM(ongoing_reward_total), 0)
		 FROM referrals WHERE referrer_id = $1`, referrerID,
	).Scan(&stats.TotalInvited, &stats.CompletedInvited, &stats.TotalReward, &stats.OngoingReward)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *referralRepository) ListAll(ctx context.Context, status string, offset, limit int) ([]service.Referral, int, error) {
	countQuery := `SELECT COUNT(*) FROM referrals`
	listQuery := `SELECT r.id, r.referrer_id, r.referee_id, r.referral_code, r.status,
		r.invitee_reward_amount, r.inviter_reward_amount,
		r.invitee_rewarded_at, r.inviter_rewarded_at,
		r.ongoing_reward_count, r.ongoing_reward_total, r.created_at, r.updated_at,
		COALESCE(u.email, ''), COALESCE(u.username, '')
		FROM referrals r LEFT JOIN users u ON r.referee_id = u.id`

	var args []any
	argIdx := 1
	if status != "" {
		countQuery += fmt.Sprintf(` WHERE status = $%d`, argIdx)
		listQuery += fmt.Sprintf(` WHERE r.status = $%d`, argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery += fmt.Sprintf(` ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	listArgs := append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var refs []service.Referral
	for rows.Next() {
		var ref service.Referral
		if err := rows.Scan(&ref.ID, &ref.ReferrerID, &ref.RefereeID, &ref.ReferralCode, &ref.Status,
			&ref.InviteeRewardAmount, &ref.InviterRewardAmount,
			&ref.InviteeRewardedAt, &ref.InviterRewardedAt,
			&ref.OngoingRewardCount, &ref.OngoingRewardTotal, &ref.CreatedAt, &ref.UpdatedAt,
			&ref.RefereeEmail, &ref.RefereeUsername); err != nil {
			return nil, 0, err
		}
		refs = append(refs, ref)
	}
	return refs, total, rows.Err()
}

func (r *referralRepository) GetLeaderboard(ctx context.Context, period string, limit int) ([]service.ReferralLeaderboardEntry, error) {
	whereClause := ""
	switch period {
	case "this_month":
		whereClause = "WHERE r.created_at >= date_trunc('month', NOW())"
	case "this_week":
		whereClause = "WHERE r.created_at >= date_trunc('week', NOW())"
	}

	query := fmt.Sprintf(
		`SELECT r.referrer_id, COALESCE(u.email, ''), COALESCE(u.username, ''), COALESCE(u.referral_code, ''),
		        COUNT(*) as invite_count,
		        COALESCE(SUM(r.inviter_reward_amount + r.ongoing_reward_total), 0) as total_reward
		 FROM referrals r LEFT JOIN users u ON r.referrer_id = u.id
		 %s
		 GROUP BY r.referrer_id, u.email, u.username, u.referral_code
		 ORDER BY invite_count DESC LIMIT $1`, whereClause)

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]service.ReferralLeaderboardEntry, 0)
	for rows.Next() {
		var e service.ReferralLeaderboardEntry
		if err := rows.Scan(&e.UserID, &e.Email, &e.Username, &e.ReferralCode, &e.InviteCount, &e.TotalReward); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetTrendByReferrerID 用户级趋势：邀请数 / 完成数 / 奖励总额（含持续奖励）按日聚合
func (r *referralRepository) GetTrendByReferrerID(ctx context.Context, referrerID int64, days int) ([]service.ReferralTrendPoint, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := r.db.QueryContext(ctx,
		`WITH days AS (
			SELECT generate_series(
				date_trunc('day', NOW() - ($1 || ' days')::interval)::date,
				date_trunc('day', NOW())::date,
				INTERVAL '1 day'
			)::date AS d
		)
		SELECT
			TO_CHAR(d.d, 'YYYY-MM-DD') AS date,
			COALESCE(COUNT(r.id), 0) AS invitations,
			COALESCE(SUM(CASE WHEN r.status = 'completed' THEN 1 ELSE 0 END), 0) AS completions,
			COALESCE(SUM(r.inviter_reward_amount + r.ongoing_reward_total), 0) AS rewards_total
		FROM days d
		LEFT JOIN referrals r ON DATE(r.created_at) = d.d AND r.referrer_id = $2
		GROUP BY d.d
		ORDER BY d.d`,
		days, referrerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []service.ReferralTrendPoint
	for rows.Next() {
		var p service.ReferralTrendPoint
		if err := rows.Scan(&p.Date, &p.Invitations, &p.Completions, &p.RewardsTotal); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetGlobalTrend 全站趋势数据
func (r *referralRepository) GetGlobalTrend(ctx context.Context, days int) ([]service.ReferralTrendPoint, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := r.db.QueryContext(ctx,
		`WITH days AS (
			SELECT generate_series(
				date_trunc('day', NOW() - ($1 || ' days')::interval)::date,
				date_trunc('day', NOW())::date,
				INTERVAL '1 day'
			)::date AS d
		)
		SELECT
			TO_CHAR(d.d, 'YYYY-MM-DD') AS date,
			COALESCE(COUNT(r.id), 0) AS invitations,
			COALESCE(SUM(CASE WHEN r.status = 'completed' THEN 1 ELSE 0 END), 0) AS completions,
			COALESCE(SUM(r.inviter_reward_amount + r.ongoing_reward_total), 0) AS rewards_total
		FROM days d
		LEFT JOIN referrals r ON DATE(r.created_at) = d.d
		GROUP BY d.d
		ORDER BY d.d`,
		days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var points []service.ReferralTrendPoint
	for rows.Next() {
		var p service.ReferralTrendPoint
		if err := rows.Scan(&p.Date, &p.Invitations, &p.Completions, &p.RewardsTotal); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// CountFirstRecharges 已完成的推广（即被邀请人已首充）
func (r *referralRepository) CountFirstRecharges(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM referrals WHERE status = 'completed'`).Scan(&n)
	return n, err
}

// CountAll 推广关系总数
func (r *referralRepository) CountAll(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM referrals`).Scan(&n)
	return n, err
}
