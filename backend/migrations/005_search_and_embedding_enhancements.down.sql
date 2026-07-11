-- Revert the consolidated post-v0.5.0 migration.

-- Drop the pg_trgm typo-tolerance title indexes (issue #188/#174). The pg_trgm
-- extension itself is left in place: dropping a shared extension in a down
-- migration is risky (other objects may come to depend on it), matching how
-- migration 001 never drops the `vector` extension it creates.
DROP INDEX IF EXISTS idx_prompts_title_trgm;
DROP INDEX IF EXISTS idx_artifacts_title_trgm;
DROP INDEX IF EXISTS idx_blueprints_title_trgm;
DROP INDEX IF EXISTS idx_memories_title_trgm;

-- Revert per-provider embedding instruction prefixes (issue #171).
ALTER TABLE public.embedding_providers
    DROP COLUMN IF EXISTS query_prefix,
    DROP COLUMN IF EXISTS document_prefix;
