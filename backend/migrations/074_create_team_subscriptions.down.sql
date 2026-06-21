DROP TRIGGER IF EXISTS update_team_subscriptions_updated_at ON team_subscriptions;
DROP INDEX IF EXISTS idx_team_subscriptions_tier;
DROP INDEX IF EXISTS idx_team_subscriptions_status;
DROP INDEX IF EXISTS idx_team_subscriptions_stripe_customer_id;
DROP INDEX IF EXISTS idx_team_subscriptions_stripe_subscription_id;
DROP INDEX IF EXISTS idx_team_subscriptions_team_id;
DROP TABLE IF EXISTS team_subscriptions;
