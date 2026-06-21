-- Migrate the embeddings vector column from 384-dim (all-MiniLM-L6-v2) to 768-dim
-- (gemini-embedding-001, served by ai-service via Vertex AI).
--
-- The embeddings table is a pure derived cache: every save is delete-then-insert,
-- so it is fully regenerable from the source entities. TRUNCATE first because
-- ALTER COLUMN ... TYPE vector(768) cannot rewrite rows that hold 384-dim vectors.
-- After this migration search is degraded until the backfill endpoint
-- (POST /bo/v1/embeddings/backfill) regenerates every embedding under the new model.
--
-- The HNSW index is dropped before the type change and recreated after: a pgvector
-- column type change is incompatible with an existing index on that column. On an
-- empty table the recreate is instant.
TRUNCATE embeddings;

DROP INDEX IF EXISTS idx_embeddings_vector_cosine;

ALTER TABLE embeddings ALTER COLUMN vector_embeddings TYPE vector(768);

CREATE INDEX idx_embeddings_vector_cosine ON embeddings USING hnsw (vector_embeddings vector_cosine_ops);
