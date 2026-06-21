-- Drop indexes first to avoid dependency issues
DROP INDEX IF EXISTS idx_subscriptions_user_id;
DROP INDEX IF EXISTS idx_subscriptions_stripe_subscription_id;
DROP INDEX IF EXISTS idx_subscriptions_stripe_customer_id;
DROP INDEX IF EXISTS idx_subscriptions_status;

-- Drop the subscriptions table as it's no longer needed
DROP TABLE IF EXISTS subscriptions;
