-- Issue #79: consolidate embedding-provider config into per-team settings.
-- Adds team scoping plus the model + chunker sizing that previously lived in the
-- global YAML `embedding` block, so a provider row fully describes an embed
-- request. The vector dimension stays a hardcoded 1024 constant (single global
-- `vector(1024)` HNSW index), so it is deliberately NOT stored here.

ALTER TABLE public.embedding_providers
    ADD COLUMN team_id uuid,
    ADD COLUMN model character varying(255),
    ADD COLUMN chunk_size integer NOT NULL DEFAULT 1000,
    ADD COLUMN chunk_overlap integer NOT NULL DEFAULT 200;

-- Backfill team_id for existing rows: prefer the team the owner owns, else any
-- membership. Rows whose owner has no team stay NULL and are treated as
-- unconfigured by the application.
UPDATE public.embedding_providers ep
SET team_id = (
    SELECT tm.team_id
    FROM public.team_members tm
    WHERE tm.user_id = ep.user_id
    ORDER BY (tm.role = 'owner') DESC, tm.created_at ASC
    LIMIT 1
)
WHERE ep.team_id IS NULL;

-- Backfill model with the previous global default so existing providers keep
-- generating comparable vectors after the YAML `embedding.model` is removed.
UPDATE public.embedding_providers
SET model = 'gemini-embedding-001'
WHERE model IS NULL OR model = '';

ALTER TABLE public.embedding_providers
    ALTER COLUMN model SET NOT NULL;

ALTER TABLE public.embedding_providers
    ADD CONSTRAINT fk_embedding_providers_team
        FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

-- Uniqueness and the single-default guarantee move from per-user to per-team.
ALTER TABLE public.embedding_providers
    DROP CONSTRAINT IF EXISTS unique_user_provider_name;
ALTER TABLE public.embedding_providers
    ADD CONSTRAINT unique_team_provider_name UNIQUE (team_id, name);

DROP INDEX IF EXISTS public.idx_embedding_providers_user_default;
CREATE UNIQUE INDEX idx_embedding_providers_team_default
    ON public.embedding_providers (team_id)
    WHERE (is_default = true);

CREATE INDEX idx_embedding_providers_team_id
    ON public.embedding_providers (team_id);
