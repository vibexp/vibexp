-- Drop the content column from embeddings table
DROP INDEX IF EXISTS idx_embeddings_content;
ALTER TABLE embeddings DROP COLUMN IF EXISTS content;
