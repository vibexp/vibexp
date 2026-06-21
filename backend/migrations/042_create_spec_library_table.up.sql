CREATE TABLE spec_library (
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
CREATE INDEX idx_spec_library_user_id ON spec_library(user_id);
CREATE INDEX idx_spec_library_project_name ON spec_library(project_name);
CREATE INDEX idx_spec_library_slug ON spec_library(slug);
CREATE INDEX idx_spec_library_created_at ON spec_library(created_at DESC);
CREATE INDEX idx_spec_library_updated_at ON spec_library(updated_at DESC);
CREATE INDEX idx_spec_library_status ON spec_library(status);
CREATE INDEX idx_spec_library_type ON spec_library(type);
CREATE INDEX idx_spec_library_metadata ON spec_library USING GIN(metadata);
CREATE INDEX idx_spec_library_project_slug ON spec_library(project_name, slug);
