-- Add composite index for better query performance on spec_library table
-- This index optimizes queries filtering by user_id, type, and subtype

CREATE INDEX IF NOT EXISTS idx_spec_library_user_type_subtype
    ON spec_library(user_id, type, subtype);
