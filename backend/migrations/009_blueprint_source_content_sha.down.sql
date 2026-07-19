-- Revert the blueprint imported-content fingerprint (issue #341, epic #334).
ALTER TABLE public.blueprints
    DROP COLUMN IF EXISTS source_content_sha;
