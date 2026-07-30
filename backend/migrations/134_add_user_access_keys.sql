-- 为用户级开发者接口保存可恢复的访问密钥。
-- 原文经过应用层 AES-GCM 加密；hash 仅用于按请求密钥快速定位用户。

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS user_access_key_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS user_access_key_encrypted TEXT,
    ADD COLUMN IF NOT EXISTS user_access_key_created_at TIMESTAMPTZ;

COMMENT ON COLUMN users.user_access_key_hash IS
    'SHA-256 hash of the user developer access key, used only for authentication lookup.';
COMMENT ON COLUMN users.user_access_key_encrypted IS
    'AES-GCM encrypted user developer access key. Kept recoverable so its owner can copy it again.';
