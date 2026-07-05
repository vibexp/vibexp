-- Revert issue #79 per-team embedding-provider columns and constraints.

DROP INDEX IF EXISTS public.idx_embedding_providers_team_id;
DROP INDEX IF EXISTS public.idx_embedding_providers_team_default;

ALTER TABLE public.embedding_providers
    DROP CONSTRAINT IF EXISTS unique_team_provider_name;
ALTER TABLE public.embedding_providers
    ADD CONSTRAINT unique_user_provider_name UNIQUE (user_id, name);

CREATE UNIQUE INDEX idx_embedding_providers_user_default
    ON public.embedding_providers (user_id)
    WHERE (is_default = true);

ALTER TABLE public.embedding_providers
    DROP CONSTRAINT IF EXISTS fk_embedding_providers_team;

ALTER TABLE public.embedding_providers
    DROP COLUMN IF EXISTS team_id,
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS chunk_size,
    DROP COLUMN IF EXISTS chunk_overlap;
