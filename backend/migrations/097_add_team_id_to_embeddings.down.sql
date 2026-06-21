DROP INDEX IF EXISTS idx_embeddings_team_id_entity;
ALTER TABLE embeddings DROP COLUMN IF EXISTS team_id;
