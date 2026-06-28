-- OAuth AS retention (issue #38): the request-backing tables grew unbounded
-- because expired rows were never deleted and there was nothing to delete on —
-- token expiry lived only inside the session_data JSON. Add a dedicated,
-- indexed expires_at column the cleanup job can sweep on, mirroring the existing
-- oauth_login_sessions.expires_at + index. The column is nullable: it is
-- populated on insert from the fosite session, and the cleanup query only
-- deletes rows whose expires_at is in the past, so any pre-existing rows
-- (NULL expires_at) are left untouched rather than risk premature deletion.
ALTER TABLE public.oauth_authorization_codes ADD COLUMN expires_at timestamp with time zone;
ALTER TABLE public.oauth_access_tokens       ADD COLUMN expires_at timestamp with time zone;
ALTER TABLE public.oauth_refresh_tokens      ADD COLUMN expires_at timestamp with time zone;
ALTER TABLE public.oauth_pkce_sessions       ADD COLUMN expires_at timestamp with time zone;

CREATE INDEX idx_oauth_authorization_codes_expires_at ON public.oauth_authorization_codes (expires_at);
CREATE INDEX idx_oauth_access_tokens_expires_at        ON public.oauth_access_tokens (expires_at);
CREATE INDEX idx_oauth_refresh_tokens_expires_at       ON public.oauth_refresh_tokens (expires_at);
CREATE INDEX idx_oauth_pkce_sessions_expires_at        ON public.oauth_pkce_sessions (expires_at);
