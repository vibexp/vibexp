-- Revert unique constraints to original state (without team_id)
-- Issue #610 rollback

-- Prompts: Revert to original constraint
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_slug_user_id_team_id_key;
ALTER TABLE prompts ADD CONSTRAINT prompts_slug_user_id_key UNIQUE (slug, user_id);

-- Artifacts: Revert to original constraint
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_slug_user_id_team_id_key;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_slug_user_id_key UNIQUE (slug, user_id);

-- Spec Library: Revert to original constraint
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS spec_library_slug_user_id_team_id_key;
ALTER TABLE spec_library ADD CONSTRAINT spec_library_slug_user_id_key UNIQUE (slug, user_id);
