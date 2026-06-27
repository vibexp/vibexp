-- Revert the full-text search fallback indexes (issue #18).

DROP INDEX IF EXISTS idx_memories_fts;
DROP INDEX IF EXISTS idx_blueprints_fts;
DROP INDEX IF EXISTS idx_artifacts_fts;
DROP INDEX IF EXISTS idx_prompts_fts;

-- Revert the memory lifecycle status column (issue #17): drop the CHECK
-- constraint first, then the column, returning memories to its 001_baseline shape.
ALTER TABLE public.memories
    DROP CONSTRAINT IF EXISTS memories_status_check;

ALTER TABLE public.memories
    DROP COLUMN IF EXISTS status;
