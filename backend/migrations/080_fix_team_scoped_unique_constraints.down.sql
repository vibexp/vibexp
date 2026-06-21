-- Rollback: Restore original unique constraints that include user_id
-- This reverts the changes from migration 080 and restores the behavior
-- where resources are unique per (slug/name, user_id, team_id)

BEGIN;

-- Restore prompts constraint with user_id
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_slug_team_id_key;
ALTER TABLE prompts ADD CONSTRAINT prompts_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id);

-- Restore artifacts constraint with user_id
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_slug_team_id_key;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id);

-- Restore spec_library constraint with user_id
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS spec_library_slug_team_id_key;
ALTER TABLE spec_library ADD CONSTRAINT spec_library_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id);

-- Restore projects constraint with user_id (original from migration 062)
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_slug_team_id_key;
ALTER TABLE projects ADD CONSTRAINT projects_user_slug_unique UNIQUE (user_id, slug);

-- Restore agents constraint with user_id (original from migration 014)
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_name_team_id_key;
ALTER TABLE agents ADD CONSTRAINT unique_agent_name_per_user UNIQUE (user_id, name);

COMMIT;
