-- Make the embeddings vector dimension a deployment-wide knob for the native,
-- pluggable embedding subsystem (issue #6). Embeddings are now produced by an
-- operator-configured, OpenAI-compatible provider instead of the removed external
-- ai-service, so the previous gemini-embedding-001 (768-dim) vectors are no longer
-- valid and must be regenerated under the new provider.
--
-- The embeddings table is a pure derived cache (every save is delete-then-insert),
-- so it is fully regenerable from the source entities. TRUNCATE first because an
-- ALTER COLUMN ... TYPE vector(N) cannot rewrite rows that hold vectors of a
-- different width, and because mixing vectors from two different models in one
-- index would corrupt similarity search. After this migration, search is degraded
-- until embeddings are regenerated via the backfill endpoint
-- (POST /bo/v1/embeddings/backfill).
--
-- DIMENSION: the column width below MUST equal EMBEDDING_DIMENSIONS. The default
-- (768) is kept so existing deployments need no env change. To run a provider with
-- a different output dimension (e.g. 1536 for OpenAI text-embedding-3-small, 384
-- for all-MiniLM-L6-v2), change BOTH the vector(N) width here and
-- EMBEDDING_DIMENSIONS to match, then re-embed. A pgvector column type change is
-- incompatible with an existing index, so the HNSW index is dropped first and
-- recreated after; on an empty table the recreate is instant.
TRUNCATE embeddings;

DROP INDEX IF EXISTS idx_embeddings_vector_cosine;

ALTER TABLE embeddings ALTER COLUMN vector_embeddings TYPE vector(768);

CREATE INDEX idx_embeddings_vector_cosine ON embeddings USING hnsw (vector_embeddings vector_cosine_ops);
