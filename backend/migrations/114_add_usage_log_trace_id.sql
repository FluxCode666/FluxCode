ALTER TABLE usage_logs
	ADD COLUMN IF NOT EXISTS trace_id VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_usage_logs_trace_id ON usage_logs(trace_id);
