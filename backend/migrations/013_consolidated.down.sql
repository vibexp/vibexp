-- Reverses 013_consolidated (post-v0.10.0 consolidation).
--
-- Each source migration's down-block, in REVERSE of the up order:
--   * 016_team_project_search                       (issue #813, epic #811)
--   * 015_seed_freshness_evaluate_schedules         (issue #732, epic #726)
--   * 014_memories_updated_at_ignores_last_accessed (issue #730, epic #726)
--   * 013_resource_freshness                        (issue #729, epic #726)
--
-- 015's down-block is kept even though its up-block was dropped from the
-- consolidation (see the up file for why). It is not dead: the freshness
-- service writes `freshness_evaluate` rows into `schedules` at runtime, and
-- `schedules` belongs to 012 and survives this rollback -- so without the
-- DELETE, rolling back past 013 would strand rows for a job whose handler no
-- longer exists, which the scheduler can only log as unregistered on every
-- tick. It must run BEFORE the 013 block for no functional reason (the two
-- touch different tables), but keeping strict reverse order keeps this file
-- readable against the up file.
--
-- 014's down-block restores the unconditional trigger from 001_baseline, and
-- must therefore run BEFORE 013's block drops the `last_accessed_*` columns
-- its WHEN clause names -- Postgres would otherwise fail to drop the columns
-- a trigger predicate depends on.

-- ===========================================================================
-- 016_team_project_search (#813)
-- ===========================================================================

-- Reverses 016 (issue #813): drops the FTS and trigram indexes backing team and
-- project keyword search.
--
-- Index-only migration, so rolling back loses no data. Searches still return
-- correct results afterwards -- the ladder's SQL is unchanged and every pass
-- simply falls back to a sequential scan, which is the pre-#813 behaviour.
--
-- pg_trgm itself is NOT dropped: migration 005 installed it and the resource
-- search ladder still depends on it.

DROP INDEX IF EXISTS idx_projects_name_trgm;
DROP INDEX IF EXISTS idx_teams_name_trgm;
DROP INDEX IF EXISTS idx_projects_fts;
DROP INDEX IF EXISTS idx_teams_fts;

-- ===========================================================================
-- 015_seed_freshness_evaluate_schedules (#732) -- down-block only
-- ===========================================================================

-- Remove the freshness evaluation schedules (issue #732).
--
-- This deletes every `freshness_evaluate` row, not only the ones the up
-- migration inserted: they are indistinguishable afterwards (the service
-- writes identical rows), and rolling back #732 removes the handler, so any
-- such row would be a schedule the scheduler can only log as unregistered on
-- every tick.
DELETE FROM schedules WHERE job_type = 'freshness_evaluate';

-- ===========================================================================
-- 014_memories_updated_at_ignores_last_accessed (#730)
-- ===========================================================================

-- Reverses 014 (issue #730): restores the unconditional
-- update_memories_updated_at trigger exactly as 001_baseline created it.
--
-- Rolling back reinstates the behaviour where ANY update to a memories row --
-- including a last-accessed-only write -- moves updated_at. That is the
-- pre-#730 behaviour and is only correct on a schema where nothing writes the
-- last_accessed_* columns.

DROP TRIGGER IF EXISTS update_memories_updated_at ON memories;

CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW
    EXECUTE FUNCTION public.update_updated_at_column();

-- ===========================================================================
-- 013_resource_freshness (#729)
-- ===========================================================================

-- Reverses 013_resource_freshness (issue #729).
--
-- Lossy by nature: dropping the tables discards recorded freshness state and
-- the whole mark/clear audit trail, and dropping the columns discards the
-- accumulated last-accessed timestamps. There is nothing to preserve them
-- into -- the up migration deliberately seeds from nothing, so re-applying it
-- returns the schema to a clean start rather than to the prior data.
--
-- DROP COLUMN is catalog-only in Postgres, so it succeeds regardless of how
-- much data the columns hold. Indexes and constraints die with their tables.

ALTER TABLE memories
    DROP COLUMN IF EXISTS last_accessed_web_at,
    DROP COLUMN IF EXISTS last_accessed_cli_at,
    DROP COLUMN IF EXISTS last_accessed_mcp_at,
    DROP COLUMN IF EXISTS last_accessed_api_at;

ALTER TABLE blueprints
    DROP COLUMN IF EXISTS last_accessed_web_at,
    DROP COLUMN IF EXISTS last_accessed_cli_at,
    DROP COLUMN IF EXISTS last_accessed_mcp_at,
    DROP COLUMN IF EXISTS last_accessed_api_at;

ALTER TABLE artifacts
    DROP COLUMN IF EXISTS last_accessed_web_at,
    DROP COLUMN IF EXISTS last_accessed_cli_at,
    DROP COLUMN IF EXISTS last_accessed_mcp_at,
    DROP COLUMN IF EXISTS last_accessed_api_at;

ALTER TABLE prompts
    DROP COLUMN IF EXISTS last_accessed_web_at,
    DROP COLUMN IF EXISTS last_accessed_cli_at,
    DROP COLUMN IF EXISTS last_accessed_mcp_at,
    DROP COLUMN IF EXISTS last_accessed_api_at;

DROP TABLE IF EXISTS resource_freshness_audit;
DROP TABLE IF EXISTS team_freshness_settings;
DROP TABLE IF EXISTS freshness_rules;
DROP TABLE IF EXISTS resource_freshness;
