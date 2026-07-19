-- Attachment relative paths (issue #338, epic #334: Sync-Ready Blueprints).
--
-- Multi-file Agent Skills (#342) need each companion file to keep its path
-- relative to the skill directory (e.g. scripts/helper.py). Attachments today
-- reduce file_name to its base name and are not unique per owner, so companions
-- would collide. Add a nullable relative_path plus a PARTIAL unique index so
-- uniqueness is enforced only for attachments that carry a path — legacy rows
-- (relative_path IS NULL) are left untouched.
--
-- NOTE ON NUMBERING: the epic issue calls this "migration 009", but #339 landed
-- as 007 (migrations were consolidated into 006 by #399), so 008 is the next
-- free slot. Renumbered; behavior unchanged.

ALTER TABLE public.attachments
    ADD COLUMN relative_path text;

CREATE UNIQUE INDEX attachments_owner_relative_path_unique
    ON public.attachments (owner_type, owner_id, relative_path)
    WHERE relative_path IS NOT NULL;
