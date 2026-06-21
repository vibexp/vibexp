-- Add provider-agnostic identity columns. google_id is kept (NOT NULL UNIQUE)
-- so the existing Google login flow keeps working unchanged. New columns are
-- nullable for now to allow a gradual rollout; they will be required once all
-- code paths write them.
ALTER TABLE users ADD COLUMN idp_provider VARCHAR(50);
ALTER TABLE users ADD COLUMN idp_subject  VARCHAR(255);

-- Backfill existing users from google_id so they can be looked up by
-- (idp_provider, idp_subject) immediately after this migration runs.
UPDATE users
SET idp_provider = 'google',
    idp_subject  = google_id
WHERE google_id IS NOT NULL
  AND idp_provider IS NULL;

-- Partial unique index avoids conflicts with rows that have not yet been
-- written through the new code path (idp_provider IS NULL).
CREATE UNIQUE INDEX idx_users_idp_provider_subject
    ON users (idp_provider, idp_subject)
    WHERE idp_provider IS NOT NULL AND idp_subject IS NOT NULL;
