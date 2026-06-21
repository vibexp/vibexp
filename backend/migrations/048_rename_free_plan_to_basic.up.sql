-- Migration: Rename "free" subscription plan to "basic" for legal compliance
-- This migration updates all references from "free" to "basic" in the database
-- Note: The subscriptions table was removed in migration 034, so we only update the users table

-- Step 1: Update users table - subscription_status
UPDATE users
SET subscription_status = 'basic', updated_at = NOW()
WHERE subscription_status = 'free';

-- Step 2: Update users table - subscription_plan
UPDATE users
SET subscription_plan = 'basic', updated_at = NOW()
WHERE subscription_plan = 'free';

-- Step 3: Update default values for new users
ALTER TABLE users ALTER COLUMN subscription_status SET DEFAULT 'basic';
ALTER TABLE users ALTER COLUMN subscription_plan SET DEFAULT 'basic';
