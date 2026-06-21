DROP INDEX IF EXISTS idx_users_idp_provider_subject;

ALTER TABLE users DROP COLUMN IF EXISTS idp_subject;
ALTER TABLE users DROP COLUMN IF EXISTS idp_provider;
