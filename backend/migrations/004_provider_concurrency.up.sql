-- Issue #144: per-provider embedding concurrency limit.
-- Adds a bound on how many embedding requests VibeXP issues to a provider at
-- once. NOT NULL DEFAULT 1 mirrors chunk_size/chunk_overlap: existing rows and
-- create requests that omit it get the safe single-threaded default. This issue
-- only introduces + plumbs the value; enforcement during processing lives in
-- #142.

ALTER TABLE public.embedding_providers
    ADD COLUMN concurrency integer NOT NULL DEFAULT 1;
