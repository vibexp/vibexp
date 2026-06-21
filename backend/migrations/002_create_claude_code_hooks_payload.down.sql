-- Drop trigger and function
DROP TRIGGER IF EXISTS update_claude_code_hooks_payload_updated_at ON claude_code_hooks_payload;
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop indexes
DROP INDEX IF EXISTS idx_claude_code_hooks_created_at;
DROP INDEX IF EXISTS idx_claude_code_hooks_tool_name;
DROP INDEX IF EXISTS idx_claude_code_hooks_event_name;
DROP INDEX IF EXISTS idx_claude_code_hooks_session_id;

-- Drop table
DROP TABLE IF EXISTS claude_code_hooks_payload;
