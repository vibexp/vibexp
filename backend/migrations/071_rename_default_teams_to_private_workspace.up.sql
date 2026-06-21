-- Rename existing "My Team" teams to "Private Workspace"
UPDATE teams
SET
    name = 'Private Workspace',
    slug = 'private-workspace',
    description = 'Your personal workspace for individual projects and resources',
    updated_at = CURRENT_TIMESTAMP
WHERE
    name = 'My Team'
    AND slug = 'my-team';

-- Note: This migration is idempotent and safe to run multiple times
