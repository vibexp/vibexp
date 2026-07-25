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
