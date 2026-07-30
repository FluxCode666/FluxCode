-- 单独以非事务方式创建索引，避免在大型 users 表上阻塞写入。
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_users_user_access_key_hash
    ON users (user_access_key_hash)
    WHERE user_access_key_hash IS NOT NULL;
