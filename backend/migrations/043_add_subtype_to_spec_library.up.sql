-- Add subtype column to spec_library table
ALTER TABLE spec_library ADD COLUMN subtype VARCHAR(50);

-- Create index for subtype for performance
CREATE INDEX idx_spec_library_subtype ON spec_library(subtype);
