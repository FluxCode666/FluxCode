package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userReferralConfigRepository struct {
	db *sql.DB
}

func NewUserReferralConfigRepository(sqlDB *sql.DB) service.UserReferralConfigRepository {
	return &userReferralConfigRepository{db: sqlDB}
}

func (r *userReferralConfigRepository) GetByUserID(ctx context.Context, userID int64) (*service.UserReferralConfig, error) {
	var cfg service.UserReferralConfig
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, invitee_reward_amount, inviter_reward_amount, max_invites, reward_expiry_days,
		        ongoing_reward_enabled, ongoing_reward_type, ongoing_reward_value,
		        ongoing_reward_max_count, ongoing_reward_duration_days, notes, created_at, updated_at
		 FROM user_referral_configs WHERE user_id = $1`, userID,
	).Scan(&cfg.ID, &cfg.UserID, &cfg.InviteeRewardAmount, &cfg.InviterRewardAmount,
		&cfg.MaxInvites, &cfg.RewardExpiryDays,
		&cfg.OngoingRewardEnabled, &cfg.OngoingRewardType, &cfg.OngoingRewardValue,
		&cfg.OngoingRewardMaxCount, &cfg.OngoingRewardDurationDays,
		&cfg.Notes, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *userReferralConfigRepository) Upsert(ctx context.Context, cfg *service.UserReferralConfig) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_referral_configs (user_id, invitee_reward_amount, inviter_reward_amount, max_invites, reward_expiry_days,
		        ongoing_reward_enabled, ongoing_reward_type, ongoing_reward_value,
		        ongoing_reward_max_count, ongoing_reward_duration_days, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		 ON CONFLICT (user_id) DO UPDATE SET
		   invitee_reward_amount = EXCLUDED.invitee_reward_amount,
		   inviter_reward_amount = EXCLUDED.inviter_reward_amount,
		   max_invites = EXCLUDED.max_invites,
		   reward_expiry_days = EXCLUDED.reward_expiry_days,
		   ongoing_reward_enabled = EXCLUDED.ongoing_reward_enabled,
		   ongoing_reward_type = EXCLUDED.ongoing_reward_type,
		   ongoing_reward_value = EXCLUDED.ongoing_reward_value,
		   ongoing_reward_max_count = EXCLUDED.ongoing_reward_max_count,
		   ongoing_reward_duration_days = EXCLUDED.ongoing_reward_duration_days,
		   notes = EXCLUDED.notes,
		   updated_at = NOW()`,
		cfg.UserID, cfg.InviteeRewardAmount, cfg.InviterRewardAmount, cfg.MaxInvites, cfg.RewardExpiryDays,
		cfg.OngoingRewardEnabled, cfg.OngoingRewardType, cfg.OngoingRewardValue,
		cfg.OngoingRewardMaxCount, cfg.OngoingRewardDurationDays, cfg.Notes)
	return err
}

func (r *userReferralConfigRepository) Delete(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM user_referral_configs WHERE user_id = $1`, userID)
	return err
}
