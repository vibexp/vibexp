-- Consolidated post-v0.5.0 migration: squashes the three unreleased migrations
-- that landed on main after the v0.5.0 release into one, so the next release
-- ships a single migration. The net schema delta over v0.5.0 is:
--
--   1. Per-provider embedding instruction prefixes (issue #171).
--   2. pg_trgm typo-tolerance fallback for keyword-mode search (issue #188/#174).
--
-- The intermediate title-weighted FTS indexes (issue #183) were added and then
-- reverted before release, netting to a no-op against migration 002's unweighted
-- combined title+body GIN indexes, so they are omitted here entirely.

-- 1. Per-provider configurable query/document instruction prefixes (issue #171).
-- Asymmetric embedding models (mxbai/BGE, E5) are trained with instruction
-- prefixes and lose ranking quality without them. These are provider config
-- (same tier as model/chunk_size), applied only to the text sent to the provider
-- at embedding time. Both are nullable and default to empty, so existing rows
-- keep exact current behaviour (no prefix).
ALTER TABLE public.embedding_providers
    ADD COLUMN query_prefix text,
    ADD COLUMN document_prefix text;

-- 2. pg_trgm typo-tolerance fallback for keyword-mode search (issue #188/#174).
-- Keyword mode (the search fallback used when a team has no embedding provider)
-- runs two exact-lexeme full-text passes: strict websearch_to_tsquery, then a
-- relaxed OR-rewrite. Both are exact matches, so a single mistyped token
-- (e.g. "widgte") returns zero rows even when the intended resource exists.
--
-- SearchKeyword (search.go) adds a THIRD fallback pass that matches the query
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
