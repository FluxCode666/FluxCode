-- 109_migrate_referral_ongoing_reward_to_type_value.sql
-- 将持续奖励配置从 amount/percent 双列模型迁移到 type/value 单列模型
--
-- 影响范围：
--   1. user_referral_configs 表：
--      - 新增 ongoing_reward_type VARCHAR(20)
--      - 新增 ongoing_reward_value DECIMAL(20,8)
--      - 数据迁移：percent>0 → ('percentage', percent)；否则 → ('fixed', amount)
--      - 删除旧列 ongoing_reward_amount / ongoing_reward_percent
--   2. settings 表：
--      - 把 referral_ongoing_reward_amount / referral_ongoing_reward_percent
--        合并为 referral_ongoing_reward_type / referral_ongoing_reward_value
--
-- 说明：
--   推广功能尚未上线，本迁移在大多数环境下应为零数据迁移；
--   保留迁移逻辑以保证已经误写入数据的环境也能平滑升级。

-- ========================================
-- 1. user_referral_configs：新增 type/value 列
-- ========================================

ALTER TABLE user_referral_configs
    ADD COLUMN IF NOT EXISTS ongoing_reward_type  VARCHAR(20)    DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS ongoing_reward_value DECIMAL(20,8)  DEFAULT NULL;

-- ========================================
-- 2. user_referral_configs：从旧列迁移数据
-- ========================================

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
         WHERE table_name = 'user_referral_configs'
           AND column_name = 'ongoing_reward_percent'
    ) THEN
        UPDATE user_referral_configs
           SET ongoing_reward_type =
                   CASE
                       WHEN COALESCE(ongoing_reward_percent, 0) > 0 THEN 'percentage'
                       WHEN ongoing_reward_amount IS NOT NULL       THEN 'fixed'
                       ELSE NULL
                   END,
               ongoing_reward_value =
                   CASE
                       WHEN COALESCE(ongoing_reward_percent, 0) > 0 THEN ongoing_reward_percent
                       ELSE ongoing_reward_amount
                   END
         WHERE ongoing_reward_type IS NULL
           AND (ongoing_reward_amount IS NOT NULL OR ongoing_reward_percent IS NOT NULL);
    END IF;
END$$;

-- ========================================
-- 3. user_referral_configs：删除旧列
-- ========================================

ALTER TABLE user_referral_configs
    DROP COLUMN IF EXISTS ongoing_reward_amount,
    DROP COLUMN IF EXISTS ongoing_reward_percent;

-- ========================================
-- 4. settings：合并旧 key 为新 key
-- ========================================

DO $$
DECLARE
    v_amount  TEXT;
    v_percent TEXT;
    v_type    TEXT;
    v_value   TEXT;
    v_amount_num  NUMERIC;
    v_percent_num NUMERIC;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables WHERE table_name = 'settings'
    ) THEN
        RETURN;
    END IF;

    SELECT value INTO v_amount  FROM settings WHERE key = 'referral_ongoing_reward_amount';
    SELECT value INTO v_percent FROM settings WHERE key = 'referral_ongoing_reward_percent';

    -- 仅当存在任一旧 key 时才执行合并
    IF v_amount IS NULL AND v_percent IS NULL THEN
        RETURN;
    END IF;

    BEGIN
        v_percent_num := NULLIF(v_percent, '')::NUMERIC;
    EXCEPTION WHEN others THEN
        v_percent_num := NULL;
    END;

    BEGIN
        v_amount_num := NULLIF(v_amount, '')::NUMERIC;
    EXCEPTION WHEN others THEN
        v_amount_num := NULL;
    END;

    IF COALESCE(v_percent_num, 0) > 0 THEN
        v_type  := 'percentage';
        v_value := v_percent_num::TEXT;
    ELSIF v_amount_num IS NOT NULL THEN
        v_type  := 'fixed';
        v_value := v_amount_num::TEXT;
    ELSE
        v_type  := 'fixed';
        v_value := '0';
    END IF;

    -- 仅当目标 key 不存在时写入，避免覆盖管理端已经手动配置的新值
    INSERT INTO settings (key, value)
    VALUES ('referral_ongoing_reward_type', v_type)
    ON CONFLICT (key) DO NOTHING;

    INSERT INTO settings (key, value)
    VALUES ('referral_ongoing_reward_value', v_value)
    ON CONFLICT (key) DO NOTHING;

    DELETE FROM settings
     WHERE key IN ('referral_ongoing_reward_amount', 'referral_ongoing_reward_percent');
END$$;

