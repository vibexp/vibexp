-- Add agent card support to agents table
ALTER TABLE agents
ADD COLUMN card_url TEXT,
ADD COLUMN agent_card JSONB;

-- Create index for card_url for lookups
CREATE INDEX idx_agents_card_url ON agents(card_url);

-- Update constraints to make name and description optional since they can come from agent card
ALTER TABLE agents
ALTER COLUMN name DROP NOT NULL,
ALTER COLUMN description DROP NOT NULL;

-- Add constraint to ensure either name/description or card_url is provided
ALTER TABLE agents
ADD CONSTRAINT check_agent_name_or_card CHECK (
    (name IS NOT NULL AND description IS NOT NULL) OR
    (card_url IS NOT NULL)
);
