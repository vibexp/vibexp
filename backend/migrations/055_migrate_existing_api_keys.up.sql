-- Migrate existing API keys to new permission model

-- 1:1 mapping for specific types (ai_tools, cli)
INSERT INTO api_key_integration_permissions (api_key_id, integration_code)
SELECT
    ak.id,
    ak.usage_type as integration_code
FROM api_keys ak
WHERE ak.usage_type IN ('ai_tools', 'cli')
  AND ak.is_legacy = false;

-- For 'mcp' usage_type, map to 'mcp_server' integration_code
INSERT INTO api_key_integration_permissions (api_key_id, integration_code)
SELECT
    ak.id,
    'mcp_server' as integration_code
FROM api_keys ak
WHERE ak.usage_type = 'mcp'
  AND ak.is_legacy = false;

-- For 'claude_plugin_marketplace' usage_type, map to 'marketplace' integration_code
INSERT INTO api_key_integration_permissions (api_key_id, integration_code)
SELECT
    ak.id,
    'marketplace' as integration_code
FROM api_keys ak
WHERE ak.usage_type = 'claude_plugin_marketplace'
  AND ak.is_legacy = false;

-- For 'everything' keys, grant all integrations
INSERT INTO api_key_integration_permissions (api_key_id, integration_code)
SELECT ak.id, ic.integration_code
FROM api_keys ak
CROSS JOIN api_key_integrations_catalog ic
WHERE ak.usage_type = 'everything'
  AND ic.is_active = true
  AND ak.is_legacy = false;

-- Mark all migrated keys as legacy
UPDATE api_keys
SET is_legacy = true,
    migration_notes = 'Migrated from usage_type: ' || usage_type
WHERE is_legacy = false;
