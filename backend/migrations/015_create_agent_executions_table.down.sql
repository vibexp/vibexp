-- Drop indexes
DROP INDEX IF EXISTS idx_agent_executions_agent_id;
DROP INDEX IF EXISTS idx_agent_executions_user_id;
DROP INDEX IF EXISTS idx_agent_executions_status;
DROP INDEX IF EXISTS idx_agent_executions_started_at;
DROP INDEX IF EXISTS idx_agent_executions_user_agent;
DROP INDEX IF EXISTS idx_agent_executions_user_status;

-- Drop table
DROP TABLE IF EXISTS agent_executions;
