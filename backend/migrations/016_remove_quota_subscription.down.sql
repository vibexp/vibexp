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
