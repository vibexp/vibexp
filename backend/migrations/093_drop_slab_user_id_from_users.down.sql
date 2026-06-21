-- Restore slab_user_id column. This down migration mirrors the original
-- 092_add_slab_user_id_to_users.up.sql so reverting 093 yields the same
-- schema as having 092 applied.
ALTER TABLE users ADD COLUMN IF NOT EXISTS slab_user_id UUID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_slab_user_id
    ON users (slab_user_id)
    WHERE slab_user_id IS NOT NULL;
