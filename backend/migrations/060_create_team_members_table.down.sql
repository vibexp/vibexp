-- Drop indexes first
DROP INDEX IF EXISTS idx_team_members_role;
DROP INDEX IF EXISTS idx_team_members_user_id;
DROP INDEX IF EXISTS idx_team_members_team_id;

-- Drop the team_members table
DROP TABLE IF EXISTS team_members;
