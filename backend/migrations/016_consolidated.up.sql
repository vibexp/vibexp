-- Consolidated post-v0.12.0 migration.
--
-- Squashes the migrations that accumulated after the v0.12.0 release into a
-- single step, applied on top of 015, mirroring the 002/006/011/013
-- consolidations (#76, #399, #710, #813). Merged here (in original order):
--   * 016_resource_labels (issue #910, epic #899)
--   * 017_memory_title    (issue #911, epic #899)
-- The two touch disjoint columns (labels on artifacts/blueprints/memories,
-- title on memories only), so each block below is the original migration
-- verbatim; nothing needed to be reconciled across them. No deployed
-- instance has applied either (neither shipped in a release), so
-- renumbering is safe.

-- ===========================================================================
-- 016_resource_labels
-- ===========================================================================

-- Migration 016: one taxonomy for every resource type (issue #910, epic #899).
--
-- Prompts have had a `labels text[]` column with a GIN index since the baseline;
-- artifacts, blueprints and memories had no taxonomy at all. Memories carried
-- one in practice, but only as a frontend convention inside `metadata.tags` --
-- unindexed, unvalidated and invisible to any non-VibeXP MCP client. This adds
-- the real column to the three remaining tables and folds the memory convention
-- into it, leaving `metadata` for genuine key-value pairs.
--
-- NOTE ON THE TABLE NAME: the blueprints table is `blueprints`. `spec_libraries`
-- survives only in legacy constraint names and operationIds.
--
-- NOT NULL (which prompts.labels is not) is deliberate: `labels` is a REQUIRED
-- array in the OpenAPI response schemas, so a NULL could only ever serialize as
-- a spec violation. The default fills existing rows without a rewrite.

ALTER TABLE public.artifacts  ADD COLUMN labels text[] NOT NULL DEFAULT '{}'::text[];
ALTER TABLE public.blueprints ADD COLUMN labels text[] NOT NULL DEFAULT '{}'::text[];
ALTER TABLE public.memories   ADD COLUMN labels text[] NOT NULL DEFAULT '{}'::text[];

COMMENT ON COLUMN public.artifacts.labels IS 'Array of labels for categorizing and filtering artifacts. Max 10 labels, each max 50 characters.';
COMMENT ON COLUMN public.blueprints.labels IS 'Array of labels for categorizing and filtering blueprints. Max 10 labels, each max 50 characters.';
COMMENT ON COLUMN public.memories.labels IS 'Array of labels for categorizing and filtering memories. Max 10 labels, each max 50 characters.';

-- Mirrors idx_prompts_labels. The default array_ops opclass serves @>, <@, &&
-- and =, which is what the list filter (`labels && $n`) needs.
CREATE INDEX idx_artifacts_labels  ON public.artifacts  USING gin (labels);
CREATE INDEX idx_blueprints_labels ON public.blueprints USING gin (labels);
CREATE INDEX idx_memories_labels   ON public.memories   USING gin (labels);

-- Backfill the memory convention. Guarded by jsonb_typeof so a scalar or object
-- parked at that key cannot abort the whole migration, and normalised the way
-- services.normalizeLabels does on every write -- trimmed, empties dropped,
-- de-duplicated on the first occurrence, truncated to 50 characters and capped
-- at 10 entries. Anything looser would land rows the write path could never
-- produce: an untrimmed label matches no `?labels=` filter (the query side IS
-- trimmed), and an over-long or over-full list is one the API would reject on
-- the next edit.
--
-- btrim names its whitespace set explicitly because bare btrim() strips SPACES
-- ONLY, where Go's strings.TrimSpace strips every ASCII whitespace character.
-- (Go additionally trims non-ASCII Unicode spaces; a tag carrying one converges
-- on its next write rather than being wrong here.)
--
-- The key is then removed from metadata: the write path
-- (services.MemoryService) folds any `tags` an old client still sends into
-- labels, so nothing re-creates it.
--
-- update_memories_updated_at is an UNCONDITIONAL BEFORE UPDATE ... FOR EACH ROW
-- trigger, so this statement would rewrite updated_at on every migrated memory
-- -- corrupting the edit signal that search recency ranking and resource
-- freshness both read. Suspend it for the one statement.
ALTER TABLE public.memories DISABLE TRIGGER update_memories_updated_at;

UPDATE public.memories
   SET labels = (
           SELECT COALESCE(array_agg(tag ORDER BY ord), '{}'::text[])
             FROM (
                   SELECT tag, ord
                     FROM (
                           SELECT DISTINCT ON (tag) tag, ord
                             FROM (
                                   SELECT left(btrim(tag, E' \t\n\r\f\v'), 50) AS tag, ord
                                     FROM jsonb_array_elements_text(metadata->'tags')
                                          WITH ORDINALITY AS elems(tag, ord)
                                    WHERE btrim(tag, E' \t\n\r\f\v') <> ''
                                  ) AS trimmed
                            ORDER BY tag, ord
                          ) AS deduped
                    ORDER BY ord
                    LIMIT 10
                  ) AS capped
       ),
       metadata = metadata - 'tags'
 WHERE jsonb_typeof(metadata->'tags') = 'array';

ALTER TABLE public.memories ENABLE TRIGGER update_memories_updated_at;

-- ===========================================================================
-- 017_memory_title
-- ===========================================================================

-- Migration 017: an optional title for memories (issue #911, epic #899).
--
-- A memory is the one resource type with nothing to call it: every surface that
-- lists, links or searches memories has to invent an identifier, and each
-- invents a different one. This gives it a real one.
--
-- NULLABLE with no default and NO BACKFILL, deliberately (decision D of #899):
-- existing memories keep returning `title: null` and the SPA keeps deriving a
-- display title from the first markdown heading. Adding a nullable column with
-- no default is a catalog-only change -- no table rewrite, no lock beyond the
-- brief ACCESS EXCLUSIVE the ALTER itself takes.
--
-- varchar(255) matches the documented `maxLength: 255` in schemas/memories.yaml
-- and the blueprint/feed-item title limits already in use.
--
-- Note the `update_memories_updated_at` trigger on this table: it is a BEFORE
-- UPDATE ... FOR EACH ROW trigger, so it plays no part in a DDL-only migration.
-- It does mean a later title edit bumps `updated_at`, which is the intended
-- behaviour -- a title change is a content edit.

ALTER TABLE public.memories ADD COLUMN title varchar(255);
