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
