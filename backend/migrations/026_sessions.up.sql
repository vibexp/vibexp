-- Server-side cookie sessions (#1646, epic #1640).
--
-- Every vx_session cookie now names a row here, and the identity-provider
-- access and refresh tokens live in the row (encrypted with
-- security.encryption_key) instead of in the cookie. The cookie carries an
-- opaque random session id; the table stores only its SHA-256, so reading the
-- table yields no usable session id. Signing out deletes the row, which ends
-- that one session for every copy of its cookie.
--
-- expires_at is the session's lifetime, the same one the cookie's Max-Age
-- gives it: a sliding session's expiry, or a day from the last write for a
-- session kept alive by a refresh token. Rows past it no longer authenticate
-- and are deleted by an hourly sweep (internal/server/session_sweep.go).

CREATE TABLE sessions (
    id_hash           bytea       PRIMARY KEY,
    user_id           uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider          text,
    auth_method       text        NOT NULL,
    authenticated_at  timestamptz,
    access_token_enc  text,
    refresh_token_enc text,
    access_expires_at timestamptz NOT NULL,
    last_refreshed_at timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    expires_at        timestamptz NOT NULL,
    CONSTRAINT sessions_id_hash_sha256 CHECK (octet_length(id_hash) = 32)
);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

COMMENT ON TABLE sessions IS
    'Server-side state of vx_session cookie sessions: the encrypted identity-provider tokens. Keyed by the SHA-256 of the id the cookie carries.';
COMMENT ON COLUMN sessions.access_token_enc IS
    'Identity-provider access token, encrypted with security.encryption_key. NULL when the session has none.';
COMMENT ON COLUMN sessions.refresh_token_enc IS
    'Identity-provider refresh token, encrypted with security.encryption_key. NULL for a sliding session.';
COMMENT ON COLUMN sessions.access_expires_at IS
    'Expiry of the access token, or of the session itself for a sliding session.';
COMMENT ON COLUMN sessions.expires_at IS
    'When the session stops authenticating; the sweep deletes rows past it.';
