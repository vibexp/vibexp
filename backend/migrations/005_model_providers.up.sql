-- Issue #110: per-team model-provider configuration.
-- Adds a new team-scoped table storing bring-your-own OpenAI-compatible LLM
-- endpoints (encrypted API key + connectivity metadata). Structurally mirrors
-- embedding_providers minus the embedding-only columns (chunk sizing, dimension,
-- concurrency). No runtime consumer reads it yet — it only stores config and
-- backs the validate probe.

CREATE TABLE public.model_providers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid,
    name character varying(255) NOT NULL,
    provider_type character varying(100) NOT NULL,
    model character varying(255) NOT NULL,
    base_url character varying(500),
    api_key_encrypted text,
    is_default boolean DEFAULT false,
    configuration jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL
);

ALTER TABLE public.model_providers
    ADD CONSTRAINT model_providers_pkey PRIMARY KEY (id);

ALTER TABLE public.model_providers
    ADD CONSTRAINT fk_model_providers_team
        FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE public.model_providers
    ADD CONSTRAINT unique_team_model_provider_name UNIQUE (team_id, name);

CREATE UNIQUE INDEX idx_model_providers_team_default
    ON public.model_providers (team_id)
    WHERE (is_default = true);

CREATE INDEX idx_model_providers_team_id
    ON public.model_providers (team_id);
