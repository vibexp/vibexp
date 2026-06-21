-- Set free plan as default for existing users who don't have subscription data
UPDATE users
SET subscription_status = 'free', subscription_plan = 'free', updated_at = NOW()
WHERE subscription_status IS NULL OR subscription_status = 'none' OR subscription_status = '';

-- Set default values for future user inserts
ALTER TABLE users ALTER COLUMN subscription_status SET DEFAULT 'free';
ALTER TABLE users ALTER COLUMN subscription_plan SET DEFAULT 'free';

-- Update any existing users with empty subscription_plan but have subscription_status
UPDATE users
SET subscription_plan = 'free', updated_at = NOW()
WHERE subscription_plan IS NULL OR subscription_plan = '' AND subscription_status = 'free';
