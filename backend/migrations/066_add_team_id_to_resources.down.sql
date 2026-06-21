-- Remove team_id from cursor_ide_hooks_payload
DROP INDEX IF EXISTS idx_cursor_ide_hooks_payload_team_id;
ALTER TABLE cursor_ide_hooks_payload DROP CONSTRAINT IF EXISTS fk_cursor_ide_hooks_payload_team;
ALTER TABLE cursor_ide_hooks_payload DROP COLUMN IF EXISTS team_id;

-- Remove team_id from claude_code_hooks_payload
DROP INDEX IF EXISTS idx_claude_code_hooks_payload_team_id;
ALTER TABLE claude_code_hooks_payload DROP CONSTRAINT IF EXISTS fk_claude_code_hooks_payload_team;
ALTER TABLE claude_code_hooks_payload DROP COLUMN IF EXISTS team_id;

-- Remove team_id from agents
DROP INDEX IF EXISTS idx_agents_team_id;
ALTER TABLE agents DROP CONSTRAINT IF EXISTS fk_agents_team;
ALTER TABLE agents DROP COLUMN IF EXISTS team_id;

-- Remove team_id from spec_library
DROP INDEX IF EXISTS idx_spec_library_team_id;
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS fk_spec_library_team;
ALTER TABLE spec_library DROP COLUMN IF EXISTS team_id;

-- Remove team_id from artifacts
DROP INDEX IF EXISTS idx_artifacts_team_id;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS fk_artifacts_team;
ALTER TABLE artifacts DROP COLUMN IF EXISTS team_id;

-- Remove team_id from memories
DROP INDEX IF EXISTS idx_memories_team_id;
ALTER TABLE memories DROP CONSTRAINT IF EXISTS fk_memories_team;
ALTER TABLE memories DROP COLUMN IF EXISTS team_id;

-- Remove team_id from prompts
DROP INDEX IF EXISTS idx_prompts_team_id;
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS fk_prompts_team;
ALTER TABLE prompts DROP COLUMN IF EXISTS team_id;
