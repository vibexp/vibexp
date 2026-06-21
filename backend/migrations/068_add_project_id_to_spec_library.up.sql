-- Step 1: Add project_id column (nullable initially for migration)
ALTER TABLE spec_library ADD COLUMN project_id UUID;

-- Step 2: Create default projects for users who have spec_library entries but no projects
INSERT INTO projects (id, user_id, name, slug, description, created_at, updated_at)
SELECT
    uuid_generate_v4(),
    s.user_id,
    'Default Project',
    'default-project',
    'Default project for migrated spec library entries',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM (
    SELECT DISTINCT user_id FROM spec_library
) s
WHERE NOT EXISTS (
    SELECT 1 FROM projects WHERE projects.user_id = s.user_id
);

-- Step 3: Migrate existing spec_library entries to their user's oldest project
-- This assigns each entry to the oldest project belonging to the same user
UPDATE spec_library s
SET project_id = (
    SELECT proj.id
    FROM projects proj
    WHERE proj.user_id = s.user_id
    ORDER BY proj.created_at ASC
    LIMIT 1
)
WHERE s.project_id IS NULL;

-- Step 4: Make project_id NOT NULL after data migration
ALTER TABLE spec_library ALTER COLUMN project_id SET NOT NULL;

-- Step 5: Add foreign key constraint with cascade delete
ALTER TABLE spec_library ADD CONSTRAINT fk_spec_library_project_id
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Step 6: Add index for performance
CREATE INDEX idx_spec_library_project_id ON spec_library(project_id);

-- Step 7: Drop the old unique constraint on (project_name, slug, user_id)
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS spec_library_project_name_slug_user_id_key;

-- Step 8: Add new unique constraint on (project_id, slug)
ALTER TABLE spec_library ADD CONSTRAINT spec_library_project_id_slug_unique UNIQUE (project_id, slug);

-- Step 9: Drop the project_name column (no longer needed)
ALTER TABLE spec_library DROP COLUMN project_name;
