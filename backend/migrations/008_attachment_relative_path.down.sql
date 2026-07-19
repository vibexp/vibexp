-- Revert attachment relative paths (issue #338, epic #334).

DROP INDEX IF EXISTS attachments_owner_relative_path_unique;

ALTER TABLE public.attachments
    DROP COLUMN IF EXISTS relative_path;
