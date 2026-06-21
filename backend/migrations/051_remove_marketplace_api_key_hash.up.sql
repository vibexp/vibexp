-- Drop the api_key_hash index
DROP INDEX IF EXISTS idx_claude_plugin_marketplaces_api_key_hash;

-- Remove the api_key_hash column from claude_plugin_marketplaces table
ALTER TABLE claude_plugin_marketplaces DROP COLUMN IF EXISTS api_key_hash;
