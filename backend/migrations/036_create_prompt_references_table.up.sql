CREATE TABLE prompt_references (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    prompt_id UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    referenced_prompt_id UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(prompt_id, referenced_prompt_id)
);

CREATE INDEX idx_prompt_references_prompt_id ON prompt_references(prompt_id);
CREATE INDEX idx_prompt_references_referenced_prompt_id ON prompt_references(referenced_prompt_id);

COMMENT ON TABLE prompt_references IS 'Tracks which prompts reference other prompts via @reference syntax';
COMMENT ON COLUMN prompt_references.prompt_id IS 'The prompt that contains the reference';
COMMENT ON COLUMN prompt_references.referenced_prompt_id IS 'The prompt being referenced';
