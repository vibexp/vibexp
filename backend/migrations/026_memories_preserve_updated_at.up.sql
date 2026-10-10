-- Let one transaction change memories without moving `updated_at` (issue
-- #1265, epic #1263).
--
-- Deleting a custom status moves every resource that carried it to the
-- resource type's default status. That is a change to the team's settings, not
-- an edit of the resource: `updated_at` feeds search recency ranking, the
-- freshness rules and the "last updated" a person reads, so it must stay put.
--
-- prompts, artifacts and blueprints have no trigger, so an UPDATE that does not
-- name `updated_at` leaves it alone. `memories` is the one resource table with
-- a BEFORE UPDATE trigger (`update_memories_updated_at`), and its function
-- stamps CURRENT_TIMESTAMP on every row the trigger fires for. The 025 down
-- migration works around that by disabling the trigger with DDL, which a
-- request cannot do: it needs table ownership and locks out every other writer.
--
-- So the trigger gains one more condition in its WHEN clause: it is skipped
-- while the transaction-local setting `vibexp.preserve_updated_at` is 'on'.
-- The status delete sets it with set_config(..., is_local => true) inside its
-- own transaction, so it ends with that transaction and no other session ever
-- sees it. current_setting(..., true) returns NULL when the setting was never
-- defined, which is DISTINCT FROM 'on', so every existing write path behaves
-- exactly as before.
--
-- The four last-accessed conditions are unchanged from 013 (#730).

DROP TRIGGER IF EXISTS update_memories_updated_at ON memories;

CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW
    WHEN (
        OLD.last_accessed_web_at IS NOT DISTINCT FROM NEW.last_accessed_web_at
    AND OLD.last_accessed_cli_at IS NOT DISTINCT FROM NEW.last_accessed_cli_at
    AND OLD.last_accessed_mcp_at IS NOT DISTINCT FROM NEW.last_accessed_mcp_at
    AND OLD.last_accessed_api_at IS NOT DISTINCT FROM NEW.last_accessed_api_at
    AND current_setting('vibexp.preserve_updated_at', true) IS DISTINCT FROM 'on'
    )
    EXECUTE FUNCTION public.update_updated_at_column();

COMMENT ON TRIGGER update_memories_updated_at ON memories IS
    'Maintains updated_at on edits. Deliberately skipped when an update touches only the last_accessed_* columns (#730), so a read never looks like an edit, and while the transaction-local setting vibexp.preserve_updated_at is ''on'' (#1265), so deleting a custom status does not look like one either.';
