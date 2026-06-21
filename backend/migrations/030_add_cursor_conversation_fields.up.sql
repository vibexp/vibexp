-- Add missing columns to cursor_ide_hooks_payload table
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'cursor_ide_hooks_payload' AND column_name = 'conversation_id') THEN
        ALTER TABLE cursor_ide_hooks_payload ADD COLUMN conversation_id VARCHAR(255);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'cursor_ide_hooks_payload' AND column_name = 'generation_id') THEN
        ALTER TABLE cursor_ide_hooks_payload ADD COLUMN generation_id VARCHAR(255);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'cursor_ide_hooks_payload' AND column_name = 'workspace_roots') THEN
        ALTER TABLE cursor_ide_hooks_payload ADD COLUMN workspace_roots TEXT[];
    END IF;
END $$;

-- Create indexes for the new columns (only if they don't exist)
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_conversation_id ON cursor_ide_hooks_payload(conversation_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_generation_id ON cursor_ide_hooks_payload(generation_id);
