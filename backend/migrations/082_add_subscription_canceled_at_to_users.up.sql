-- Add subscription_canceled_at column to users table
-- This tracks when a subscription cancellation was scheduled (Stripe cancel_at_period_end)
-- NULL means the subscription will auto-renew

ALTER TABLE users ADD COLUMN IF NOT EXISTS subscription_canceled_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX IF NOT EXISTS idx_users_subscription_canceled_at ON users(subscription_canceled_at);

COMMENT ON COLUMN users.subscription_canceled_at IS 'Timestamp when subscription cancellation was scheduled (Stripe cancel_at_period_end). NULL means subscription will auto-renew.';
