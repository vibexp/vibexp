-- Add unique constraint to prevent duplicate projects with same git_url in a team
-- This index also serves for query performance (unique indexes are used for lookups)
-- This prevents race conditions in concurrent import operations
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_team_id_git_url_unique
ON projects(team_id, git_url)
WHERE git_url IS NOT NULL AND git_url != '';

-- Add comment to document the constraint purpose
COMMENT ON INDEX idx_projects_team_id_git_url_unique IS 'Ensures a GitHub repository can only be imported once per team, preventing duplicate projects. Also serves as index for GetProjectByGitURL queries.';
