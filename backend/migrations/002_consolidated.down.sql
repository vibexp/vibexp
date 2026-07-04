-- Revert the consolidated post-v0.2.0 migration, in reverse order of the up.

-- Embedded OAuth 2.1 Authorization Server (issue #31); the expires_at columns
-- (issue #38) are dropped with their tables.
DROP TABLE IF EXISTS public.oauth_login_sessions;
DROP TABLE IF EXISTS public.oauth_signing_keys;
DROP TABLE IF EXISTS public.oauth_pkce_sessions;
DROP TABLE IF EXISTS public.oauth_refresh_tokens;
DROP TABLE IF EXISTS public.oauth_access_tokens;
DROP TABLE IF EXISTS public.oauth_authorization_codes;
DROP TABLE IF EXISTS public.oauth_clients;

-- Full-text search fallback indexes (issue #18).
DROP INDEX IF EXISTS idx_memories_fts;
DROP INDEX IF EXISTS idx_blueprints_fts;
DROP INDEX IF EXISTS idx_artifacts_fts;
DROP INDEX IF EXISTS idx_prompts_fts;

-- Memory lifecycle status column (issue #17): drop the CHECK constraint first,
-- then the column, returning memories to its 001_baseline shape.
ALTER TABLE public.memories
    DROP CONSTRAINT IF EXISTS memories_status_check;

ALTER TABLE public.memories
    DROP COLUMN IF EXISTS status;
