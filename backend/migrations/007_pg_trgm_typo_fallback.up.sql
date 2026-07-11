-- Issue #188: pg_trgm typo-tolerance fallback for keyword-mode search.
--
-- Keyword mode (the search fallback used when a team has no embedding provider)
-- runs two exact-lexeme full-text passes: strict websearch_to_tsquery, then a
-- relaxed OR-rewrite (#174). Both are exact matches, so a single mistyped token
-- (e.g. "widgte") returns zero rows even when the intended resource exists.
--
-- Add a THIRD fallback pass in SearchKeyword (search.go) that matches the query
-- against each resource's TITLE/NAME only, using pg_trgm word_similarity (best
-- matching substring/word, not whole-document similarity() which dilutes to near
-- zero on long bodies). It runs ONLY when both full-text passes return nothing,
-- so precise/relaxed queries keep their current behaviour and ranking.
--
-- pg_trgm ships with PostgreSQL. Self-hosters on managed Postgres may need to
-- allow CREATE EXTENSION for the connecting role (as with the baseline `vector`
-- extension in migration 001); this migration fails fast with a clear error if
-- the role cannot create it.
--
-- One GIN gin_trgm_ops index per source table, on the SAME title expression the
-- query emits (search.go entitySource.titleExpr) so the `%>` word-similarity
-- operator is index-accelerated. The query qualifies columns as src.* but
-- Postgres resolves src.name -> name etc. to the same underlying columns, so the
-- qualified query expression still matches these unqualified index expressions
-- (same convention as the migration-006 weighted FTS indexes). Memories have no
-- real title (theirs is LEFT(text, 100)), so their index mirrors that expression.

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE INDEX IF NOT EXISTS idx_prompts_title_trgm ON prompts
    USING gin ((coalesce(name, '')) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_artifacts_title_trgm ON artifacts
    USING gin ((coalesce(title, '')) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_blueprints_title_trgm ON blueprints
    USING gin ((coalesce(title, '')) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_memories_title_trgm ON memories
    USING gin ((coalesce(LEFT(text, 100), '')) gin_trgm_ops);
