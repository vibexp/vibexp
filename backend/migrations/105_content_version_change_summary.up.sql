-- Extend the generic content-versioning core for the redesigned Version History:
-- a per-version human-readable change summary and an actor-type distinction
-- (human vs system). Both are generic — they apply to every resource_type, so
-- Prompts, Blueprints, Memory, etc. inherit them when they adopt versioning.

ALTER TABLE content_versions
    ADD COLUMN change_summary TEXT,
    ADD COLUMN actor_type     TEXT NOT NULL DEFAULT 'human'
        CHECK (actor_type IN ('human', 'system'));

COMMENT ON COLUMN content_versions.change_summary IS 'Optional human-readable summary of the change captured at this version (e.g. "Tightened the wording"); NULL when none was supplied';
COMMENT ON COLUMN content_versions.actor_type IS 'Who authored this version: human (a user edit) or system (e.g. a restore or future auto-save)';
