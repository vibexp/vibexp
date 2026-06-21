-- Remove migrated owner entries from team_members
-- Note: This removes only the owner entries that were created by the migration
DELETE FROM team_members
WHERE role = 'owner'
AND (team_id, user_id) IN (
    SELECT id, owner_id FROM teams
);
