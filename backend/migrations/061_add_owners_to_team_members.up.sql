-- Migrate existing team owners to team_members table
-- This ensures backward compatibility and preserves existing ownership relationships
INSERT INTO team_members (team_id, user_id, role, created_at, updated_at)
SELECT id, owner_id, 'owner', created_at, NOW()
FROM teams
ON CONFLICT (team_id, user_id) DO NOTHING;
