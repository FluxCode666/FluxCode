-- 111_sales_commissions.sql
-- 销售佣金系统：新增销售用户标记、佣金记录、结算批次和结算明细表。

-- ========================================
-- 1. 扩展 users 表
-- ========================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_sales BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS sales_commission_rate DECIMAL(8,4) NOT NULL DEFAULT 0;

COMMENT ON COLUMN users.is_sales IS '是否为销售用户';
COMMENT ON COLUMN users.sales_commission_rate IS '销售佣金比例，单位为百分比';

-- ========================================
-- 2. sales_commission_records 佣金记录表
-- ========================================
CREATE TABLE IF NOT EXISTS sales_commission_records (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL,
    referee_user_id BIGINT NOT NULL,
    referral_id BIGINT NOT NULL,
    payment_order_id BIGINT NOT NULL,
    order_pay_amount_cny DECIMAL(20,2) NOT NULL,
    order_credited_amount DECIMAL(20,8) NOT NULL,
    commission_rate DECIMAL(8,4) NOT NULL,
    commission_total_cny DECIMAL(20,2) NOT NULL,
    credited_used_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    unlocked_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    settled_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'frozen',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS sales_commission_records_payment_order_id_key
    ON sales_commission_records (payment_order_id);

CREATE INDEX IF NOT EXISTS idx_sales_commission_sales_user
    ON sales_commission_records (sales_user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_sales_commission_referee
    ON sales_commission_records (referee_user_id, id);

CREATE INDEX IF NOT EXISTS idx_sales_commission_status
    ON sales_commission_records (status);

COMMENT ON COLUMN sales_commission_records.commission_total_cny IS '订单产生的佣金总额，单位 CNY';
COMMENT ON COLUMN sales_commission_records.credited_used_amount IS '被邀请用户已消耗的本次充值到账额度';

-- ========================================
-- 3. sales_commission_settlements 佣金结算批次表
-- ========================================
CREATE TABLE IF NOT EXISTS sales_commission_settlements (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL,
    amount_cny DECIMAL(20,2) NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_settlements_sales_user
    ON sales_commission_settlements (sales_user_id, created_at);

-- ========================================
-- 4. sales_commission_settlement_items 佣金结算明细表
-- ========================================
CREATE TABLE IF NOT EXISTS sales_commission_settlement_items (
    id BIGSERIAL PRIMARY KEY,
    settlement_id BIGINT NOT NULL,
    commission_record_id BIGINT NOT NULL,
    amount_cny DECIMAL(20,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_settlement_items_record
    ON sales_commission_settlement_items (commission_record_id);

