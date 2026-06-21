CREATE TABLE embedding_providers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    provider_type VARCHAR(100) NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    base_url VARCHAR(500),
    api_key_encrypted TEXT,
    configuration JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT unique_user_provider_name UNIQUE (user_id, name)
);

CREATE UNIQUE INDEX idx_embedding_providers_user_default ON embedding_providers(user_id) WHERE is_default = true;
CREATE INDEX idx_embedding_providers_user_id ON embedding_providers(user_id);
CREATE INDEX idx_embedding_providers_type ON embedding_providers(provider_type);
