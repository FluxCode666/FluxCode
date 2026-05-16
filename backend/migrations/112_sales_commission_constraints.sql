-- 112_sales_commission_constraints.sql
-- 为销售佣金账本补充外键、检查约束和结算批次明细查询索引。

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_users_sales_commission_rate'
          AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT chk_users_sales_commission_rate
            CHECK (sales_commission_rate >= 0 AND sales_commission_rate <= 100);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_records_sales_user'
          AND conrelid = 'sales_commission_records'::regclass
    ) THEN
        ALTER TABLE sales_commission_records
            ADD CONSTRAINT fk_sales_commission_records_sales_user
            FOREIGN KEY (sales_user_id) REFERENCES users(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_records_referee_user'
          AND conrelid = 'sales_commission_records'::regclass
    ) THEN
        ALTER TABLE sales_commission_records
            ADD CONSTRAINT fk_sales_commission_records_referee_user
            FOREIGN KEY (referee_user_id) REFERENCES users(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_records_referral'
          AND conrelid = 'sales_commission_records'::regclass
    ) THEN
        ALTER TABLE sales_commission_records
            ADD CONSTRAINT fk_sales_commission_records_referral
            FOREIGN KEY (referral_id) REFERENCES referrals(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_records_payment_order'
          AND conrelid = 'sales_commission_records'::regclass
    ) THEN
        ALTER TABLE sales_commission_records
            ADD CONSTRAINT fk_sales_commission_records_payment_order
            FOREIGN KEY (payment_order_id) REFERENCES payment_orders(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sales_commission_records_amounts'
          AND conrelid = 'sales_commission_records'::regclass
    ) THEN
        ALTER TABLE sales_commission_records
            ADD CONSTRAINT chk_sales_commission_records_amounts CHECK (
                order_pay_amount_cny > 0
                AND order_credited_amount > 0
                AND commission_rate >= 0
                AND commission_rate <= 100
                AND commission_total_cny >= 0
                AND credited_used_amount >= 0
                AND credited_used_amount <= order_credited_amount
                AND unlocked_cny >= 0
                AND unlocked_cny <= commission_total_cny
                AND settled_cny >= 0
                AND settled_cny <= unlocked_cny
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sales_commission_records_status'
          AND conrelid = 'sales_commission_records'::regclass
    ) THEN
        ALTER TABLE sales_commission_records
            ADD CONSTRAINT chk_sales_commission_records_status CHECK (
                status IN ('frozen', 'partial_unlocked', 'unlocked', 'settled', 'settlement_blocked')
            );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_settlements_sales_user'
          AND conrelid = 'sales_commission_settlements'::regclass
    ) THEN
        ALTER TABLE sales_commission_settlements
            ADD CONSTRAINT fk_sales_commission_settlements_sales_user
            FOREIGN KEY (sales_user_id) REFERENCES users(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_settlements_created_by'
          AND conrelid = 'sales_commission_settlements'::regclass
    ) THEN
        ALTER TABLE sales_commission_settlements
            ADD CONSTRAINT fk_sales_commission_settlements_created_by
            FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sales_commission_settlements_amount'
          AND conrelid = 'sales_commission_settlements'::regclass
    ) THEN
        ALTER TABLE sales_commission_settlements
            ADD CONSTRAINT chk_sales_commission_settlements_amount
            CHECK (amount_cny > 0);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_sales_commission_settlement_items_settlement
    ON sales_commission_settlement_items (settlement_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_settlement_items_settlement'
          AND conrelid = 'sales_commission_settlement_items'::regclass
    ) THEN
        ALTER TABLE sales_commission_settlement_items
            ADD CONSTRAINT fk_sales_commission_settlement_items_settlement
            FOREIGN KEY (settlement_id) REFERENCES sales_commission_settlements(id) ON DELETE CASCADE;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_sales_commission_settlement_items_record'
          AND conrelid = 'sales_commission_settlement_items'::regclass
    ) THEN
        ALTER TABLE sales_commission_settlement_items
            ADD CONSTRAINT fk_sales_commission_settlement_items_record
            FOREIGN KEY (commission_record_id) REFERENCES sales_commission_records(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_sales_commission_settlement_items_amount'
          AND conrelid = 'sales_commission_settlement_items'::regclass
    ) THEN
        ALTER TABLE sales_commission_settlement_items
            ADD CONSTRAINT chk_sales_commission_settlement_items_amount
            CHECK (amount_cny > 0);
    END IF;
END $$;

