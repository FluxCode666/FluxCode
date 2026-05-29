-- 117_system_prompt_user_scope.sql
-- Adds user-scope control defaults for system prompt injection.

INSERT INTO settings (key, value)
VALUES
    ('system_prompt_user_scope_enabled', 'false'),
    ('system_prompt_user_scope_mode', 'all'),
    ('system_prompt_user_scope_user_ids', '[]')
ON CONFLICT (key) DO NOTHING;
