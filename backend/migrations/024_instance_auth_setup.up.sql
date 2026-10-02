-- First-run authentication setup state (#1236, epic #1230).
--
-- An instance with no enabled identity provider has no way to sign in, so it
-- cannot reach the admin panel that configures sign-in. While that holds the
-- server mints a one-time setup token at boot and logs a setup URL; this table
-- is the token's only storage. It holds the token's HASH, never the token, so
-- a database read cannot be replayed as a setup URL, and it lives in the
-- database rather than in process memory so every replica validates the same
-- token.
--
-- Storage only: whether setup mode is active is computed by
-- services.SetupModeService, not stored here.

CREATE TABLE instance_auth_setup (
    -- Singleton key: always true, so the primary key admits exactly one row.
    id           boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- SHA-256 of the setup token. NULL once the token is consumed.
    token_hash   bytea CHECK (token_hash IS NULL OR octet_length(token_hash) = 32),
    expires_at   timestamptz,
    -- Set when a root instance admin's first provider sign-in ends setup.
    consumed_at  timestamptz,
    -- Who consumed it; SET NULL when that user is deleted.
    consumed_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    -- True while setup was deliberately re-armed (break-glass CLI, #1237) and
    -- not yet consumed: setup mode stays active even with a provider enabled.
    rearmed      boolean NOT NULL DEFAULT false,
    -- Bumped by every mint and by consumption. A setup session carries the
    -- generation it was issued at, so a bump invalidates every outstanding one.
    generation   bigint NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- A live token always has an expiry.
    CONSTRAINT instance_auth_setup_token_has_expiry CHECK (
        token_hash IS NULL OR expires_at IS NOT NULL)
);

COMMENT ON TABLE instance_auth_setup IS
    'First-run authentication setup state. At most one row (singleton key id = true); no row = no setup token was ever issued.';
COMMENT ON COLUMN instance_auth_setup.token_hash IS
    'SHA-256 of the one-time setup token. The token itself is never stored. NULL once consumed.';
COMMENT ON COLUMN instance_auth_setup.generation IS
    'Bumped by every mint and by consumption; a setup session issued at an older generation is invalid.';
