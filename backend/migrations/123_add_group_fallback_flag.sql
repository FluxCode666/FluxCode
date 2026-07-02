-- 添加分组兜底目标标记
ALTER TABLE groups
ADD COLUMN IF NOT EXISTS is_fallback_group BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_groups_is_fallback_group
ON groups(is_fallback_group)
WHERE deleted_at IS NULL AND is_fallback_group = TRUE;

COMMENT ON COLUMN groups.is_fallback_group IS '是否允许作为其他分组的兜底目标';
