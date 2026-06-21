-- Super optimized version of migration 066
-- All users already have default_team_id from migration 058, so we can skip team creation
-- Just add team_id columns and populate them from users.default_team_id

-- Add team_id to prompts table
ALTER TABLE prompts ADD COLUMN team_id UUID;
UPDATE prompts p SET team_id = u.default_team_id FROM users u WHERE p.user_id::uuid = u.id;
ALTER TABLE prompts ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE prompts ADD CONSTRAINT fk_prompts_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_prompts_team_id ON prompts(team_id);

-- Add team_id to memories table
ALTER TABLE memories ADD COLUMN team_id UUID;
UPDATE memories m SET team_id = u.default_team_id FROM users u WHERE m.user_id::uuid = u.id;
ALTER TABLE memories ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE memories ADD CONSTRAINT fk_memories_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_memories_team_id ON memories(team_id);

-- Add team_id to artifacts table
ALTER TABLE artifacts ADD COLUMN team_id UUID;
UPDATE artifacts a SET team_id = u.default_team_id FROM users u WHERE a.user_id::uuid = u.id;
ALTER TABLE artifacts ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE artifacts ADD CONSTRAINT fk_artifacts_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_artifacts_team_id ON artifacts(team_id);

-- Add team_id to spec_library table
ALTER TABLE spec_library ADD COLUMN team_id UUID;
UPDATE spec_library s SET team_id = u.default_team_id FROM users u WHERE s.user_id::uuid = u.id;
ALTER TABLE spec_library ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE spec_library ADD CONSTRAINT fk_spec_library_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_spec_library_team_id ON spec_library(team_id);

-- Add team_id to agents table
ALTER TABLE agents ADD COLUMN team_id UUID;
UPDATE agents a SET team_id = u.default_team_id FROM users u WHERE a.user_id::uuid = u.id;
ALTER TABLE agents ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE agents ADD CONSTRAINT fk_agents_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_agents_team_id ON agents(team_id);

-- Add team_id to claude_code_hooks_payload table
ALTER TABLE claude_code_hooks_payload ADD COLUMN team_id UUID;
UPDATE claude_code_hooks_payload c SET team_id = u.default_team_id FROM users u WHERE c.user_id::uuid = u.id;
ALTER TABLE claude_code_hooks_payload ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE claude_code_hooks_payload ADD CONSTRAINT fk_claude_code_hooks_payload_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_claude_code_hooks_payload_team_id ON claude_code_hooks_payload(team_id);

-- Add team_id to cursor_ide_hooks_payload table
ALTER TABLE cursor_ide_hooks_payload ADD COLUMN team_id UUID;
UPDATE cursor_ide_hooks_payload c SET team_id = u.default_team_id FROM users u WHERE c.user_id::uuid = u.id;
ALTER TABLE cursor_ide_hooks_payload ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE cursor_ide_hooks_payload ADD CONSTRAINT fk_cursor_ide_hooks_payload_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
CREATE INDEX idx_cursor_ide_hooks_payload_team_id ON cursor_ide_hooks_payload(team_id);
