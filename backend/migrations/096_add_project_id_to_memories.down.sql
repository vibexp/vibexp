-- WARNING: This down migration permanently deletes auto-created default projects
-- that were inserted by the up migration. This data loss is irreversible.

-- Remove auto-created default projects from up migration (identified by marker description)
DELETE FROM projects WHERE description = 'Default project for migrated memories';

-- Step 1: Drop the index
DROP INDEX IF EXISTS idx_memories_project_id;

-- Step 2: Drop the foreign key constraint
ALTER TABLE memories DROP CONSTRAINT IF EXISTS fk_memories_project_id;

-- Step 3: Drop the project_id column
ALTER TABLE memories DROP COLUMN IF EXISTS project_id;
