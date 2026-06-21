-- Migration: Update team_subscriptions foreign key constraint
-- Change from ON DELETE CASCADE to ON DELETE RESTRICT to prevent accidental team deletion
-- when an active subscription exists. This provides database-level protection against
-- race conditions where a subscription could be created between checking and deleting.

-- Drop the existing foreign key constraint
ALTER TABLE team_subscriptions
    DROP CONSTRAINT IF EXISTS team_subscriptions_team_id_fkey;

-- Re-add the foreign key constraint with ON DELETE RESTRICT
ALTER TABLE team_subscriptions
    ADD CONSTRAINT team_subscriptions_team_id_fkey
    FOREIGN KEY (team_id)
    REFERENCES teams(id)
    ON DELETE RESTRICT;

COMMENT ON CONSTRAINT team_subscriptions_team_id_fkey ON team_subscriptions IS
    'Prevents team deletion when subscriptions exist, forcing proper subscription cleanup first';
