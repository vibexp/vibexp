-- Reverse of 011_consolidated.up.sql: each merged migration's down script
-- verbatim, in reverse order (018 first, 011 last).
--
-- ⚠️ LOSSY, STRUCTURE-ONLY where the up migration deleted data: the hook
-- payload rows, ai_tools grants and Claude Code activities (015), the
-- team_subscriptions rows and per-user billing values (016), the device_tokens
-- rows (017), the stripped web_push JSONB keys (018), the deleted
-- github_installations rows (013) and each user's suspension state (011) are
-- NOT recoverable here. Restore from a backup if any of it mattered.

-- ===========================================================================
-- reverse 018_remove_web_push_preferences_keys
-- ===========================================================================

-- Reverse of 018_remove_web_push_preferences_keys.up.sql.
--
-- ⚠️ NO-OP BY DESIGN. The up migration deletes `web_push` keys from a JSONB
-- document without recording their prior values, so they are NOT recoverable
-- here — the same structure-only caveat as migrations 015–017, one step
-- further: there is not even structure to restore. Restore from a backup if
-- the values mattered (they did not: nothing has read web_push since #688).
--
-- Rolling back does NOT bring web push back: the delivery channel, the device
-- token endpoints, the Firebase dependencies and the API contract field were
-- all removed in the application. A down-then-up cycle therefore stays at the
-- post-018 state, which is correct — there is no pre-018 shape worth
-- reconstructing.

SELECT 1;

-- ===========================================================================
-- reverse 017_remove_firebase_web_push
-- ===========================================================================

-- Reverse of 017_remove_firebase_web_push.up.sql.
--
-- ⚠️ THIS RESTORES STRUCTURE ONLY. The device_tokens rows are DROPPED by the up
-- migration and are NOT recoverable here. Rolling back gives you the empty
-- table, not the registrations that were there before. Restore from a backup if
-- the data mattered. (Same wording and caveat as migrations 015 and 016.)
--
-- Note that rolling this migration back does NOT bring web push back: the
-- client code, the API endpoints and the delivery channel were removed from the
-- application in the same change. The table is recreated purely so a
-- down-then-up cycle lands on the original schema.
--
-- The DDL below is copied verbatim from 001_baseline.up.sql rather than
-- reconstructed.

CREATE TABLE IF NOT EXISTS public.device_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    platform character varying(16) NOT NULL,
    user_agent text,
    last_used_at timestamp with time zone DEFAULT now(),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.device_tokens
    DROP CONSTRAINT IF EXISTS device_tokens_pkey;
ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX IF NOT EXISTS device_tokens_token_idx ON public.device_tokens USING btree (token);
CREATE INDEX IF NOT EXISTS device_tokens_user_id_idx ON public.device_tokens USING btree (user_id);

ALTER TABLE ONLY public.device_tokens
    DROP CONSTRAINT IF EXISTS device_tokens_user_id_fkey;
ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- ===========================================================================
-- reverse 016_remove_quota_subscription
-- ===========================================================================

-- Reverse of 016_remove_quota_subscription.up.sql.
--
-- ⚠️ THIS RESTORES STRUCTURE ONLY. The team_subscriptions rows and the per-user
-- billing values are DROPPED by the up migration and are NOT recoverable here.
-- Rolling back gives you the empty table and the columns at their defaults
-- ('basic' for subscription_status and subscription_plan, NULL for the rest) —
-- not the data that was there before. Restore from a backup if the data
-- mattered. (Same wording and caveat as migration 015, epic #610.)
--
-- The DDL below is copied verbatim from 001_baseline.up.sql rather than
-- reconstructed, so a down-then-up cycle lands on the original schema.

ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS stripe_customer_id character varying(255),
    ADD COLUMN IF NOT EXISTS subscription_status character varying(50) DEFAULT 'basic'::character varying,
    ADD COLUMN IF NOT EXISTS trial_ends_at timestamp with time zone,
    ADD COLUMN IF NOT EXISTS subscription_plan character varying(50) DEFAULT 'basic'::character varying,
    ADD COLUMN IF NOT EXISTS subscription_canceled_at timestamp with time zone;

COMMENT ON COLUMN public.users.subscription_canceled_at IS 'Timestamp when subscription cancellation was scheduled (Stripe cancel_at_period_end). NULL means subscription will auto-renew.';

CREATE INDEX IF NOT EXISTS idx_users_stripe_customer_id ON public.users USING btree (stripe_customer_id);
CREATE INDEX IF NOT EXISTS idx_users_subscription_canceled_at ON public.users USING btree (subscription_canceled_at);

CREATE TABLE IF NOT EXISTS public.team_subscriptions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    stripe_subscription_id character varying(255) NOT NULL,
    stripe_customer_id character varying(255) NOT NULL,
    tier character varying(50) NOT NULL,
    seat_count integer NOT NULL,
    status character varying(50) NOT NULL,
    billing_interval character varying(20) NOT NULL,
    current_period_start timestamp with time zone NOT NULL,
    current_period_end timestamp with time zone NOT NULL,
    trial_end timestamp with time zone,
    canceled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT team_subscriptions_billing_interval_valid CHECK (((billing_interval)::text = ANY ((ARRAY['month'::character varying, 'year'::character varying])::text[]))),
    CONSTRAINT team_subscriptions_seat_count_positive CHECK ((seat_count > 0)),
    CONSTRAINT team_subscriptions_status_valid CHECK (((status)::text = ANY ((ARRAY['incomplete'::character varying, 'incomplete_expired'::character varying, 'trialing'::character varying, 'active'::character varying, 'past_due'::character varying, 'canceled'::character varying, 'unpaid'::character varying])::text[]))),
    CONSTRAINT team_subscriptions_tier_valid CHECK (((tier)::text = ANY ((ARRAY['starter'::character varying, 'professional'::character varying, 'enterprise'::character varying])::text[])))
);

COMMENT ON TABLE public.team_subscriptions IS 'Stores team subscription data from Stripe for per-seat pricing';
COMMENT ON COLUMN public.team_subscriptions.tier IS 'Pricing tier: starter, professional, enterprise';
COMMENT ON COLUMN public.team_subscriptions.seat_count IS 'Number of paid seats (licensed members)';
COMMENT ON COLUMN public.team_subscriptions.status IS 'Stripe subscription status: trialing, active, past_due, canceled, unpaid';
COMMENT ON CONSTRAINT team_subscriptions_status_valid ON public.team_subscriptions IS 'Valid Stripe subscription statuses: incomplete, incomplete_expired, trialing, active, past_due, canceled, unpaid';

ALTER TABLE ONLY public.team_subscriptions
    DROP CONSTRAINT IF EXISTS team_subscriptions_pkey;
ALTER TABLE ONLY public.team_subscriptions
    ADD CONSTRAINT team_subscriptions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_subscriptions
    DROP CONSTRAINT IF EXISTS team_subscriptions_stripe_subscription_id_key;
ALTER TABLE ONLY public.team_subscriptions
    ADD CONSTRAINT team_subscriptions_stripe_subscription_id_key UNIQUE (stripe_subscription_id);

CREATE INDEX IF NOT EXISTS idx_team_subscriptions_status ON public.team_subscriptions USING btree (status);
CREATE INDEX IF NOT EXISTS idx_team_subscriptions_stripe_customer_id ON public.team_subscriptions USING btree (stripe_customer_id);
CREATE INDEX IF NOT EXISTS idx_team_subscriptions_stripe_subscription_id ON public.team_subscriptions USING btree (stripe_subscription_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_subscriptions_team_id ON public.team_subscriptions USING btree (team_id);
CREATE INDEX IF NOT EXISTS idx_team_subscriptions_tier ON public.team_subscriptions USING btree (tier);

DROP TRIGGER IF EXISTS update_team_subscriptions_updated_at ON public.team_subscriptions;
CREATE TRIGGER update_team_subscriptions_updated_at BEFORE UPDATE ON public.team_subscriptions FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

ALTER TABLE ONLY public.team_subscriptions
    DROP CONSTRAINT IF EXISTS team_subscriptions_team_id_fkey;
ALTER TABLE ONLY public.team_subscriptions
    ADD CONSTRAINT team_subscriptions_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;

COMMENT ON CONSTRAINT team_subscriptions_team_id_fkey ON public.team_subscriptions IS 'Prevents team deletion when subscriptions exist, forcing proper subscription cleanup first';

-- ===========================================================================
-- reverse 015_remove_ai_tool_hooks
-- ===========================================================================

-- Reverse 015_remove_ai_tool_hooks (epic #610, issue #614).
--
-- IMPORTANT — THIS RESTORES STRUCTURE ONLY, NOT DATA.
-- The up migration deletes rows irrecoverably. Rolling back recreates the two payload
-- tables (empty), re-inserts the `ai_tools` catalog row, and restores the original
-- chk_usage_type. It CANNOT restore:
--   * hook payload rows in claude_code_hooks_payload / cursor_ide_hooks_payload;
--   * api_key_integration_permissions rows granting `ai_tools`;
--   * activities rows of type claude_code_session / claude_code_tool / claude_code_prompt;
--   * which api_keys previously had usage_type = 'ai_tools' (they were re-pointed to
--     'everything' and are indistinguishable from keys that were always 'everything').
-- Restore those from a backup if you need them.

-- 1. Restore the original chk_usage_type, which also permitted 'ai_tools'.
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;
ALTER TABLE api_keys
    ADD CONSTRAINT chk_usage_type CHECK (
        (usage_type)::text = ANY ((ARRAY[
            'ai_tools'::character varying,
            'cli'::character varying,
            'mcp'::character varying,
            'everything'::character varying
        ])::text[])
    );

-- 2. Re-insert the catalog row, with the id the baseline seeded, so any restored grant
--    rows referencing it line up again.
INSERT INTO public.api_key_integrations_catalog
    (id, integration_code, integration_name, description, is_active, created_at, updated_at)
VALUES (
    'a79a0d56-146d-486d-8a35-8375327bd6a4',
    'ai_tools',
    'AI Tools Integration',
    'Access for Claude Code, Cursor IDE, and other AI-powered development tools',
    true,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (integration_code) DO NOTHING;

-- 3. Recreate claude_code_hooks_payload with its sequence, indexes, trigger and FK.
CREATE TABLE IF NOT EXISTS public.claude_code_hooks_payload (
    id integer NOT NULL,
    session_id character varying(255) NOT NULL,
    transcript_path text,
    cwd text,
    hook_event_name character varying(100) NOT NULL,
    tool_name character varying(100),
    tool_input jsonb,
    tool_response jsonb,
    prompt text,
    message text,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    user_id character varying(255),
    team_id uuid NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.claude_code_hooks_payload_id_seq
    AS integer START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
ALTER SEQUENCE public.claude_code_hooks_payload_id_seq
    OWNED BY public.claude_code_hooks_payload.id;
ALTER TABLE ONLY public.claude_code_hooks_payload
    ALTER COLUMN id SET DEFAULT nextval('public.claude_code_hooks_payload_id_seq'::regclass);

ALTER TABLE ONLY public.claude_code_hooks_payload
    ADD CONSTRAINT claude_code_hooks_payload_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.claude_code_hooks_payload
    ADD CONSTRAINT fk_claude_code_hooks_payload_team FOREIGN KEY (team_id)
    REFERENCES public.teams(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_created_at
    ON public.claude_code_hooks_payload USING btree (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_event_name
    ON public.claude_code_hooks_payload USING btree (hook_event_name);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_payload_team_id
    ON public.claude_code_hooks_payload USING btree (team_id);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_session_id
    ON public.claude_code_hooks_payload USING btree (session_id);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_tool_name
    ON public.claude_code_hooks_payload USING btree (tool_name);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_user_created_at
    ON public.claude_code_hooks_payload USING btree (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_user_id
    ON public.claude_code_hooks_payload USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_user_session
    ON public.claude_code_hooks_payload USING btree (user_id, session_id);

CREATE TRIGGER update_claude_code_hooks_payload_updated_at
    BEFORE UPDATE ON public.claude_code_hooks_payload
    FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- 4. Recreate cursor_ide_hooks_payload with its sequence, indexes, trigger and FK.
CREATE TABLE IF NOT EXISTS public.cursor_ide_hooks_payload (
    id integer NOT NULL,
    user_id character varying(255),
    session_id character varying(255) NOT NULL,
    conversation_id character varying(255),
    generation_id character varying(255),
    hook_event_name character varying(100) NOT NULL,
    tool_name character varying(100),
    workspace_roots text[],
    configuration jsonb,
    reference jsonb,
    context jsonb,
    input jsonb,
    output jsonb,
    induced_failure jsonb,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    team_id uuid NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.cursor_ide_hooks_payload_id_seq
    AS integer START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
ALTER SEQUENCE public.cursor_ide_hooks_payload_id_seq
    OWNED BY public.cursor_ide_hooks_payload.id;
ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ALTER COLUMN id SET DEFAULT nextval('public.cursor_ide_hooks_payload_id_seq'::regclass);

ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ADD CONSTRAINT cursor_ide_hooks_payload_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ADD CONSTRAINT fk_cursor_ide_hooks_payload_team FOREIGN KEY (team_id)
    REFERENCES public.teams(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_conversation_id
    ON public.cursor_ide_hooks_payload USING btree (conversation_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_created_at
    ON public.cursor_ide_hooks_payload USING btree (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_event_name
    ON public.cursor_ide_hooks_payload USING btree (hook_event_name);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_generation_id
    ON public.cursor_ide_hooks_payload USING btree (generation_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_payload_team_id
    ON public.cursor_ide_hooks_payload USING btree (team_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_session_id
    ON public.cursor_ide_hooks_payload USING btree (session_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_tool_name
    ON public.cursor_ide_hooks_payload USING btree (tool_name);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_user_id
    ON public.cursor_ide_hooks_payload USING btree (user_id);

CREATE TRIGGER update_cursor_ide_hooks_payload_updated_at
    BEFORE UPDATE ON public.cursor_ide_hooks_payload
    FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- ===========================================================================
-- reverse 014_team_email_providers
-- ===========================================================================

-- Reverse 014_team_email_providers (#501). Dropping the table takes its primary
-- key, FK and unique constraint with it.
--
-- This is lossy by nature: every team with its own provider silently reverts to
-- the instance provider from config.yaml, and their configuration -- including
-- the encrypted credential and the delivery health history -- is gone. Mail
-- keeps flowing, but from the operator's address rather than the team's.

DROP TABLE IF EXISTS public.team_email_providers;

-- ===========================================================================
-- reverse 013_per_team_github_app
-- ===========================================================================

-- Reverse 013_per_team_github_app (#477).
--
-- LOSSY IN BOTH DIRECTIONS. The up migration deleted every github_installations
-- row and this cannot restore them. It also deletes the rows created since,
-- deliberately: they reference per-team Apps whose credentials this migration
-- drops along with github_app_configs, so they would be unusable in the
-- pre-#477 world exactly as the old rows were unusable in the new one. Deleting
-- them is also what makes the global installation_id UNIQUE re-addable -- two
-- teams installing their own App on the same org is legal after 013 and would
-- otherwise make this migration fail on the restored constraint.

DELETE FROM public.github_installations;

DROP INDEX IF EXISTS idx_github_installations_app_config_id;

ALTER TABLE public.github_installations
    DROP CONSTRAINT IF EXISTS unique_app_installation;

ALTER TABLE public.github_installations
    ADD CONSTRAINT github_installations_installation_id_key UNIQUE (installation_id);

ALTER TABLE public.github_installations
    DROP CONSTRAINT IF EXISTS fk_installations_app_config;

ALTER TABLE public.github_installations
    DROP COLUMN IF EXISTS app_config_id;

DROP TABLE IF EXISTS public.github_app_configs;

-- Restore the dead-but-present schema exactly as 001_baseline defined it, so a
-- down-then-up cycle lands back on the same shape.
CREATE TABLE public.github_installation_repositories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    repository_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    full_name character varying(500) NOT NULL,
    private boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT github_installation_repositories_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT unique_installation_repository UNIQUE (installation_id, repository_id);

CREATE INDEX idx_github_installation_repositories_installation_id
    ON public.github_installation_repositories USING btree (installation_id);

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT github_installation_repositories_installation_id_fkey
        FOREIGN KEY (installation_id) REFERENCES public.github_installations(id) ON DELETE CASCADE;

COMMENT ON TABLE public.webhook_events IS
    'Tracks processed Stripe webhook events to prevent duplicate processing (idempotency)';

-- ===========================================================================
-- reverse 012_team_search_settings
-- ===========================================================================

-- Reverse 012_team_search_settings (#488). Dropping the table takes its CHECK
-- constraints and primary-key index with it.
--
-- This is lossy by nature: every overriding team silently reverts to the
-- instance defaults from config.yaml, and their tuned profiles are gone.

DROP TABLE IF EXISTS team_search_settings;

-- ===========================================================================
-- reverse 011_user_status
-- ===========================================================================

-- Reverse 011_user_status (#454). Dropping the column takes its CHECK
-- constraint with it, but the index is dropped explicitly first so the down
-- migration is readable and order-independent.
--
-- This is lossy by nature: which accounts were suspended cannot be recovered,
-- and every user becomes active again on re-application of the up migration.

DROP INDEX IF EXISTS idx_users_status_not_active;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_status_check;

ALTER TABLE users
    DROP COLUMN IF EXISTS status;
