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
