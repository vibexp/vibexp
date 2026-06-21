-- Add mcp_expose column to prompts table
ALTER TABLE prompts ADD COLUMN mcp_expose BOOLEAN NOT NULL DEFAULT TRUE;

-- Add index for query performance
CREATE INDEX idx_prompts_mcp_expose ON prompts(mcp_expose);

-- Add comment to explain the column
COMMENT ON COLUMN prompts.mcp_expose IS 'Whether the prompt is discoverable via MCP (Model Context Protocol) tools';
