-- Create table to store Claude Code hooks payload
CREATE TABLE claude_code_hooks_payload (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(255) NOT NULL,
    transcript_path TEXT,
    cwd TEXT,
    hook_event_name VARCHAR(100) NOT NULL,
    tool_name VARCHAR(100),
    tool_input JSONB,
    tool_response JSONB,
    prompt TEXT,
    message TEXT,
    payload JSONB NOT NULL, -- Store the complete payload as JSON
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for better query performance
CREATE INDEX idx_claude_code_hooks_session_id ON claude_code_hooks_payload(session_id);
CREATE INDEX idx_claude_code_hooks_event_name ON claude_code_hooks_payload(hook_event_name);
CREATE INDEX idx_claude_code_hooks_tool_name ON claude_code_hooks_payload(tool_name);
CREATE INDEX idx_claude_code_hooks_created_at ON claude_code_hooks_payload(created_at DESC);

-- Create trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_claude_code_hooks_payload_updated_at
    BEFORE UPDATE ON claude_code_hooks_payload
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
