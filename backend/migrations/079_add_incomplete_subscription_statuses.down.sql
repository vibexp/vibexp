-- Rollback: Remove incomplete and incomplete_expired statuses
-- WARNING: This will fail if any subscriptions have these statuses

-- Drop the constraint
ALTER TABLE team_subscriptions
    DROP CONSTRAINT team_subscriptions_status_valid;

-- Restore original constraint (without incomplete statuses)
ALTER TABLE team_subscriptions
    ADD CONSTRAINT team_subscriptions_status_valid
    CHECK (status IN ('trialing', 'active', 'past_due', 'canceled', 'unpaid'));
