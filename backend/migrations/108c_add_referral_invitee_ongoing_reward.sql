-- 050_add_referral_invitee_ongoing_reward.sql
-- 为 referrals 表添加被邀请人持续充值奖励的计数和金额字段

ALTER TABLE referrals ADD COLUMN IF NOT EXISTS invitee_ongoing_reward_count INT NOT NULL DEFAULT 0;
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS invitee_ongoing_reward_total DOUBLE PRECISION NOT NULL DEFAULT 0;
