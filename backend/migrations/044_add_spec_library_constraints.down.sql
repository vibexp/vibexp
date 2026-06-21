-- Remove CHECK constraints from spec_library table

ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS check_spec_library_subtype;
ALTER TABLE spec_library DROP CONSTRAINT IF EXISTS check_spec_library_type;
