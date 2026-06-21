-- Re-add the api_key_hash column
ALTER TABLE claude_plugin_marketplaces ADD COLUMN api_key_hash VARCHAR(255);

-- Re-create the index
CREATE INDEX idx_claude_plugin_marketplaces_api_key_hash ON claude_plugin_marketplaces(api_key_hash);
