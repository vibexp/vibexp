-- Revert issue #110 per-team model-provider table.

DROP INDEX IF EXISTS public.idx_model_providers_team_id;
DROP INDEX IF EXISTS public.idx_model_providers_team_default;

DROP TABLE IF EXISTS public.model_providers;
