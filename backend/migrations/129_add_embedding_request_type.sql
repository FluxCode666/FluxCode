-- Extend usage_logs.request_type without scanning the hot table while holding
-- the replacement lock. Migration 131 validates the new constraint under the
-- weaker PostgreSQL validation lock after this short transaction commits.
SET LOCAL lock_timeout = '5s';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_logs_request_type_check_embedding'
          AND conrelid = 'usage_logs'::regclass
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_request_type_check_embedding
            CHECK (request_type IN (0, 1, 2, 3, 4)) NOT VALID;
    END IF;
END
$$;

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;
