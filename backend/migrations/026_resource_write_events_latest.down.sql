-- Reverts 026_resource_write_events_latest: drops the two lookup indexes. No
-- data is lost; the reads they serve fall back to slower plans.
DROP INDEX IF EXISTS idx_content_versions_team_created_unsourced;
DROP INDEX IF EXISTS idx_resource_write_events_resource_latest;
