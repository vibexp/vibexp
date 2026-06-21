-- Enable pgvector extension if not already enabled
CREATE EXTENSION IF NOT EXISTS vector;

-- Create embeddings table
CREATE TABLE embeddings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID NOT NULL,
    vector_embeddings vector(384) NOT NULL,
    model_id VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for optimal query performance
CREATE INDEX idx_embeddings_entity_type_id ON embeddings(entity_type, entity_id);
CREATE INDEX idx_embeddings_user_id ON embeddings(user_id);

-- Vector index using HNSW for similarity search
CREATE INDEX idx_embeddings_vector_cosine ON embeddings USING hnsw (vector_embeddings vector_cosine_ops);

-- Trigger for auto-updating updated_at
CREATE TRIGGER update_embeddings_updated_at
    BEFORE UPDATE ON embeddings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
