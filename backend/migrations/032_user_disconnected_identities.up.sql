-- Disconnected sign-in identities (#1460, epic #1426).
--
-- Disconnecting an identity from Account settings removes its user_identities
-- row. Sign-in links a provider account to an existing account by a VERIFIED
-- email (#1430), so without a record the very next sign-in through that
-- provider account would attach it again. A person who disconnects a provider
-- account (one they believe is compromised, say) expects it to stay out.
--
-- One row per (user, provider, subject) the owner disconnected. A sign-in that
-- would link that pair to that account by email is refused until the owner,
-- signed in another way, allows it again from Account settings, which deletes
-- the row. An identity is never recorded for an account it was not attached
-- to, so an admin-created or invited account's first sign-in is unaffected.
--
--   provider  the provider slug of the removed row (user_identities.provider);
--             a legacy Google alias removed with it is not recorded separately.
--   subject   the provider's subject of the removed row.
CREATE TABLE user_disconnected_identities (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider        text NOT NULL,
    subject         text NOT NULL,
    disconnected_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT user_disconnected_identities_user_provider_subject_key UNIQUE (user_id, provider, subject)
);
