-- Remove composite index from spec_library table

DROP INDEX IF EXISTS idx_spec_library_user_type_subtype;
