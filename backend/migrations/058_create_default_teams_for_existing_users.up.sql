-- Create default team for each existing user
INSERT INTO teams (owner_id, name, slug, description, created_at, updated_at)
SELECT
    id as owner_id,
    'My Team' as name,
    'my-team' as slug,
    'Default team' as description,
    CURRENT_TIMESTAMP as created_at,
    CURRENT_TIMESTAMP as updated_at
FROM users
WHERE NOT EXISTS (
    SELECT 1 FROM teams WHERE teams.owner_id = users.id
);

-- Update users to reference their default team
UPDATE users
SET default_team_id = teams.id
FROM teams
WHERE teams.owner_id = users.id
AND users.default_team_id IS NULL;
