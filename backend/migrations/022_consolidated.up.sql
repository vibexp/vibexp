-- Consolidated post-v0.15.0 migration.
--
-- Squashes the migrations that accumulated after the v0.15.0 release into a
-- single step, applied on top of 021, mirroring the 002/006/011/013/016/019
-- consolidations (#76, #399, #710, #839, #976, #1368). Merged here (in
-- original order):
--   * 022_resource_write_events        (issue #1392)
--   * 023_user_identities              (issue #1427, epic #1426)
--   * 024_user_session_epoch           (issue #1428, epic #1426)
--   * 025_instance_auth_policy         (issue #1429, epic #1426)
--   * 026_resource_write_events_latest (issue #1418)
--   * 027_api_access_revocation        (issue #1449, epic #1426)
--   * 028_user_credentials             (issue #1431, epic #1426)
--   * 029_invitee_email_lowercase      (issue #1434, epic #1426)
--   * 030_password_changed_at          (issue #1437, epic #1426)
--   * 031_account_tokens               (issue #1432, epic #1426)
--   * 032_user_disconnected_identities (issue #1460, epic #1426)
-- Each block below is the original migration verbatim, with one exception: the
-- case-conflict error raised by the 023_user_identities block names this
-- migration's version (022) and the version an operator re-points to before
-- retrying (21). The original text named 023 and 22, and following it after
-- the consolidation would mark this whole migration applied without running
-- it. Nothing else needed to be reconciled across the blocks. Where a block's
-- own comments cite one of the numbers above (022, 024, ...), read it as the
-- block of that name in this file. No deployed instance has applied any of
-- them (none shipped in a release), so renumbering is safe.

-- ===========================================================================
-- 022_resource_write_events
-- ===========================================================================

-- Record how a resource write arrived (#1392).
--
-- resource_access_events already says which channel READ a resource. Nothing
-- said which channel WROTE one: content_versions.actor_type is only
-- 'human' or 'system', so an agent writing over MCP was stored as 'human',
-- and the creation metrics count by resource type alone.
--
-- resource_write_events is the write-side twin of resource_access_events: one
-- append-only row per successful create or update of a prompt, artifact,
-- blueprint or memory. content_versions cannot carry the metric by itself. It
-- gets a row only when an update changes the content, that row holds the PRIOR
-- content, a create writes none, and rows are pruned to a retention cap.
--
-- source uses the vocabulary of resource_access_events.source. There is no
-- 'unknown' value and no backfill: a resource with no create event was written
-- before this migration, and the metrics report it as unknown. Guessing 'web'
-- for old rows would corrupt the number this table exists to make true.
--
-- assistant_name is the name an MCP client reported for itself
-- (clientInfo.name), trimmed and capped by the server. It is a label, not an
-- identity. It is NULL for every write that is not MCP and for MCP sessions
-- that reported none.
--
-- project_id and api_key_id carry no foreign key on purpose: the row is a
-- record of what happened, and it must outlive a deleted project or a revoked
-- key. team_id cascades (a deleted team takes its history with it) and user_id
-- is set to NULL, as on resource_access_events.
CREATE TABLE resource_write_events (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    project_id     uuid,
    user_id        uuid REFERENCES users (id) ON DELETE SET NULL,
    resource_type  text NOT NULL,
    resource_id    uuid NOT NULL,
    action         text NOT NULL,
    source         text NOT NULL,
    assistant_name varchar(30),
    api_key_id     uuid,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT resource_write_events_resource_type_check
        CHECK (resource_type IN ('prompt', 'artifact', 'blueprint', 'memory')),
    CONSTRAINT resource_write_events_action_check
        CHECK (action IN ('create', 'update')),
    CONSTRAINT resource_write_events_source_check
        CHECK (source IN ('web', 'cli', 'mcp', 'api'))
);

-- The per-source write totals read one team's events for a time window.
CREATE INDEX idx_resource_write_events_team_created
    ON resource_write_events (team_id, created_at DESC);

-- The creation metrics LEFT JOIN each resource to its create event. Unique, so
-- the join can never fan a resource out into two rows and the by-source
-- breakdown sums to the existing totals by construction.
CREATE UNIQUE INDEX idx_resource_write_events_create
    ON resource_write_events (resource_type, resource_id)
    WHERE action = 'create';

-- The same two facts on a version snapshot, for the version history. Both
-- columns are nullable with no DEFAULT, so adding them rewrites nothing and
-- every version written before this migration reads NULL (unknown), never
-- 'web'.
--
-- The CHECK is not free: Postgres validates it by scanning content_versions
-- once, under the ALTER's ACCESS EXCLUSIVE lock. Every existing row has a NULL
-- source and passes. It is a read of a table whose rows are pruned to a
-- per-resource retention cap, not a rewrite, so it is added in one step
-- instead of as NOT VALID followed by a separate VALIDATE.
ALTER TABLE content_versions
    ADD COLUMN source text,
    ADD COLUMN assistant_name varchar(30),
    ADD CONSTRAINT content_versions_source_check
        CHECK (source IS NULL OR source IN ('web', 'cli', 'mcp', 'api'));

-- ===========================================================================
-- 023_user_identities
-- ===========================================================================

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
        -- the next boot reports just "dirty database version 22".
        RAISE EXCEPTION 'migration 022 stopped: these accounts have email addresses that differ only by letter case: %. Email addresses become case-insensitive in this release. Nothing was changed. To continue: change the email of, or delete, all but one account in each group; run UPDATE schema_migrations SET version = 21, dirty = false; then start VibeXP again.', conflicts;
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

-- ===========================================================================
-- 024_user_session_epoch
-- ===========================================================================

-- Per-user session revocation counter (#1428, epic #1426).
--
-- A session is a stateless encrypted cookie, so nothing on the server could
-- end one before its own expiry. users.session_epoch is that missing handle:
-- every cookie carries the epoch it was minted under, and the authentication
-- chokepoint rejects a cookie whose epoch differs from this column. Bumping the
-- column therefore ends every existing cookie session of that user at once.
--
-- The default is 0 and a cookie minted before this migration carries no epoch,
-- which decodes as 0, so nobody is signed out by the upgrade. API keys and
-- MCP/OAuth bearer tokens do not carry an epoch and are not affected by it.
ALTER TABLE users ADD COLUMN session_epoch INTEGER NOT NULL DEFAULT 0;

-- ===========================================================================
-- 025_instance_auth_policy
-- ===========================================================================

-- Instance authentication policy (#1429, epic #1426).
--
-- Two instance-wide switches an admin sets in Admin -> Settings ->
-- Authentication, next to the sign-in providers and the access allowlist:
--
--   password_login_enabled  whether people may sign in with an email and
--                           password. Off by default: password login is opt-in.
--   registration_mode       who may get an account. 'open' is today's
--                           behaviour (anyone the allowlist admits may sign up
--                           through a provider); 'invite_only' admits only
--                           people who were invited or created by an admin.
--
-- No row is seeded. No row stored means the defaults (password login off,
-- registration open), which is exactly how the instance behaved before this
-- migration, so the upgrade changes nothing. This mirrors
-- instance_auth_allowlist, where no row means open access.
--
-- Every write also bumps instance_auth_settings_version in the same
-- transaction, which is the cache key of the resolver that serves the policy.
CREATE TABLE instance_auth_policy (
    -- Singleton key: always true, so the primary key admits exactly one row.
    id                      boolean PRIMARY KEY DEFAULT true CHECK (id),
    password_login_enabled  boolean NOT NULL DEFAULT false,
    -- The values mirror models.RegistrationMode*; change both together.
    registration_mode       text NOT NULL DEFAULT 'open'
        CONSTRAINT instance_auth_policy_registration_mode_check
        CHECK (registration_mode IN ('open', 'invite_only')),
    created_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by              uuid REFERENCES users (id) ON DELETE SET NULL,
    version                 bigint NOT NULL DEFAULT 1
);

COMMENT ON TABLE instance_auth_policy IS
    'Instance authentication policy: the password login switch and the registration mode. At most one row (singleton key id = true); no row = the defaults (password login off, registration open).';

-- ===========================================================================
-- 026_resource_write_events_latest
-- ===========================================================================

-- Resolve a resource's most recent write event without a per-row scan (#1418).
--
-- The list and detail reads of prompts, artifacts, blueprints and memories now
-- report last_write_source: for each resource row, the source of its newest
-- resource_write_events row. Migration 022 indexed only the create event
-- (idx_resource_write_events_create, partial on action = 'create') and the
-- team's window (team_id, created_at), so "the latest event of this resource",
-- which is as often an update, had no index and would have filtered the whole
-- team's event log once per listed row.
--
-- (resource_type, resource_id, created_at DESC) makes that lookup one index
-- descent per row that stops at the first entry. source and assistant_name are
-- INCLUDEd because they are the only two columns the reads project, so the
-- lookup is index-only and never visits the heap.
CREATE INDEX idx_resource_write_events_resource_latest
    ON resource_write_events (resource_type, resource_id, created_at DESC)
    INCLUDE (source, assistant_name);

-- The write metrics count the content changes of a window that carry no
-- channel: version snapshots with a NULL source. Partial, because those rows
-- stop being written once every write path records its channel, so the index
-- stays the size of the pre-recording history instead of growing with the
-- table.
CREATE INDEX idx_content_versions_team_created_unsourced
    ON content_versions (team_id, created_at DESC)
    WHERE source IS NULL;

-- ===========================================================================
-- 027_api_access_revocation
-- ===========================================================================

-- Revoking all of a user's API access at once (#1449, epic #1426).
--
-- An API key or a stored OAuth grant can be deleted, but an MCP/CLI access
-- token that was already issued is a signed JWT: it is verified by signature
-- alone and never looked up, so deleting its row does not stop it before its
-- own expiry. users.api_tokens_valid_after is the missing handle, the bearer
-- token counterpart of users.session_epoch (024): the authentication
-- chokepoint refuses a bearer token whose iat is before this timestamp.
--
-- NULL means "never revoked": every token is judged on its own validity, so
-- nobody's tools are disconnected by the upgrade. Cookie sessions and API keys
-- are not affected by the column (sessions have session_epoch, keys are rows).
ALTER TABLE users ADD COLUMN api_tokens_valid_after timestamptz;

COMMENT ON COLUMN users.api_tokens_valid_after IS
    'Bearer access tokens issued (iat) before this moment do not authenticate. NULL when API access was never revoked. Set by the API access revocation (#1449).';

-- Revocation deletes a user's stored OAuth rows by subject (the user id). The
-- tables were only indexed by signature, request id and expiry, so that would
-- be a sequential scan of every grant on the instance.
CREATE INDEX idx_oauth_access_tokens_subject ON oauth_access_tokens (subject);
CREATE INDEX idx_oauth_refresh_tokens_subject ON oauth_refresh_tokens (subject);
CREATE INDEX idx_oauth_authorization_codes_subject ON oauth_authorization_codes (subject);
CREATE INDEX idx_oauth_pkce_sessions_subject ON oauth_pkce_sessions (subject);

-- ===========================================================================
-- 028_user_credentials
-- ===========================================================================

-- Password credentials (#1431, epic #1426).
--
-- One optional row per user: the argon2id hash of their password and the state
-- of the per-account sign-in throttle. A user with no row has no password and
-- signs in through a provider (or not at all, until an invitation or setup link
-- gives them one). Nothing is backfilled: no account has a password before this
-- migration.
--
--   password_hash    The encoded argon2id hash, parameters and salt included
--                    ("$argon2id$v=19$m=...,t=...,p=...$salt$key"). Never the
--                    password, and never returned by the API.
--   failed_attempts  Consecutive failed sign-ins since the last success.
--   locked_until     While in the future, sign-in is refused without checking
--                    the password. Locks start short and grow to a cap; there is
--                    no permanent lock, so an attacker cannot lock a user out
--                    for good.
--   disabled_at      Set when an administrator disables the password without
--                    removing it. A disabled credential never signs in.
--
-- The row goes with the user (ON DELETE CASCADE): a deleted account leaves no
-- hash behind.
CREATE TABLE user_credentials (
    user_id          uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash    text NOT NULL,
    failed_attempts  integer NOT NULL DEFAULT 0
        CONSTRAINT user_credentials_failed_attempts_check CHECK (failed_attempts >= 0),
    locked_until     timestamptz,
    disabled_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE user_credentials IS
    'Password credential of a user: the argon2id hash and the per-account sign-in throttle. At most one row per user; no row = no password.';
COMMENT ON COLUMN user_credentials.password_hash IS
    'Encoded argon2id hash (PHC string format, parameters and salt included). Never plaintext.';

-- ===========================================================================
-- 029_invitee_email_lowercase
-- ===========================================================================

-- Team invitation addresses are stored lower case (#1434, epic #1426).
--
-- users.email has been lower case since 023, while invitee_email kept whatever
-- casing the inviter typed, so every lookup of "the invitations for this
-- account" had to compare lower(invitee_email) and could not use
-- idx_team_invitations_invitee_email. With the address normalized on write
-- (models.NormalizeEmail) and pinned here, the lookup is a plain equality on
-- that index again. It now also decides who may register in invite_only mode,
-- so it must not depend on how an address was typed.
--
-- No unique constraint involves invitee_email, so lower-casing cannot conflict.
UPDATE team_invitations SET invitee_email = lower(invitee_email) WHERE invitee_email <> lower(invitee_email);

ALTER TABLE team_invitations
    ADD CONSTRAINT team_invitations_invitee_email_lowercase CHECK (invitee_email = lower(invitee_email));

COMMENT ON COLUMN team_invitations.invitee_email IS
    'Lower case (team_invitations_invitee_email_lowercase). Written through models.NormalizeEmail.';

-- ===========================================================================
-- 030_password_changed_at
-- ===========================================================================

-- When a user's password was last set or changed (#1437, epic #1426).
--
-- The Account settings page shows "password last changed". user_credentials
-- .updated_at cannot answer that: every sign-in attempt, every success and a
-- transparent rehash all bump it. password_changed_at moves only when a new
-- password is stored (UserCredentialRepository.Upsert).
--
-- Existing rows get the migration time. Password credentials first shipped in
-- 028, in the same release, so no released instance holds one.
ALTER TABLE user_credentials
    ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

COMMENT ON COLUMN user_credentials.password_changed_at IS
    'When the password was last set or changed. Not moved by sign-in attempts or by a rehash of the same password.';

-- ===========================================================================
-- 031_account_tokens
-- ===========================================================================

-- One-time account links (#1432, epic #1426).
--
-- A password account is never self-registered: a person gets one through an
-- invitation or an administrator and sets the password from a link, and a
-- forgotten password is recovered the same way. Each such link is one row here.
--
--   token_hash   SHA-256 of the token. The token itself is 32 random bytes and
--                exists only in the link; it is never stored, so a copy of this
--                table opens no account. A fast hash is right: there is nothing
--                to brute-force in 256 bits of randomness.
--   purpose      'setup' creates the account's first password; 'reset' replaces
--                a forgotten one.
--   delivery     'email' when the link was sent to the account's mailbox, 'link'
--                when an administrator copied it by hand. Completing an emailed
--                link proves the mailbox and may mark the email verified; a
--                copied link proves nothing about who holds it.
--   consumed_at  Set once, by the request that completes the link. A link is
--                single use.
--   created_by   The administrator who issued the link; NULL for a link the
--                system issued (forgot password) or once that user is deleted.
--
-- A user has at most one live link per purpose AND delivery (the partial unique
-- index). A link an administrator issues removes every live link of its
-- purpose, emailed or copied, so re-issuing one always ends the earlier ones
-- (#1490). A forgot-password request removes only the previous emailed link:
-- anyone can make it for any address, and it must not end a link an
-- administrator handed over, so an emailed and a copied link of one purpose
-- can still be live together. Completing any link removes every other live
-- link of the user, of either purpose: once a password is set, no older link
-- may set another.
-- Dead rows (expired or consumed) are deleted by an hourly sweep in the server
-- process.
CREATE TABLE account_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The values mirror models.AccountTokenPurpose*; change both together.
    purpose      text NOT NULL
        CONSTRAINT account_tokens_purpose_check CHECK (purpose IN ('setup', 'reset')),
    -- The values mirror models.AccountTokenDelivery*; change both together.
    delivery     text NOT NULL
        CONSTRAINT account_tokens_delivery_check CHECK (delivery IN ('email', 'link')),
    token_hash   bytea NOT NULL,
    expires_at   timestamptz NOT NULL,
    consumed_at  timestamptz,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT account_tokens_token_hash_key UNIQUE (token_hash)
);

-- At most one live link per user, purpose and delivery.
CREATE UNIQUE INDEX account_tokens_one_live_per_delivery
    ON account_tokens (user_id, purpose, delivery)
    WHERE consumed_at IS NULL;

-- The sweep deletes by expiry.
CREATE INDEX account_tokens_expires_at ON account_tokens (expires_at);

COMMENT ON TABLE account_tokens IS
    'One-time account links for setting a first password (setup) or replacing a forgotten one (reset). Only the SHA-256 of each token is stored.';

-- ===========================================================================
-- 032_user_disconnected_identities
-- ===========================================================================

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
--             Keyed by slug: a provider an administrator recreates under a new
--             slug is a different provider, and its sign-ins are not refused.
--   subject   the provider's subject of the removed row.
CREATE TABLE user_disconnected_identities (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider        text NOT NULL,
    subject         text NOT NULL,
    disconnected_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT user_disconnected_identities_user_provider_subject_key UNIQUE (user_id, provider, subject)
);
