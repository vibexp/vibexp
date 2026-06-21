-- Create user preferences table to store various user preferences including email notification settings
CREATE TABLE IF NOT EXISTS user_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    preferences JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    version INTEGER DEFAULT 1,
    CONSTRAINT user_preferences_user_id_unique UNIQUE (user_id)
);

-- Index for quick lookups by user_id
CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id);

-- Comment on table
COMMENT ON TABLE user_preferences IS 'Stores user preferences including email notification settings';
COMMENT ON COLUMN user_preferences.preferences IS 'JSONB containing preferences like email_notification settings';
