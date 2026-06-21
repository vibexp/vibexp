-- Drop check constraint
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;

-- Drop index
DROP INDEX IF EXISTS idx_api_keys_usage_type;

-- Remove usage_type column
ALTER TABLE api_keys DROP COLUMN usage_type;
