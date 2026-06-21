-- Rollback: Revert team_subscriptions foreign key constraint
-- Change back from ON DELETE RESTRICT to ON DELETE CASCADE

-- Drop the RESTRICT constraint
ALTER TABLE team_subscriptions
    DROP CONSTRAINT IF EXISTS team_subscriptions_team_id_fkey;

-- Re-add the original CASCADE constraint
ALTER TABLE team_subscriptions
    ADD CONSTRAINT team_subscriptions_team_id_fkey
    FOREIGN KEY (team_id)
    REFERENCES teams(id)
    ON DELETE CASCADE;
