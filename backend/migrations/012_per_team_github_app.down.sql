-- Reverse 012_per_team_github_app (#477).
--
-- LOSSY IN BOTH DIRECTIONS. The up migration deleted every github_installations
-- row and this cannot restore them. It also deletes the rows created since,
-- deliberately: they reference per-team Apps whose credentials this migration
-- drops along with github_app_configs, so they would be unusable in the
-- pre-#477 world exactly as the old rows were unusable in the new one. Deleting
-- them is also what makes the global installation_id UNIQUE re-addable -- two
-- teams installing their own App on the same org is legal after 012 and would
-- otherwise make this migration fail on the restored constraint.

DELETE FROM public.github_installations;

DROP INDEX IF EXISTS idx_github_installations_app_config_id;

ALTER TABLE public.github_installations
    DROP CONSTRAINT IF EXISTS unique_app_installation;

ALTER TABLE public.github_installations
    ADD CONSTRAINT github_installations_installation_id_key UNIQUE (installation_id);

ALTER TABLE public.github_installations
    DROP CONSTRAINT IF EXISTS fk_installations_app_config;

ALTER TABLE public.github_installations
    DROP COLUMN IF EXISTS app_config_id;

DROP TABLE IF EXISTS public.github_app_configs;

-- Restore the dead-but-present schema exactly as 001_baseline defined it, so a
-- down-then-up cycle lands back on the same shape.
CREATE TABLE public.github_installation_repositories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    repository_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    full_name character varying(500) NOT NULL,
    private boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT github_installation_repositories_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT unique_installation_repository UNIQUE (installation_id, repository_id);

CREATE INDEX idx_github_installation_repositories_installation_id
    ON public.github_installation_repositories USING btree (installation_id);

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT github_installation_repositories_installation_id_fkey
        FOREIGN KEY (installation_id) REFERENCES public.github_installations(id) ON DELETE CASCADE;

COMMENT ON TABLE public.webhook_events IS
    'Tracks processed Stripe webhook events to prevent duplicate processing (idempotency)';
