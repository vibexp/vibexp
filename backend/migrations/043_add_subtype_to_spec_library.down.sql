-- Remove subtype column and index
DROP INDEX IF EXISTS idx_spec_library_subtype;
ALTER TABLE spec_library DROP COLUMN IF EXISTS subtype;
