-- Increase avatar_url column size to accommodate modern Google profile picture URLs
-- Modern Google URLs can exceed 500 characters due to CDN paths, parameters, and tokens
ALTER TABLE users ALTER COLUMN avatar_url TYPE VARCHAR(2048);
