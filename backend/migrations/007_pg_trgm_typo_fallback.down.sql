-- Revert issue #188 pg_trgm typo-tolerance fallback indexes.
--
-- Drop only the trigram indexes. The pg_trgm extension itself is left in place:
-- dropping a shared extension in a down-migration is risky (other objects may
-- come to depend on it), matching how migration 001 never drops the `vector`
-- extension it creates.

DROP INDEX IF EXISTS idx_prompts_title_trgm;
DROP INDEX IF EXISTS idx_artifacts_title_trgm;
DROP INDEX IF EXISTS idx_blueprints_title_trgm;
DROP INDEX IF EXISTS idx_memories_title_trgm;
