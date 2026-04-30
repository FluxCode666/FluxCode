-- 108_add_referral_reward_tables.sql
-- 推广奖励系统：新增 gift_balance_records, referrals, user_referral_configs 表
-- 扩展 users 表添加 referral_code, referred_by 字段

BEGIN;

-- ========================================
-- 1. 扩展 users 表
-- ========================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS referral_code VARCHAR(20) DEFAULT '',
    ADD COLUMN IF NOT EXISTS referred_by BIGINT DEFAULT NULL;

-- 推广码唯一部分索引（排除软删除和空值）
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_referral_code_unique
    ON users (referral_code)
    WHERE deleted_at IS NULL AND referral_code != '';

-- referred_by 索引用于快速查询某用户邀请的所有下线
CREATE INDEX IF NOT EXISTS idx_users_referred_by ON users (referred_by) WHERE referred_by IS NOT NULL;

-- ========================================
-- 2. gift_balance_records 赠送余额记录表
-- ========================================
CREATE TABLE IF NOT EXISTS gift_balance_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    remaining DECIMAL(20,8) NOT NULL DEFAULT 0,
    source VARCHAR(50) NOT NULL DEFAULT '',
    source_ref_id BIGINT DEFAULT NULL,
    note TEXT DEFAULT '',
    expires_at TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 按用户查询有效赠送余额（FIFO 扣减需要按时间排序）
CREATE INDEX IF NOT EXISTS idx_gift_balance_records_user_remaining
    ON gift_balance_records (user_id, created_at)
    WHERE remaining > 0;

-- 按到期时间查询（过期清理后台任务）
CREATE INDEX IF NOT EXISTS idx_gift_balance_records_expires_at
    ON gift_balance_records (expires_at)
    WHERE expires_at IS NOT NULL AND remaining > 0;

-- 按来源和来源引用 ID 查询（幂等性检查）
CREATE INDEX IF NOT EXISTS idx_gift_balance_records_source_ref
    ON gift_balance_records (source, source_ref_id)
    WHERE source_ref_id IS NOT NULL;

-- ========================================
-- 3. referrals 推广关系表
-- ========================================
CREATE TABLE IF NOT EXISTS referrals (
    id BIGSERIAL PRIMARY KEY,
    referrer_id BIGINT NOT NULL,
    referee_id BIGINT NOT NULL,
    referral_code VARCHAR(20) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    invitee_reward_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    inviter_reward_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    invitee_rewarded_at TIMESTAMPTZ DEFAULT NULL,
    inviter_rewarded_at TIMESTAMPTZ DEFAULT NULL,
    ongoing_reward_count INT NOT NULL DEFAULT 0,
    ongoing_reward_total DECIMAL(20,8) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 被邀请人唯一约束（一个用户只能被邀请一次）
CREATE UNIQUE INDEX IF NOT EXISTS idx_referrals_referee_id_unique ON referrals (referee_id);

-- 推广人查询其邀请列表
CREATE INDEX IF NOT EXISTS idx_referrals_referrer_id ON referrals (referrer_id, created_at DESC);

-- 按状态筛选
CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals (status);

-- ========================================
-- 4. user_referral_configs 用户推广配置覆盖表
-- ========================================
CREATE TABLE IF NOT EXISTS user_referral_configs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    invitee_reward_amount DECIMAL(20,8) DEFAULT NULL,
    inviter_reward_amount DECIMAL(20,8) DEFAULT NULL,
    max_invites INT DEFAULT NULL,
    reward_expiry_days INT DEFAULT NULL,
    ongoing_reward_enabled BOOLEAN DEFAULT NULL,
    ongoing_reward_type VARCHAR(20) DEFAULT NULL,
    ongoing_reward_value DECIMAL(20,8) DEFAULT NULL,
    ongoing_reward_max_count INT DEFAULT NULL,
    ongoing_reward_duration_days INT DEFAULT NULL,
    notes TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 每个用户只能有一条配置
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_referral_configs_user_id_unique ON user_referral_configs (user_id);

COMMIT;
