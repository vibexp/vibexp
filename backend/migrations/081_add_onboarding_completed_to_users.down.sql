-- Rollback onboarding tracking fields
DROP INDEX IF EXISTS idx_users_onboarding_completed;
ALTER TABLE users DROP COLUMN IF EXISTS onboarding_completed_at;
ALTER TABLE users DROP COLUMN IF EXISTS onboarding_completed;
