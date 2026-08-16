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
