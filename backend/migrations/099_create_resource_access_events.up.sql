-- Create resource_access_events table for tracking resource detail-access events
CREATE TABLE resource_access_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    resource_type TEXT NOT NULL,
    resource_id   UUID NOT NULL,
    source        TEXT NOT NULL,
    api_key_id    UUID,
    user_agent    TEXT,
    source_ip     INET,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Create indexes for optimal query performance
CREATE INDEX idx_rae_resource_created   ON resource_access_events (team_id, resource_type, resource_id, created_at DESC);
CREATE INDEX idx_rae_created_at         ON resource_access_events (created_at);
CREATE INDEX idx_rae_team_type_created  ON resource_access_events (team_id, resource_type, created_at DESC);

-- Add comments for documentation
COMMENT ON TABLE resource_access_events IS 'Records detail-access events for team resources (e.g. prompt/agent/artifact opens) for access analytics';
COMMENT ON COLUMN resource_access_events.team_id IS 'Team that owns the accessed resource';
COMMENT ON COLUMN resource_access_events.user_id IS 'User who accessed the resource; NULL when the user is later deleted';
COMMENT ON COLUMN resource_access_events.resource_type IS 'Type of resource accessed: prompt, agent, artifact, etc.';
COMMENT ON COLUMN resource_access_events.resource_id IS 'ID of the specific resource accessed';
COMMENT ON COLUMN resource_access_events.source IS 'Origin of the access: web, cli, mcp, etc.';
COMMENT ON COLUMN resource_access_events.api_key_id IS 'API key used for the access, when the access was authenticated via API key';
COMMENT ON COLUMN resource_access_events.user_agent IS 'User agent string of the client that performed the access';
COMMENT ON COLUMN resource_access_events.source_ip IS 'Source IP address of the client that performed the access';
