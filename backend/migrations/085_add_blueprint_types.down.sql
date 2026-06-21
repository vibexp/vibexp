-- Revert blueprint types and subtypes to original values

-- Clean up any data using new types/subtypes before reverting constraints
UPDATE blueprints SET type = 'general' WHERE type NOT IN ('general', 'claude-code');
UPDATE blueprints SET subtype = NULL WHERE type != 'claude-code';
UPDATE blueprints SET subtype = NULL WHERE subtype NOT IN ('sub-agents', 'skills', 'slash-commands', 'others');

-- Revert CHECK constraint for type field to original types only
ALTER TABLE blueprints DROP CONSTRAINT IF EXISTS check_blueprints_type;
ALTER TABLE blueprints ADD CONSTRAINT check_blueprints_type
    CHECK (type IN ('general', 'claude-code'));

-- Revert CHECK constraint for subtype field to original subtypes only
ALTER TABLE blueprints DROP CONSTRAINT IF EXISTS check_blueprints_subtype;
ALTER TABLE blueprints ADD CONSTRAINT check_blueprints_subtype
    CHECK (
        subtype IS NULL OR
        subtype IN ('sub-agents', 'skills', 'slash-commands', 'others')
    );
