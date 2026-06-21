-- Remove legacy tracking fields
ALTER TABLE api_keys DROP COLUMN IF EXISTS migration_notes;
ALTER TABLE api_keys DROP COLUMN IF EXISTS is_legacy;
