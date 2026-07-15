CREATE TABLE IF NOT EXISTS media_tasks (
    id BIGSERIAL PRIMARY KEY,
    public_id VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    channel_id BIGINT NULL,
    account_id BIGINT NULL,
    media_type VARCHAR(16) NOT NULL CHECK (media_type IN ('image','video')),
    operation VARCHAR(40) NOT NULL,
    requested_model VARCHAR(128) NOT NULL,
    upstream_model VARCHAR(128) NOT NULL DEFAULT '',
    adapter VARCHAR(64) NOT NULL DEFAULT '',
    native_async_mode VARCHAR(16) NOT NULL DEFAULT 'unsupported' CHECK (native_async_mode IN ('unsupported','optional','required')),
    client_async BOOLEAN NOT NULL DEFAULT FALSE,
    sync_fallback BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','in_progress','completed','failed')),
    stage VARCHAR(20) NOT NULL DEFAULT 'queued' CHECK (stage IN ('queued','scheduling','submitting','generating','polling','storing','settling','completed','failed')),
    progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    request_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    candidate_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    request_fingerprint VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL DEFAULT '',
    upstream_task_id TEXT NULL,
    poll_metadata JSONB NULL,
    billing_snapshot JSONB NULL,
    settlement_plan JSONB NULL,
    billing_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    precharged_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    final_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    refunded_amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    worker_id VARCHAR(128) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ NULL,
    version BIGINT NOT NULL DEFAULT 1,
    submitted_at TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    sync_fallback_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS media_artifacts (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES media_tasks(id) ON DELETE CASCADE,
    direction VARCHAR(16) NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    media_type VARCHAR(16) NOT NULL CHECK (media_type IN ('image','video')),
    content_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    width INTEGER NULL,
    height INTEGER NULL,
    duration_seconds DOUBLE PRECISION NULL,
    resolution VARCHAR(32) NOT NULL DEFAULT '',
    fps DOUBLE PRECISION NULL,
    storage_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    object_key TEXT NULL,
    public_url TEXT NULL,
    upstream_reference TEXT NULL,
    expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (task_id, direction, position)
);

CREATE TABLE IF NOT EXISTS media_model_definitions (
    id BIGSERIAL PRIMARY KEY,
    model_id VARCHAR(128) NOT NULL UNIQUE,
    media_type VARCHAR(16) NOT NULL CHECK (media_type IN ('image','video')),
    operations JSONB NOT NULL DEFAULT '[]'::jsonb,
    constraints JSONB NOT NULL DEFAULT '{}'::jsonb,
    billing_unit VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE groups ADD COLUMN IF NOT EXISTS allow_video_generation BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS media_cross_platform_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_media_tasks_user_created ON media_tasks(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_tasks_status_lease ON media_tasks(status, lease_until);
CREATE INDEX IF NOT EXISTS idx_media_tasks_account ON media_tasks(account_id) WHERE account_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_tasks_idempotency ON media_tasks(user_id, api_key_id, idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_media_artifacts_task ON media_artifacts(task_id, direction, position);
CREATE INDEX IF NOT EXISTS idx_media_model_definitions_enabled ON media_model_definitions(enabled, media_type);
