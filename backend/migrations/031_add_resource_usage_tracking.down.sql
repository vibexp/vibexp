-- Drop trigger first
DROP TRIGGER IF EXISTS update_resource_usage_timestamp ON resource_usage;

-- Drop function
DROP FUNCTION IF EXISTS resource_usage_update_timestamp();

-- Drop indexes
DROP INDEX IF EXISTS idx_resource_usage_user_period;
DROP INDEX IF EXISTS idx_resource_usage_resource_type;

-- Drop table
DROP TABLE IF EXISTS resource_usage;
