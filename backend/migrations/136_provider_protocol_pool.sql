-- Provider capability pool. Accounts remain the scheduling identity; the new
-- tables describe provider lifecycle, protocol endpoints and model capability.

CREATE TABLE IF NOT EXISTS provider_profiles (
    id                        BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    display_name              VARCHAR(100) NOT NULL,
    status                    VARCHAR(32) NOT NULL DEFAULT 'draft',
    allow_protocol_conversion BOOLEAN NOT NULL DEFAULT FALSE,
    base_url                  VARCHAR(500),
    auth_type                 VARCHAR(32),
    default_headers           JSONB NOT NULL DEFAULT '{}'::jsonb,
    version                   BIGINT NOT NULL DEFAULT 1,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_provider_profiles_status CHECK (status IN ('draft', 'active', 'disabled', 'review_required')),
    CONSTRAINT chk_provider_profiles_version CHECK (version > 0)
);

CREATE INDEX IF NOT EXISTS idx_provider_profiles_status ON provider_profiles(status);

CREATE TABLE IF NOT EXISTS provider_protocol_endpoints (
    id              BIGSERIAL PRIMARY KEY,
    provider_id     BIGINT NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    protocol_family VARCHAR(32) NOT NULL,
    wire_profile    VARCHAR(64) NOT NULL DEFAULT 'canonical_v1',
    base_url        VARCHAR(500),
    path            VARCHAR(255) NOT NULL,
    auth_type       VARCHAR(32),
    headers         JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    version         BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_provider_protocol_endpoint UNIQUE (provider_id, protocol_family),
    CONSTRAINT chk_provider_protocol_family CHECK (protocol_family IN ('chat_completions', 'responses', 'anthropic_messages', 'embeddings')),
    CONSTRAINT chk_provider_protocol_endpoint_version CHECK (version > 0)
);

CREATE INDEX IF NOT EXISTS idx_provider_protocol_endpoints_family_enabled
    ON provider_protocol_endpoints(protocol_family, enabled);

