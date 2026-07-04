-- 125_add_group_allow_image_generation.sql
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_image_generation BOOLEAN NOT NULL DEFAULT FALSE;
