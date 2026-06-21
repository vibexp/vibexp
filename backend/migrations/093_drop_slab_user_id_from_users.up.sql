-- Drop slab_user_id column added in migration 092.
--
-- Migration 092 was introduced by PR #1192 (cross-domain callback for
-- auth.shaharialab.com handoff) and reverted at the code level by PR #1197
-- after the central auth service work was postponed. The revert removed
-- the 092 source files, but production already had version 92 applied,
-- which caused boot-time migration reconciliation to fail (no source for
-- the recorded version).
--
-- We roll forward instead of rolling back: this migration drops the
-- column and its index, leaving the schema equivalent to pre-092 state
-- and advancing schema_migrations to 93 so source and DB agree.
--
-- Safety:
--   - No code on main references slab_user_id, SlabUserID, or the column;
--     verified via `git grep` before authoring this migration.
--   - The column was nullable and never backfilled in production (the
--     consuming endpoint /auth/cross-domain-callback was reverted before
--     real traffic flowed through it).
--   - DROP COLUMN on a nullable, unindexed-except-partial column is a
--     metadata-only operation in Postgres for the unique partial index;
--     dropping the index first keeps the operation predictable.
DROP INDEX IF EXISTS idx_users_slab_user_id;

ALTER TABLE users DROP COLUMN IF EXISTS slab_user_id;
