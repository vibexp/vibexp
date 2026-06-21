-- Drop indexes first
DROP INDEX IF EXISTS idx_claude_code_hooks_user_created_at;
DROP INDEX IF EXISTS idx_claude_code_hooks_user_session;
DROP INDEX IF EXISTS idx_claude_code_hooks_user_id;

-- Drop the user_id column
ALTER TABLE claude_code_hooks_payload
DROP COLUMN IF EXISTS user_id;
