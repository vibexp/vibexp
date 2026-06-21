-- Rollback migration: Remove team_id from projects table

-- Drop index
DROP INDEX IF EXISTS idx_projects_team_id;

-- Drop foreign key constraint
ALTER TABLE projects DROP CONSTRAINT IF EXISTS fk_projects_team;

-- Drop team_id column
ALTER TABLE projects DROP COLUMN IF EXISTS team_id;
