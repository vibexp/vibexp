-- Add labels column to prompts table
ALTER TABLE prompts ADD COLUMN IF NOT EXISTS labels TEXT[] DEFAULT '{}';

-- Create GIN index for efficient label querying
CREATE INDEX IF NOT EXISTS idx_prompts_labels ON prompts USING GIN (labels);

-- Add comment to document the column
COMMENT ON COLUMN prompts.labels IS 'Array of labels for categorizing and filtering prompts. Max 10 labels, each max 50 characters.';
