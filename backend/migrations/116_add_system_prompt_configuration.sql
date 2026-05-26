-- 116_add_system_prompt_configuration.sql
-- Adds hierarchical system prompt configuration for API keys, groups, and platform defaults.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS system_prompt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS system_prompt_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS system_prompt TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS system_prompt_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';

INSERT INTO settings (key, value)
VALUES
    ('system_prompt_anthropic', ''),
    ('system_prompt_mode_anthropic', 'inherit'),
    ('system_prompt_openai', ''),
    ('system_prompt_mode_openai', 'inherit'),
    ('system_prompt_gemini', ''),
    ('system_prompt_mode_gemini', 'inherit'),
    ('system_prompt_antigravity', ''),
    ('system_prompt_mode_antigravity', 'inherit')
ON CONFLICT (key) DO NOTHING;
