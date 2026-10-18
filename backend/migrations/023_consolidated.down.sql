-- Consolidated post-v0.16.0 migration -- down.
--
-- Reverses the blocks in the opposite order from the up migration (025 first,
-- 023 last), each block's own down verbatim.

-- ===========================================================================
-- 025_instance_audit_settings
-- ===========================================================================

-- Reverts 025_instance_audit_settings: drops the instance audit-log settings
-- table. Any stored settings are lost; the built-in defaults apply again.
DROP TABLE IF EXISTS instance_audit_settings;

-- ===========================================================================
-- 024_audit_events
-- ===========================================================================

DROP TABLE IF EXISTS audit_events;

-- ===========================================================================
-- 023_resource_write_events_user_index
-- ===========================================================================

DROP INDEX IF EXISTS idx_rwe_user_created;
