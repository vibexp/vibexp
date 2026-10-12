-- Reverts 022_resource_write_events: drops the write-event log and the two
-- content_versions columns. The recorded write channels are lost; resources and
-- their versions are untouched.
ALTER TABLE content_versions
    DROP CONSTRAINT IF EXISTS content_versions_source_check,
    DROP COLUMN IF EXISTS assistant_name,
    DROP COLUMN IF EXISTS source;

DROP TABLE IF EXISTS resource_write_events;
