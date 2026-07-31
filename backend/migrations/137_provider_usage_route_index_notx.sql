-- usage_logs is a hot table. Build the provider-route diagnostic index online
-- after migration 134 has added the referenced columns.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_provider_route
    ON usage_logs(account_id, ingress_protocol, upstream_protocol, created_at DESC);
