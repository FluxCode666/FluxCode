-- Add canonical media routing metadata and explicit artifact storage provenance.

ALTER TABLE media_model_definitions
    ADD COLUMN IF NOT EXISTS vendor VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS default_adapter VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS default_async_mode VARCHAR(16) NOT NULL DEFAULT 'unsupported';

ALTER TABLE media_artifacts
    ADD COLUMN IF NOT EXISTS storage_provider VARCHAR(32) NOT NULL DEFAULT 'legacy';

CREATE TABLE IF NOT EXISTS media_model_aliases (
    id BIGSERIAL PRIMARY KEY,
    requested_model_id VARCHAR(128) NOT NULL UNIQUE,
    model_definition_id BIGINT NOT NULL REFERENCES media_model_definitions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS group_media_model_scopes (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    model_definition_id BIGINT NOT NULL REFERENCES media_model_definitions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, model_definition_id)
);

CREATE INDEX IF NOT EXISTS idx_media_model_aliases_definition
    ON media_model_aliases(model_definition_id);
CREATE INDEX IF NOT EXISTS idx_group_media_model_scopes_definition
    ON group_media_model_scopes(model_definition_id);
