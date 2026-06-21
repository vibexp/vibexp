-- Create table to store Cursor IDE hooks payload
CREATE TABLE cursor_ide_hooks_payload (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255),
    session_id VARCHAR(255) NOT NULL, -- Will store conversation_id from Cursor
    conversation_id VARCHAR(255), -- Cursor IDE's conversation identifier
    generation_id VARCHAR(255), -- Cursor IDE's generation identifier
    hook_event_name VARCHAR(100) NOT NULL,
    tool_name VARCHAR(100),
    workspace_roots TEXT[], -- Array of workspace root paths
    configuration JSONB,
    reference JSONB,
    context JSONB,
    input JSONB,
    output JSONB,
    induced_failure JSONB,
    payload JSONB NOT NULL, -- Store the complete payload as JSON
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for better query performance
CREATE INDEX idx_cursor_ide_hooks_session_id ON cursor_ide_hooks_payload(session_id);
CREATE INDEX idx_cursor_ide_hooks_conversation_id ON cursor_ide_hooks_payload(conversation_id);
CREATE INDEX idx_cursor_ide_hooks_generation_id ON cursor_ide_hooks_payload(generation_id);
CREATE INDEX idx_cursor_ide_hooks_event_name ON cursor_ide_hooks_payload(hook_event_name);
CREATE INDEX idx_cursor_ide_hooks_tool_name ON cursor_ide_hooks_payload(tool_name);
CREATE INDEX idx_cursor_ide_hooks_created_at ON cursor_ide_hooks_payload(created_at DESC);
CREATE INDEX idx_cursor_ide_hooks_user_id ON cursor_ide_hooks_payload(user_id);

-- Create trigger to update updated_at timestamp (reuse existing function)
CREATE TRIGGER update_cursor_ide_hooks_payload_updated_at
    BEFORE UPDATE ON cursor_ide_hooks_payload
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
