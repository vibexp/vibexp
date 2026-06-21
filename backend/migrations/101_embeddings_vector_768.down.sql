-- Revert the embeddings vector column from 768-dim back to 384-dim. The table is a
-- derived cache, so TRUNCATE is safe; the 768-dim rows cannot be narrowed in place,
-- and downgrading requires re-running the original (now-removed) MiniLM pipeline to
-- repopulate anyway.
TRUNCATE embeddings;

DROP INDEX IF EXISTS idx_embeddings_vector_cosine;

ALTER TABLE embeddings ALTER COLUMN vector_embeddings TYPE vector(384);

CREATE INDEX idx_embeddings_vector_cosine ON embeddings USING hnsw (vector_embeddings vector_cosine_ops);
