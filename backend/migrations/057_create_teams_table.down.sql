-- Remove default_team_id from users
ALTER TABLE users DROP COLUMN IF EXISTS default_team_id;

-- Drop teams table
DROP TABLE IF EXISTS teams;
