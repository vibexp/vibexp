-- Reverts 030_password_changed_at. Passwords themselves are untouched.
ALTER TABLE user_credentials DROP COLUMN IF EXISTS password_changed_at;
