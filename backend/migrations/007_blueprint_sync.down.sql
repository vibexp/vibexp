-- Revert the blueprint sync foundation (issue #339, epic #334).
--
-- Drops the columns and constraint added by the up migration. pgcrypto is left
-- installed (other objects may depend on it; dropping an extension is not safe
-- to assume). content and slug/path data on remaining columns is untouched.

ALTER TABLE public.blueprints
    DROP CONSTRAINT IF EXISTS blueprints_project_id_path_unique;

ALTER TABLE public.blueprints
    DROP COLUMN IF EXISTS path,
    DROP COLUMN IF EXISTS path_derived,
    DROP COLUMN IF EXISTS raw_content,
    DROP COLUMN IF EXISTS content_sha,
    DROP COLUMN IF EXISTS source_repo,
    DROP COLUMN IF EXISTS source_commit_sha,
    DROP COLUMN IF EXISTS source_blob_sha,
    DROP COLUMN IF EXISTS imported_at;

ALTER TABLE public.content_versions
    DROP COLUMN IF EXISTS raw_content;
