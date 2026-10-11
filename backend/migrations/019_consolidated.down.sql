-- Consolidated post-v0.14.0 migration -- down.
--
-- Reverses the blocks in the opposite order from the up migration (027 first,
-- 019 last), each block's own down verbatim.

-- ===========================================================================
-- 027_resource_locks
-- ===========================================================================

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

-- ===========================================================================
-- 026_memories_preserve_updated_at
-- ===========================================================================

-- Restore the trigger as 013 left it: skipped only when an update touches
-- nothing but the last_accessed_* columns (#730).

DROP TRIGGER IF EXISTS update_memories_updated_at ON memories;

CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW
    WHEN (
        OLD.last_accessed_web_at IS NOT DISTINCT FROM NEW.last_accessed_web_at
    AND OLD.last_accessed_cli_at IS NOT DISTINCT FROM NEW.last_accessed_cli_at
    AND OLD.last_accessed_mcp_at IS NOT DISTINCT FROM NEW.last_accessed_mcp_at
    AND OLD.last_accessed_api_at IS NOT DISTINCT FROM NEW.last_accessed_api_at
    )
    EXECUTE FUNCTION public.update_updated_at_column();

COMMENT ON TRIGGER update_memories_updated_at ON memories IS
    'Maintains updated_at on edits. Deliberately skipped when an update touches only the last_accessed_* columns (#730), so a read never looks like an edit.';

-- ===========================================================================
-- 025_team_statuses
-- ===========================================================================

-- Rollback of 025_team_statuses (issue #1264), in reverse order of the up.
--
-- THIS ROLLBACK IS LOSSY. Once teams have custom statuses, resources carry
-- values the pre-025 schema cannot hold: the re-added CHECKs admit only the
-- system set, and a slug may be longer than varchar(20). So every resource
-- whose status is not one of its type's system statuses is first reassigned to
-- that type's default ('draft' for prompts, 'active' for the other three --
-- models.DefaultStatusFor). Rows already on a system status are not written
-- at all. The custom status definitions themselves go with the table.

-- memories has an unconditional BEFORE UPDATE trigger that rewrites updated_at.
-- Reassigning a status here is not an edit, and updated_at is an edit signal
-- (search recency, resource freshness), so suspend the trigger for the one
-- statement, as 016_resource_labels does. The other three tables have no such
-- trigger.
ALTER TABLE memories DISABLE TRIGGER update_memories_updated_at;

UPDATE memories SET status = 'active'
 WHERE status NOT IN ('active', 'draft', 'archived');

ALTER TABLE memories ENABLE TRIGGER update_memories_updated_at;

UPDATE blueprints SET status = 'active'
 WHERE status NOT IN ('active', 'expired');

UPDATE artifacts SET status = 'active'
 WHERE status NOT IN ('active', 'draft', 'archived');

UPDATE prompts SET status = 'draft'
 WHERE status NOT IN ('draft', 'published');

ALTER TABLE memories ALTER COLUMN status TYPE varchar(20);
ALTER TABLE blueprints ALTER COLUMN status TYPE varchar(20);
ALTER TABLE artifacts ALTER COLUMN status TYPE varchar(20);
ALTER TABLE prompts ALTER COLUMN status TYPE varchar(20);

ALTER TABLE memories
    ADD CONSTRAINT memories_status_check CHECK (status IN ('active', 'draft', 'archived'));
ALTER TABLE prompts
    ADD CONSTRAINT prompts_status_check CHECK (status IN ('draft', 'published'));

DROP TABLE IF EXISTS team_statuses;

-- ===========================================================================
-- 024_instance_auth_setup
-- ===========================================================================

DROP TABLE IF EXISTS instance_auth_setup;

-- ===========================================================================
-- 023_instance_auth_settings
-- ===========================================================================

DROP TABLE IF EXISTS instance_admins;
DROP TABLE IF EXISTS instance_auth_settings_version;
DROP TABLE IF EXISTS instance_auth_allowlist;
DROP TABLE IF EXISTS instance_auth_providers;

-- ===========================================================================
-- 022_instance_search_ai_summary_settings
-- ===========================================================================

-- Drops the instance search ranking and AI summary defaults (#1197). With the
-- rows gone the instance falls back to its built-in defaults.
DROP TABLE IF EXISTS instance_ai_summary_settings;
DROP TABLE IF EXISTS instance_search_settings;

-- ===========================================================================
-- 021_instance_settings_audit
-- ===========================================================================

-- Drops the instance settings audit log (issue #1187). The index dies with the
-- table. Rolling this back discards the record of every instance settings
-- change made while it was in place -- there is no other store to recover it
-- from.
DROP TABLE IF EXISTS instance_settings_audit;

-- ===========================================================================
-- 020_instance_email_provider
-- ===========================================================================

DROP TABLE IF EXISTS public.instance_email_provider;

-- ===========================================================================
-- 019_resource_access_events_user_index
-- ===========================================================================

DROP INDEX IF EXISTS public.idx_rae_user_created;
