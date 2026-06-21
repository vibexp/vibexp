-- Drop index
DROP INDEX IF EXISTS idx_prompts_mcp_expose;

-- Drop mcp_expose column
ALTER TABLE prompts DROP COLUMN IF EXISTS mcp_expose;
