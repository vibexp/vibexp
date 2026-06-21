-- Create integrations catalog table
CREATE TABLE api_key_integrations_catalog (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    integration_code VARCHAR(50) UNIQUE NOT NULL,
    integration_name VARCHAR(100) NOT NULL,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create index on integration_code for faster lookups
CREATE INDEX idx_integrations_catalog_code ON api_key_integrations_catalog(integration_code);

-- Seed initial integrations (only user-facing integrations)
INSERT INTO api_key_integrations_catalog (integration_code, integration_name, description) VALUES
('ai_tools', 'AI Tools Integration', 'Access for Claude Code, Cursor IDE, and other AI-powered development tools'),
('cli', 'VibeXP CLI', 'Access for VibeXP command-line interface'),
('mcp_server', 'MCP Server', 'Access for Model Context Protocol server endpoints'),
('marketplace', 'Claude Plugin Marketplace', 'Access for Claude Plugin Marketplace APIs');
