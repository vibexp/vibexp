-- Drop trigger and function
DROP TRIGGER IF EXISTS agents_updated_at_trigger ON agents;
DROP FUNCTION IF EXISTS update_agents_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_agents_user_id;
DROP INDEX IF EXISTS idx_agents_status;
DROP INDEX IF EXISTS idx_agents_user_status;
DROP INDEX IF EXISTS idx_agents_created_at;
DROP INDEX IF EXISTS idx_agents_last_run;

-- Drop table
DROP TABLE IF EXISTS agents;
