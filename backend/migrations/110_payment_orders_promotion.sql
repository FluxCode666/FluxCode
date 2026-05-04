-- payment_orders 增加促销活动相关字段
-- 与 109_promotions.sql 配套使用：当订单创建/履约时记录使用的活动信息，便于审计、报表、退款对账。

ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS promotion_id BIGINT,
    ADD COLUMN IF NOT EXISTS promotion_rule_id BIGINT,
    ADD COLUMN IF NOT EXISTS original_amount DECIMAL(20,2),
    ADD COLUMN IF NOT EXISTS discount_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS bonus_amount DECIMAL(20,2) NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_payment_orders_promotion ON payment_orders(promotion_id);

COMMENT ON COLUMN payment_orders.promotion_id IS '命中的促销活动 ID（NULL 表示未应用活动）';
COMMENT ON COLUMN payment_orders.promotion_rule_id IS '命中的子规则 ID（订阅活动）';
COMMENT ON COLUMN payment_orders.original_amount IS '折扣前金额（订阅=plan.price，充值=用户输入金额）';
COMMENT ON COLUMN payment_orders.discount_amount IS '减少的金额（订阅按减额/折扣后差值；充值 reduce_pay 模式的实付节省）';
COMMENT ON COLUMN payment_orders.bonus_amount IS '充值 bonus_credit 模式额外赠送的到账金额';
