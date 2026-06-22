-- Standardize the embeddings vector dimension on 1024 for the native, pluggable
-- embedding subsystem (issue #6). Embeddings are now produced by an
-- operator-configured, OpenAI-compatible provider instead of the removed external
-- ai-service, so the previous gemini-embedding-001 (768-dim) vectors are no longer
-- valid and must be regenerated under the new provider.
--
-- Why 1024: it is the broadest OpenAI-compatible embedding width — native for
-- Bedrock Titan V2, Cohere v3, Mistral, mxbai-embed-large and bge-large, and
-- requestable from the Matryoshka models (OpenAI text-embedding-3-*, Gemini
-- gemini-embedding-001) via the request's `dimensions` field. The width is fixed
-- (not an end-user knob): the schema ships in the image, so operators pick an
-- embedding model that outputs 1024 dimensions rather than changing this value.
--
-- The embeddings table is a pure derived cache (every save is delete-then-insert),
-- so it is fully regenerable from the source entities. TRUNCATE first because an
-- ALTER COLUMN ... TYPE vector(N) cannot rewrite rows of a different width, and
-- mixing vectors from two models in one index would corrupt similarity search.
-- After this migration, search is degraded until embeddings are regenerated via
-- the backfill endpoint (POST /bo/v1/embeddings/backfill). A pgvector column type
-- change is incompatible with an existing index, so the HNSW index is dropped
-- first and recreated after; on an empty table the recreate is instant.
TRUNCATE embeddings;

DROP INDEX IF EXISTS idx_embeddings_vector_cosine;

ALTER TABLE embeddings ALTER COLUMN vector_embeddings TYPE vector(1024);

CREATE INDEX idx_embeddings_vector_cosine ON embeddings USING hnsw (vector_embeddings vector_cosine_ops);
