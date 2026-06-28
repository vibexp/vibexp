-- Revert the OAuth AS retention expires_at columns (issue #38).
DROP INDEX IF EXISTS idx_oauth_pkce_sessions_expires_at;
DROP INDEX IF EXISTS idx_oauth_refresh_tokens_expires_at;
DROP INDEX IF EXISTS idx_oauth_access_tokens_expires_at;
DROP INDEX IF EXISTS idx_oauth_authorization_codes_expires_at;

ALTER TABLE public.oauth_pkce_sessions       DROP COLUMN IF EXISTS expires_at;
ALTER TABLE public.oauth_refresh_tokens      DROP COLUMN IF EXISTS expires_at;
ALTER TABLE public.oauth_access_tokens       DROP COLUMN IF EXISTS expires_at;
ALTER TABLE public.oauth_authorization_codes DROP COLUMN IF EXISTS expires_at;
