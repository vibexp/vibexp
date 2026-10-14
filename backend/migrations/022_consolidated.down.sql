-- Consolidated post-v0.15.0 migration -- down.
--
-- Reverses the blocks in the opposite order from the up migration (032 first,
-- 022 last), each block's own down verbatim.

-- ===========================================================================
-- 032_user_disconnected_identities
-- ===========================================================================

DROP TABLE IF EXISTS user_disconnected_identities;

-- ===========================================================================
-- 031_account_tokens
-- ===========================================================================

-- Reverts the account_tokens migration: drops every outstanding account link. Links that
-- were already sent stop working; passwords that were set through one are kept.
DROP TABLE IF EXISTS account_tokens;

-- ===========================================================================
-- 030_password_changed_at
-- ===========================================================================

-- Reverts 030_password_changed_at. Passwords themselves are untouched.
ALTER TABLE user_credentials DROP COLUMN IF EXISTS password_changed_at;

-- ===========================================================================
-- 029_invitee_email_lowercase
-- ===========================================================================

-- Reverts 029_invitee_email_lowercase: drops the lower-case CHECK. Addresses
-- stay lower case; their original casing was not kept.
ALTER TABLE team_invitations
    DROP CONSTRAINT IF EXISTS team_invitations_invitee_email_lowercase;

COMMENT ON COLUMN team_invitations.invitee_email IS NULL;

-- ===========================================================================
-- 028_user_credentials
-- ===========================================================================

-- Reverts 028_user_credentials: drops every stored password hash and the
-- sign-in throttle state. Accounts are kept; a user who signed in only with a
-- password has no way in until they are given another sign-in method.
DROP TABLE IF EXISTS user_credentials;

-- ===========================================================================
-- 027_api_access_revocation
-- ===========================================================================

-- Reverts 027_api_access_revocation: drops the bearer-token cut-off and the
-- subject indexes. A revocation recorded only in the cut-off is forgotten, so
-- an access token issued before it authenticates again until it expires; API
-- keys and OAuth rows that were deleted stay deleted.
DROP INDEX IF EXISTS idx_oauth_pkce_sessions_subject;
DROP INDEX IF EXISTS idx_oauth_authorization_codes_subject;
DROP INDEX IF EXISTS idx_oauth_refresh_tokens_subject;
DROP INDEX IF EXISTS idx_oauth_access_tokens_subject;

ALTER TABLE users DROP COLUMN IF EXISTS api_tokens_valid_after;

-- ===========================================================================
-- 026_resource_write_events_latest
-- ===========================================================================

-- Reverts 026_resource_write_events_latest: drops the two lookup indexes. No
-- data is lost; the reads they serve fall back to slower plans.
DROP INDEX IF EXISTS idx_content_versions_team_created_unsourced;
DROP INDEX IF EXISTS idx_resource_write_events_resource_latest;

-- ===========================================================================
-- 025_instance_auth_policy
-- ===========================================================================

-- Reverts 025_instance_auth_policy: drops the stored authentication policy.
-- The instance goes back to the behaviour it has with no row stored: password
-- login off and open registration. Audit entries of the policy are kept.
DROP TABLE IF EXISTS instance_auth_policy;

-- ===========================================================================
-- 024_user_session_epoch
-- ===========================================================================

-- Reverts 024_user_session_epoch: drops the per-user session revocation
-- counter. Cookies minted under a non-zero epoch keep decoding (the field is
-- ignored again), so sessions that had been revoked become valid until their
-- own expiry.
ALTER TABLE users DROP COLUMN IF EXISTS session_epoch;

-- ===========================================================================
-- 023_user_identities
-- ===========================================================================

-- Reverts 023_user_identities: drops the identities table, the verified-at
-- column and the lower-case CHECK. users.idp_provider, idp_subject and
-- google_id were never removed, so the inline identity each account signed up
-- with is intact; a second identity linked since is lost. Email addresses stay
-- lower case: their original casing was not kept.
DROP TABLE IF EXISTS user_identities;

ALTER TABLE users
    DROP COLUMN IF EXISTS email_verified_at,
    DROP CONSTRAINT IF EXISTS users_email_lowercase;

COMMENT ON COLUMN users.email IS NULL;

-- ===========================================================================
-- 022_resource_write_events
-- ===========================================================================

-- Reverts 022_resource_write_events: drops the write-event log and the two
-- content_versions columns. The recorded write channels are lost; resources and
-- their versions are untouched.
ALTER TABLE content_versions
    DROP CONSTRAINT IF EXISTS content_versions_source_check,
    DROP COLUMN IF EXISTS assistant_name,
    DROP COLUMN IF EXISTS source;

DROP TABLE IF EXISTS resource_write_events;
