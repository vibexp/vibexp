-- Table: teams
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(50) NOT NULL,
    description TEXT DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT teams_owner_slug_unique UNIQUE (owner_id, slug)
);

-- Indexes
CREATE INDEX idx_teams_owner_id ON teams(owner_id);
CREATE INDEX idx_teams_slug ON teams(slug);
CREATE INDEX idx_teams_created_at ON teams(created_at DESC);

-- Add default_team_id to users table
ALTER TABLE users ADD COLUMN default_team_id UUID REFERENCES teams(id) ON DELETE SET NULL;
CREATE INDEX idx_users_default_team_id ON users(default_team_id);
