-- Revert the embedded OAuth 2.1 Authorization Server tables (issue #31).
DROP TABLE IF EXISTS public.oauth_login_sessions;
DROP TABLE IF EXISTS public.oauth_signing_keys;
DROP TABLE IF EXISTS public.oauth_pkce_sessions;
DROP TABLE IF EXISTS public.oauth_refresh_tokens;
DROP TABLE IF EXISTS public.oauth_access_tokens;
DROP TABLE IF EXISTS public.oauth_authorization_codes;
DROP TABLE IF EXISTS public.oauth_clients;
