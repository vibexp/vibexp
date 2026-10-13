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
