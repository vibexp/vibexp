-- Set subscription_period_start for existing users based on their status
-- For users with active subscriptions, we'll use their created_at date as a starting point
-- If they have trial_ends_at, we'll backdate from there

-- For users with active subscription, set period start based on created_at date
-- (with adjustment if it would put them past their subscription renewal)
UPDATE users
SET subscription_period_start =
    -- If trial_ends_at exists and is in the future, backdate one month from trial end
    CASE
        WHEN trial_ends_at IS NOT NULL AND trial_ends_at > CURRENT_TIMESTAMP
        THEN trial_ends_at - INTERVAL '1 month'
        -- Otherwise use created_at date, but ensure it's within one month of now
        ELSE GREATEST(created_at, CURRENT_TIMESTAMP - INTERVAL '1 month')
    END
WHERE subscription_status = 'active';

-- For users on trial, set period start one month before trial end date
UPDATE users
SET subscription_period_start = trial_ends_at - INTERVAL '1 month'
WHERE subscription_status = 'trial_active'
  AND trial_ends_at IS NOT NULL;

-- For all other users (free plan), set to beginning of current month
UPDATE users
SET subscription_period_start = DATE_TRUNC('month', CURRENT_TIMESTAMP)
WHERE subscription_period_start IS NULL;