CREATE TABLE IF NOT EXISTS logical_models (
    id           BIGSERIAL PRIMARY KEY,
    name         VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    version      BIGINT NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_logical_models_version CHECK (version > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_logical_models_lower_name ON logical_models(LOWER(name));
CREATE INDEX IF NOT EXISTS idx_logical_models_enabled ON logical_models(enabled);

CREATE TABLE IF NOT EXISTS provider_model_capabilities (
    id                   BIGSERIAL PRIMARY KEY,
    provider_id          BIGINT NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    logical_model_id     BIGINT NOT NULL REFERENCES logical_models(id) ON DELETE RESTRICT,
    endpoint_id          BIGINT REFERENCES provider_protocol_endpoints(id) ON DELETE SET NULL,
    protocol_family      VARCHAR(32) NOT NULL,
    upstream_model       VARCHAR(200) NOT NULL,
    wire_profile         VARCHAR(64) NOT NULL DEFAULT 'canonical_v1',
    feature_profile      VARCHAR(64) NOT NULL,
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    legacy_compatibility BOOLEAN NOT NULL DEFAULT FALSE,
    version              BIGINT NOT NULL DEFAULT 1,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_provider_model_capability UNIQUE (provider_id, logical_model_id, protocol_family),
    CONSTRAINT chk_provider_model_capability_protocol CHECK (protocol_family IN ('chat_completions', 'responses', 'anthropic_messages', 'embeddings')),
    CONSTRAINT chk_provider_model_capability_version CHECK (version > 0),
    CONSTRAINT chk_provider_model_capability_embedding CHECK (
        (protocol_family = 'embeddings' AND feature_profile = 'embeddings_v1') OR
        (protocol_family <> 'embeddings' AND feature_profile <> 'embeddings_v1')
    )
);

CREATE INDEX IF NOT EXISTS idx_provider_capabilities_lookup
    ON provider_model_capabilities(logical_model_id, protocol_family, enabled);
CREATE INDEX IF NOT EXISTS idx_provider_capabilities_provider
    ON provider_model_capabilities(provider_id, enabled);

CREATE TABLE IF NOT EXISTS provider_migration_reviews (
    id               BIGSERIAL PRIMARY KEY,
    provider_id      BIGINT NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    group_id         BIGINT REFERENCES groups(id) ON DELETE CASCADE,
    status           VARCHAR(32) NOT NULL DEFAULT 'pending',
    reason           TEXT NOT NULL DEFAULT '',
    evidence         JSONB NOT NULL DEFAULT '{}'::jsonb,
    snapshot_version BIGINT NOT NULL DEFAULT 1,
    reviewed_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_provider_migration_review_status CHECK (status IN ('pending', 'approved', 'rejected')),
    CONSTRAINT chk_provider_migration_review_version CHECK (snapshot_version > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_provider_migration_review_scope
    ON provider_migration_reviews(provider_id, COALESCE(group_id, 0), snapshot_version);
CREATE INDEX IF NOT EXISTS idx_provider_migration_reviews_status
    ON provider_migration_reviews(status);

CREATE TABLE IF NOT EXISTS group_route_snapshots (
    id          BIGSERIAL PRIMARY KEY,
    group_id    BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    version     BIGINT NOT NULL,
    status      VARCHAR(32) NOT NULL DEFAULT 'draft',
    manifest    JSONB NOT NULL DEFAULT '{}'::jsonb,
    shadow_diff JSONB NOT NULL DEFAULT '{}'::jsonb,
    approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_group_route_snapshot UNIQUE (group_id, version),
    CONSTRAINT chk_group_route_snapshot_status CHECK (status IN ('draft', 'review_required', 'approved', 'active', 'superseded')),
    CONSTRAINT chk_group_route_snapshot_version CHECK (version > 0)
);

CREATE INDEX IF NOT EXISTS idx_group_route_snapshots_status
    ON group_route_snapshots(group_id, status);

ALTER TABLE groups ADD COLUMN IF NOT EXISTS active_route_snapshot_version BIGINT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS previous_route_snapshot_version BIGINT;

ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS logical_model VARCHAR(100);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS ingress_protocol VARCHAR(32);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_protocol VARCHAR(32);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS route_identity VARCHAR(255);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS wire_profile VARCHAR(64);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS conversion_used BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS raw_upstream_usage JSONB;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS usage_completeness VARCHAR(16) NOT NULL DEFAULT 'complete';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_usage_logs_usage_completeness'
    ) THEN
        ALTER TABLE usage_logs ADD CONSTRAINT chk_usage_logs_usage_completeness
            CHECK (usage_completeness IN ('complete', 'partial', 'missing'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS provider_route_attempts (
    id                  BIGSERIAL PRIMARY KEY,
    trace_id            VARCHAR(64) NOT NULL,
    group_id            BIGINT NOT NULL,
    provider_id         BIGINT NOT NULL,
    capability_id       BIGINT NOT NULL,
    endpoint_id         BIGINT NOT NULL DEFAULT 0,
    route_identity      VARCHAR(512) NOT NULL,
    logical_model       VARCHAR(100) NOT NULL,
    upstream_model      VARCHAR(200) NOT NULL DEFAULT '',
    ingress_protocol    VARCHAR(32) NOT NULL,
    upstream_protocol   VARCHAR(32) NOT NULL,
    wire_profile        VARCHAR(64) NOT NULL DEFAULT '',
    route_tier          VARCHAR(16) NOT NULL,
    conversion_used     BOOLEAN NOT NULL DEFAULT FALSE,
    outcome             VARCHAR(16) NOT NULL,
    status_code         INTEGER NOT NULL DEFAULT 0,
    failure_category    VARCHAR(64) NOT NULL DEFAULT '',
    upstream_request_id VARCHAR(255) NOT NULL DEFAULT '',
    duration_ms         BIGINT NOT NULL DEFAULT 0,
    bytes_committed     BIGINT NOT NULL DEFAULT 0,
    final_reason        TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_provider_route_attempt_tier CHECK (route_tier IN ('native', 'conversion')),
    CONSTRAINT chk_provider_route_attempt_outcome CHECK (outcome IN ('succeeded', 'failed', 'rejected')),
    CONSTRAINT chk_provider_route_attempt_protocols CHECK (
        ingress_protocol IN ('chat_completions', 'responses', 'anthropic_messages', 'embeddings') AND
        upstream_protocol IN ('chat_completions', 'responses', 'anthropic_messages', 'embeddings')
    )
);

CREATE INDEX IF NOT EXISTS idx_provider_route_attempts_trace
    ON provider_route_attempts(trace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_provider_route_attempts_created
    ON provider_route_attempts(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_provider_route_attempts_group
    ON provider_route_attempts(group_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_provider_route_attempts_provider
    ON provider_route_attempts(provider_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_provider_route_attempts_protocols
    ON provider_route_attempts(ingress_protocol, upstream_protocol, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_provider_route_attempts_outcome
    ON provider_route_attempts(outcome, failure_category, created_at DESC);

-- Conservative additive backfill. Every account becomes a provider identity,
-- but no model/protocol capability or adapter permission is inferred here.
-- Existing groups remain on the legacy router until an operator declares,
-- tests and approves a provider route snapshot.
INSERT INTO provider_profiles (
    id, display_name, status, allow_protocol_conversion, base_url, auth_type, version, created_at, updated_at
)
SELECT
    a.id,
    a.name,
    CASE WHEN a.status = 'active' THEN 'review_required' ELSE 'disabled' END,
    FALSE,
    NULLIF(a.credentials->>'base_url', ''),
    a.type,
    1,
    a.created_at,
    a.updated_at
FROM accounts a
WHERE a.deleted_at IS NULL
ON CONFLICT (id) DO NOTHING;

INSERT INTO provider_migration_reviews (provider_id, status, reason, evidence, snapshot_version)
SELECT
    p.id,
    'pending',
    'model/protocol capability requires evidence-based migration review',
    jsonb_build_object('legacy_platform', a.platform, 'legacy_type', a.type),
    1
FROM provider_profiles p
JOIN accounts a ON a.id = p.id
WHERE NOT EXISTS (
    SELECT 1 FROM provider_migration_reviews r
    WHERE r.provider_id = p.id AND r.group_id IS NULL AND r.snapshot_version = 1
);
