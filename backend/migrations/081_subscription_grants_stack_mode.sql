-- 081_subscription_grants_stack_mode.sql
-- Consolidate stacked subscription grant foundation from the old system.

CREATE TABLE IF NOT EXISTS subscription_grants (
    id                BIGSERIAL PRIMARY KEY,
    subscription_id   BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    starts_at         TIMESTAMPTZ NOT NULL,
    expires_at        TIMESTAMPTZ NOT NULL,
    daily_usage_usd   DECIMAL(20, 10) NOT NULL DEFAULT 0,
    weekly_usage_usd  DECIMAL(20, 10) NOT NULL DEFAULT 0,
    monthly_usage_usd DECIMAL(20, 10) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_subscription_grants_subscription_id_active
    ON subscription_grants(subscription_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_grants_active_lookup
    ON subscription_grants(subscription_id, starts_at, expires_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_grants_expires_at_active
    ON subscription_grants(expires_at)
    WHERE deleted_at IS NULL;

INSERT INTO subscription_grants (subscription_id, starts_at, expires_at)
SELECT us.id, us.starts_at, us.expires_at
FROM user_subscriptions us
WHERE us.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM subscription_grants sg
    WHERE sg.subscription_id = us.id
      AND sg.deleted_at IS NULL
  );

WITH single_grant_subs AS (
    SELECT
        sg.subscription_id,
        MAX(sg.id) AS grant_id
    FROM subscription_grants sg
    WHERE sg.deleted_at IS NULL
    GROUP BY sg.subscription_id
    HAVING COUNT(*) = 1
)
UPDATE subscription_grants sg
SET
    daily_usage_usd = us.daily_usage_usd,
    weekly_usage_usd = us.weekly_usage_usd,
    monthly_usage_usd = us.monthly_usage_usd,
    updated_at = NOW()
FROM single_grant_subs sgs
JOIN user_subscriptions us ON us.id = sgs.subscription_id
WHERE sg.id = sgs.grant_id
  AND sg.deleted_at IS NULL
  AND us.deleted_at IS NULL
  AND sg.daily_usage_usd = 0
  AND sg.weekly_usage_usd = 0
  AND sg.monthly_usage_usd = 0;

ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS subscription_mode VARCHAR(16);
