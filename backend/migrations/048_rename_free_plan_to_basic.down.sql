-- Rollback migration: Revert "basic" subscription plan back to "free"
-- This migration reverts all changes made in the up migration
-- Note: The subscriptions table was removed in migration 034, so we only revert the users table

-- Step 1: Revert default values for new users
ALTER TABLE users ALTER COLUMN subscription_status SET DEFAULT 'free';
ALTER TABLE users ALTER COLUMN subscription_plan SET DEFAULT 'free';

-- Step 2: Revert users table - subscription_plan
UPDATE users
SET subscription_plan = 'free', updated_at = NOW()
WHERE subscription_plan = 'basic';

-- Step 3: Revert users table - subscription_status
UPDATE users
SET subscription_status = 'free', updated_at = NOW()
WHERE subscription_status = 'basic';
