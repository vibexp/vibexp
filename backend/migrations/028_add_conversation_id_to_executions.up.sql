-- Add conversation_id to agent_executions table for conversation tracking
-- This enables grouping related executions into conversations
ALTER TABLE agent_executions
    ADD COLUMN conversation_id VARCHAR(255) NULL;

-- Create index for efficient conversation queries
-- This index includes user_id, agent_id, and conversation_id for optimal query performance
CREATE INDEX idx_agent_executions_conversation
    ON agent_executions(user_id, agent_id, conversation_id)
    WHERE conversation_id IS NOT NULL;

-- Add comment to explain the field
COMMENT ON COLUMN agent_executions.conversation_id IS 'Groups related executions into conversations. First execution generates this ID, subsequent messages in same conversation use the same ID.';
