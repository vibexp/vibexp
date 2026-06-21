-- Add team_id to unique constraints for team-scoped resource isolation
-- Issue #610: Only the selected team's resources should be retrieved, create and update need team id

-- This migration is wrapped in a transaction to ensure atomicity
-- If any step fails, the entire migration is rolled back
BEGIN;

-- Pre-flight validation: Check for NULL team_id values and duplicates
-- This must pass BEFORE we drop any constraints to ensure data integrity
DO $$
DECLARE
    duplicate_count INT;
BEGIN
    -- Check prompts table for NULL team_id
    IF EXISTS (SELECT 1 FROM prompts WHERE team_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot add team_id to unique constraint: NULL values exist in prompts.team_id';
    END IF;

    -- Check prompts table for duplicates that would violate new constraint
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, user_id, team_id, COUNT(*) as cnt
        FROM prompts
        GROUP BY slug, user_id, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot add unique constraint: % duplicate (slug, user_id, team_id) combinations exist in prompts', duplicate_count;
    END IF;

    -- Check artifacts table for NULL team_id
    IF EXISTS (SELECT 1 FROM artifacts WHERE team_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot add team_id to unique constraint: NULL values exist in artifacts.team_id';
    END IF;

    -- Check artifacts table for duplicates
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, user_id, team_id, COUNT(*) as cnt
        FROM artifacts
        GROUP BY slug, user_id, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot add unique constraint: % duplicate (slug, user_id, team_id) combinations exist in artifacts', duplicate_count;
    END IF;

    -- Check spec_library table for NULL team_id
    IF EXISTS (SELECT 1 FROM spec_library WHERE team_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot add team_id to unique constraint: NULL values exist in spec_library.team_id';
    END IF;

    -- Check spec_library table for duplicates
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, user_id, team_id, COUNT(*) as cnt
        FROM spec_library
        GROUP BY slug, user_id, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot add unique constraint: % duplicate (slug, user_id, team_id) combinations exist in spec_library', duplicate_count;
    END IF;

    -- Check memories table for NULL team_id
    IF EXISTS (SELECT 1 FROM memories WHERE team_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot add team_id to unique constraint: NULL values exist in memories.team_id';
    END IF;

    -- Check agents table for NULL team_id
    IF EXISTS (SELECT 1 FROM agents WHERE team_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'Cannot add team_id to unique constraint: NULL values exist in agents.team_id';
    END IF;
END $$;

-- Prompts: Update unique constraint to include team_id
-- Drop old constraint first, then add new one
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_slug_user_id_key;
ALTER TABLE prompts ADD CONSTRAINT prompts_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id);

-- Artifacts: Update unique constraint to include team_id
-- Note: artifacts table may have different existing constraints depending on migration history
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_project_name_slug_user_id_key;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_slug_user_id_key;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id);

-- Spec Library: Update unique constraint to include team_id
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS spec_library_slug_user_id_key;
ALTER TABLE spec_library ADD CONSTRAINT spec_library_slug_user_id_team_id_key UNIQUE (slug, user_id, team_id);

-- Commit the transaction
-- If we reach this point, all constraints were successfully added
COMMIT;

-- Note: Memories and Agents tables may not have slug-based unique constraints
-- Verify schema and adjust constraints accordingly as needed
-- For now, we're focusing on resources with slug fields
