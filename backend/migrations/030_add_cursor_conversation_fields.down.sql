-- Rollback: Remove the added columns
DROP INDEX IF EXISTS idx_cursor_ide_hooks_generation_id;
DROP INDEX IF EXISTS idx_cursor_ide_hooks_conversation_id;

ALTER TABLE cursor_ide_hooks_payload
DROP COLUMN IF EXISTS workspace_roots,
DROP COLUMN IF EXISTS generation_id,
DROP COLUMN IF EXISTS conversation_id;
