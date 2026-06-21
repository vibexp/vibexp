-- Table: claude_plugin_marketplaces
-- Stores user-created plugin marketplaces
CREATE TABLE claude_plugin_marketplaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    marketplace_id VARCHAR(36) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    visibility VARCHAR(20) NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
    category VARCHAR(100) DEFAULT '',
    api_key_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}'
);

-- Indexes for claude_plugin_marketplaces
CREATE INDEX idx_claude_plugin_marketplaces_user_id ON claude_plugin_marketplaces(user_id);
CREATE INDEX idx_claude_plugin_marketplaces_marketplace_id ON claude_plugin_marketplaces(marketplace_id);
CREATE INDEX idx_claude_plugin_marketplaces_visibility ON claude_plugin_marketplaces(visibility);
CREATE INDEX idx_claude_plugin_marketplaces_category ON claude_plugin_marketplaces(category);
CREATE INDEX idx_claude_plugin_marketplaces_is_active ON claude_plugin_marketplaces(is_active);
CREATE INDEX idx_claude_plugin_marketplaces_created_at ON claude_plugin_marketplaces(created_at DESC);
CREATE INDEX idx_claude_plugin_marketplaces_metadata ON claude_plugin_marketplaces USING GIN(metadata);

-- Table: claude_plugin_marketplace_assignments
-- Maps specs from spec_library to marketplaces
CREATE TABLE claude_plugin_marketplace_assignments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    marketplace_id UUID NOT NULL REFERENCES claude_plugin_marketplaces(id) ON DELETE CASCADE,
    spec_id UUID NOT NULL REFERENCES spec_library(id) ON DELETE CASCADE,
    plugin_id VARCHAR(100) NOT NULL,
    display_order INTEGER DEFAULT 0,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    is_active BOOLEAN DEFAULT true,
    UNIQUE(marketplace_id, spec_id)
);

-- Indexes for claude_plugin_marketplace_assignments
CREATE INDEX idx_claude_plugin_marketplace_assignments_marketplace_id ON claude_plugin_marketplace_assignments(marketplace_id);
CREATE INDEX idx_claude_plugin_marketplace_assignments_spec_id ON claude_plugin_marketplace_assignments(spec_id);
CREATE INDEX idx_claude_plugin_marketplace_assignments_plugin_id ON claude_plugin_marketplace_assignments(plugin_id);
CREATE INDEX idx_claude_plugin_marketplace_assignments_display_order ON claude_plugin_marketplace_assignments(display_order);
CREATE INDEX idx_claude_plugin_marketplace_assignments_is_active ON claude_plugin_marketplace_assignments(is_active);

-- Table: claude_plugin_installations
-- Tracks plugin installations from marketplaces
CREATE TABLE claude_plugin_installations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    marketplace_id UUID NOT NULL REFERENCES claude_plugin_marketplaces(id) ON DELETE CASCADE,
    plugin_id VARCHAR(100) NOT NULL,
    installed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    user_identifier VARCHAR(255),
    os VARCHAR(50),
    claude_code_version VARCHAR(50),
    metadata JSONB DEFAULT '{}'
);

-- Indexes for claude_plugin_installations
CREATE INDEX idx_claude_plugin_installations_marketplace_id ON claude_plugin_installations(marketplace_id);
CREATE INDEX idx_claude_plugin_installations_plugin_id ON claude_plugin_installations(plugin_id);
CREATE INDEX idx_claude_plugin_installations_installed_at ON claude_plugin_installations(installed_at DESC);
CREATE INDEX idx_claude_plugin_installations_os ON claude_plugin_installations(os);
CREATE INDEX idx_claude_plugin_installations_metadata ON claude_plugin_installations USING GIN(metadata);

-- Table: claude_plugin_ratings
-- Stores user ratings and reviews for plugins
CREATE TABLE claude_plugin_ratings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    marketplace_id UUID NOT NULL REFERENCES claude_plugin_marketplaces(id) ON DELETE CASCADE,
    plugin_id VARCHAR(100) NOT NULL,
    rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    review TEXT DEFAULT '',
    user_identifier VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    metadata JSONB DEFAULT '{}'
);

-- Indexes for claude_plugin_ratings
CREATE INDEX idx_claude_plugin_ratings_marketplace_id ON claude_plugin_ratings(marketplace_id);
CREATE INDEX idx_claude_plugin_ratings_plugin_id ON claude_plugin_ratings(plugin_id);
CREATE INDEX idx_claude_plugin_ratings_rating ON claude_plugin_ratings(rating);
CREATE INDEX idx_claude_plugin_ratings_created_at ON claude_plugin_ratings(created_at DESC);
CREATE INDEX idx_claude_plugin_ratings_metadata ON claude_plugin_ratings USING GIN(metadata);
