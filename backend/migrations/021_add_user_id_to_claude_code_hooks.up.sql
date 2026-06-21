-- Add user_id column to claude_code_hooks_payload table for user isolation
-- Making it nullable initially to allow for data migration

ALTER TABLE claude_code_hooks_payload
ADD COLUMN user_id VARCHAR(255);

-- Create index for better query performance with user filtering
CREATE INDEX idx_claude_code_hooks_user_id ON claude_code_hooks_payload(user_id);

-- Create composite index for common queries (user_id + session_id)
CREATE INDEX idx_claude_code_hooks_user_session ON claude_code_hooks_payload(user_id, session_id);

-- Create composite index for time-based queries with user filtering
CREATE INDEX idx_claude_code_hooks_user_created_at ON claude_code_hooks_payload(user_id, created_at DESC);
