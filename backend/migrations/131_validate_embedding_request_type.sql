-- Validation uses a weaker lock than adding a validated CHECK and does not
-- block ordinary inserts/updates on the hot usage table.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE usage_logs
    VALIDATE CONSTRAINT usage_logs_request_type_check_embedding;
