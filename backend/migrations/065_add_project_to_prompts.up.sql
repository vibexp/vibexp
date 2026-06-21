-- Step 1: Add project_id column (nullable initially for migration)
ALTER TABLE prompts ADD COLUMN project_id UUID;

-- Step 2: Create default projects for users who have prompts but no projects
INSERT INTO projects (id, user_id, name, slug, description, created_at, updated_at)
SELECT
    uuid_generate_v4(),
    p.user_id,
    'Default Project',
    'default-project',
    'Default project for migrated prompts',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM (
    SELECT DISTINCT user_id FROM prompts
) p
WHERE NOT EXISTS (
    SELECT 1 FROM projects WHERE projects.user_id = p.user_id
);

-- Step 3: Migrate existing prompts to their user's oldest project
-- This assigns each prompt to the oldest project belonging to the same user
UPDATE prompts p
SET project_id = (
    SELECT proj.id
    FROM projects proj
    WHERE proj.user_id = p.user_id
    ORDER BY proj.created_at ASC
    LIMIT 1
)
WHERE p.project_id IS NULL;

-- Step 4: Make project_id NOT NULL after data migration
ALTER TABLE prompts ALTER COLUMN project_id SET NOT NULL;

-- Step 5: Add foreign key constraint with cascade delete
ALTER TABLE prompts ADD CONSTRAINT fk_prompts_project_id
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Step 6: Add index for performance
CREATE INDEX idx_prompts_project_id ON prompts(project_id);
