-- 082_add_redeem_code_welfare_no.sql
-- 为 redeem_codes 表添加福利号字段，用于限制同一用户同一福利号仅可兑换一次。
-- 从老项目迁移（对应老项目 migration 028）。

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS welfare_no VARCHAR(64);

COMMENT ON COLUMN redeem_codes.welfare_no IS '福利号（同一用户同一福利号仅可兑换一次）';

-- 同一用户（used_by）同一福利号（welfare_no）仅允许出现一次（非福利码该字段为 NULL，不参与约束）。
CREATE UNIQUE INDEX IF NOT EXISTS idx_redeem_codes_used_by_welfare_no_unique
    ON redeem_codes(used_by, welfare_no);
