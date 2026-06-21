-- Remove subscription_canceled_at column from users table

DROP INDEX IF EXISTS idx_users_subscription_canceled_at;

ALTER TABLE users DROP COLUMN IF EXISTS subscription_canceled_at;
