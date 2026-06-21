-- Add indexes on (team_id, user_id) for query performance optimization
-- Issue #610: Team-scoped list operations need efficient filtering
--
-- Note: CONCURRENTLY keyword removed to allow migration to run inside transaction.
-- Indexes will still be created successfully, just with brief table locks during creation.
-- This is acceptable during migrations as they typically run during deployment windows.

-- Agents: Index for listing agents by team and user
CREATE INDEX IF NOT EXISTS idx_agents_team_id_user_id
ON agents(team_id, user_id)
WHERE team_id IS NOT NULL;

-- Prompts: Index for listing prompts by team and user
CREATE INDEX IF NOT EXISTS idx_prompts_team_id_user_id
ON prompts(team_id, user_id)
WHERE team_id IS NOT NULL;

-- Artifacts: Index for listing artifacts by team and user
CREATE INDEX IF NOT EXISTS idx_artifacts_team_id_user_id
ON artifacts(team_id, user_id)
WHERE team_id IS NOT NULL;

-- Memories: Index for listing memories by team and user
CREATE INDEX IF NOT EXISTS idx_memories_team_id_user_id
ON memories(team_id, user_id)
WHERE team_id IS NOT NULL;

-- Spec Library: Index for listing spec libraries by team and user
CREATE INDEX IF NOT EXISTS idx_spec_library_team_id_user_id
ON spec_library(team_id, user_id)
WHERE team_id IS NOT NULL;

-- WHERE clause creates partial index only for non-NULL team_id values
-- This optimizes query performance while keeping index size smaller
