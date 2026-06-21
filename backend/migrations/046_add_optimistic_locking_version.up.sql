-- Add version column to tables for optimistic locking

-- Artifacts table
ALTER TABLE artifacts ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Memories table
ALTER TABLE memories ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Spec Library table
ALTER TABLE spec_library ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Users table
ALTER TABLE users ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Agents table
ALTER TABLE agents ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Embedding Providers table
ALTER TABLE embedding_providers ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Prompts table
ALTER TABLE prompts ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- API Keys table
ALTER TABLE api_keys ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Prompt Shares table
ALTER TABLE prompt_shares ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

-- Agent Executions table
ALTER TABLE agent_executions ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
