-- Add simple CHECK constraints for data integrity
-- Business logic validation (e.g., subtype requirements based on type) is handled in application code

-- Clean up any existing invalid data before adding constraints
UPDATE spec_library SET type = 'general' WHERE type NOT IN ('general', 'claude-code');
UPDATE spec_library SET subtype = NULL WHERE type != 'claude-code';

-- Add CHECK constraint for type field
ALTER TABLE spec_library ADD CONSTRAINT check_spec_library_type
    CHECK (type IN ('general', 'claude-code'));

-- Add CHECK constraint for subtype field
ALTER TABLE spec_library ADD CONSTRAINT check_spec_library_subtype
    CHECK (
        subtype IS NULL OR
        subtype IN ('sub-agents', 'skills', 'slash-commands', 'others')
    );
