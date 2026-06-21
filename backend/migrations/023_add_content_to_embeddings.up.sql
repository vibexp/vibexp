-- Add content column to embeddings table to store the chunked content text
ALTER TABLE embeddings ADD COLUMN content TEXT NOT NULL DEFAULT '';

-- Create an index on content for faster text search if needed
CREATE INDEX idx_embeddings_content ON embeddings USING gin(to_tsvector('english', content));
