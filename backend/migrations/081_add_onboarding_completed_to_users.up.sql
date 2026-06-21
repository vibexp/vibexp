-- Add onboarding tracking fields to users table
ALTER TABLE users ADD COLUMN onboarding_completed BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN onboarding_completed_at TIMESTAMP WITH TIME ZONE;

-- Create index for onboarding_completed for efficient filtering
CREATE INDEX idx_users_onboarding_completed ON users(onboarding_completed);
