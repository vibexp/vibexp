-- Rollback migration: remove permissions and reset legacy flags
DELETE FROM api_key_integration_permissions WHERE api_key_id IN (SELECT id FROM api_keys WHERE is_legacy = true);

UPDATE api_keys
SET is_legacy = false,
    migration_notes = NULL
WHERE is_legacy = true;
