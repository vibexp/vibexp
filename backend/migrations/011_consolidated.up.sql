-- Consolidated post-v0.8.0 migration.
--
-- Squashes the migrations that accumulated after the v0.8.0 release into a
-- single step, applied on top of 010, mirroring the 002/006 consolidations
-- (#76, #399). Merged here (in original order):
--   * 011_user_status                      (issue #454)
--   * 012_team_search_settings             (issue #488, epic #487)
--   * 013_per_team_github_app              (issue #477, epic #476)
--   * 014_team_email_providers             (issue #501, epic #499)
--   * 015_remove_ai_tool_hooks             (issue #614, epic #610)
--   * 016_remove_quota_subscription        (issue #653, epic #646)
--   * 017_remove_firebase_web_push         (issue #688)
--   * 018_remove_web_push_preferences_keys (issue #690)
-- The eight are independent (disjoint tables/columns), so each block below is
-- the original migration verbatim; nothing needed to be reconciled across them.
-- No deployed instance has applied any of them (none shipped in a release), so
-- renumbering is safe.

-- ===========================================================================
-- 011_user_status
-- ===========================================================================

-- User suspension lifecycle (#454).
--
-- Adds a representable "account is off" state. The DEFAULT backfills every
-- existing row to 'active' in the same statement, so there is no separate
-- backfill step and no window where a row has no status.
--
-- Suspension is instance-local: it does NOT disable the account at the upstream
-- identity provider. It is enforced per request at every authentication entry
-- point, so suspending a user immediately invalidates their existing sessions,
-- API keys and OAuth/MCP tokens rather than waiting for them to expire.

ALTER TABLE users
    ADD COLUMN status character varying(20) NOT NULL DEFAULT 'active';

ALTER TABLE users
    ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended'));

-- Partial index: suspended accounts are the rare case, and the only query that
-- filters on this column is the admin listing looking for non-active users.
-- A full index would be almost entirely 'active' entries and earn nothing.
CREATE INDEX idx_users_status_not_active ON users (status) WHERE status <> 'active';

COMMENT ON COLUMN users.status IS
    'Account lifecycle: active (normal) or suspended (blocked at every auth entry point). Instance-local; does not affect the upstream IdP.';

-- ===========================================================================
-- 012_team_search_settings
-- ===========================================================================

-- Per-team search ranking overrides (#488, epic #487).
--
-- Search ranking is currently frozen into a process-lifetime singleton, built
-- at startup from the `search:` block of config.yaml and therefore identical
-- for every team on the instance. This table is the storage foundation for
-- letting a team own its own ranking profile.
--
-- Whole-row override, NOT per-field NULL inheritance: a team either owns its
-- complete profile or inherits the instance defaults entirely. That keeps the
-- mental model coherent — a team's relevance weight is never blended with the
-- instance's created weight. Consequently every profile column is NOT NULL.

CREATE TABLE team_search_settings (
    -- team_id is the PRIMARY KEY (not a surrogate id + UNIQUE, as
    -- user_preferences does): the one-row-per-team singleton is then enforced
    -- by the schema itself rather than by application code.
    team_id                 uuid PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,

    recency_ranking_enabled boolean          NOT NULL,
    rank_weight_relevance   double precision NOT NULL,
    rank_weight_created     double precision NOT NULL,
    rank_weight_updated     double precision NOT NULL,
    rank_half_life_days     double precision NOT NULL,

    created_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version                 bigint      NOT NULL DEFAULT 1,

    -- These CHECKs deliberately mirror validateSearchRankingConfig
    -- (backend/internal/config/config.go) bound for bound, so a team can never
    -- persist a profile the operator themself could not configure. If either
    -- side's bounds change, change both.
    CONSTRAINT team_search_settings_weights_nonneg CHECK (
        rank_weight_relevance >= 0 AND rank_weight_created >= 0 AND rank_weight_updated >= 0),
    CONSTRAINT team_search_settings_weights_nonzero CHECK (
        rank_weight_relevance + rank_weight_created + rank_weight_updated > 0),
    CONSTRAINT team_search_settings_half_life CHECK (
        rank_half_life_days > 0 AND rank_half_life_days <= 36500)
);

-- No index beyond the primary key: the only query this table ever serves is
-- `WHERE team_id = $1`, which the PK already covers.

COMMENT ON TABLE team_search_settings IS
    'Optional per-team override of the instance search ranking defaults (config.yaml `search:`). Row absent = inherit instance defaults. rank_candidate_cap is deliberately absent: it stays instance-only.';

COMMENT ON COLUMN team_search_settings.rank_half_life_days IS
    'Freshness half-life in days, capped at 36500 (100 years) to keep the days->time.Duration conversion clear of int64 nanosecond overflow.';

-- ===========================================================================
-- 013_per_team_github_app
-- ===========================================================================

-- Issue #477: per-team GitHub App configuration (epic #476).
--
-- GitHub App credentials lived in config.yaml, so one instance meant one App
-- and one GitHub identity for every team. Everything downstream was already
-- team-scoped (github_installations.team_id, routes under /{team_id}/...);
-- only the credentials were global. This migration adds the table that holds a
-- team's own App and binds installations to the App that created them.
--
-- Deliberately absent:
--   * no base_url column  -> GitHub Enterprise Server support is deferred
--   * no is_default flag  -> UNIQUE (team_id) means one App per team, so a
--                            default would never have anything to choose from
--
-- This migration only stores credentials; the service, encryption, resolver and
-- config removal land in the sibling issues of the epic.

CREATE TABLE public.github_app_configs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    -- Creator, informational only: it records who registered the App and is
    -- never used to scope a read (the team FK is the tenancy boundary).
    user_id uuid,
    app_id character varying(50) NOT NULL,
    -- Builds the https://github.com/apps/<slug>/installations/new install URL.
    app_slug character varying(255) NOT NULL,
    -- Not a secret: GitHub shows it on the App settings page and the UI echoes
    -- it back so an operator can confirm which App is wired up.
    client_id character varying(255) NOT NULL,
    private_key_encrypted text NOT NULL,
    client_secret_encrypted text NOT NULL,
    webhook_secret_encrypted text NOT NULL,
    -- Opaque routing token embedded in this App's webhook URL. The public
    -- webhook route has no team context, so this token is what resolves the
    -- delivery to a team.
    webhook_token character varying(64) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL
);

ALTER TABLE public.github_app_configs
    ADD CONSTRAINT github_app_configs_pkey PRIMARY KEY (id);

ALTER TABLE public.github_app_configs
    ADD CONSTRAINT fk_github_app_configs_team
        FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE public.github_app_configs
    ADD CONSTRAINT unique_team_github_app UNIQUE (team_id);

-- A GitHub App has exactly ONE hook_url, so two teams registering the same App
-- would leave the second team's webhook token permanently dead. Rejecting the
-- duplicate at write time beats shipping a silently broken integration.
ALTER TABLE public.github_app_configs
    ADD CONSTRAINT unique_github_app_id UNIQUE (app_id);

CREATE UNIQUE INDEX idx_github_app_configs_webhook_token
    ON public.github_app_configs (webhook_token);

-- NOT NULL does not bar the empty string, and both of these columns turn an
-- empty value into a silent cross-tenant failure rather than an error:
--   * webhook_token is a routing key on a PUBLIC, unauthenticated route. An
--     empty token would let an empty URL segment resolve a real team's config.
--   * an empty app_id would trip unique_github_app_id and report the misleading
--     "already registered by another team" to the second writer.
-- Both columns are writable through the optimistic-locked UPDATE, so a caller
-- that omits one would otherwise blank it without complaint.
ALTER TABLE public.github_app_configs
    ADD CONSTRAINT github_app_configs_webhook_token_not_empty
        CHECK (length(webhook_token) > 0);

ALTER TABLE public.github_app_configs
    ADD CONSTRAINT github_app_configs_app_id_not_empty
        CHECK (length(app_id) > 0);

COMMENT ON TABLE public.github_app_configs IS
    'Per-team GitHub App credentials. Replaces the instance-wide config.yaml App (#476).';

--
-- Bind installations to the App that created them.
--

ALTER TABLE public.github_installations
    ADD COLUMN app_config_id uuid;

-- Breaking, and intended: every existing row references the instance-wide App
-- whose credentials leave config.yaml later in this epic, so those rows are
-- unusable regardless. There is no value to backfill app_config_id with --
-- no github_app_configs row exists yet. Affected teams reconnect their own App.
DELETE FROM public.github_installations;

ALTER TABLE public.github_installations
    ALTER COLUMN app_config_id SET NOT NULL;

-- Deleting an App config disconnects its installations; the client-cache
-- eviction for that path is wired in the resolver sub-issue (#480).
ALTER TABLE public.github_installations
    ADD CONSTRAINT fk_installations_app_config
        FOREIGN KEY (app_config_id) REFERENCES public.github_app_configs(id) ON DELETE CASCADE;

-- The global installation_id UNIQUE is wrong once Apps are per-team: two teams
-- installing THEIR OWN App on the same GitHub org is legitimate. Scope the
-- uniqueness to the App instead.
ALTER TABLE public.github_installations
    DROP CONSTRAINT github_installations_installation_id_key;

ALTER TABLE public.github_installations
    ADD CONSTRAINT unique_app_installation UNIQUE (app_config_id, installation_id);

CREATE INDEX idx_github_installations_app_config_id
    ON public.github_installations (app_config_id);

--
-- Housekeeping.
--

-- Dead schema: an FK, a unique constraint and an index, but no Go model, no
-- repository and no query anywhere in the codebase.
DROP TABLE IF EXISTS public.github_installation_repositories;

-- GitHub deliveries have shared this table for delivery-id dedup since the
-- GitHub App landed; the comment never caught up.
COMMENT ON TABLE public.webhook_events IS
    'Tracks processed inbound webhook events (Stripe and GitHub App deliveries) to prevent duplicate processing (idempotency)';

-- ===========================================================================
-- 014_team_email_providers
-- ===========================================================================

-- Issue #501: per-team email provider configuration (epic #499).
--
-- Mail credentials live only in config.yaml today, so every team on an instance
-- sends through the operator's provider and from the operator's address. This
-- table holds a team's own provider so its transactional mail comes from its own
-- domain and its own sending reputation.
--
-- Deliberately absent:
--   * no enabled / is_active flag -> the ABSENCE OF A ROW is the instance
--                                   fallback, so a disabled row would be a
--                                   second way to express the same state
--   * no is_default flag         -> UNIQUE (team_id) means one provider per
--                                  team, so a default has nothing to choose from
--
-- This migration only stores configuration; encryption, validation, the send-time
-- resolver and the HTTP surface land in the sibling issues of the epic.

CREATE TABLE public.team_email_providers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    -- Last editor, informational only: it records who configured the provider
    -- and is never used to scope a read (the team FK is the tenancy boundary).
    user_id uuid,
    -- One of smtp|mailgun|postmark|sendgrid, matching the values accepted by
    -- implementations.NewEmailProvider. Left as a varchar rather than an enum so
    -- adding a provider stays a code change with no migration.
    provider_type character varying(50) NOT NULL,
    -- Non-secret per-type fields (SMTP host/port/username, Mailgun domain and
    -- base URL, Postmark message stream). Shaped per provider_type, so it is
    -- jsonb rather than a column per provider knob.
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- The single credential every supported provider has (SMTP password,
    -- Mailgun sending key, Postmark server token, SendGrid API key), as
    -- base64 AES-256-GCM ciphertext. NOT NULL because a row always represents a
    -- fully configured provider -- there is no half-configured state to store.
    secret_encrypted text NOT NULL,
    -- Envelope identity. 320 = 64-octet local part + "@" + 255-octet domain,
    -- the maximum length of an email address.
    from_address character varying(320) NOT NULL,
    from_name character varying(255),
    reply_to character varying(320),
    -- Delivery health. last_success_at and last_error_at are kept side by side
    -- rather than collapsed into one status column so the CURRENT state is
    -- derived by comparing them (later timestamp wins) while the last failure
    -- stays readable for diagnosis after recovery.
    last_success_at timestamp with time zone,
    last_error text,
    last_error_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL
);

ALTER TABLE public.team_email_providers
    ADD CONSTRAINT team_email_providers_pkey PRIMARY KEY (id);

ALTER TABLE public.team_email_providers
    ADD CONSTRAINT fk_team_email_providers_team
        FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

-- One provider per team, enforced by the schema so no code path can create a
-- second. This is the deliberate divergence from model_providers /
-- embedding_providers, which are many-rows-per-team with a partial unique index
-- on (team_id) WHERE is_default.
ALTER TABLE public.team_email_providers
    ADD CONSTRAINT unique_team_email_provider UNIQUE (team_id);

-- No index beyond the PK and the unique constraint: every query this table
-- serves is keyed on team_id, which unique_team_email_provider already covers.

COMMENT ON TABLE public.team_email_providers IS
    'Optional per-team outbound email provider. Row absent = the team falls back to the instance provider from config.yaml. At most one row per team (unique_team_email_provider).';

COMMENT ON COLUMN public.team_email_providers.secret_encrypted IS
    'Base64 AES-256-GCM ciphertext of the provider''s single credential. Never logged, never serialized (models.TeamEmailProvider tags it json:"-").';

COMMENT ON COLUMN public.team_email_providers.settings IS
    'Non-secret per-type fields, shaped by provider_type (e.g. SMTP host/port/username, Mailgun domain/base_url, Postmark message_stream).';

-- ===========================================================================
-- 015_remove_ai_tool_hooks
-- ===========================================================================

-- Remove the AI-tool hook-ingestion feature at the data layer (epic #610, issue #614).
--
-- Drops the two hook payload tables, purges the orphaned Claude Code activity rows,
-- and retires the `ai_tools` API-key integration scope.
--
-- Step order is load-bearing:
--   * legacy `api_keys.usage_type = 'ai_tools'` values are migrated BEFORE chk_usage_type
--     is rewritten, otherwise the new constraint fails to validate on any real database;
--   * grant rows are deleted BEFORE the catalog row, to respect the FK between them.
--
-- No API key is deleted and no key loses the ability to authenticate: keys scoped only
-- to `ai_tools` are re-pointed to `everything`. This slightly widens those keys' scope,
-- accepted deliberately — they were single-purpose for a feature that no longer exists,
-- and the alternative silently bricks credentials users already hold.

-- 1. Re-point legacy single-purpose keys before tightening the CHECK constraint.
UPDATE api_keys SET usage_type = 'everything' WHERE usage_type = 'ai_tools';

-- 2. Rewrite chk_usage_type to allow only the three surviving usage types.
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;
ALTER TABLE api_keys
    ADD CONSTRAINT chk_usage_type CHECK (
        (usage_type)::text = ANY ((ARRAY[
            'cli'::character varying,
            'mcp'::character varying,
            'everything'::character varying
        ])::text[])
    );

-- 3. Drop the ai_tools grants, then the catalog row they reference.
DELETE FROM api_key_integration_permissions WHERE integration_code = 'ai_tools';
DELETE FROM api_key_integrations_catalog WHERE integration_code = 'ai_tools';

-- 4. Purge the historical Claude Code activity rows. Decided in refinement: these are
--    deleted, not preserved as history, and all destructive DML lives in this migration.
DELETE FROM activities
 WHERE activity_type IN ('claude_code_session', 'claude_code_tool', 'claude_code_prompt');

-- 5. Drop the payload tables. CASCADE takes the sequences, indexes, updated_at triggers
--    and team foreign keys with them, so they need not be enumerated.
DROP TABLE IF EXISTS public.claude_code_hooks_payload CASCADE;
DROP TABLE IF EXISTS public.cursor_ide_hooks_payload CASCADE;

-- ===========================================================================
-- 016_remove_quota_subscription
-- ===========================================================================

-- Remove the subscription/quota data layer (epic #646, issue #653).
--
-- VibeXP is free, open-source and self-hosted: there is no billing. The Go code
-- that read this data was deleted in #650 (ResourceUsageService), #651
-- (plan/quota models + TeamSubscriptionRepository) and #652 (the five billing
-- columns on the user payload), so nothing selects any of it any more.
--
-- DEPLOY ORDERING: #652 must already be running. It removed the SELECTs naming
-- the users columns below; a binary from before it errors on every user load
-- once they are gone.
--
-- Locking: DROP COLUMN in Postgres is a metadata-only operation — no table
-- rewrite — so the ACCESS EXCLUSIVE lock on `users` is held only briefly even
-- though it is the hottest table in the schema.

-- CASCADE carries the primary key, the unique constraint on
-- stripe_subscription_id, the four CHECK constraints, the five indexes, the
-- updated_at trigger and the team_id foreign key. That FK was ON DELETE
-- RESTRICT with the comment "Prevents team deletion when subscriptions exist";
-- dropping it removes a deletion blocker that could never fire, matching #648's
-- removal of the ACTIVE_SUBSCRIPTION_EXISTS / SUBSCRIPTION_CANCELING arms.
DROP TABLE IF EXISTS public.team_subscriptions CASCADE;

-- idx_users_stripe_customer_id and idx_users_subscription_canceled_at are
-- dropped automatically with their columns; enumerating them here would fail on
-- a re-run.
ALTER TABLE public.users
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS subscription_status,
    DROP COLUMN IF EXISTS trial_ends_at,
    DROP COLUMN IF EXISTS subscription_plan,
    DROP COLUMN IF EXISTS subscription_canceled_at;

-- ===========================================================================
-- 017_remove_firebase_web_push
-- ===========================================================================

-- Remove the Firebase Cloud Messaging web-push data layer (issue #688).
--
-- VibeXP is free, open-source and self-hosted. Web push was the last hard
-- dependency on a proprietary Google service: it required the operator to stand
-- up a Firebase project and set seven VITE_FIREBASE_* variables, and #320
-- established that the deprecated getToken/deleteToken client APIs cannot be
-- migrated to Firebase's documented replacement at all — the Go Admin SDK's
-- messaging.Message exposes only Token/Topic/Condition, with no field able to
-- address a Firebase Installation ID.
--
-- The Go code that read this table (DeviceTokenRepository, the
-- POST/DELETE /api/v1/device-tokens handlers and WebPushChannel) is deleted in
-- the same change, so nothing selects any of it any more.
--
-- DEPLOY ORDERING: unlike #652/#653 this needs no staged rollout. VibeXP ships
-- as a single combined image, so the binary that stopped reading device_tokens
-- and this migration land in the same release — there is no window in which an
-- older binary queries a dropped table.
--
-- Locking: DROP TABLE takes a brief ACCESS EXCLUSIVE lock on device_tokens
-- only. The CASCADE below carries device_tokens_pkey, the unique index
-- device_tokens_token_idx, device_tokens_user_id_idx, and the
-- device_tokens_user_id_fkey foreign key to users(id). That FK was
-- ON DELETE CASCADE, so it never blocked a user deletion and dropping it
-- removes no behaviour.

DROP TABLE IF EXISTS public.device_tokens CASCADE;

-- NOTE: the `web_push` keys inside user_preferences.preferences (a JSONB blob)
-- are deliberately left in place. They are not columns and no CHECK constraint
-- references them; Go simply ignores the key on unmarshal. They are removed
-- together with the `web_push` field in the preferences API contract, which is
-- deferred until @vibexp/api-client republishes without it (see #688).

-- ===========================================================================
-- 018_remove_web_push_preferences_keys
-- ===========================================================================

-- Remove the stale `web_push` keys from user_preferences.preferences (issue #690).
--
-- Migration 017 (#688) dropped the Firebase device-token data layer but
-- deliberately left these JSONB keys in place: the `web_push` field was still
-- `required` in the published preferences API contract, so stored documents
-- had to keep matching it until the contract itself changed. This migration is
-- the data half of that contract change — the field is removed from
-- schemas/preferences.yaml and models.Preferences* in the same release, so the
-- keys have no reader left.
--
-- The keys live at two shapes inside the JSONB blob:
--   preferences.notifications.channels.web_push          (global channel switch)
--   preferences.notifications.types.<type>.web_push      (per-type switch)
-- #- '{a,b}' deletes a key at a nested path; the second statement rewrites each
-- per-type object without its web_push key via a lateral jsonb_each expansion.
-- Both are no-ops for rows that never carried the keys.
--
-- DEPLOY ORDERING: same single-image argument as 017 — the binary that stopped
-- reading/writing web_push and this migration ship together, so there is no
-- window in which an older binary expects the keys. Go's encoding/json already
-- ignored unknown keys, so even a rolled-back binary is safe against a
-- stripped document.
--
-- Locking: two UPDATEs take row locks on user_preferences only; the table is
-- small (one row per user) and no index or constraint references the JSONB
-- contents.

UPDATE public.user_preferences
SET preferences = preferences #- '{notifications,channels,web_push}'
WHERE preferences #> '{notifications,channels}' ? 'web_push';

UPDATE public.user_preferences up
SET preferences = jsonb_set(
    preferences,
    '{notifications,types}',
    COALESCE((
        SELECT jsonb_object_agg(t.key, t.value - 'web_push')
        FROM jsonb_each(preferences #> '{notifications,types}') AS t(key, value)
    ), '{}'::jsonb)
)
WHERE EXISTS (
    SELECT 1
    FROM jsonb_each(preferences #> '{notifications,types}') AS t(key, value)
    WHERE t.value ? 'web_push'
);
