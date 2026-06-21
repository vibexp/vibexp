-- Revert the change-summary and actor-type columns added to the content-versioning core.

ALTER TABLE content_versions
    DROP COLUMN IF EXISTS change_summary,
    DROP COLUMN IF EXISTS actor_type;
