-- Create junction table for many-to-many relationship
CREATE TABLE api_key_integration_permissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    integration_code VARCHAR(50) NOT NULL REFERENCES api_key_integrations_catalog(integration_code) ON DELETE CASCADE,
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(api_key_id, integration_code)
);

-- Create indexes for performance
CREATE INDEX idx_api_key_permissions_key_id ON api_key_integration_permissions(api_key_id);
CREATE INDEX idx_api_key_permissions_integration ON api_key_integration_permissions(integration_code);
