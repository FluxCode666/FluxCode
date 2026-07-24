-- Keep Ops error ingestion writable while the correlation index is built.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_channel_id_created_at
    ON ops_error_logs(channel_id, created_at DESC)
    WHERE channel_id IS NOT NULL;
