-- Add is_personal flag to distinguish personal workspaces from team workspaces
ALTER TABLE teams ADD COLUMN is_personal BOOLEAN DEFAULT false NOT NULL;

-- Create index for quick lookups
CREATE INDEX idx_teams_is_personal ON teams(is_personal);

COMMENT ON COLUMN teams.is_personal IS 'True for default personal workspace (cannot invite members), false for team workspaces';
