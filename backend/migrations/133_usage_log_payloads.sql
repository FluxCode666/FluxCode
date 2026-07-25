-- Persist successful gateway request/response payloads for authorized employee analysis.
-- Payloads are encrypted before they enter the Redis Stream and are inserted here by
-- an at-least-once consumer. event_id provides the database-side idempotency guard.

CREATE TABLE IF NOT EXISTS usage_log_payloads (
    id                      BIGSERIAL PRIMARY KEY,
    event_id                UUID         NOT NULL,
    usage_log_id            BIGINT REFERENCES usage_logs(id) ON DELETE SET NULL,
    user_id                 BIGINT       NOT NULL,
    api_key_id              BIGINT       NOT NULL,
    group_id                BIGINT,
    trace_id                VARCHAR(64),
    request_id              VARCHAR(64)  NOT NULL,
    client_request_id       VARCHAR(128),
    method                  VARCHAR(8)   NOT NULL,
    endpoint                VARCHAR(256) NOT NULL,
    route_pattern           VARCHAR(256),
    model                   VARCHAR(100),
    status_code             SMALLINT     NOT NULL,
    stream                  BOOLEAN      NOT NULL DEFAULT FALSE,
    client_disconnected     BOOLEAN      NOT NULL DEFAULT FALSE,
    duration_ms             INTEGER      NOT NULL DEFAULT 0,
    request_content_type    VARCHAR(128),
    response_content_type   VARCHAR(128),
    request_body            TEXT,
    response_body           TEXT,
    request_body_bytes      BIGINT       NOT NULL DEFAULT 0,
    response_body_bytes     BIGINT       NOT NULL DEFAULT 0,
    request_body_truncated  BOOLEAN      NOT NULL DEFAULT FALSE,
    response_body_truncated BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at              TIMESTAMPTZ  NOT NULL,
    recorded_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_usage_log_payloads_event_id UNIQUE (event_id),
    CONSTRAINT chk_usage_log_payloads_status
        CHECK (status_code >= 200 AND status_code < 300),
    CONSTRAINT chk_usage_log_payloads_body_bytes
        CHECK (request_body_bytes >= 0 AND response_body_bytes >= 0),
    CONSTRAINT chk_usage_log_payloads_duration
        CHECK (duration_ms >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_usage_log_payloads_usage_log_id
    ON usage_log_payloads (usage_log_id)
    WHERE usage_log_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_log_payloads_user_created
    ON usage_log_payloads (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_usage_log_payloads_api_key_created
    ON usage_log_payloads (api_key_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_usage_log_payloads_model_created
    ON usage_log_payloads (model, created_at DESC)
    WHERE model IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_log_payloads_unlinked
    ON usage_log_payloads (created_at, id)
    WHERE usage_log_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_usage_log_payloads_created_brin
    ON usage_log_payloads USING BRIN (created_at);

COMMENT ON TABLE usage_log_payloads IS
    'Raw successful gateway request/response payloads for restricted employee analysis; rows are deleted only by explicit administrator action.';
COMMENT ON COLUMN usage_log_payloads.usage_log_id IS
    'Optional one-to-one link to usage_logs; set to NULL when the usage log is deleted so payload lifecycle remains administrator-controlled.';
COMMENT ON COLUMN usage_log_payloads.request_body IS
    'Raw JSON request payload. NULL means unsupported content type, empty body, or over capture limit.';
COMMENT ON COLUMN usage_log_payloads.response_body IS
    'Raw JSON/SSE/text response payload. NULL means unsupported content type, empty body, or over capture limit.';
