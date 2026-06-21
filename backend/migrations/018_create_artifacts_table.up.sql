CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_name VARCHAR(80) NOT NULL DEFAULT 'shared',
    slug VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    title VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    type VARCHAR(50) NOT NULL DEFAULT 'general',
    metadata JSONB DEFAULT '{}',
    UNIQUE(project_name, slug, user_id)
);

-- Indexes for performance
CREATE INDEX idx_artifacts_user_id ON artifacts(user_id);
CREATE INDEX idx_artifacts_project_name ON artifacts(project_name);
CREATE INDEX idx_artifacts_slug ON artifacts(slug);
CREATE INDEX idx_artifacts_created_at ON artifacts(created_at DESC);
CREATE INDEX idx_artifacts_updated_at ON artifacts(updated_at DESC);
CREATE INDEX idx_artifacts_status ON artifacts(status);
CREATE INDEX idx_artifacts_type ON artifacts(type);
CREATE INDEX idx_artifacts_metadata ON artifacts USING GIN(metadata);
CREATE INDEX idx_artifacts_project_slug ON artifacts(project_name, slug);
