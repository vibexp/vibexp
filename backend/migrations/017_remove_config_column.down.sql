-- Re-add config column if rollback is needed
ALTER TABLE agents ADD COLUMN config JSONB NOT NULL DEFAULT '{}';
