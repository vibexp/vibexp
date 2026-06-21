-- Add team_id to projects table following the pattern from migration 066
-- All users already have default_team_id from migration 058

-- Add team_id column to projects table
ALTER TABLE projects ADD COLUMN team_id UUID;

-- Populate team_id from users.default_team_id for existing projects
UPDATE projects p SET team_id = u.default_team_id FROM users u WHERE p.user_id::uuid = u.id;

-- Make team_id NOT NULL after population
ALTER TABLE projects ALTER COLUMN team_id SET NOT NULL;

-- Add foreign key constraint with CASCADE delete
ALTER TABLE projects ADD CONSTRAINT fk_projects_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

-- Add index for team_id
CREATE INDEX idx_projects_team_id ON projects(team_id);
