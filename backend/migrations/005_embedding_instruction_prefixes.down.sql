-- Revert issue #171 per-provider embedding instruction prefixes.

ALTER TABLE public.embedding_providers
    DROP COLUMN IF EXISTS query_prefix,
    DROP COLUMN IF EXISTS document_prefix;
