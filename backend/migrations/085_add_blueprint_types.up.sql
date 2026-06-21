-- Add new blueprint types: claude, cursor, codex
-- Add new subtypes for each blueprint type
-- Business logic validation (e.g., subtype requirements based on type) is handled in application code

-- Drop and recreate CHECK constraint for type field to include new types
ALTER TABLE blueprints DROP CONSTRAINT IF EXISTS check_blueprints_type;
ALTER TABLE blueprints ADD CONSTRAINT check_blueprints_type
    CHECK (type IN ('general', 'claude-code', 'claude', 'cursor', 'codex'));

-- Drop and recreate CHECK constraint for subtype field to include new subtypes
-- New subtypes:
--   - claude-md (for claude type)
--   - agents, commands, rules, cursor-md (for cursor type)
--   - rules, skills, agents-md (for codex type)
-- Note: 'skills' and 'rules' are shared across multiple types
ALTER TABLE blueprints DROP CONSTRAINT IF EXISTS check_blueprints_subtype;
ALTER TABLE blueprints ADD CONSTRAINT check_blueprints_subtype
    CHECK (
        subtype IS NULL OR
        subtype IN ('sub-agents', 'skills', 'slash-commands', 'others', 'claude-md', 'agents', 'commands', 'rules', 'cursor-md', 'agents-md')
    );
