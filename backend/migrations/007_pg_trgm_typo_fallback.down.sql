-- Revert issue #188 pg_trgm typo-tolerance fallback indexes, and restore issue
-- #183's title-weighted FTS indexes (reversing the two changes this migration's
-- up applied).
--
-- Drop only the trigram indexes. The pg_trgm extension itself is left in place:
-- dropping a shared extension in a down-migration is risky (other objects may
-- come to depend on it), matching how migration 001 never drops the `vector`
-- extension it creates.

DROP INDEX IF EXISTS idx_prompts_title_trgm;
DROP INDEX IF EXISTS idx_artifacts_title_trgm;
DROP INDEX IF EXISTS idx_blueprints_title_trgm;
DROP INDEX IF EXISTS idx_memories_title_trgm;

-- Restore the title-weighted FTS indexes (setweight title=A/body=D). Memories
-- have no real title, so they use a single D-weighted body vector.
DROP INDEX IF EXISTS idx_prompts_fts;
DROP INDEX IF EXISTS idx_artifacts_fts;
DROP INDEX IF EXISTS idx_blueprints_fts;
DROP INDEX IF EXISTS idx_memories_fts;

CREATE INDEX IF NOT EXISTS idx_prompts_fts ON prompts
    USING gin ((setweight(to_tsvector('english', coalesce(name, '')), 'A') || setweight(to_tsvector('english', coalesce(body, '')), 'D')));

CREATE INDEX IF NOT EXISTS idx_artifacts_fts ON artifacts
    USING gin ((setweight(to_tsvector('english', coalesce(title, '')), 'A') || setweight(to_tsvector('english', coalesce(content, '')), 'D')));

CREATE INDEX IF NOT EXISTS idx_blueprints_fts ON blueprints
    USING gin ((setweight(to_tsvector('english', coalesce(title, '')), 'A') || setweight(to_tsvector('english', coalesce(content, '')), 'D')));

CREATE INDEX IF NOT EXISTS idx_memories_fts ON memories
    USING gin (setweight(to_tsvector('english', coalesce(text, '')), 'D'));
