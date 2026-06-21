-- Add slab_user_id (canonical Shaharia Lab user id from auth.shaharialab.com).
-- Nullable initially so the column can be populated incrementally as users
-- bridge through the new /auth/cross-domain-callback flow. A unique index
-- guarantees one local user per upstream identity once populated.
ALTER TABLE users ADD COLUMN slab_user_id UUID;

CREATE UNIQUE INDEX idx_users_slab_user_id
    ON users (slab_user_id)
    -- WHERE clause kept for explicit readability; NULL values are NULLS DISTINCT
    -- in PG 15+ unique indexes by default, so this is functionally redundant.
    WHERE slab_user_id IS NOT NULL;
