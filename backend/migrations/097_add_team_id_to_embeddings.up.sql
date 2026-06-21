-- Denormalize team_id onto embeddings so team-scoped semantic search filters
-- directly on this table (no per-entity source join for the team filter) and so
-- entity deletes are no longer keyed on the deleter's user_id (orphan fix).
-- Nullable + backfilled; rows whose source entity is gone stay NULL (harmless --
-- the inner join in search excludes them anyway).
--
-- INVARIANT: this denormalization is only correct while a source entity's team_id
-- is immutable (today the entity Update SQL pins team_id and team transfers are
-- rejected). If a "move resource to another team" feature is ever added, it MUST
-- re-embed the entity (or directly update embeddings.team_id), otherwise
-- team-scoped search would silently mis-scope the stale chunks.
--
-- SCALE NOTE: the backfill UPDATEs and the (non-concurrent) CREATE INDEX run inside
-- golang-migrate's single startup transaction -- fine at current volume; if the
-- embeddings table grows large, split the index into a separately, concurrently
-- applied step to avoid holding heavy locks while a new revision boots.
ALTER TABLE embeddings ADD COLUMN team_id UUID REFERENCES teams(id) ON DELETE CASCADE;

CREATE INDEX idx_embeddings_team_id_entity ON embeddings(team_id, entity_type);

-- Backfill by joining embeddings.entity_id -> each source table's id.
UPDATE embeddings e SET team_id = src.team_id FROM prompts src
  WHERE e.entity_type = 'prompt' AND e.entity_id = src.id;
UPDATE embeddings e SET team_id = src.team_id FROM artifacts src
  WHERE e.entity_type = 'artifact' AND e.entity_id = src.id;
UPDATE embeddings e SET team_id = src.team_id FROM blueprints src
  WHERE e.entity_type = 'blueprint' AND e.entity_id = src.id;
UPDATE embeddings e SET team_id = src.team_id FROM memories src
  WHERE e.entity_type = 'memory' AND e.entity_id = src.id;
UPDATE embeddings e SET team_id = src.team_id FROM feed_items src
  WHERE e.entity_type = 'feed_item' AND e.entity_id = src.id;
UPDATE embeddings e SET team_id = src.team_id FROM feed_item_replies src
  WHERE e.entity_type = 'feed_item_reply' AND e.entity_id = src.id;
