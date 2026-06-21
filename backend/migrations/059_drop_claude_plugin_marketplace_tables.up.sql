-- Drop claude plugin marketplace tables
-- These tables are no longer needed as the feature is being removed

-- Drop tables in reverse order to respect foreign key constraints
DROP TABLE IF EXISTS claude_plugin_ratings;
DROP TABLE IF EXISTS claude_plugin_installations;
DROP TABLE IF EXISTS claude_plugin_marketplace_assignments;
DROP TABLE IF EXISTS claude_plugin_marketplaces;

-- Remove claude_plugin_marketplace from api_keys usage_type constraint
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;
ALTER TABLE api_keys ADD CONSTRAINT chk_usage_type
CHECK (usage_type IN ('ai_tools', 'cli', 'mcp', 'everything'));
