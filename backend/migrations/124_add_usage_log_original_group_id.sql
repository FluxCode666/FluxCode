-- 记录兜底请求的入口原分组；group_id 仍表示实际承接/计费分组。
ALTER TABLE usage_logs
ADD COLUMN IF NOT EXISTS original_group_id BIGINT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_original_group_id_created_at
ON usage_logs(original_group_id, created_at)
WHERE original_group_id IS NOT NULL;

COMMENT ON COLUMN usage_logs.original_group_id IS '触发兜底前的入口原分组 ID；NULL 表示未发生分组兜底';
