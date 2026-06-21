-- Add last_synced_at column to agents table to track when agent card was last synced
ALTER TABLE agents ADD COLUMN last_synced_at TIMESTAMP WITH TIME ZONE;

-- Create index for better query performance
CREATE INDEX idx_agents_last_synced_at ON agents(last_synced_at);
