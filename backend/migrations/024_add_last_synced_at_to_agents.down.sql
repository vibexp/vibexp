-- Remove last_synced_at column and its index
DROP INDEX IF EXISTS idx_agents_last_synced_at;
ALTER TABLE agents DROP COLUMN IF EXISTS last_synced_at;
