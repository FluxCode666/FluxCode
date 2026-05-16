-- 113_payment_order_subscription_mode.sql
-- Add subscription_mode column to payment_orders for extend/stack choice.

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS subscription_mode VARCHAR(16);
