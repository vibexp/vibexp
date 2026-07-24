-- User suspension lifecycle (#454).
--
-- Adds a representable "account is off" state. The DEFAULT backfills every
-- existing row to 'active' in the same statement, so there is no separate
-- backfill step and no window where a row has no status.
--
-- Suspension is instance-local: it does NOT disable the account at the upstream
-- identity provider. It is enforced per request at every authentication entry
-- point, so suspending a user immediately invalidates their existing sessions,
-- API keys and OAuth/MCP tokens rather than waiting for them to expire.

ALTER TABLE users
    ADD COLUMN status character varying(20) NOT NULL DEFAULT 'active';

ALTER TABLE users
    ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended'));

-- Partial index: suspended accounts are the rare case, and the only query that
-- filters on this column is the admin listing looking for non-active users.
-- A full index would be almost entirely 'active' entries and earn nothing.
CREATE INDEX idx_users_status_not_active ON users (status) WHERE status <> 'active';

COMMENT ON COLUMN users.status IS
    'Account lifecycle: active (normal) or suspended (blocked at every auth entry point). Instance-local; does not affect the upstream IdP.';
