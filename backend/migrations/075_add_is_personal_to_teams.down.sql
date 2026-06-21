DROP INDEX IF EXISTS idx_teams_is_personal;
ALTER TABLE teams DROP COLUMN IF EXISTS is_personal;
