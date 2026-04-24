-- 为 outbox 去重查询添加复合索引，避免全表扫描。
-- 去重 SQL: WHERE event_type = $1 AND account_id IS NOT DISTINCT FROM $2
--           AND group_id IS NOT DISTINCT FROM $3 AND created_at >= ...
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_scheduler_outbox_dedup
    ON scheduler_outbox (event_type, account_id, group_id, created_at DESC);
