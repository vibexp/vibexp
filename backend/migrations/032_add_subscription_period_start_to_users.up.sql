-- Add subscription_period_start column to users table
ALTER TABLE users ADD COLUMN subscription_period_start TIMESTAMP WITH TIME ZONE;

-- Create an index for performance
CREATE INDEX idx_users_subscription_period_start ON users(subscription_period_start);

-- Add comment for documentation
COMMENT ON COLUMN users.subscription_period_start IS 'The start date of the current subscription period. The end date is calculated as start + 1 month.';
