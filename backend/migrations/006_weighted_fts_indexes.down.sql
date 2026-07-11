-- Revert issue #183 weighted FTS indexes: restore the migration-002 unweighted
-- combined title+body GIN indexes.

DROP INDEX IF EXISTS idx_prompts_fts;
DROP INDEX IF EXISTS idx_artifacts_fts;
DROP INDEX IF EXISTS idx_blueprints_fts;
DROP INDEX IF EXISTS idx_memories_fts;

CREATE INDEX IF NOT EXISTS idx_prompts_fts ON prompts
    USING gin (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(body, '')));

CREATE INDEX IF NOT EXISTS idx_artifacts_fts ON artifacts
    USING gin (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '')));

CREATE INDEX IF NOT EXISTS idx_blueprints_fts ON blueprints
    USING gin (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '')));

CREATE INDEX IF NOT EXISTS idx_memories_fts ON memories
    USING gin (to_tsvector('english', coalesce(LEFT(text, 100), '') || ' ' || coalesce(text, '')));
