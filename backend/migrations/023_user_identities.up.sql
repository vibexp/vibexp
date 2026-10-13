-- Sign-in identities move off the users row, and users.email becomes lower
-- case and case-insensitively unique (#1427, epic #1426).
--
-- Until now a user had exactly one identity, stored inline (idp_provider,
-- idp_subject, plus the legacy google_id), and users.email was unique only
-- case-sensitively and written in whatever casing each code path was given.
-- Both block an account from holding several sign-in methods.
--
-- This migration:
--   1. refuses to run if two accounts differ only by the case of their email;
--   2. lower-cases users.email and pins it with a CHECK, which together with
--      the existing users_email_key unique constraint makes the address
--      case-insensitively unique;
--   3. adds users.email_verified_at;
--   4. creates user_identities and backfills it from the inline columns.
--
-- users.idp_provider, idp_subject and google_id stay and are still written;
-- they are no longer what a sign-in is looked up by.

-- 1. Case conflicts. Nothing below can pick a winner between two accounts, so
-- stop before changing anything and name the rows.
DO $$
DECLARE
    conflicts text;
BEGIN
    SELECT string_agg(grp.detail, '; ' ORDER BY grp.lower_email)
      INTO conflicts
      FROM (
            SELECT lower(email) AS lower_email,
                   string_agg(format('%s (user id %s)', email, id), ', ' ORDER BY created_at, id) AS detail
              FROM users
             GROUP BY lower(email)
            HAVING count(*) > 1
           ) AS grp;

    IF conflicts IS NOT NULL THEN
        -- Everything the operator needs is in the message itself: the
        -- migration runner reports only the message (a HINT is dropped), and
        -- the next boot reports just "dirty database version 23".
        RAISE EXCEPTION 'migration 023 stopped: these accounts have email addresses that differ only by letter case: %. Email addresses become case-insensitive in this release. Nothing was changed. To continue: change the email of, or delete, all but one account in each group; run UPDATE schema_migrations SET version = 22, dirty = false; then start VibeXP again.', conflicts;
    END IF;
END $$;

-- 2. Lower-case and pin.
UPDATE users SET email = lower(email) WHERE email <> lower(email);

ALTER TABLE users
    ADD CONSTRAINT users_email_lowercase CHECK (email = lower(email));

-- 3. When a sign-in provider first confirmed the stored address belongs to the
-- user. NULL for every existing account: it was never recorded.
ALTER TABLE users
    ADD COLUMN email_verified_at timestamptz;

COMMENT ON COLUMN users.email IS
    'Lower case (users_email_lowercase), so users_email_key makes it case-insensitively unique. Written through models.NormalizeEmail.';
COMMENT ON COLUMN users.email_verified_at IS
    'When a sign-in provider first reported the stored email as verified; reset when the email changes. NULL when no provider has verified it.';

-- 4. Identities.
CREATE TABLE user_identities (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- A provider slug (instance_auth_providers.slug), 'dev' for the local dev
    -- login, or the reserved 'legacy_google_id'. Deliberately no foreign key:
    -- deleting a provider must not delete the accounts' identities.
    provider     character varying(63)  NOT NULL,
    subject      character varying(255) NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- NULL for a backfilled identity that has not signed in since.
    last_used_at timestamptz,
    CONSTRAINT user_identities_provider_subject_key UNIQUE (provider, subject)
);

CREATE INDEX idx_user_identities_user_id ON user_identities (user_id);

COMMENT ON TABLE user_identities IS
    'Sign-in identities. One row per (provider, subject) a user can sign in with; a user may hold several.';
COMMENT ON COLUMN user_identities.provider IS
    'Provider slug, ''dev'', or the reserved ''legacy_google_id'' (a pre-slug Google subject, claimable only by a Google-type provider). Slugs cannot contain an underscore, so the reserved value never collides with one.';

INSERT INTO user_identities (user_id, provider, subject, created_at)
SELECT id, idp_provider, idp_subject, COALESCE(created_at, CURRENT_TIMESTAMP)
  FROM users
 WHERE idp_provider IS NOT NULL AND idp_subject IS NOT NULL
ON CONFLICT (provider, subject) DO NOTHING;

INSERT INTO user_identities (user_id, provider, subject, created_at)
SELECT id, 'legacy_google_id', google_id, COALESCE(created_at, CURRENT_TIMESTAMP)
  FROM users
 WHERE google_id IS NOT NULL
ON CONFLICT (provider, subject) DO NOTHING;
