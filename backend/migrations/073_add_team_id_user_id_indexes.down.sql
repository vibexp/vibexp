-- Remove indexes on (team_id, user_id) added in migration 073
-- Note: CONCURRENTLY removed to allow operation inside transaction

DROP INDEX IF EXISTS idx_spec_library_team_id_user_id;
DROP INDEX IF EXISTS idx_memories_team_id_user_id;
DROP INDEX IF EXISTS idx_artifacts_team_id_user_id;
DROP INDEX IF EXISTS idx_prompts_team_id_user_id;
DROP INDEX IF EXISTS idx_agents_team_id_user_id;
