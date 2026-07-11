-- Issue #188: pg_trgm typo-tolerance fallback for keyword-mode search.
-- Also reverts issue #183's title-weighted FTS indexes back to unweighted
-- (see the second section below).
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
-- (same convention as the combined title+body FTS indexes). Memories have no
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

-- Revert issue #183's title-weighted FTS indexes back to unweighted.
--
-- A keyword-mode gold-set benchmark (Epic #170 method) measured the #183 weighted
-- ranking (setweight title=A/body=D + ts_rank_cd) as a net regression
-- (Recall@5 0.68 -> 0.61, nDCG@10 0.75 -> 0.71): most real-world queries resolve
-- via the relaxed OR pass, where a generic query word matching a title outranks a
-- document that carries the relevant matches in its body. search.go therefore
-- reverts ftsExpr to the unweighted to_tsvector(title || ' ' || body) + plain
-- ts_rank. The previous weighted GIN indexes no longer match that expression, so
-- replace them with indexes on the unweighted expression (otherwise the FTS match
-- falls back to sequential scans).

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
