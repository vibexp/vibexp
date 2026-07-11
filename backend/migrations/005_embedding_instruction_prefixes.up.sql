-- Issue #171: per-provider configurable query/document instruction prefixes.
-- Asymmetric embedding models (mxbai/BGE, E5) are trained with instruction
-- prefixes and lose ranking quality without them. These are provider config
-- (same tier as model/chunk_size), applied only to the text sent to the
-- provider at embedding time. Both are nullable and default to empty, so
-- existing rows keep exact current behaviour (no prefix).

ALTER TABLE public.embedding_providers
    ADD COLUMN query_prefix text,
    ADD COLUMN document_prefix text;
