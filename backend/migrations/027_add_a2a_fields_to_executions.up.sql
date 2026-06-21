-- Add A2A protocol fields to agent_executions table
-- All fields are nullable to maintain backward compatibility
ALTER TABLE agent_executions
    ADD COLUMN task_id VARCHAR(255) NULL,
    ADD COLUMN context_id VARCHAR(255) NULL,
    ADD COLUMN current_state VARCHAR(50) NULL,
    ADD COLUMN artifacts JSONB DEFAULT '[]';

-- Update status constraint to include A2A states
-- Keep existing states (running, success, error) and add new A2A states
ALTER TABLE agent_executions
    DROP CONSTRAINT IF EXISTS agent_executions_status_check;

ALTER TABLE agent_executions
    ADD CONSTRAINT agent_executions_status_check
    CHECK (status IN ('running', 'success', 'error', 'pending', 'submitted', 'working', 'completed', 'failed', 'cancelled'));

-- Create indexes for A2A fields (only for non-NULL values)
CREATE INDEX idx_agent_executions_task_id ON agent_executions(task_id) WHERE task_id IS NOT NULL;
CREATE INDEX idx_agent_executions_context_id ON agent_executions(context_id) WHERE context_id IS NOT NULL;
CREATE INDEX idx_agent_executions_current_state ON agent_executions(current_state) WHERE current_state IS NOT NULL;
