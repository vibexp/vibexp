-- Rollback: Restore avatar_url column to original length
-- Warning: This may truncate URLs longer than 500 characters
ALTER TABLE users ALTER COLUMN avatar_url TYPE VARCHAR(500);
