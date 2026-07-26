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
