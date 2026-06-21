-- Rename spec_library table to blueprints
-- This migration renames the table, all indexes, and all constraints

-- Step 1: Rename the table
ALTER TABLE IF EXISTS spec_library RENAME TO blueprints;

-- Step 2: Rename all indexes
ALTER INDEX IF EXISTS idx_spec_library_user_id RENAME TO idx_blueprints_user_id;
ALTER INDEX IF EXISTS idx_spec_library_project_name RENAME TO idx_blueprints_project_name;
ALTER INDEX IF EXISTS idx_spec_library_slug RENAME TO idx_blueprints_slug;
ALTER INDEX IF EXISTS idx_spec_library_created_at RENAME TO idx_blueprints_created_at;
ALTER INDEX IF EXISTS idx_spec_library_updated_at RENAME TO idx_blueprints_updated_at;
ALTER INDEX IF EXISTS idx_spec_library_status RENAME TO idx_blueprints_status;
ALTER INDEX IF EXISTS idx_spec_library_type RENAME TO idx_blueprints_type;
ALTER INDEX IF EXISTS idx_spec_library_metadata RENAME TO idx_blueprints_metadata;
ALTER INDEX IF EXISTS idx_spec_library_project_slug RENAME TO idx_blueprints_project_slug;
ALTER INDEX IF EXISTS idx_spec_library_subtype RENAME TO idx_blueprints_subtype;
ALTER INDEX IF EXISTS idx_spec_library_project_id RENAME TO idx_blueprints_project_id;
ALTER INDEX IF EXISTS idx_spec_library_team_id RENAME TO idx_blueprints_team_id;
ALTER INDEX IF EXISTS idx_spec_library_team_id_user_id RENAME TO idx_blueprints_team_id_user_id;
ALTER INDEX IF EXISTS idx_spec_library_user_type_subtype RENAME TO idx_blueprints_user_type_subtype;

-- Step 3: Rename all constraints
ALTER TABLE IF EXISTS blueprints RENAME CONSTRAINT check_spec_library_type TO check_blueprints_type;
ALTER TABLE IF EXISTS blueprints RENAME CONSTRAINT check_spec_library_subtype TO check_blueprints_subtype;
ALTER TABLE IF EXISTS blueprints RENAME CONSTRAINT fk_spec_library_project_id TO fk_blueprints_project_id;
ALTER TABLE IF EXISTS blueprints RENAME CONSTRAINT fk_spec_library_team TO fk_blueprints_team;
ALTER TABLE IF EXISTS blueprints RENAME CONSTRAINT spec_library_slug_team_id_key TO blueprints_slug_team_id_key;
ALTER TABLE IF EXISTS blueprints RENAME CONSTRAINT spec_library_project_id_slug_unique TO blueprints_project_id_slug_unique;

-- Note: Some old constraints may not exist in all environments due to migration history
-- These renames are safe to fail if the constraint doesn't exist
DO $$
BEGIN
    -- Try to rename old unique constraints if they still exist
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'spec_library_project_name_slug_user_id_key') THEN
        ALTER TABLE blueprints RENAME CONSTRAINT spec_library_project_name_slug_user_id_key TO blueprints_project_name_slug_user_id_key;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'spec_library_slug_user_id_key') THEN
        ALTER TABLE blueprints RENAME CONSTRAINT spec_library_slug_user_id_key TO blueprints_slug_user_id_key;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'spec_library_slug_user_id_team_id_key') THEN
        ALTER TABLE blueprints RENAME CONSTRAINT spec_library_slug_user_id_team_id_key TO blueprints_slug_user_id_team_id_key;
    END IF;
END $$;
