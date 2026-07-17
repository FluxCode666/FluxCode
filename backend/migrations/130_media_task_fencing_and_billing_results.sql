ALTER TABLE media_tasks
    ADD COLUMN IF NOT EXISTS claim_token VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS additional_charged_amount NUMERIC(20,8) NOT NULL DEFAULT 0;

DROP INDEX IF EXISTS idx_media_artifacts_task;
