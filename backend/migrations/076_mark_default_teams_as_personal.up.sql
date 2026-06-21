-- Mark all default teams (referenced in users.default_team_id) as personal workspaces
-- Only mark teams as personal if they're default teams AND the owner matches the user
-- This prevents incorrectly marking collaborative teams that were manually set as default
UPDATE teams
SET is_personal = true
WHERE id IN (
    SELECT t.id
    FROM teams t
    INNER JOIN users u ON t.id = u.default_team_id
    WHERE u.default_team_id IS NOT NULL
      AND t.owner_id = u.id
);

-- Log the number of teams marked as personal
DO $$
DECLARE
    marked_count INTEGER;
BEGIN
    GET DIAGNOSTICS marked_count = ROW_COUNT;
    RAISE NOTICE 'Marked % teams as personal workspaces', marked_count;
END $$;
