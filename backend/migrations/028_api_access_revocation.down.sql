-- Reverts 028_api_access_revocation: drops the bearer-token cut-off and the
-- subject indexes. A revocation recorded only in the cut-off is forgotten, so
-- an access token issued before it authenticates again until it expires; API
-- keys and OAuth rows that were deleted stay deleted.
DROP INDEX IF EXISTS idx_oauth_pkce_sessions_subject;
DROP INDEX IF EXISTS idx_oauth_authorization_codes_subject;
DROP INDEX IF EXISTS idx_oauth_refresh_tokens_subject;
DROP INDEX IF EXISTS idx_oauth_access_tokens_subject;

ALTER TABLE users DROP COLUMN IF EXISTS api_tokens_valid_after;
