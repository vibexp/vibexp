-- Remove default values
ALTER TABLE users ALTER COLUMN subscription_status DROP DEFAULT;
ALTER TABLE users ALTER COLUMN subscription_plan DROP DEFAULT;

-- Note: We don't revert the data changes to users table as that could be destructive
-- The migration up script only sets consistent defaults, not changing active subscriptions
