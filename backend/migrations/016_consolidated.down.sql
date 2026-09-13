-- Consolidated post-v0.12.0 migration -- down.
--
-- Reverses the blocks in the opposite order from the up migration (017 then
-- 016), each block's own down verbatim.

-- ===========================================================================
-- 017_memory_title
-- ===========================================================================

-- Reverses migration 017. Nothing was backfilled, so nothing has to be restored:
-- titles only ever existed in this column and go with it.
ALTER TABLE public.memories DROP COLUMN IF EXISTS title;

-- ===========================================================================
-- 016_resource_labels
-- ===========================================================================

-- Reverses migration 016. The memory backfill is restored first: dropping the
-- column before writing metadata.tags back would lose the values permanently.
--
-- `tags` is written back only where the key is absent or already an array,
-- mirroring the up migration's jsonb_typeof guard: a scalar or object parked at
-- that key is an ordinary metadata entry that 016 up left exactly where it was,
-- and `||` is a shallow merge that would clobber it. Such a row loses its
-- `labels` on rollback -- the column is dropped and 015's schema has nowhere to
-- put them -- but those were going to be lost either way; the preserved entry
-- was not.
ALTER TABLE public.memories DISABLE TRIGGER update_memories_updated_at;

UPDATE public.memories
   SET metadata = COALESCE(metadata, '{}'::jsonb)
                  || jsonb_build_object('tags', to_jsonb(labels))
 WHERE labels IS NOT NULL
   AND array_length(labels, 1) > 0
   AND (metadata->'tags' IS NULL OR jsonb_typeof(metadata->'tags') = 'array');

ALTER TABLE public.memories ENABLE TRIGGER update_memories_updated_at;

DROP INDEX IF EXISTS public.idx_memories_labels;
DROP INDEX IF EXISTS public.idx_blueprints_labels;
DROP INDEX IF EXISTS public.idx_artifacts_labels;

ALTER TABLE public.memories   DROP COLUMN IF EXISTS labels;
ALTER TABLE public.blueprints DROP COLUMN IF EXISTS labels;
ALTER TABLE public.artifacts  DROP COLUMN IF EXISTS labels;
