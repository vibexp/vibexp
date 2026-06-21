-- Drop existing constraint
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;

-- Restore original constraint without claude_plugin_marketplace
ALTER TABLE api_keys ADD CONSTRAINT chk_usage_type
CHECK (usage_type IN ('ai_tools', 'cli', 'mcp', 'everything'));
