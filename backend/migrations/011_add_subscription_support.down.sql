-- Drop indexes
DROP INDEX IF EXISTS idx_subscriptions_status;
DROP INDEX IF EXISTS idx_subscriptions_stripe_customer_id;
DROP INDEX IF EXISTS idx_subscriptions_stripe_subscription_id;
DROP INDEX IF EXISTS idx_subscriptions_user_id;
DROP INDEX IF EXISTS idx_users_subscription_status;
DROP INDEX IF EXISTS idx_users_stripe_customer_id;

-- Drop subscriptions table
DROP TABLE IF EXISTS subscriptions;

-- Remove subscription columns from users table
ALTER TABLE users DROP COLUMN IF EXISTS subscription_plan;
ALTER TABLE users DROP COLUMN IF EXISTS trial_ends_at;
ALTER TABLE users DROP COLUMN IF EXISTS subscription_status;
ALTER TABLE users DROP COLUMN IF EXISTS stripe_customer_id;
