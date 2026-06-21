-- Create content_versions table for generic, polymorphic content-version history
CREATE TABLE content_versions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    resource_type  TEXT NOT NULL,
    resource_id    UUID NOT NULL,
    version_number INT  NOT NULL,
    content        TEXT NOT NULL,
    created_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (resource_type, resource_id, version_number)
);

-- Index for newest-first listing and version lookups per resource
CREATE INDEX idx_content_versions_resource ON content_versions (resource_type, resource_id, version_number DESC);

-- Add comments for documentation
COMMENT ON TABLE content_versions IS 'Polymorphic content-version snapshots for team resources (e.g. artifacts), keyed by (resource_type, resource_id)';
COMMENT ON COLUMN content_versions.team_id IS 'Team that owns the versioned resource';
COMMENT ON COLUMN content_versions.resource_type IS 'Type of the versioned resource: artifact, etc.';
COMMENT ON COLUMN content_versions.resource_id IS 'ID of the specific resource the snapshot belongs to';
COMMENT ON COLUMN content_versions.version_number IS 'Monotonic per-resource version number, computed at insert time';
COMMENT ON COLUMN content_versions.content IS 'Snapshot of the resource content at this version';
COMMENT ON COLUMN content_versions.created_by IS 'User who triggered the snapshot; NULL when the user is later deleted';
