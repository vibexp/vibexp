-- Revert issue #144 per-provider embedding concurrency column.

ALTER TABLE public.embedding_providers
    DROP COLUMN IF EXISTS concurrency;
