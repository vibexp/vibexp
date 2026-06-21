-- Remove the subscription_period_start column from users table
DROP INDEX IF EXISTS idx_users_subscription_period_start;
ALTER TABLE users DROP COLUMN IF EXISTS subscription_period_start;
