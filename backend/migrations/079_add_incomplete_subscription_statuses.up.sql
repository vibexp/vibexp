-- Migration: Add incomplete and incomplete_expired statuses to team_subscriptions
-- Reason: Stripe sends 'incomplete' status before first payment completes
-- Issue: #638 - webhook failures due to constraint violation

-- Drop the existing constraint
ALTER TABLE team_subscriptions
    DROP CONSTRAINT team_subscriptions_status_valid;

-- Add new constraint with incomplete statuses
ALTER TABLE team_subscriptions
    ADD CONSTRAINT team_subscriptions_status_valid
    CHECK (status IN ('incomplete', 'incomplete_expired', 'trialing', 'active', 'past_due', 'canceled', 'unpaid'));

COMMENT ON CONSTRAINT team_subscriptions_status_valid ON team_subscriptions IS
    'Valid Stripe subscription statuses: incomplete, incomplete_expired, trialing, active, past_due, canceled, unpaid';
