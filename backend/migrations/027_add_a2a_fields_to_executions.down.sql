-- Remove A2A fields from agent_executions table
ALTER TABLE agent_executions
    DROP COLUMN IF EXISTS task_id,
    DROP COLUMN IF EXISTS context_id,
    DROP COLUMN IF EXISTS current_state,
    DROP COLUMN IF EXISTS artifacts;

-- Restore original status constraint
ALTER TABLE agent_executions
    DROP CONSTRAINT IF EXISTS agent_executions_status_check;

ALTER TABLE agent_executions
    ADD CONSTRAINT agent_executions_status_check
    CHECK (status IN ('running', 'success', 'error'));

-- Drop indexes (will be automatically removed with columns, but explicit for clarity)
DROP INDEX IF EXISTS idx_agent_executions_task_id;
DROP INDEX IF EXISTS idx_agent_executions_context_id;
DROP INDEX IF EXISTS idx_agent_executions_current_state;
