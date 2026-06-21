-- Remove agent card support from agents table
ALTER TABLE agents
DROP CONSTRAINT IF EXISTS check_agent_name_or_card;

-- Make name and description required again
ALTER TABLE agents
ALTER COLUMN name SET NOT NULL,
ALTER COLUMN description SET NOT NULL;

-- Drop indexes
DROP INDEX IF EXISTS idx_agents_card_url;

-- Remove columns
ALTER TABLE agents
DROP COLUMN IF EXISTS card_url,
DROP COLUMN IF EXISTS agent_card;
