-- 127_channel_model_pricing_capabilities.sql
-- Add display-only capability tags for public model pricing pages.

ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
