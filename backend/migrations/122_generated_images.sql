-- 122_generated_images.sql
-- Archive generated images for the admin gallery across OpenAI and future image providers.

CREATE TABLE IF NOT EXISTS generated_images (
    id              BIGSERIAL PRIMARY KEY,
    provider        VARCHAR(32)  NOT NULL DEFAULT 'openai',
    user_id         BIGINT       NOT NULL,
    api_key_id      BIGINT       NOT NULL,
    account_id      BIGINT       NOT NULL,
    request_id      VARCHAR(128),
    model           VARCHAR(100),
    prompt          TEXT,
    revised_prompt  TEXT,
    response_format VARCHAR(20)  NOT NULL DEFAULT 'b64_json',
    source          VARCHAR(32)  NOT NULL DEFAULT 'b64_json',
    content_type    VARCHAR(100) NOT NULL DEFAULT 'image/png',
    image_data      BYTEA        NOT NULL,
    size_bytes      INT          NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS generatedimage_created_at
    ON generated_images (created_at);
CREATE INDEX IF NOT EXISTS generatedimage_provider_created_at
    ON generated_images (provider, created_at);
CREATE INDEX IF NOT EXISTS generatedimage_user_id_created_at
    ON generated_images (user_id, created_at);
CREATE INDEX IF NOT EXISTS generatedimage_api_key_id_created_at
    ON generated_images (api_key_id, created_at);
CREATE INDEX IF NOT EXISTS generatedimage_account_id_created_at
    ON generated_images (account_id, created_at);
CREATE INDEX IF NOT EXISTS generatedimage_request_id
    ON generated_images (request_id);
