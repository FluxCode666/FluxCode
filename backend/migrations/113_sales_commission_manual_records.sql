-- 113_sales_commission_manual_records.sql
-- Allow manual sales commission records that are not backed by a payment order.

DROP INDEX IF EXISTS sales_commission_records_payment_order_id_key;

ALTER TABLE sales_commission_records
    ALTER COLUMN payment_order_id DROP NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS sales_commission_records_payment_order_id_key
    ON sales_commission_records (payment_order_id)
    WHERE payment_order_id IS NOT NULL;
