-- Revert issue #1665 per-provider embedding request batch size and timeout.
ALTER TABLE public.embedding_providers
    DROP COLUMN IF EXISTS request_timeout_seconds,
    DROP COLUMN IF EXISTS batch_size;
