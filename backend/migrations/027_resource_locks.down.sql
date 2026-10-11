-- Rollback of 027_resource_locks (issue #1286), in reverse order of the up.
--
-- THIS ROLLBACK IS LOSSY: every lock and the whole lock history are dropped.
-- Dropping a column also drops the constraint, the foreign key and the partial
-- index that depend on it, but each is named here so the rollback reads as the
-- mirror of the up. No row is updated, so updated_at is untouched.

DROP TABLE IF EXISTS resource_lock_events;

DROP INDEX IF EXISTS idx_artifacts_locked;
DROP INDEX IF EXISTS idx_blueprints_locked;
DROP INDEX IF EXISTS idx_prompts_locked;

ALTER TABLE artifacts
    DROP CONSTRAINT IF EXISTS artifacts_lock_state_check,
    DROP COLUMN IF EXISTS lock_comment,
    DROP COLUMN IF EXISTS locked_at,
    DROP COLUMN IF EXISTS locked_by;

ALTER TABLE blueprints
    DROP CONSTRAINT IF EXISTS blueprints_lock_state_check,
    DROP COLUMN IF EXISTS lock_comment,
    DROP COLUMN IF EXISTS locked_at,
    DROP COLUMN IF EXISTS locked_by;

ALTER TABLE prompts
    DROP CONSTRAINT IF EXISTS prompts_lock_state_check,
    DROP COLUMN IF EXISTS lock_comment,
    DROP COLUMN IF EXISTS locked_at,
    DROP COLUMN IF EXISTS locked_by;
