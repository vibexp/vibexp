-- Drop the GIN index
DROP INDEX IF EXISTS idx_prompts_labels;

-- Remove labels column from prompts table
ALTER TABLE prompts DROP COLUMN IF EXISTS labels;
