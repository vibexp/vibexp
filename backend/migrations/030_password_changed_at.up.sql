-- When a user's password was last set or changed (#1437, epic #1426).
--
-- The Account settings page shows "password last changed". user_credentials
-- .updated_at cannot answer that: every sign-in attempt, every success and a
-- transparent rehash all bump it. password_changed_at moves only when a new
-- password is stored (UserCredentialRepository.Upsert).
--
-- Existing rows get the migration time. Password credentials first shipped in
-- 028, in the same release, so no released instance holds one.
ALTER TABLE user_credentials
    ADD COLUMN password_changed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP;

COMMENT ON COLUMN user_credentials.password_changed_at IS
    'When the password was last set or changed. Not moved by sign-in attempts or by a rehash of the same password.';
