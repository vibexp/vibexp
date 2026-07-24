-- Reverse 011_user_status (#454). Dropping the column takes its CHECK
-- constraint with it, but the index is dropped explicitly first so the down
-- migration is readable and order-independent.
--
-- This is lossy by nature: which accounts were suspended cannot be recovered,
-- and every user becomes active again on re-application of the up migration.

DROP INDEX IF EXISTS idx_users_status_not_active;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_status_check;

ALTER TABLE users
    DROP COLUMN IF EXISTS status;
