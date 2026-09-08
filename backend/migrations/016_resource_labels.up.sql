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
-- parked at that key cannot abort the whole migration, and capped at the same
-- 10 x 50 limits the API validates, so the backfill cannot land a row the API
-- would reject on the next edit. The key is then removed from metadata: the
-- write path (services.MemoryService) folds any `tags` an old client still
-- sends into labels, so nothing re-creates it.
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
                   SELECT left(tag, 50) AS tag, ord
                     FROM jsonb_array_elements_text(metadata->'tags')
                          WITH ORDINALITY AS elems(tag, ord)
                    WHERE tag <> ''
                    ORDER BY ord
                    LIMIT 10
                  ) AS capped
       ),
       metadata = metadata - 'tags'
 WHERE jsonb_typeof(metadata->'tags') = 'array';

ALTER TABLE public.memories ENABLE TRIGGER update_memories_updated_at;
