-- Remove the AI-tool hook-ingestion feature at the data layer (epic #610, issue #614).
--
-- Drops the two hook payload tables, purges the orphaned Claude Code activity rows,
-- and retires the `ai_tools` API-key integration scope.
--
-- Step order is load-bearing:
--   * legacy `api_keys.usage_type = 'ai_tools'` values are migrated BEFORE chk_usage_type
--     is rewritten, otherwise the new constraint fails to validate on any real database;
--   * grant rows are deleted BEFORE the catalog row, to respect the FK between them.
--
-- No API key is deleted and no key loses the ability to authenticate: keys scoped only
-- to `ai_tools` are re-pointed to `everything`. This slightly widens those keys' scope,
-- accepted deliberately — they were single-purpose for a feature that no longer exists,
-- and the alternative silently bricks credentials users already hold.

-- 1. Re-point legacy single-purpose keys before tightening the CHECK constraint.
UPDATE api_keys SET usage_type = 'everything' WHERE usage_type = 'ai_tools';

-- 2. Rewrite chk_usage_type to allow only the three surviving usage types.
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;
ALTER TABLE api_keys
    ADD CONSTRAINT chk_usage_type CHECK (
        (usage_type)::text = ANY ((ARRAY[
            'cli'::character varying,
            'mcp'::character varying,
            'everything'::character varying
        ])::text[])
    );

-- 3. Drop the ai_tools grants, then the catalog row they reference.
DELETE FROM api_key_integration_permissions WHERE integration_code = 'ai_tools';
DELETE FROM api_key_integrations_catalog WHERE integration_code = 'ai_tools';

-- 4. Purge the historical Claude Code activity rows. Decided in refinement: these are
--    deleted, not preserved as history, and all destructive DML lives in this migration.
DELETE FROM activities
 WHERE activity_type IN ('claude_code_session', 'claude_code_tool', 'claude_code_prompt');

-- 5. Drop the payload tables. CASCADE takes the sequences, indexes, updated_at triggers
--    and team foreign keys with them, so they need not be enumerated.
DROP TABLE IF EXISTS public.claude_code_hooks_payload CASCADE;
DROP TABLE IF EXISTS public.cursor_ide_hooks_payload CASCADE;
