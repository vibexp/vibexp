-- ===========================================================================
-- Stop `memories.updated_at` from moving when only a last-accessed column is
-- written (issue #730, epic #726).
--
-- Of the four resource tables, `memories` is the ONLY one carrying a
-- BEFORE UPDATE trigger (`update_memories_updated_at`, from 001_baseline).
-- Its function sets `NEW.updated_at = CURRENT_TIMESTAMP` unconditionally, so
-- ANY update to the row moves updated_at -- including the per-medium
-- last-accessed write #730 performs on every detail read.
--
-- That would make a READ indistinguishable from an EDIT, and updated_at is
-- load-bearing in at least three places:
--   * search recency ranking weights it (`rank_weight_updated`), so merely
--     reading a memory would push it up the results;
--   * the freshness reversibility rules (#733) treat an edit as a distinct
--     signal from an access;
--   * the UI reports it as "last updated" to humans.
-- prompts/artifacts/blueprints have no such trigger and are unaffected.
--
-- The fix is a WHEN clause rather than dropping the trigger. Dropping it looks
-- tempting -- `memory.go`'s UPDATE already passes `updated_at = $7` explicitly,
-- so the column appears app-managed -- but that value is currently OVERWRITTEN
-- by this trigger on every edit, meaning the trigger, not the application, is
-- the de-facto writer. Removing it would silently promote whatever the service
-- happens to pass (possibly a zero time) to the stored value. Narrowing when
-- the trigger fires changes nothing about existing edits.
--
-- The predicate reads "fire only when no last-accessed column changed":
-- every pre-existing write path touches content columns and never these four,
-- so it behaves exactly as before; the #730 path touches only these four and
-- is therefore skipped. Enumerating the last-accessed columns rather than the
-- content columns also keeps this future-proof -- a new content column added
-- to `memories` later still bumps updated_at with no further migration.
-- ===========================================================================

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
