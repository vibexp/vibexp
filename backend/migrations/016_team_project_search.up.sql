-- ===========================================================================
-- Keyword search indexes for teams and projects (issue #813, epic #811).
--
-- Teams had no search path at all, and project search was `ILIKE '%term%'` on
-- name/description/slug -- a leading wildcard, so `idx_projects_name` could not
-- serve it and every search sequentially scanned the table, ordered by
-- created_at rather than relevance, with no typo tolerance.
--
-- These four indexes back the ranking ladder in postgres/entity_search.go,
-- which mirrors the one SearchKeyword already runs over the four resource
-- domains: strict FTS -> relaxed (OR-rewritten) FTS -> pg_trgm word similarity.
--
-- pg_trgm is ALREADY installed by migration 005, so there is no CREATE
-- EXTENSION here and no new operator action for self-hosters on managed
-- Postgres.
--
-- INDEX EXPRESSIONS MUST STAY BYTE-IDENTICAL TO THE EXPRESSIONS THE QUERIES
-- EMIT, or the planner silently ignores them and every pass seq-scans. The Go
-- side renders these same strings (entity_search.go: ftsMatchExpr / trgmNameExpr)
-- and team_project_search_integration_test.go asserts the plans actually use
-- them. Column qualification is the one permitted difference: the queries say
-- `t.name` / `p.name` where these say `name`, and Postgres resolves both to the
-- same column (the same convention as the 005 trigram indexes).
--
-- The FTS expression deliberately covers name + description only. `git_url` is
-- matched EXACTLY (score 1.0) instead of through FTS: the english parser turns
-- a repository URL into url/host tokens
-- ('github.com/shaharia-lab/games-for-agents', 'github.com'), which no
-- word-shaped query matches, so indexing it for FTS would cost index size and
-- buy nothing.
-- ===========================================================================

CREATE INDEX IF NOT EXISTS idx_teams_fts ON teams
    USING gin (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, '')));

CREATE INDEX IF NOT EXISTS idx_projects_fts ON projects
    USING gin (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, '')));

-- Trigram indexes on the NAME only: a typo is tolerated against the short,
-- high-signal name, never the body (whole-document similarity dilutes to near
-- zero on long text). This is also the pass that carries slash/hyphen-heavy
-- project names -- the english parser tokenizes `shaharia-lab/games-for-agents`
-- as a file path (`/games-for-agents`), so neither FTS pass matches a query of
-- "games for agents" at all, while word_similarity scores it 1.0.
CREATE INDEX IF NOT EXISTS idx_teams_name_trgm ON teams
    USING gin ((coalesce(name, '')) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_projects_name_trgm ON projects
    USING gin ((coalesce(name, '')) gin_trgm_ops);
