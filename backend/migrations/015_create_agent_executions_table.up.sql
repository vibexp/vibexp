CREATE TABLE IF NOT EXISTS agent_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'error')),
    input JSONB NOT NULL DEFAULT '{}',
    output JSONB DEFAULT '{}',
    error TEXT,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMP WITH TIME ZONE,
    duration INTEGER -- duration in milliseconds
);

-- Create indexes for better query performance
CREATE INDEX idx_agent_executions_agent_id ON agent_executions(agent_id);
CREATE INDEX idx_agent_executions_user_id ON agent_executions(user_id);
CREATE INDEX idx_agent_executions_status ON agent_executions(status);
CREATE INDEX idx_agent_executions_started_at ON agent_executions(started_at);
CREATE INDEX idx_agent_executions_user_agent ON agent_executions(user_id, agent_id);
CREATE INDEX idx_agent_executions_user_status ON agent_executions(user_id, status);

-- Add constraint to ensure ended_at is after started_at when present
ALTER TABLE agent_executions ADD CONSTRAINT check_execution_time_order
    CHECK (ended_at IS NULL OR ended_at >= started_at);
