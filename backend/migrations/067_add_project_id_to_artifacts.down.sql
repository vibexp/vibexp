-- Step 1: Add back the project_name column
ALTER TABLE artifacts ADD COLUMN project_name VARCHAR(80);

-- Step 2: Populate project_name from project slug
UPDATE artifacts a
SET project_name = (
    SELECT p.slug
    FROM projects p
    WHERE p.id = a.project_id
);

-- Step 3: Set default for any null values
UPDATE artifacts SET project_name = 'shared' WHERE project_name IS NULL;

-- Step 4: Make project_name NOT NULL
ALTER TABLE artifacts ALTER COLUMN project_name SET NOT NULL;

-- Step 5: Set default value
ALTER TABLE artifacts ALTER COLUMN project_name SET DEFAULT 'shared';

-- Step 6: Drop the new unique constraint
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_project_id_slug_unique;

-- Step 7: Add back the old unique constraint
ALTER TABLE artifacts ADD CONSTRAINT artifacts_project_name_slug_user_id_key UNIQUE (project_name, slug, user_id);

-- Step 8: Drop the index
DROP INDEX IF EXISTS idx_artifacts_project_id;

-- Step 9: Drop the foreign key constraint
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS fk_artifacts_project_id;

-- Step 10: Drop the project_id column
ALTER TABLE artifacts DROP COLUMN project_id;
