-- Durable, idempotent accounting journal for media task precharge and settlement.
--
-- Each task has exactly one precharge operation and at most one terminal
-- settlement operation. The allocation JSON records the original balance,
-- gift-balance, or subscription-grant sources so a later refund can reverse
-- the same funds instead of crediting an unrelated balance bucket.
CREATE TABLE IF NOT EXISTS media_billing_operations (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES media_tasks(id) ON DELETE RESTRICT,
    task_public_id VARCHAR(64) NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    account_id BIGINT NULL,
    idempotency_key VARCHAR(96) NOT NULL UNIQUE,
    operation VARCHAR(16) NOT NULL CHECK (operation IN ('precharge', 'success', 'failure')),
    request_fingerprint VARCHAR(64) NOT NULL,
    precharged_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    final_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    refunded_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    additional_charged_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    allocation JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (task_id, operation)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_media_billing_operations_terminal
    ON media_billing_operations(task_id)
    WHERE operation IN ('success', 'failure');

CREATE INDEX IF NOT EXISTS idx_media_billing_operations_public_id
    ON media_billing_operations(task_public_id, created_at);

CREATE INDEX IF NOT EXISTS idx_media_billing_operations_user_created
    ON media_billing_operations(user_id, created_at DESC);
