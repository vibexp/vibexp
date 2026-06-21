-- Rollback: Remove conversation_id from agent_executions table
DROP INDEX IF EXISTS idx_agent_executions_conversation;

ALTER TABLE agent_executions
    DROP COLUMN IF EXISTS conversation_id;
