-- ===========================================================================
-- Resource locks -- storage foundation (issue #1286, epic #1285).
--
-- A prompt, blueprint or artifact can be locked by a team member so nobody
-- edits it by accident. This migration adds the two things that need storing:
-- the lock itself, on the resource row, and an append-only history of lock and
-- unlock events. Nothing reads or writes either yet; the API (#1288) and the
-- write guards (#1289, #1290) build on it.
--
-- The lock lives ON THE ROW rather than in a side table so that a list query
-- needs no join to know whether a row is locked, and so that the lock goes
-- away with the resource without a cascade to maintain.
--
-- "Locked" is ONE predicate everywhere: locked_by IS NOT NULL. locked_by is a
-- foreign key to users with ON DELETE SET NULL, so deleting the locker's
-- account unlocks every resource they held, by construction and with no
-- application code. After that SET NULL the row still carries the old
-- locked_at and lock_comment; they are stale and every reader must ignore
-- them, which is why the CHECK below is one-directional and why no reader may
-- test locked_at or lock_comment to decide whether a row is locked.
--
-- Three nullable columns with no default are a catalog-only change: no table
-- rewrite, and no row is updated, so updated_at is untouched.
-- ===========================================================================

ALTER TABLE prompts
    ADD COLUMN locked_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN locked_at    timestamptz,
    ADD COLUMN lock_comment text,
    ADD CONSTRAINT prompts_lock_state_check
        CHECK (locked_by IS NULL OR (locked_at IS NOT NULL AND lock_comment IS NOT NULL));

ALTER TABLE blueprints
    ADD COLUMN locked_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN locked_at    timestamptz,
    ADD COLUMN lock_comment text,
    ADD CONSTRAINT blueprints_lock_state_check
        CHECK (locked_by IS NULL OR (locked_at IS NOT NULL AND lock_comment IS NOT NULL));

ALTER TABLE artifacts
    ADD COLUMN locked_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN locked_at    timestamptz,
    ADD COLUMN lock_comment text,
    ADD CONSTRAINT artifacts_lock_state_check
        CHECK (locked_by IS NULL OR (locked_at IS NOT NULL AND lock_comment IS NOT NULL));

COMMENT ON COLUMN prompts.locked_by IS
    'The member holding the lock; NULL means the prompt is unlocked. This column alone decides lock state. SET NULL on user delete, so deleting the locker unlocks the prompt.';
COMMENT ON COLUMN prompts.locked_at IS
    'When the lock was taken. Meaningful only while locked_by is set: it is left behind, stale, when the locker''s account is deleted.';
COMMENT ON COLUMN prompts.lock_comment IS
    'Why the prompt was locked, as given by the locker. Meaningful only while locked_by is set, like locked_at.';

COMMENT ON COLUMN blueprints.locked_by IS
    'The member holding the lock; NULL means the blueprint is unlocked. This column alone decides lock state. SET NULL on user delete, so deleting the locker unlocks the blueprint.';
COMMENT ON COLUMN blueprints.locked_at IS
    'When the lock was taken. Meaningful only while locked_by is set: it is left behind, stale, when the locker''s account is deleted.';
COMMENT ON COLUMN blueprints.lock_comment IS
    'Why the blueprint was locked, as given by the locker. Meaningful only while locked_by is set, like locked_at.';

COMMENT ON COLUMN artifacts.locked_by IS
    'The member holding the lock; NULL means the artifact is unlocked. This column alone decides lock state. SET NULL on user delete, so deleting the locker unlocks the artifact.';
COMMENT ON COLUMN artifacts.locked_at IS
    'When the lock was taken. Meaningful only while locked_by is set: it is left behind, stale, when the locker''s account is deleted.';
COMMENT ON COLUMN artifacts.lock_comment IS
    'Why the artifact was locked, as given by the locker. Meaningful only while locked_by is set, like locked_at.';

-- The `locked` list filter asks for a team's locked rows. Locked rows are
-- expected to be a small minority, so a partial index holds only those and
-- costs nothing on the unlocked majority. (The opposite filter, unlocked
-- rows, is nearly the whole table and is served by the existing team indexes.)
CREATE INDEX idx_prompts_locked ON prompts (team_id) WHERE locked_by IS NOT NULL;
CREATE INDEX idx_blueprints_locked ON blueprints (team_id) WHERE locked_by IS NOT NULL;
CREATE INDEX idx_artifacts_locked ON artifacts (team_id) WHERE locked_by IS NOT NULL;

CREATE TABLE resource_lock_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id   uuid NOT NULL,
    action        text NOT NULL,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    comment       text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE resource_lock_events IS
    'Append-only history of resource lock and unlock events (epic #1285). Rows are never updated or deleted except by team cascade.';
COMMENT ON COLUMN resource_lock_events.team_id IS
    'The team that owns the resource. Cascades, because an entry about a team that no longer exists is unreadable.';
COMMENT ON COLUMN resource_lock_events.resource_type IS
    'Singular resource type: prompt, blueprint or artifact. Validated in the repository, not by a CHECK (same convention as team_settings_audit.surface).';
COMMENT ON COLUMN resource_lock_events.resource_id IS
    'The locked or unlocked resource. No foreign key: it is polymorphic across the three resource tables, and the history deliberately outlives the resource (as team_settings_audit outlives its subject).';
COMMENT ON COLUMN resource_lock_events.action IS
    'What happened: lock or unlock. Validated in the repository, not by a CHECK.';
COMMENT ON COLUMN resource_lock_events.actor_user_id IS
    'Who locked or unlocked. SET NULL on user deletion rather than CASCADE: the entry must outlive the actor.';
COMMENT ON COLUMN resource_lock_events.comment IS
    'The lock comment for a lock event; NULL for an unlock.';

-- The history read is "this resource's events, newest first". `id DESC` is a
-- deterministic tiebreaker: now() is transaction-start time, so two events
-- written in one transaction carry the same created_at.
CREATE INDEX idx_resource_lock_events_resource
    ON resource_lock_events (team_id, resource_type, resource_id, created_at DESC, id DESC);
