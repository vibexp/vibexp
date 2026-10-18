-- Consolidated post-v0.16.0 migration.
--
-- Squashes the migrations that accumulated after the v0.16.0 release into a
-- single step, applied on top of 022, mirroring the 002/006/011/013/016/019/022
-- consolidations (#76, #399, #710, #839, #976, #1368, #1499). Merged here (in
-- original order):
--   * 023_resource_write_events_user_index (issue #1508)
--   * 024_audit_events                     (issue #1511, epic #1510)
--   * 025_instance_audit_settings          (issue #1512, epic #1510)
--
-- Each block below is the original migration verbatim. Nothing needed to be
-- reconciled across the blocks, and none uses CONCURRENTLY, so all of them run
-- in the one transaction golang-migrate gives a file. Where a block's own
-- comments cite one of the numbers above (023, 024, 025), read it as the block
-- of that name in this file. No deployed instance has applied any of them
-- (none shipped in a release), so renumbering is safe.

-- ===========================================================================
-- 023_resource_write_events_user_index
-- ===========================================================================

-- Index a user's write events by time (#1508).
--
-- GET /auth/me reports when the user's AI tools first wrote a resource back
-- (knowledge_loop.first_write_at): MIN(created_at) over this user's
-- resource_write_events from the cli/mcp/api sources. The table's existing
-- indexes lead with team_id or the resource, so without this one that lookup
-- scans every write event on each /auth/me call. The read side already has
-- the matching idx_rae_user_created on resource_access_events.
--
-- Plain CREATE INDEX (not CONCURRENTLY): migrations run before the server
-- serves.
CREATE INDEX IF NOT EXISTS idx_rwe_user_created
    ON resource_write_events USING btree (user_id, created_at);

-- ===========================================================================
-- 024_audit_events
-- ===========================================================================

-- The instance audit log (#1511, epic #1510).
--
-- One append-only row per audited operation: resource writes, sign-in and other
-- security events, team and instance administration, and authorization
-- denials. Reads are not recorded. The table is written asynchronously by the
-- audit recorder (internal/services/audit) and read only by instance admins.
--
-- No foreign keys, on purpose: a row records what happened and must outlive the
-- user, team, project, API key or resource it names. That is also why the
-- display names (actor, team, project, resource, API key) are snapshotted into
-- the row when it is written, so the log stays readable after a deletion.
-- Every id column is therefore plain text, and nothing cascades into it.
--
-- Rows leave only by the retention purge (batched DELETE on created_at); there
-- is no update path. actor_user_id is NULL for a system job and for a failed
-- sign-in with an unknown email, which stores the attempted address in
-- actor_email instead.
--
-- ip is text rather than inet so the admin free-text search can match it with
-- the same escaped ILIKE as the name columns. ip and user_agent are NULL when
-- the instance turned client-info capture off.
CREATE TABLE audit_events (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    actor_user_id  text,
    actor_email    text        NOT NULL DEFAULT '',
    actor_name     text        NOT NULL DEFAULT '',
    actor_type     text        NOT NULL,
    team_id        text,
    team_name      text,
    project_id     text,
    project_name   text,
    operation      text        NOT NULL,
    resource_type  text,
    resource_id    text,
    resource_name  text,
    source         text        NOT NULL,
    assistant_name text,
    api_key_id     text,
    api_key_name   text,
    outcome        text        NOT NULL,
    ip             text,
    user_agent     text,
    request_id     text,
    metadata       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT audit_events_actor_type_check
        CHECK (actor_type IN ('user', 'system', 'anonymous')),
    CONSTRAINT audit_events_source_check
        CHECK (source IN ('web', 'cli', 'mcp', 'api', 'system')),
    CONSTRAINT audit_events_outcome_check
        CHECK (outcome IN ('success', 'denied', 'failure'))
);

-- The admin list reads newest first with a keyset cursor on (created_at, id).
CREATE INDEX idx_audit_events_created ON audit_events (created_at DESC, id DESC);
-- The per-user, per-team and per-operation filters, each bounded by time.
CREATE INDEX idx_audit_events_actor_created ON audit_events (actor_user_id, created_at DESC);
CREATE INDEX idx_audit_events_team_created ON audit_events (team_id, created_at DESC);
CREATE INDEX idx_audit_events_operation_created ON audit_events (operation, created_at DESC);

-- ===========================================================================
-- 025_instance_audit_settings
-- ===========================================================================

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
