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

func (r *referralRepository) GetLeaderboard(ctx context.Context, limit int) ([]service.ReferralLeaderboardEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.referrer_id, COALESCE(u.email, ''), COALESCE(u.username, ''), COALESCE(u.referral_code, ''),
		        COUNT(*) as invite_count,
		        COALESCE(SUM(r.inviter_reward_amount + r.ongoing_reward_total), 0) as total_reward
		 FROM referrals r LEFT JOIN users u ON r.referrer_id = u.id
		 GROUP BY r.referrer_id, u.email, u.username, u.referral_code
		 ORDER BY invite_count DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []service.ReferralLeaderboardEntry
	for rows.Next() {
		var e service.ReferralLeaderboardEntry
		if err := rows.Scan(&e.UserID, &e.Email, &e.Username, &e.ReferralCode, &e.InviteCount, &e.TotalReward); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
