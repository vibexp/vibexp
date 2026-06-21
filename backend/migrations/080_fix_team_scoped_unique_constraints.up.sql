-- Fix team-scoped unique constraints
-- Issue #692: Remove user_id from unique constraints to allow users to create
-- resources with the same slug/name in different teams they're members of
--
-- Problem: Current constraints enforce uniqueness per (slug, user_id, team_id),
-- which prevents a user from creating resources with the same slug in different teams.
--
-- Solution: Enforce uniqueness per (slug, team_id) only, allowing team-level isolation
-- while permitting the same user to use the same slug across different teams.

BEGIN;

-- Pre-flight validation: Check for duplicates that would violate new constraints
-- This ensures data integrity before we modify constraints
DO $$
DECLARE
    duplicate_count INT;
BEGIN
    -- Validate prompts: Check for duplicates on (slug, team_id)
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, team_id, COUNT(*) as cnt
        FROM prompts
        GROUP BY slug, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot update constraint: % duplicate (slug, team_id) combinations exist in prompts. Run cleanup query first.', duplicate_count;
    END IF;

    -- Validate artifacts: Check for duplicates on (slug, team_id)
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, team_id, COUNT(*) as cnt
        FROM artifacts
        GROUP BY slug, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot update constraint: % duplicate (slug, team_id) combinations exist in artifacts. Run cleanup query first.', duplicate_count;
    END IF;

    -- Validate spec_library: Check for duplicates on (slug, team_id)
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, team_id, COUNT(*) as cnt
        FROM spec_library
        GROUP BY slug, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot update constraint: % duplicate (slug, team_id) combinations exist in spec_library. Run cleanup query first.', duplicate_count;
    END IF;

    -- Validate projects: Check for duplicates on (slug, team_id)
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT slug, team_id, COUNT(*) as cnt
        FROM projects
        GROUP BY slug, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot update constraint: % duplicate (slug, team_id) combinations exist in projects. Run cleanup query first.', duplicate_count;
    END IF;

    -- Validate agents: Check for duplicates on (name, team_id)
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT name, team_id, COUNT(*) as cnt
        FROM agents
        GROUP BY name, team_id
        HAVING COUNT(*) > 1
    ) duplicates;
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION 'Cannot update constraint: % duplicate (name, team_id) combinations exist in agents. Run cleanup query first.', duplicate_count;
    END IF;
END $$;

-- Fix prompts table
-- Drop incorrect constraint that includes user_id
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_slug_user_id_team_id_key;
-- Add correct team-scoped constraint
ALTER TABLE prompts ADD CONSTRAINT prompts_slug_team_id_key UNIQUE (slug, team_id);

-- Fix artifacts table
-- Drop incorrect constraint that includes user_id
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_slug_user_id_team_id_key;
-- Add correct team-scoped constraint
ALTER TABLE artifacts ADD CONSTRAINT artifacts_slug_team_id_key UNIQUE (slug, team_id);

-- Fix spec_library table
-- Drop incorrect constraint that includes user_id
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS spec_library_slug_user_id_team_id_key;
-- Add correct team-scoped constraint
ALTER TABLE spec_library ADD CONSTRAINT spec_library_slug_team_id_key UNIQUE (slug, team_id);

-- Fix projects table
-- Drop old constraint from migration 062 that only considers user_id
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_user_slug_unique;
-- Add correct team-scoped constraint
ALTER TABLE projects ADD CONSTRAINT projects_slug_team_id_key UNIQUE (slug, team_id);

-- Fix agents table
-- Drop old constraint from migration 014 that only considers user_id
ALTER TABLE agents DROP CONSTRAINT IF EXISTS unique_agent_name_per_user;
-- Add correct team-scoped constraint
ALTER TABLE agents ADD CONSTRAINT agents_name_team_id_key UNIQUE (name, team_id);

COMMIT;

-- Validation queries for manual testing:
--
-- Check for any remaining duplicates in prompts:
-- SELECT slug, team_id, COUNT(*) as cnt FROM prompts GROUP BY slug, team_id HAVING COUNT(*) > 1;
--
-- Check for any remaining duplicates in artifacts:
-- SELECT slug, team_id, COUNT(*) as cnt FROM artifacts GROUP BY slug, team_id HAVING COUNT(*) > 1;
--
-- Check for any remaining duplicates in spec_library:
-- SELECT slug, team_id, COUNT(*) as cnt FROM spec_library GROUP BY slug, team_id HAVING COUNT(*) > 1;
--
-- Check for any remaining duplicates in projects:
-- SELECT slug, team_id, COUNT(*) as cnt FROM projects GROUP BY slug, team_id HAVING COUNT(*) > 1;
--
-- Check for any remaining duplicates in agents:
-- SELECT name, team_id, COUNT(*) as cnt FROM agents GROUP BY name, team_id HAVING COUNT(*) > 1;
--
-- Test scenario:
-- 1. User Alice creates prompt "test-prompt" in Team A → Should succeed
-- 2. User Alice creates prompt "test-prompt" in Team B → Should succeed (different team)
-- 3. User Bob (member of Team A) creates prompt "test-prompt" in Team A → Should fail (duplicate within team)
