-- Full-text search fallback (issue #18): when no embedding provider is configured,
-- search reads the source tables directly with PostgreSQL FTS instead of the empty
-- embeddings table. These GIN indexes cover the exact combined title+body tsvector
-- expression SearchKeyword matches and ranks on, so the @@ filter avoids a
-- sequential scan. The expression must stay byte-for-byte in sync with ftsExpr /
-- buildKeywordBranch in internal/repositories/postgres/search.go, otherwise the
-- planner cannot use the index (search stays correct but slower).

CREATE INDEX IF NOT EXISTS idx_prompts_fts ON prompts
    USING gin (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(body, '')));

CREATE INDEX IF NOT EXISTS idx_artifacts_fts ON artifacts
    USING gin (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '')));

CREATE INDEX IF NOT EXISTS idx_blueprints_fts ON blueprints
    USING gin (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '')));

CREATE INDEX IF NOT EXISTS idx_memories_fts ON memories
    USING gin (to_tsvector('english', coalesce(LEFT(text, 100), '') || ' ' || coalesce(text, '')));
