-- Create the generic, polymorphic attachments table backing file attachments
-- for resources. Keyed by (owner_type, owner_id) so future resource types
-- (memory, blueprint, prompt) can adopt attachments with no schema change —
-- this mirrors the embeddings(entity_type, entity_id) and
-- resource_access_events(resource_type, resource_id) polymorphic conventions.
CREATE TABLE attachments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    owner_type     TEXT NOT NULL,
    owner_id       UUID NOT NULL,
    file_name      TEXT NOT NULL,
    content_type   TEXT NOT NULL,
    size_bytes     BIGINT NOT NULL,
    gcs_object_key TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary access path: list/sum attachments for a given owner within a team.
CREATE INDEX idx_attachments_owner ON attachments (team_id, owner_type, owner_id);

COMMENT ON TABLE attachments IS 'Generic file attachments for resources, keyed polymorphically by (owner_type, owner_id); binary stored in GCS';
COMMENT ON COLUMN attachments.team_id IS 'Team that owns the attachment; cascade-deletes with the team';
COMMENT ON COLUMN attachments.user_id IS 'User who uploaded the attachment; NULL when the user is later deleted';
COMMENT ON COLUMN attachments.owner_type IS 'Polymorphic owner type: artifact (future: memory, blueprint, prompt)';
COMMENT ON COLUMN attachments.owner_id IS 'Polymorphic owner ID; intentionally no FK (cf. embeddings) — cleanup is app-level';
COMMENT ON COLUMN attachments.gcs_object_key IS 'Object key in the GCS attachments bucket: {team_id}/{owner_type}/{owner_id}/{uuid}-{filename}';
