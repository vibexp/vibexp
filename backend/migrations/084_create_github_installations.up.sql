-- Create github_installations table to store GitHub App installation data
CREATE TABLE IF NOT EXISTS github_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    installation_id BIGINT NOT NULL UNIQUE,
    account_login VARCHAR(255) NOT NULL,
    account_type VARCHAR(50) NOT NULL,
    target_type VARCHAR(50) NOT NULL,
    encrypted_access_token TEXT NOT NULL,
    token_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    permissions JSONB DEFAULT '{}',
    events TEXT[] DEFAULT '{}',
    suspended_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT unique_team_installation UNIQUE (team_id, installation_id)
);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_github_installations_team_id ON github_installations(team_id);
CREATE INDEX IF NOT EXISTS idx_github_installations_installation_id ON github_installations(installation_id);

-- Create github_installation_repositories table to track selected repositories
CREATE TABLE IF NOT EXISTS github_installation_repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    installation_id UUID NOT NULL REFERENCES github_installations(id) ON DELETE CASCADE,
    repository_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    full_name VARCHAR(500) NOT NULL,
    private BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT unique_installation_repository UNIQUE (installation_id, repository_id)
);

-- Create index for faster repository lookups
CREATE INDEX IF NOT EXISTS idx_github_installation_repositories_installation_id ON github_installation_repositories(installation_id);
