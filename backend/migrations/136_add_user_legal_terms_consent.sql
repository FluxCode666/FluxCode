-- 用户服务条款同意记录。
-- 新增字段默认 false，确保上线迁移时现有用户都需要重新确认条款。
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS legal_terms_accepted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS legal_terms_version VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS legal_terms_accepted_at TIMESTAMPTZ;

-- The first rollout must invalidate any pre-existing or partially migrated
-- consent state so every account must accept the updated terms when signing in.
UPDATE users
SET legal_terms_accepted = FALSE,
    legal_terms_version = '',
    legal_terms_accepted_at = NULL
WHERE legal_terms_accepted IS DISTINCT FROM FALSE
   OR legal_terms_version IS DISTINCT FROM ''
   OR legal_terms_accepted_at IS NOT NULL;

COMMENT ON COLUMN users.legal_terms_accepted IS
    'Whether the user accepted the currently published legal documents.';
COMMENT ON COLUMN users.legal_terms_version IS
    'Version of the legal documents accepted by the user.';
COMMENT ON COLUMN users.legal_terms_accepted_at IS
    'UTC timestamp when the user accepted the legal documents.';
