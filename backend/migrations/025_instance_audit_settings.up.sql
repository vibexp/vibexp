-- Instance audit-log settings (#1512, epic #1510): how long audit events are
-- kept and whether a new event stores the client's IP address and user agent.
--
-- IP addresses and emails are personal data, so an instance admin must be able
-- to shorten the retention and switch client-info capture off. The daily
-- retention purge reads retention_weeks; the audit recorder reads
-- capture_ip_ua.
--
-- It has the same singleton shape as instance_site_info (020) and
-- instance_search_settings (019_consolidated). No row means the built-in
-- defaults apply (models.DefaultAuditRetentionWeeks,
-- models.DefaultAuditCaptureClientInfo), so DELETE is the reset.
--
-- The retention CHECK mirrors models.MinAuditRetentionWeeks and
-- models.MaxAuditRetentionWeeks; a change on either side must change both. It
-- is the storage backstop: the service validates first.

CREATE TABLE instance_audit_settings (
    -- Singleton key. It can only ever hold `true` (CHECK) and is the primary
    -- key, so a second row is a unique violation and no code path can create
    -- one.
    id              boolean PRIMARY KEY DEFAULT true CHECK (id),
    retention_weeks integer NOT NULL,
    capture_ip_ua   boolean NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Last editor. SET NULL on user deletion: removing an admin must not reset
    -- the instance's audit settings.
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    version     bigint NOT NULL DEFAULT 1,
    -- Mirrors models.MinAuditRetentionWeeks and models.MaxAuditRetentionWeeks.
    CONSTRAINT instance_audit_settings_retention_weeks CHECK (retention_weeks BETWEEN 1 AND 52)
);

COMMENT ON TABLE instance_audit_settings IS
    'Instance audit-log settings. At most one row (singleton key id = true); no row = the built-in defaults, so DELETE is the reset.';
COMMENT ON COLUMN instance_audit_settings.id IS
    'Singleton key: always true, so the primary key admits exactly one row.';
COMMENT ON COLUMN instance_audit_settings.retention_weeks IS
    'How many weeks audit events are kept before the daily purge deletes them, 1 to 52. The bounds mirror models.MinAuditRetentionWeeks and models.MaxAuditRetentionWeeks; change both together.';
COMMENT ON COLUMN instance_audit_settings.capture_ip_ua IS
    'Whether a newly recorded audit event stores the client IP address and user agent. Events already stored keep theirs.';
COMMENT ON COLUMN instance_audit_settings.updated_by IS
    'Last editor. SET NULL when that user is deleted.';
