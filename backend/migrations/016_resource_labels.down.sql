-- Reverses migration 016. The memory backfill is restored first: dropping the
-- column before writing metadata.tags back would lose the values permanently.
ALTER TABLE public.memories DISABLE TRIGGER update_memories_updated_at;

UPDATE public.memories
   SET metadata = COALESCE(metadata, '{}'::jsonb)
                  || jsonb_build_object('tags', to_jsonb(labels))
 WHERE labels IS NOT NULL
   AND array_length(labels, 1) > 0;

ALTER TABLE public.memories ENABLE TRIGGER update_memories_updated_at;

DROP INDEX IF EXISTS public.idx_memories_labels;
DROP INDEX IF EXISTS public.idx_blueprints_labels;
DROP INDEX IF EXISTS public.idx_artifacts_labels;

ALTER TABLE public.memories   DROP COLUMN IF EXISTS labels;
ALTER TABLE public.blueprints DROP COLUMN IF EXISTS labels;
ALTER TABLE public.artifacts  DROP COLUMN IF EXISTS labels;
