-- Preserve safe channel correlation for embedding Ops records without storing content.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS channel_id BIGINT REFERENCES channels(id) ON DELETE SET NULL;

COMMENT ON COLUMN ops_error_logs.channel_id IS 'Pricing channel selected for safe operational correlation; request/response content is not stored.';
