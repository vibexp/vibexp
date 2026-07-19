-- Blueprint imported-content fingerprint (issue #341, epic #334).
--
-- Update-aware re-import needs to tell an unedited imported blueprint (its raw
-- still byte-identical to what was imported) from a VibeXP-edited one. The edit
-- signal is "current content_sha != the content_sha captured at import" — but
-- content_sha is regenerated on every VibeXP edit (#340), so the import-time
-- value must be preserved separately. source_content_sha is that immutable,
-- server-set fingerprint (SHA-256 of the imported raw bytes), a sibling of the
-- other source_* provenance columns. Nullable; legacy/pre-#341 rows are NULL and
-- re-import treats them conservatively (cannot confirm unedited -> conflict).
ALTER TABLE public.blueprints
    ADD COLUMN source_content_sha character varying(64);
