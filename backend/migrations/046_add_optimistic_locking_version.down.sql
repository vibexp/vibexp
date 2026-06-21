-- Remove version columns for optimistic locking

ALTER TABLE agent_executions DROP COLUMN IF EXISTS version;
ALTER TABLE prompt_shares DROP COLUMN IF EXISTS version;
ALTER TABLE api_keys DROP COLUMN IF EXISTS version;
ALTER TABLE prompts DROP COLUMN IF EXISTS version;
ALTER TABLE embedding_providers DROP COLUMN IF EXISTS version;
ALTER TABLE agents DROP COLUMN IF EXISTS version;
ALTER TABLE users DROP COLUMN IF EXISTS version;
ALTER TABLE spec_library DROP COLUMN IF EXISTS version;
ALTER TABLE memories DROP COLUMN IF EXISTS version;
ALTER TABLE artifacts DROP COLUMN IF EXISTS version;
