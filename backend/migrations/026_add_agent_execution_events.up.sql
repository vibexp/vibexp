CREATE TABLE IF NOT EXISTS agent_execution_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL REFERENCES agent_executions(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('task', 'status-update', 'artifact-update')),
    event_data JSONB NOT NULL DEFAULT '{}',
    sequence_number INTEGER NOT NULL,
    received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for better query performance
CREATE INDEX idx_agent_execution_events_execution_id ON agent_execution_events(execution_id);
CREATE INDEX idx_agent_execution_events_event_type ON agent_execution_events(event_type);
CREATE INDEX idx_agent_execution_events_sequence ON agent_execution_events(execution_id, sequence_number);

-- Add unique constraint to ensure no duplicate sequence numbers per execution
ALTER TABLE agent_execution_events ADD CONSTRAINT unique_execution_sequence
    UNIQUE (execution_id, sequence_number);
