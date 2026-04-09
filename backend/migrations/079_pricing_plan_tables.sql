-- 079_pricing_plan_tables.sql
-- Pricing plan groups + plans (for public pricing page / admin management)
-- Consolidated from old project migrations: 028, 029, 030, 032
-- PostgreSQL 15+

-- 1) pricing_plan_groups
CREATE TABLE IF NOT EXISTS pricing_plan_groups (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    sort_order  INT NOT NULL DEFAULT 0,
    status      VARCHAR(20) NOT NULL DEFAULT 'active', -- active/inactive
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pricing_plan_groups_status ON pricing_plan_groups(status);
CREATE INDEX IF NOT EXISTS idx_pricing_plan_groups_sort_order ON pricing_plan_groups(sort_order);

-- 2) pricing_plans
CREATE TABLE IF NOT EXISTS pricing_plans (
    id               BIGSERIAL PRIMARY KEY,
    group_id         BIGINT NOT NULL REFERENCES pricing_plan_groups(id) ON DELETE CASCADE,
    name             VARCHAR(100) NOT NULL,
    description      TEXT,

    -- price: use price_text for special display ("Free", "Contact us"), otherwise use amount+currency+period.
    price_amount     DECIMAL(20, 8),
    price_currency   VARCHAR(10) NOT NULL DEFAULT 'CNY',
    price_period     VARCHAR(20) NOT NULL DEFAULT 'month',
    price_text       TEXT,

    -- features: array of strings
    features         JSONB NOT NULL DEFAULT '[]',

    -- contact methods (wechat/qq/etc.)
    contact_methods  JSONB NOT NULL DEFAULT '[]',

    -- card display fields
    icon_url         TEXT,
    badge_text       TEXT,
    tagline          TEXT,

    is_featured      BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order       INT NOT NULL DEFAULT 0,
    status           VARCHAR(20) NOT NULL DEFAULT 'active', -- active/inactive

    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pricing_plans_group_id ON pricing_plans(group_id);
CREATE INDEX IF NOT EXISTS idx_pricing_plans_status ON pricing_plans(status);
CREATE INDEX IF NOT EXISTS idx_pricing_plans_sort_order ON pricing_plans(sort_order);
CREATE INDEX IF NOT EXISTS idx_pricing_plans_is_featured ON pricing_plans(is_featured);
