-- Rollback: Rename "Private Workspace" back to "My Team"
UPDATE teams
SET
    name = 'My Team',
    slug = 'my-team',
    description = 'Default team',
    updated_at = CURRENT_TIMESTAMP
WHERE
    name = 'Private Workspace'
    AND slug = 'private-workspace';
