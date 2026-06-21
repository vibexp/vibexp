-- Step 1: Add project_id column (nullable initially for migration)
ALTER TABLE artifacts ADD COLUMN project_id UUID;

-- Step 2: Create default projects for users who have artifacts but no projects
INSERT INTO projects (id, user_id, name, slug, description, created_at, updated_at)
SELECT
    uuid_generate_v4(),
    a.user_id,
    'Default Project',
    'default-project',
    'Default project for migrated artifacts',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM (
    SELECT DISTINCT user_id FROM artifacts
) a
WHERE NOT EXISTS (
    SELECT 1 FROM projects WHERE projects.user_id = a.user_id
);

-- Step 3: Migrate existing artifacts to their user's oldest project
-- This assigns each artifact to the oldest project belonging to the same user
UPDATE artifacts a
SET project_id = (
    SELECT proj.id
    FROM projects proj
    WHERE proj.user_id = a.user_id
    ORDER BY proj.created_at ASC
    LIMIT 1
)
WHERE a.project_id IS NULL;

-- Step 4: Make project_id NOT NULL after data migration
ALTER TABLE artifacts ALTER COLUMN project_id SET NOT NULL;

-- Step 5: Add foreign key constraint with cascade delete
ALTER TABLE artifacts ADD CONSTRAINT fk_artifacts_project_id
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Step 6: Add index for performance
CREATE INDEX idx_artifacts_project_id ON artifacts(project_id);

-- Step 7: Drop the old unique constraint on (project_name, slug, user_id)
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_project_name_slug_user_id_key;

-- Step 8: Add new unique constraint on (project_id, slug)
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_id_slug_unique UNIQUE (project_id, slug);

-- Step 9: Drop the project_name column (no longer needed)
ALTER TABLE artifacts DROP COLUMN project_name;
