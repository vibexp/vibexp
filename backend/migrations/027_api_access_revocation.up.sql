-- Revoking all of a user's API access at once (#1449, epic #1426).
--
-- An API key or a stored OAuth grant can be deleted, but an MCP/CLI access
-- token that was already issued is a signed JWT: it is verified by signature
-- alone and never looked up, so deleting its row does not stop it before its
-- own expiry. users.api_tokens_valid_after is the missing handle, the bearer
-- token counterpart of users.session_epoch (024): the authentication
-- chokepoint refuses a bearer token whose iat is before this timestamp.
--
-- NULL means "never revoked": every token is judged on its own validity, so
-- nobody's tools are disconnected by the upgrade. Cookie sessions and API keys
-- are not affected by the column (sessions have session_epoch, keys are rows).
ALTER TABLE users ADD COLUMN api_tokens_valid_after timestamptz;

COMMENT ON COLUMN users.api_tokens_valid_after IS
    'Bearer access tokens issued (iat) before this moment do not authenticate. NULL when API access was never revoked. Set by the API access revocation (#1449).';

-- Revocation deletes a user's stored OAuth rows by subject (the user id). The
-- tables were only indexed by signature, request id and expiry, so that would
-- be a sequential scan of every grant on the instance.
CREATE INDEX idx_oauth_access_tokens_subject ON oauth_access_tokens (subject);
CREATE INDEX idx_oauth_refresh_tokens_subject ON oauth_refresh_tokens (subject);
CREATE INDEX idx_oauth_authorization_codes_subject ON oauth_authorization_codes (subject);
CREATE INDEX idx_oauth_pkce_sessions_subject ON oauth_pkce_sessions (subject);
