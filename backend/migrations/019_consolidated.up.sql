-- Consolidated post-v0.14.0 migration.
--
-- Squashes the migrations that accumulated after the v0.14.0 release into a
-- single step, applied on top of 018, mirroring the 002/006/011/013/016
-- consolidations (#76, #399, #710, #839, #976). Merged here (in original
-- order):
--   * 019_resource_access_events_user_index   (issue #1136)
--   * 020_instance_email_provider             (issue #1186, epic #1185)
--   * 021_instance_settings_audit             (issue #1187, epic #1185)
--   * 022_instance_search_ai_summary_settings (issue #1197, epic #1196)
--   * 023_instance_auth_settings              (issue #1231, epic #1230)
--   * 024_instance_auth_setup                 (issue #1236, epic #1230)
--   * 025_team_statuses                       (issue #1264, epic #1263)
--   * 026_memories_preserve_updated_at        (issue #1265, epic #1263)
--   * 027_resource_locks                      (issue #1286, epic #1285)
-- Each block below is the original migration verbatim; nothing needed to be
-- reconciled across them. Where a block's own comments cite one of the numbers
-- above (020, 025, ...), read it as the block of that name in this file. No
-- deployed instance has applied any of them (none shipped in a release), so
-- renumbering is safe.

-- ===========================================================================
-- 019_resource_access_events_user_index
-- ===========================================================================

-- Per-user access analytics for the admin user detail page (#1136).
--
-- resource_access_events was indexed only by created_at and by team_id-leading
-- composites, so a per-user query had to filter a range scan across every
-- user's events. This index makes the per-user series and top-resources queries
-- an index range scan on (user_id, created_at).
--
-- Plain CREATE INDEX (not CONCURRENTLY): migrations run before the server
-- serves, and the table is bounded by retention.access_event_days.
CREATE INDEX IF NOT EXISTS idx_rae_user_created ON public.resource_access_events USING btree (user_id, created_at);

-- ===========================================================================
-- 020_instance_email_provider
-- ===========================================================================

-- Instance-level outbound email provider (#1186, epic #1185).
--
-- Instance mail configuration has only ever lived in the `email:` section of
-- config.yaml, built once at wire time, so an instance admin could not change it
-- without file access and a restart, and there was nowhere to persist delivery
-- health or the instance-only contact/privacy fields. This table is that
-- storage. It is shaped like team_email_providers (011_consolidated) with three
-- deliberate differences:
--
--   * a singleton key instead of team_id -> the instance has exactly one
--                                          provider or none, and the database,
--                                          not application code, guarantees it
--   * secret_encrypted is NULLABLE       -> unauthenticated SMTP relays (and the
--                                          dev Mailpit setup) are legitimate
--                                          instance configurations; NULL means
--                                          "no credential"
--   * instance-only fields               -> contact_recipient_address and
--                                          privacy_policy_url have no per-team
--                                          counterpart
--
-- Storage only: nothing reads this table until the service/resolver lands
-- (#1188), so existing behaviour is unchanged.

CREATE TABLE public.instance_email_provider (
    -- Singleton key. The column can only ever hold `true` (CHECK below) and is
    -- the primary key, so a second row is a unique violation and no code path
    -- can create one. Every write is therefore naturally ON CONFLICT (id).
    id boolean DEFAULT true NOT NULL,
    -- One of smtp|mailgun|postmark|sendgrid, matching the values accepted by
    -- implementations.NewEmailProvider. A varchar rather than an enum so adding
    -- a provider stays a code change with no migration.
    provider_type character varying(50) NOT NULL,
    -- Non-secret per-type fields (SMTP host/port/username, Mailgun domain and
    -- base URL, Postmark message stream), shaped per provider_type.
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- The provider's single credential (SMTP password, Mailgun sending key,
    -- Postmark server token, SendGrid API key) as base64 AES-256-GCM
    -- ciphertext. NULLABLE, unlike team_email_providers: NULL means the
    -- instance sends without a credential (e.g. an internal SMTP relay).
    -- Whether a given provider_type requires one is the service's decision.
    secret_encrypted text,
    -- Envelope identity. 320 = 64-octet local part + "@" + 255-octet domain.
    from_address character varying(320) NOT NULL,
    from_name character varying(255),
    reply_to character varying(320),
    -- Instance-only. Where the contact form delivers, and the privacy policy
    -- linked from outbound mail. Both nullable: empty means "fall back".
    contact_recipient_address character varying(320),
    privacy_policy_url text,
    -- Delivery health, kept as two timestamps rather than one status column so
    -- the current state is derived (later timestamp wins) while the last
    -- failure stays readable after recovery.
    last_success_at timestamp with time zone,
    last_error text,
    last_error_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    -- Last editor. NULL for the boot-time config.yaml import (no actor), and
    -- SET NULL on user deletion: removing an admin must not remove the
    -- instance's mail configuration.
    updated_by uuid,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT instance_email_provider_pkey PRIMARY KEY (id),
    CONSTRAINT instance_email_provider_singleton CHECK (id),
    CONSTRAINT fk_instance_email_provider_updated_by
        FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL
);

COMMENT ON TABLE public.instance_email_provider IS
    'Instance-level outbound email provider. At most one row (singleton key id = true); row absent = no stored instance configuration.';
COMMENT ON COLUMN public.instance_email_provider.id IS
    'Singleton key: always true (instance_email_provider_singleton), so the primary key admits exactly one row.';
COMMENT ON COLUMN public.instance_email_provider.secret_encrypted IS
    'Encrypted provider credential. NULL = no credential (e.g. unauthenticated SMTP relay).';

-- ===========================================================================
-- 021_instance_settings_audit
-- ===========================================================================

-- ===========================================================================
-- Instance settings audit -- append-only log of instance-level settings
-- changes (issue #1187, epic #1185).
--
-- Instance email configuration is moving out of config.yaml into the database
-- (#1186) and becomes editable at runtime by any instance admin. Without a log,
-- a credential rotation or a from-address change is unattributable. This table
-- is that record: who changed which instance setting, when, and what it looked
-- like before and after.
--
-- team_settings_audit (015) cannot hold these events: its team_id is NOT NULL
-- and a foreign key, and its readers are tenancy-scoped. It stays team-scoped;
-- this is its instance-level counterpart and mirrors its shape and access
-- pattern minus the team.
--
-- The `setting` column keys the entry, so future instance settings reuse this
-- table rather than adding another. Like 015 it is APPEND-ONLY: the repository
-- exposes no update and no delete.
-- ===========================================================================

CREATE TABLE instance_settings_audit (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    setting       text NOT NULL CHECK (setting <> ''),
    action        text NOT NULL CHECK (action IN ('upsert', 'delete', 'import')),
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    before        jsonb,
    after         jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE instance_settings_audit IS
    'Append-only log of instance-level settings changes (epic #1185). Rows are never updated or deleted.';
COMMENT ON COLUMN instance_settings_audit.setting IS
    'Which instance setting changed, e.g. ''email_provider''. Only non-empty is enforced here, so a new setting needs no migration; the Go constants are the closed set.';
COMMENT ON COLUMN instance_settings_audit.action IS
    'What happened: ''upsert'' (created or replaced), ''delete'', or ''import'' (the boot-time config.yaml import).';
COMMENT ON COLUMN instance_settings_audit.actor_user_id IS
    'Who made the change. NULL for the boot-time import, which has no actor. SET NULL on user deletion rather than CASCADE: the entry must outlive the account, or deleting it would erase the record of what it did.';
COMMENT ON COLUMN instance_settings_audit.before IS
    'Redacted snapshot of the setting before the change. NULL when the change created it. NEVER a credential -- only a "secret": "changed"|"unchanged" marker.';
COMMENT ON COLUMN instance_settings_audit.after IS
    'Redacted snapshot of the setting after the change. NULL when the change deleted it. NEVER a credential -- only a "secret": "changed"|"unchanged" marker.';

-- The audit list is "this setting's entries, newest first", paged by a keyset
-- cursor on (created_at, id).
--
-- `id DESC` is a load-bearing tiebreaker, not cosmetic: `now()` is
-- transaction-start time, so several entries written in one transaction share
-- one created_at, and without the tiebreaker the keyset cursor could not
-- position between them.
CREATE INDEX idx_instance_settings_audit_setting_created
    ON instance_settings_audit (setting, created_at DESC, id DESC);

-- ===========================================================================
-- 022_instance_search_ai_summary_settings
-- ===========================================================================

-- Instance-level search ranking and AI summary defaults (#1197, epic #1196).
--
-- Both have only ever lived in config.yaml (`search:` and `ai_summary:`),
-- captured once at wire time, so an instance admin could not change them
-- without file access and a restart -- and the published Docker image exposes
-- no `search:` knobs at all. These two tables are the storage that lets the
-- instance defaults be edited at runtime.
--
-- They mirror team_search_settings (011_consolidated) and
-- team_ai_summary_settings (017) with a singleton key in place of team_id, the
-- same shape instance_email_provider (020) uses. A team row cannot hold an
-- instance value: a NULL team_id would break the tenancy-only repository
-- invariant (epic decision 6).
--
-- No row means "use the built-in defaults", so DELETE is the reset (epic
-- decision 7). Every CHECK below mirrors a Go constant -- models.MaxSearchRank*,
-- config.MaxAISummaryTopN, config.MaxAISummaryOutputTokens and
-- models.AISummaryStyles -- and a change on either side must change both. The
-- CHECKs are the storage backstop; the services validate first.
--
-- Storage only: nothing reads these tables until the resolvers land (#1198,
-- #1199), so existing behaviour is unchanged.

CREATE TABLE instance_search_settings (
    -- Singleton key. It can only ever hold `true` (CHECK) and is the primary
    -- key, so a second row is a unique violation and no code path can create
    -- one.
    id                      boolean PRIMARY KEY DEFAULT true CHECK (id),
    recency_ranking_enabled boolean          NOT NULL,
    rank_weight_relevance   double precision NOT NULL,
    rank_weight_created     double precision NOT NULL,
    rank_weight_updated     double precision NOT NULL,
    rank_half_life_days     double precision NOT NULL,
    -- Instance-only: the re-rank candidate pool sizes the work one search asks
    -- of the database, so no team row carries it.
    rank_candidate_cap      integer          NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Last editor. NULL for the boot-time config.yaml import (no actor), and
    -- SET NULL on user deletion: removing an admin must not reset the
    -- instance's defaults.
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    version     bigint NOT NULL DEFAULT 1,
    CONSTRAINT instance_search_settings_weights_nonneg CHECK (
        rank_weight_relevance >= 0 AND rank_weight_created >= 0 AND rank_weight_updated >= 0),
    CONSTRAINT instance_search_settings_weights_nonzero CHECK (
        rank_weight_relevance + rank_weight_created + rank_weight_updated > 0),
    -- Mirrors models.MaxSearchRankHalfLifeDays.
    CONSTRAINT instance_search_settings_half_life CHECK (
        rank_half_life_days > 0 AND rank_half_life_days <= 36500),
    -- Mirrors models.MaxSearchRankCandidateCap.
    CONSTRAINT instance_search_settings_candidate_cap CHECK (
        rank_candidate_cap BETWEEN 1 AND 5000)
);

COMMENT ON TABLE instance_search_settings IS
    'Instance-level search ranking defaults. At most one row (singleton key id = true); no row = built-in defaults, so DELETE is the reset.';
COMMENT ON COLUMN instance_search_settings.id IS
    'Singleton key: always true, so the primary key admits exactly one row.';
COMMENT ON COLUMN instance_search_settings.rank_half_life_days IS
    'Recency half-life in days, (0, 36500]. The bound mirrors models.MaxSearchRankHalfLifeDays; change both together.';
COMMENT ON COLUMN instance_search_settings.rank_candidate_cap IS
    'Instance-only re-rank candidate pool, [1, 5000]. The bound mirrors models.MaxSearchRankCandidateCap; change both together.';
COMMENT ON COLUMN instance_search_settings.updated_by IS
    'Last editor. NULL for the boot-time config.yaml import, and SET NULL when that user is deleted.';

CREATE TABLE instance_ai_summary_settings (
    id                  boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- The team-inheritable defaults (the columns team_ai_summary_settings
    -- overrides). There are deliberately NO max_top_n /
    -- max_output_tokens_ceiling columns (epic decision 3): the hard limits are
    -- the constants the CHECKs below mirror.
    enabled             boolean NOT NULL,
    -- Mirrors config.MaxAISummaryTopN.
    top_n               integer NOT NULL CHECK (top_n BETWEEN 1 AND 10),
    -- Mirrors models.AISummaryStyles.
    style               character varying(20) NOT NULL
                            CHECK (style IN ('concise', 'balanced', 'detailed')),
    -- Mirrors config.MaxAISummaryOutputTokens. The team table checks only
    -- > 0; the instance row pins the hard limit (epic decision 3).
    max_output_tokens   integer NOT NULL CHECK (max_output_tokens BETWEEN 1 AND 32768),
    -- Instance-only budgets: they size the work one request may ask of the
    -- operator's model and hardware, so a team cannot raise them.
    per_document_chars  integer NOT NULL CHECK (per_document_chars >= 1),
    total_context_chars integer NOT NULL,
    request_timeout_ms  integer NOT NULL CHECK (request_timeout_ms >= 1),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by uuid REFERENCES users (id) ON DELETE SET NULL,
    version    bigint NOT NULL DEFAULT 1,
    CONSTRAINT instance_ai_summary_settings_context_budget CHECK (
        total_context_chars >= per_document_chars)
);

COMMENT ON TABLE instance_ai_summary_settings IS
    'Instance-level AI summary defaults and budgets. At most one row (singleton key id = true); no row = built-in defaults, so DELETE is the reset.';
COMMENT ON COLUMN instance_ai_summary_settings.id IS
    'Singleton key: always true, so the primary key admits exactly one row.';
COMMENT ON COLUMN instance_ai_summary_settings.top_n IS
    'Default documents per summary, [1, 10]. The bound mirrors config.MaxAISummaryTopN; change both together.';
COMMENT ON COLUMN instance_ai_summary_settings.style IS
    'Default summary style. The set mirrors models.AISummaryStyles; change both together.';
COMMENT ON COLUMN instance_ai_summary_settings.max_output_tokens IS
    'Default output token budget, [1, 32768]. The bound mirrors config.MaxAISummaryOutputTokens; change both together.';
COMMENT ON COLUMN instance_ai_summary_settings.total_context_chars IS
    'Instance-only total context budget; never smaller than per_document_chars.';
COMMENT ON COLUMN instance_ai_summary_settings.request_timeout_ms IS
    'Instance-only summary request timeout, in milliseconds.';
COMMENT ON COLUMN instance_ai_summary_settings.updated_by IS
    'Last editor. NULL for the boot-time config.yaml import, and SET NULL when that user is deleted.';

-- ===========================================================================
-- 023_instance_auth_settings
-- ===========================================================================

-- DB-managed authentication settings (#1231, epic #1230).
--
-- Identity providers, the sign-in access allowlist and instance admins have
-- only ever lived in config.yaml / env, read once at boot, so an instance admin
-- could not change them without file access and a restart. These four tables
-- are the storage that lets them be managed at runtime.
--
-- The invariants every writer must respect -- at most one google and one
-- github provider, a unique url-safe slug, an issuer URL on exactly the OIDC
-- providers -- are enforced here, so no code path (admin API, boot import,
-- break-glass CLI) can store an inconsistent provider set.
--
-- Storage only: nothing reads these tables until the resolvers land (#1232,
-- #1233, #1234, #1235), so login behaviour is unchanged.

CREATE TABLE instance_auth_providers (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type                    character varying(20) NOT NULL
                                CHECK (type IN ('google', 'github', 'oidc')),
    -- Immutable, url-safe identifier used in login/callback routes. The
    -- pattern mirrors services.instanceAuthProviderSlugPattern; change both
    -- together.
    slug                    character varying(63) NOT NULL UNIQUE
                                CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    display_name            text    NOT NULL,
    enabled                 boolean NOT NULL DEFAULT false,
    sort_order              integer NOT NULL DEFAULT 0,
    client_id               text    NOT NULL,
    -- Ciphertext from the encryption service; never plaintext. NULL when no
    -- secret is stored.
    client_secret_encrypted text,
    issuer_url              text,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Last editor. NULL for the boot-time config.yaml import, and SET NULL
    -- when that user is deleted.
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    -- An OIDC provider is discovered from its issuer; google and github have
    -- fixed endpoints and must not carry one.
    CONSTRAINT instance_auth_providers_issuer_iff_oidc CHECK (
        (type = 'oidc') = (issuer_url IS NOT NULL))
);

-- At most one google and one github provider: their endpoints are fixed, so a
-- second row of either could only be a duplicate. Any number of OIDC rows.
CREATE UNIQUE INDEX instance_auth_providers_one_per_builtin_type
    ON instance_auth_providers (type) WHERE type IN ('google', 'github');

CREATE INDEX instance_auth_providers_order ON instance_auth_providers (sort_order, slug);

COMMENT ON TABLE instance_auth_providers IS
    'DB-managed sign-in identity providers. At most one google and one github row; any number of oidc rows.';
COMMENT ON COLUMN instance_auth_providers.slug IS
    'Immutable url-safe identifier, ^[a-z0-9][a-z0-9-]{0,62}$. The pattern mirrors services.instanceAuthProviderSlugPattern; change both together.';
COMMENT ON COLUMN instance_auth_providers.client_secret_encrypted IS
    'OAuth client secret as ciphertext from the encryption service. Never plaintext; NULL when none is stored.';
COMMENT ON COLUMN instance_auth_providers.issuer_url IS
    'OIDC issuer URL. Required for type oidc and forbidden otherwise.';

CREATE TABLE instance_auth_allowlist (
    -- Singleton key: always true, so the primary key admits exactly one row.
    id          boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- Normalized (trimmed, lower-cased, deduplicated) by
    -- services.ValidateInstanceAuthAllowlist before they are stored.
    domains     text[] NOT NULL DEFAULT '{}',
    emails      text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    version     bigint NOT NULL DEFAULT 1
);

COMMENT ON TABLE instance_auth_allowlist IS
    'Sign-in access allowlist (email domains and addresses). At most one row (singleton key id = true); no row = open access.';

CREATE TABLE instance_auth_settings_version (
    id          boolean PRIMARY KEY DEFAULT true CHECK (id),
    version     bigint NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE instance_auth_settings_version IS
    'Change counter for instance_auth_providers and instance_auth_allowlist: every write to either bumps it in the same transaction. Seeded with one row so readers never see none; the cache key of the provider and allowlist resolvers.';

INSERT INTO instance_auth_settings_version (id, version) VALUES (true, 1);

CREATE TABLE instance_admins (
    user_id     uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Who granted it. NULL for a grant with no actor (boot import, CLI), and
    -- SET NULL when that user is deleted: the grant outlives its grantor.
    granted_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE instance_admins IS
    'Instance admins granted in the database, in addition to those named in config.yaml. Deleting the user removes the grant.';

-- ===========================================================================
-- 024_instance_auth_setup
-- ===========================================================================

-- First-run authentication setup state (#1236, epic #1230).
--
-- An instance with no enabled identity provider has no way to sign in, so it
-- cannot reach the admin panel that configures sign-in. While that holds the
-- server mints a one-time setup token at boot and logs a setup URL; this table
-- is the token's only storage. It holds the token's HASH, never the token, so
-- a database read cannot be replayed as a setup URL, and it lives in the
-- database rather than in process memory so every replica validates the same
-- token.
--
-- Storage only: whether setup mode is active is computed by
-- services.SetupModeService, not stored here.

CREATE TABLE instance_auth_setup (
    -- Singleton key: always true, so the primary key admits exactly one row.
    id           boolean PRIMARY KEY DEFAULT true CHECK (id),
    -- SHA-256 of the setup token. NULL once the token is consumed.
    token_hash   bytea CHECK (token_hash IS NULL OR octet_length(token_hash) = 32),
    expires_at   timestamptz,
    -- Set when a root instance admin's first provider sign-in ends setup.
    consumed_at  timestamptz,
    -- Who consumed it; SET NULL when that user is deleted.
    consumed_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    -- True while setup was deliberately re-armed (break-glass CLI, #1237) and
    -- not yet consumed: setup mode stays active even with a provider enabled.
    rearmed      boolean NOT NULL DEFAULT false,
    -- Bumped by every mint and by consumption. A setup session carries the
    -- generation it was issued at, so a bump invalidates every outstanding one.
    generation   bigint NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- A live token always has an expiry.
    CONSTRAINT instance_auth_setup_token_has_expiry CHECK (
        token_hash IS NULL OR expires_at IS NOT NULL)
);

COMMENT ON TABLE instance_auth_setup IS
    'First-run authentication setup state. At most one row (singleton key id = true); no row = no setup token was ever issued.';
COMMENT ON COLUMN instance_auth_setup.token_hash IS
    'SHA-256 of the one-time setup token. The token itself is never stored. NULL once consumed.';
COMMENT ON COLUMN instance_auth_setup.generation IS
    'Bumped by every mint and by consumption; a setup session issued at an older generation is invalid.';

-- ===========================================================================
-- 025_team_statuses
-- ===========================================================================

-- ===========================================================================
-- Team-defined custom statuses -- storage foundation (issue #1264, epic #1263).
--
-- Until now a resource's status was a closed, code-defined set per resource
-- type (models.PromptStatuses and friends in internal/models/status.go). This
-- migration adds the table a team's OWN statuses live in, and removes the two
-- things in the schema that made a custom status unstorable: the CHECK
-- constraints on prompts and memories, and the varchar(20) cap on all four
-- resource tables.
--
-- The SYSTEM statuses are deliberately not seeded here. They stay in Go, so
-- there is one definition of them and no row a team could edit or delete; the
-- catalog service composes the system subset with this table's rows.
-- ===========================================================================

CREATE TABLE team_statuses (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    resource_type varchar(20) NOT NULL,
    slug          varchar(50) NOT NULL,
    name          varchar(50) NOT NULL,
    created_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT team_statuses_team_resource_slug_key UNIQUE (team_id, resource_type, slug)
);

COMMENT ON TABLE team_statuses IS
    'A team''s custom resource statuses (epic #1263). System statuses are not stored here: they are defined in Go (internal/models/status.go).';
COMMENT ON COLUMN team_statuses.resource_type IS
    'Singular resource type the status applies to: prompt, artifact, blueprint or memory. Validated in the service layer, not by a CHECK (same convention as team_settings_audit.surface).';
COMMENT ON COLUMN team_statuses.slug IS
    'The value written to the resource''s status column. Unique per (team_id, resource_type); that unique index also serves the per-team, per-type list.';
COMMENT ON COLUMN team_statuses.created_by IS
    'The member who created the status. SET NULL on user delete: the status outlives its author.';

-- A custom status must be storable on every resource type. prompts and
-- memories were the two tables whose CHECK admitted only the system set
-- (prompts_status_check from 001_baseline, memories_status_check from
-- 002_consolidated); artifacts and blueprints never had one. From here on the
-- service layer is the only gate, as it always was for those two.
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_status_check;
ALTER TABLE memories DROP CONSTRAINT IF EXISTS memories_status_check;

-- Widen status to the length of team_statuses.slug. varchar(20) -> varchar(50)
-- only raises the length limit, which Postgres applies as a catalog change: no
-- table rewrite, no index rebuild, and no row trigger fires, so existing rows
-- (and their updated_at) are untouched.
ALTER TABLE prompts ALTER COLUMN status TYPE varchar(50);
ALTER TABLE artifacts ALTER COLUMN status TYPE varchar(50);
ALTER TABLE blueprints ALTER COLUMN status TYPE varchar(50);
ALTER TABLE memories ALTER COLUMN status TYPE varchar(50);

-- ===========================================================================
-- 026_memories_preserve_updated_at
-- ===========================================================================

-- Let one transaction change memories without moving `updated_at` (issue
-- #1265, epic #1263).
--
-- Deleting a custom status moves every resource that carried it to the
-- resource type's default status. That is a change to the team's settings, not
-- an edit of the resource: `updated_at` feeds search recency ranking, the
-- freshness rules and the "last updated" a person reads, so it must stay put.
--
-- prompts, artifacts and blueprints have no trigger, so an UPDATE that does not
-- name `updated_at` leaves it alone. `memories` is the one resource table with
-- a BEFORE UPDATE trigger (`update_memories_updated_at`), and its function
-- stamps CURRENT_TIMESTAMP on every row the trigger fires for. The 025 down
-- migration works around that by disabling the trigger with DDL, which a
-- request cannot do: it needs table ownership and locks out every other writer.
--
-- So the trigger gains one more condition in its WHEN clause: it is skipped
-- while the transaction-local setting `vibexp.preserve_updated_at` is 'on'.
-- The status delete sets it with set_config(..., is_local => true) inside its
-- own transaction, so it ends with that transaction and no other session ever
-- sees it. current_setting(..., true) returns NULL when the setting was never
-- defined, which is DISTINCT FROM 'on', so every existing write path behaves
-- exactly as before.
--
-- The four last-accessed conditions are unchanged from 013 (#730).

DROP TRIGGER IF EXISTS update_memories_updated_at ON memories;

CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW
    WHEN (
        OLD.last_accessed_web_at IS NOT DISTINCT FROM NEW.last_accessed_web_at
    AND OLD.last_accessed_cli_at IS NOT DISTINCT FROM NEW.last_accessed_cli_at
    AND OLD.last_accessed_mcp_at IS NOT DISTINCT FROM NEW.last_accessed_mcp_at
    AND OLD.last_accessed_api_at IS NOT DISTINCT FROM NEW.last_accessed_api_at
    AND current_setting('vibexp.preserve_updated_at', true) IS DISTINCT FROM 'on'
    )
    EXECUTE FUNCTION public.update_updated_at_column();

COMMENT ON TRIGGER update_memories_updated_at ON memories IS
    'Maintains updated_at on edits. Deliberately skipped when an update touches only the last_accessed_* columns (#730), so a read never looks like an edit, and while the transaction-local setting vibexp.preserve_updated_at is ''on'' (#1265), so deleting a custom status does not look like one either.';

-- ===========================================================================
-- 027_resource_locks
-- ===========================================================================

-- ===========================================================================
-- Resource locks -- storage foundation (issue #1286, epic #1285).
--
-- A prompt, blueprint or artifact can be locked by a team member so nobody
-- edits it by accident. This migration adds the two things that need storing:
-- the lock itself, on the resource row, and an append-only history of lock and
-- unlock events. Nothing reads or writes either yet; the API (#1288) and the
-- write guards (#1289, #1290) build on it.
--
-- The lock lives ON THE ROW rather than in a side table so that a list query
-- needs no join to know whether a row is locked, and so that the lock goes
-- away with the resource without a cascade to maintain.
--
-- "Locked" is ONE predicate everywhere: locked_by IS NOT NULL. locked_by is a
-- foreign key to users with ON DELETE SET NULL, so deleting the locker's
-- account unlocks every resource they held, by construction and with no
-- application code. After that SET NULL the row still carries the old
-- locked_at and lock_comment; they are stale and every reader must ignore
-- them, which is why the CHECK below is one-directional and why no reader may
-- test locked_at or lock_comment to decide whether a row is locked.
--
-- Three nullable columns with no default are a catalog-only change: no table
-- rewrite, and no row is updated, so updated_at is untouched.
-- ===========================================================================

ALTER TABLE prompts
    ADD COLUMN locked_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN locked_at    timestamptz,
    ADD COLUMN lock_comment text,
    ADD CONSTRAINT prompts_lock_state_check
        CHECK (locked_by IS NULL OR (locked_at IS NOT NULL AND lock_comment IS NOT NULL));

ALTER TABLE blueprints
    ADD COLUMN locked_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN locked_at    timestamptz,
    ADD COLUMN lock_comment text,
    ADD CONSTRAINT blueprints_lock_state_check
        CHECK (locked_by IS NULL OR (locked_at IS NOT NULL AND lock_comment IS NOT NULL));

ALTER TABLE artifacts
    ADD COLUMN locked_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN locked_at    timestamptz,
    ADD COLUMN lock_comment text,
    ADD CONSTRAINT artifacts_lock_state_check
        CHECK (locked_by IS NULL OR (locked_at IS NOT NULL AND lock_comment IS NOT NULL));

COMMENT ON COLUMN prompts.locked_by IS
    'The member holding the lock; NULL means the prompt is unlocked. This column alone decides lock state. SET NULL on user delete, so deleting the locker unlocks the prompt.';
COMMENT ON COLUMN prompts.locked_at IS
    'When the lock was taken. Meaningful only while locked_by is set: it is left behind, stale, when the locker''s account is deleted.';
COMMENT ON COLUMN prompts.lock_comment IS
    'Why the prompt was locked, as given by the locker. Meaningful only while locked_by is set, like locked_at.';

COMMENT ON COLUMN blueprints.locked_by IS
    'The member holding the lock; NULL means the blueprint is unlocked. This column alone decides lock state. SET NULL on user delete, so deleting the locker unlocks the blueprint.';
COMMENT ON COLUMN blueprints.locked_at IS
    'When the lock was taken. Meaningful only while locked_by is set: it is left behind, stale, when the locker''s account is deleted.';
COMMENT ON COLUMN blueprints.lock_comment IS
    'Why the blueprint was locked, as given by the locker. Meaningful only while locked_by is set, like locked_at.';

COMMENT ON COLUMN artifacts.locked_by IS
    'The member holding the lock; NULL means the artifact is unlocked. This column alone decides lock state. SET NULL on user delete, so deleting the locker unlocks the artifact.';
COMMENT ON COLUMN artifacts.locked_at IS
    'When the lock was taken. Meaningful only while locked_by is set: it is left behind, stale, when the locker''s account is deleted.';
COMMENT ON COLUMN artifacts.lock_comment IS
    'Why the artifact was locked, as given by the locker. Meaningful only while locked_by is set, like locked_at.';

-- The `locked` list filter asks for a team's locked rows. Locked rows are
-- expected to be a small minority, so a partial index holds only those and
-- costs nothing on the unlocked majority. (The opposite filter, unlocked
-- rows, is nearly the whole table and is served by the existing team indexes.)
CREATE INDEX idx_prompts_locked ON prompts (team_id) WHERE locked_by IS NOT NULL;
CREATE INDEX idx_blueprints_locked ON blueprints (team_id) WHERE locked_by IS NOT NULL;
CREATE INDEX idx_artifacts_locked ON artifacts (team_id) WHERE locked_by IS NOT NULL;

CREATE TABLE resource_lock_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id   uuid NOT NULL,
    action        text NOT NULL,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    comment       text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE resource_lock_events IS
    'Append-only history of resource lock and unlock events (epic #1285). Rows are never updated or deleted except by team cascade.';
COMMENT ON COLUMN resource_lock_events.team_id IS
    'The team that owns the resource. Cascades, because an entry about a team that no longer exists is unreadable.';
COMMENT ON COLUMN resource_lock_events.resource_type IS
    'Singular resource type: prompt, blueprint or artifact. Validated in the repository, not by a CHECK (same convention as team_settings_audit.surface).';
COMMENT ON COLUMN resource_lock_events.resource_id IS
    'The locked or unlocked resource. No foreign key: it is polymorphic across the three resource tables, and the history deliberately outlives the resource (as team_settings_audit outlives its subject).';
COMMENT ON COLUMN resource_lock_events.action IS
    'What happened: lock or unlock. Validated in the repository, not by a CHECK.';
COMMENT ON COLUMN resource_lock_events.actor_user_id IS
    'Who locked or unlocked. SET NULL on user deletion rather than CASCADE: the entry must outlive the actor.';
COMMENT ON COLUMN resource_lock_events.comment IS
    'The lock comment for a lock event; NULL for an unlock.';

-- The history read is "this resource's events, newest first". `id DESC` is a
-- deterministic tiebreaker: now() is transaction-start time, so two events
-- written in one transaction carry the same created_at.
CREATE INDEX idx_resource_lock_events_resource
    ON resource_lock_events (team_id, resource_type, resource_id, created_at DESC, id DESC);
