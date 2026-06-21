-- Rollback: Rename blueprints table back to spec_library
-- This migration reverses all table, index, and constraint renames

-- Step 1: Rename the table back
ALTER TABLE IF EXISTS blueprints RENAME TO spec_library;

-- Step 2: Rename all indexes back
ALTER INDEX IF EXISTS idx_blueprints_user_id RENAME TO idx_spec_library_user_id;
ALTER INDEX IF EXISTS idx_blueprints_project_name RENAME TO idx_spec_library_project_name;
ALTER INDEX IF EXISTS idx_blueprints_slug RENAME TO idx_spec_library_slug;
ALTER INDEX IF EXISTS idx_blueprints_created_at RENAME TO idx_spec_library_created_at;
ALTER INDEX IF EXISTS idx_blueprints_updated_at RENAME TO idx_spec_library_updated_at;
ALTER INDEX IF EXISTS idx_blueprints_status RENAME TO idx_spec_library_status;
ALTER INDEX IF EXISTS idx_blueprints_type RENAME TO idx_spec_library_type;
ALTER INDEX IF EXISTS idx_blueprints_metadata RENAME TO idx_spec_library_metadata;
ALTER INDEX IF EXISTS idx_blueprints_project_slug RENAME TO idx_spec_library_project_slug;
ALTER INDEX IF EXISTS idx_blueprints_subtype RENAME TO idx_spec_library_subtype;
ALTER INDEX IF EXISTS idx_blueprints_project_id RENAME TO idx_spec_library_project_id;
ALTER INDEX IF EXISTS idx_blueprints_team_id RENAME TO idx_spec_library_team_id;
ALTER INDEX IF EXISTS idx_blueprints_team_id_user_id RENAME TO idx_spec_library_team_id_user_id;
ALTER INDEX IF EXISTS idx_blueprints_user_type_subtype RENAME TO idx_spec_library_user_type_subtype;

-- Step 3: Rename all constraints back
ALTER TABLE IF EXISTS spec_library RENAME CONSTRAINT check_blueprints_type TO check_spec_library_type;
ALTER TABLE IF EXISTS spec_library RENAME CONSTRAINT check_blueprints_subtype TO check_spec_library_subtype;
ALTER TABLE IF EXISTS spec_library RENAME CONSTRAINT fk_blueprints_project_id TO fk_spec_library_project_id;
ALTER TABLE IF EXISTS spec_library RENAME CONSTRAINT fk_blueprints_team TO fk_spec_library_team;
ALTER TABLE IF EXISTS spec_library RENAME CONSTRAINT blueprints_slug_team_id_key TO spec_library_slug_team_id_key;
ALTER TABLE IF EXISTS spec_library RENAME CONSTRAINT blueprints_project_id_slug_unique TO spec_library_project_id_slug_unique;

-- Note: Some old constraints may not exist in all environments due to migration history
-- These renames are safe to fail if the constraint doesn't exist
DO $$
BEGIN
    -- Try to rename old unique constraints back if they exist
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'blueprints_project_name_slug_user_id_key') THEN
        ALTER TABLE spec_library RENAME CONSTRAINT blueprints_project_name_slug_user_id_key TO spec_library_project_name_slug_user_id_key;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'blueprints_slug_user_id_key') THEN
        ALTER TABLE spec_library RENAME CONSTRAINT blueprints_slug_user_id_key TO spec_library_slug_user_id_key;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'blueprints_slug_user_id_team_id_key') THEN
        ALTER TABLE spec_library RENAME CONSTRAINT blueprints_slug_user_id_team_id_key TO spec_library_slug_user_id_team_id_key;
    END IF;
END $$;
