-- Create comprehensive activities table for tracking all application activities
CREATE TABLE activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_type VARCHAR(50) NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id VARCHAR(255),
    session_id VARCHAR(255),
    description TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    source_ip INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for optimal query performance
CREATE INDEX idx_activities_user_id ON activities(user_id);
CREATE INDEX idx_activities_type ON activities(activity_type);
CREATE INDEX idx_activities_entity_type ON activities(entity_type);
CREATE INDEX idx_activities_entity_id ON activities(entity_id);
CREATE INDEX idx_activities_session_id ON activities(session_id);
CREATE INDEX idx_activities_created_at ON activities(created_at DESC);
CREATE INDEX idx_activities_user_created ON activities(user_id, created_at DESC);
CREATE INDEX idx_activities_type_created ON activities(activity_type, created_at DESC);

-- Create composite index for common filtering patterns
CREATE INDEX idx_activities_composite ON activities(user_id, activity_type, created_at DESC);

-- Add comment for documentation
COMMENT ON TABLE activities IS 'Comprehensive activity tracking for all application actions including authentication, API usage, resource management, and user interactions';
COMMENT ON COLUMN activities.activity_type IS 'Type of activity: auth_login, auth_logout, api_key_created, prompt_created, context_created, claude_code_session, etc.';
COMMENT ON COLUMN activities.entity_type IS 'Type of entity involved: user, api_key, prompt, context, session, work_report, etc.';
COMMENT ON COLUMN activities.entity_id IS 'ID of the specific entity involved in the activity';
COMMENT ON COLUMN activities.session_id IS 'Session identifier for grouping related activities';
COMMENT ON COLUMN activities.description IS 'Human-readable description of the activity';
COMMENT ON COLUMN activities.metadata IS 'Additional activity-specific data in JSON format';
