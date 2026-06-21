-- Create the resource-type-agnostic `types` table backing user/team
-- customizable category taxonomies for resources. Keyed by (resource_type,
-- slug) within a team so future resource types (prompt, memory, blueprint) can
-- adopt custom types with no schema change — mirrors the
-- attachments(owner_type, owner_id) and embeddings(entity_type, entity_id)
-- polymorphic conventions.
--
-- System default types are GLOBAL rows (team_id IS NULL, is_system = TRUE) and
-- are visible to every team; team-owned custom types carry a team_id and
-- is_system = FALSE. Because PostgreSQL treats NULL as distinct in unique
-- constraints, global-row and team-row uniqueness are each enforced by a
-- separate partial unique index rather than one composite constraint.
CREATE TABLE types (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       UUID REFERENCES teams(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    slug          VARCHAR(255) NOT NULL,
    name          VARCHAR(255) NOT NULL,
    is_system     BOOLEAN NOT NULL DEFAULT FALSE,
    created_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Global system types: unique per (resource_type, slug) when not team-scoped.
CREATE UNIQUE INDEX idx_types_global_unique
    ON types (resource_type, slug)
    WHERE team_id IS NULL;

-- Team custom types: unique per (team_id, resource_type, slug).
CREATE UNIQUE INDEX idx_types_team_unique
    ON types (team_id, resource_type, slug)
    WHERE team_id IS NOT NULL;

-- Primary list access path: every type for a resource within a team (the list
-- query unions the global rows with the team's own rows).
CREATE INDEX idx_types_team_resource ON types (team_id, resource_type);

COMMENT ON TABLE types IS 'Resource-type-agnostic, team-customizable category taxonomy keyed by (resource_type, slug); global system rows have team_id NULL';
COMMENT ON COLUMN types.team_id IS 'Owning team; NULL for global system defaults visible to all teams';
COMMENT ON COLUMN types.resource_type IS 'Polymorphic resource type the type applies to: artifacts (future: prompt, memory, blueprint)';
COMMENT ON COLUMN types.is_system IS 'TRUE for built-in defaults that cannot be edited or deleted by users';
COMMENT ON COLUMN types.created_by IS 'User who created the custom type; NULL for system defaults and after the creator is deleted';

-- Seed the three artifact system defaults as global rows. The two underscored
-- slugs are renamed to their hyphenated form here (general is unchanged).
INSERT INTO types (team_id, resource_type, slug, name, is_system) VALUES
    (NULL, 'artifacts', 'general',         'General',         TRUE),
    (NULL, 'artifacts', 'work-reports',    'Work reports',    TRUE),
    (NULL, 'artifacts', 'static-contexts', 'Static contexts', TRUE);

-- Migrate existing artifact type values to the hyphenated default slugs. The
-- underscored values predate the customizable-types system; general unchanged.
UPDATE artifacts SET type = 'work-reports'    WHERE type = 'work_reports';
UPDATE artifacts SET type = 'static-contexts' WHERE type = 'static_contexts';
