-- Add usage_type column with default value
ALTER TABLE api_keys ADD COLUMN usage_type VARCHAR(20) NOT NULL DEFAULT 'everything';

-- Create index for better query performance
CREATE INDEX idx_api_keys_usage_type ON api_keys(usage_type);

-- Add check constraint to enforce valid usage types at database level
ALTER TABLE api_keys ADD CONSTRAINT chk_usage_type
CHECK (usage_type IN ('ai_tools', 'cli', 'mcp', 'everything'));
