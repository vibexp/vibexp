-- Issue #183: title-weighted keyword-mode (FTS) ranking.
--
-- Keyword-mode search (the fallback used when a team has no embedding provider)
-- now builds its tsvector with setweight() — title = weight A, body = weight D —
-- and ranks with ts_rank_cd, so a query term in a resource's title outranks the
-- same term buried in its body. The migration-002 GIN indexes covered the OLD
-- unweighted expression (to_tsvector(title || ' ' || body)); they can no longer
-- satisfy the weighted expression, so a keyword search would seq-scan.
--
-- Replace them with GIN indexes on the exact weighted expression ftsExpr() now
-- emits (search.go). The expression must stay byte-for-byte in sync with ftsExpr:
-- the query qualifies columns as src.* but Postgres resolves src.name -> name etc.
-- to the same underlying columns, so the qualified query expression still matches
-- these unqualified index expressions. Memories have no real title (their "title"
-- is only LEFT(text, 100)), so they use a single D-weighted body vector.

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
