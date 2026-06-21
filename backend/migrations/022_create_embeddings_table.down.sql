DROP TRIGGER IF EXISTS update_embeddings_updated_at ON embeddings;
DROP INDEX IF EXISTS idx_embeddings_vector_cosine;
DROP INDEX IF EXISTS idx_embeddings_user_id;
DROP INDEX IF EXISTS idx_embeddings_entity_type_id;
DROP TABLE IF EXISTS embeddings;
