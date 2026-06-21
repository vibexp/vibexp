-- Drop index
DROP INDEX IF EXISTS idx_prompts_project_id;

-- Drop foreign key constraint
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS fk_prompts_project_id;

-- Remove project_id column
ALTER TABLE prompts DROP COLUMN IF EXISTS project_id;

-- Note: Default projects created during migration are NOT removed
-- to preserve data integrity. They can be manually deleted if needed.
