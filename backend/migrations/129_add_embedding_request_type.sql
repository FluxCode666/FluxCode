-- Extend usage_logs.request_type for synchronous embedding requests.
-- Replacing the named constraint is idempotent and preserves all existing values.
ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type IN (0, 1, 2, 3, 4));
