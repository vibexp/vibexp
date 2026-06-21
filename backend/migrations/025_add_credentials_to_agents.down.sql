-- Remove credentials column and its index
DROP INDEX IF EXISTS idx_agents_credentials;

ALTER TABLE agents
DROP COLUMN IF EXISTS credentials;
