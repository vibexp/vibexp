-- Add subscription_period_start column back to users
ALTER TABLE users ADD COLUMN subscription_period_start TIMESTAMP WITH TIME ZONE;

-- Recreate resource_usage table
CREATE TABLE resource_usage (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    resource_type VARCHAR(50) NOT NULL,
    count INT NOT NULL DEFAULT 0,
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, resource_type, period_start, period_end)
);

-- Create indexes for performance
CREATE INDEX idx_resource_usage_user_period ON resource_usage(user_id, period_start, period_end);
CREATE INDEX idx_resource_usage_resource_type ON resource_usage(resource_type);

-- Create a trigger to update updated_at on changes
CREATE OR REPLACE FUNCTION resource_usage_update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated_at = CURRENT_TIMESTAMP;
   RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_resource_usage_timestamp
BEFORE UPDATE ON resource_usage
FOR EACH ROW
EXECUTE PROCEDURE resource_usage_update_timestamp();
