-- Drop the cursor_ide_hooks_payload table and its indexes
DROP TRIGGER IF EXISTS update_cursor_ide_hooks_payload_updated_at ON cursor_ide_hooks_payload;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_user_id;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_created_at;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_tool_name;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_event_name;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_generation_id;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_conversation_id;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_session_id;
DROP TABLE IF EXISTS cursor_ide_hooks_payload;
