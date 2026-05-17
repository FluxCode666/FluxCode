ALTER TABLE users
    ADD COLUMN IF NOT EXISTS sales_commission_mode VARCHAR(16) NOT NULL DEFAULT 'fixed',
    ADD COLUMN IF NOT EXISTS sales_commission_min_monthly_sales DECIMAL(20,2) NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS sales_commission_tiers (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month_sales_from_cny DECIMAL(20,2) NOT NULL,
    month_sales_to_cny DECIMAL(20,2),
    commission_rate DECIMAL(8,4) NOT NULL,
    sort_order INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sales_commission_tiers_sales_user
    ON sales_commission_tiers (sales_user_id, sort_order, id);

CREATE TABLE IF NOT EXISTS sales_commission_monthly_snapshots (
    id BIGSERIAL PRIMARY KEY,
    sales_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    commission_month DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    commission_mode VARCHAR(16) NOT NULL,
    fixed_commission_rate DECIMAL(8,4) NOT NULL DEFAULT 0,
    min_monthly_sales_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    tiers_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sales_commission_monthly_snapshots UNIQUE (sales_user_id, commission_month)
);

ALTER TABLE sales_commission_records
    ADD COLUMN IF NOT EXISTS commission_month DATE DEFAULT DATE_TRUNC('month', TIMEZONE('Asia/Shanghai', NOW()))::date,
    ADD COLUMN IF NOT EXISTS snapshot_id BIGINT REFERENCES sales_commission_monthly_snapshots(id),
    ADD COLUMN IF NOT EXISTS commission_mode VARCHAR(16) NOT NULL DEFAULT 'fixed',
    ADD COLUMN IF NOT EXISTS commission_event_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS monthly_sales_before_cny DECIMAL(20,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_sales_after_cny DECIMAL(20,2) NOT NULL DEFAULT 0;

UPDATE sales_commission_records scr
SET commission_event_at = COALESCE(po.paid_at, po.completed_at, scr.created_at),
    commission_month = DATE_TRUNC('month', TIMEZONE('Asia/Shanghai', COALESCE(po.paid_at, po.completed_at, scr.created_at)))::date
FROM payment_orders po
WHERE scr.payment_order_id = po.id
  AND (scr.commission_event_at IS NULL OR scr.commission_month IS NULL);

UPDATE sales_commission_records
SET commission_event_at = COALESCE(commission_event_at, created_at),
    commission_month = COALESCE(commission_month, DATE_TRUNC('month', TIMEZONE('Asia/Shanghai', COALESCE(commission_event_at, created_at)))::date)
WHERE commission_event_at IS NULL OR commission_month IS NULL;

ALTER TABLE sales_commission_records
    ALTER COLUMN commission_month SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sales_commission_records_month
    ON sales_commission_records (sales_user_id, commission_month, commission_event_at, id);
