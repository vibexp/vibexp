DROP INDEX IF EXISTS idx_users_slab_user_id;

ALTER TABLE users DROP COLUMN IF EXISTS slab_user_id;
