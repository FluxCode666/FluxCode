-- 充值与订阅促销活动 (Promotions)
--
-- 提供两类活动：
--   * recharge      ：充值活动（全局，不按品分）
--   * subscription  ：订阅活动（按 plan 维度，单活动可挂多个 plan）
--
-- 折扣形式：
--   * 充值：reduce_pay (降低实付) 或 bonus_credit (加送到账)，活动级二选一
--   * 订阅：rate (按比例) 或 amount (按减额)，plan 级二选一（在 promotion_plan_rules 中）
--
-- 同型多活动并存时由 resolver 在订单创建时挑选"对用户最优"的一个，不叠加。

CREATE TABLE IF NOT EXISTS promotions (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    promotion_type VARCHAR(20) NOT NULL,                  -- 'recharge' | 'subscription'
    discount_mode VARCHAR(20) NOT NULL DEFAULT '',        -- recharge: 'reduce_pay' | 'bonus_credit' ; subscription: '' (子规则覆盖)
    -- 充值活动配置（subscription 类型时为 NULL）
    recharge_rate DECIMAL(10,4),                          -- reduce_pay 模式：实付倍率，0<rate<=1，例 0.91
    recharge_bonus_rate DECIMAL(10,4),                    -- bonus_credit 模式：到账倍率，>=1，例 1.10
    -- 通用属性
    max_uses_per_user INT NOT NULL DEFAULT 0,             -- 0 = 不限
    starts_at TIMESTAMPTZ,                                -- NULL = 不限开始
    ends_at TIMESTAMPTZ,                                  -- NULL = 不限结束
    status VARCHAR(20) NOT NULL DEFAULT 'active',         -- 'active' | 'disabled'
    priority INT NOT NULL DEFAULT 0,                      -- 同型多活动并存的辅助排序
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_promotions_type_status ON promotions(promotion_type, status);
CREATE INDEX IF NOT EXISTS idx_promotions_window ON promotions(starts_at, ends_at);

-- 订阅活动按 plan 配置的子规则
CREATE TABLE IF NOT EXISTS promotion_plan_rules (
    id BIGSERIAL PRIMARY KEY,
    promotion_id BIGINT NOT NULL REFERENCES promotions(id) ON DELETE CASCADE,
    plan_id BIGINT NOT NULL,                              -- 不加 FK 以兼容 subscription_plans 硬删除
    discount_mode VARCHAR(20) NOT NULL,                   -- 'rate' | 'amount'
    discount_rate DECIMAL(10,4),                          -- mode=rate 时使用，0<rate<=1
    discount_amount DECIMAL(20,2),                        -- mode=amount 时使用，>=0
    min_price_floor DECIMAL(20,2) NOT NULL DEFAULT 0.01,  -- 优惠后价格最低保护
    max_uses_per_user INT NOT NULL DEFAULT 0,             -- 0 = 跟随活动级
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(promotion_id, plan_id)
);

CREATE INDEX IF NOT EXISTS idx_promotion_plan_rules_plan ON promotion_plan_rules(plan_id);

-- 用户使用记录（履约成功后插入；用于限次校验、报表与对账）
CREATE TABLE IF NOT EXISTS promotion_usages (
    id BIGSERIAL PRIMARY KEY,
    promotion_id BIGINT NOT NULL REFERENCES promotions(id) ON DELETE CASCADE,
    plan_id BIGINT,                                       -- 充值活动为 NULL；订阅活动记录 plan
    user_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,                             -- payment_orders.id
    discount_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    bonus_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_promotion_usages_user ON promotion_usages(user_id, promotion_id, plan_id);
CREATE INDEX IF NOT EXISTS idx_promotion_usages_order ON promotion_usages(order_id);

COMMENT ON TABLE promotions IS '充值/订阅促销活动主表';
COMMENT ON TABLE promotion_plan_rules IS '订阅活动按 plan 独立配置的折扣规则';
COMMENT ON TABLE promotion_usages IS '用户使用活动的记录，用于限次与报表';
COMMENT ON COLUMN promotions.promotion_type IS '活动类型: recharge | subscription';
COMMENT ON COLUMN promotions.discount_mode IS '充值活动模式: reduce_pay | bonus_credit；订阅活动留空（由 plan 规则定义）';
COMMENT ON COLUMN promotions.recharge_rate IS '充值 reduce_pay 模式的实付倍率，例 0.91 表示 9.1 折';
COMMENT ON COLUMN promotions.recharge_bonus_rate IS '充值 bonus_credit 模式的到账倍率，例 1.10 表示充 100 到 110';
COMMENT ON COLUMN promotions.max_uses_per_user IS '每用户限次，0 表示不限';
COMMENT ON COLUMN promotion_plan_rules.discount_mode IS '订阅折扣模式: rate (按比例) | amount (按减额)';
COMMENT ON COLUMN promotion_plan_rules.min_price_floor IS '优惠后价格最低保护，避免出现 0 或负数';
COMMENT ON COLUMN promotion_plan_rules.max_uses_per_user IS '该 plan 每用户限次，0 表示跟随活动级 max_uses_per_user';
