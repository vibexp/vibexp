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
