-- Revert the full-text search fallback indexes (issue #18).

DROP INDEX IF EXISTS idx_memories_fts;
DROP INDEX IF EXISTS idx_blueprints_fts;
DROP INDEX IF EXISTS idx_artifacts_fts;
DROP INDEX IF EXISTS idx_prompts_fts;
