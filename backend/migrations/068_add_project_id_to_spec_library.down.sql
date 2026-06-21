-- Step 1: Add back the project_name column
ALTER TABLE spec_library ADD COLUMN project_name VARCHAR(80);

-- Step 2: Populate project_name from project slug
UPDATE spec_library s
SET project_name = (
    SELECT p.slug
    FROM projects p
    WHERE p.id = s.project_id
);

-- Step 3: Set default for any null values
UPDATE spec_library SET project_name = 'shared' WHERE project_name IS NULL;

-- Step 4: Make project_name NOT NULL
ALTER TABLE spec_library ALTER COLUMN project_name SET NOT NULL;

-- Step 5: Set default value
ALTER TABLE spec_library ALTER COLUMN project_name SET DEFAULT 'shared';

-- Step 6: Drop the new unique constraint
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS spec_library_project_id_slug_unique;

-- Step 7: Add back the old unique constraint
ALTER TABLE spec_library ADD CONSTRAINT spec_library_project_name_slug_user_id_key UNIQUE (project_name, slug, user_id);

-- Step 8: Drop the index
DROP INDEX IF EXISTS idx_spec_library_project_id;

-- Step 9: Drop the foreign key constraint
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS fk_spec_library_project_id;

-- Step 10: Drop the project_id column
ALTER TABLE spec_library DROP COLUMN project_id;
