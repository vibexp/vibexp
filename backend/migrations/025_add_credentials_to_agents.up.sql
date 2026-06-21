-- Add credentials column to agents table for storing encrypted authentication credentials
ALTER TABLE agents
ADD COLUMN credentials JSONB;

-- Add comment to explain the structure
COMMENT ON COLUMN agents.credentials IS 'Encrypted authentication credentials stored as JSONB with structure: {"security_scheme_name": {"type": "apiKey"|"http", "value": "encrypted_credential", "metadata": {}}}';

-- Create index for faster lookups when checking if agent has credentials
CREATE INDEX idx_agents_credentials ON agents USING GIN (credentials) WHERE credentials IS NOT NULL;
