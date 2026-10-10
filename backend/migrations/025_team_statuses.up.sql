-- ===========================================================================
-- Team-defined custom statuses -- storage foundation (issue #1264, epic #1263).
--
-- Until now a resource's status was a closed, code-defined set per resource
-- type (models.PromptStatuses and friends in internal/models/status.go). This
-- migration adds the table a team's OWN statuses live in, and removes the two
-- things in the schema that made a custom status unstorable: the CHECK
-- constraints on prompts and memories, and the varchar(20) cap on all four
-- resource tables.
--
-- The SYSTEM statuses are deliberately not seeded here. They stay in Go, so
-- there is one definition of them and no row a team could edit or delete; the
-- catalog service composes the system subset with this table's rows.
-- ===========================================================================

CREATE TABLE team_statuses (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    resource_type varchar(20) NOT NULL,
    slug          varchar(50) NOT NULL,
    name          varchar(50) NOT NULL,
    created_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT team_statuses_team_resource_slug_key UNIQUE (team_id, resource_type, slug)
);

COMMENT ON TABLE team_statuses IS
    'A team''s custom resource statuses (epic #1263). System statuses are not stored here: they are defined in Go (internal/models/status.go).';
COMMENT ON COLUMN team_statuses.resource_type IS
    'Singular resource type the status applies to: prompt, artifact, blueprint or memory. Validated in the service layer, not by a CHECK (same convention as team_settings_audit.surface).';
COMMENT ON COLUMN team_statuses.slug IS
    'The value written to the resource''s status column. Unique per (team_id, resource_type); that unique index also serves the per-team, per-type list.';
COMMENT ON COLUMN team_statuses.created_by IS
    'The member who created the status. SET NULL on user delete: the status outlives its author.';

-- A custom status must be storable on every resource type. prompts and
-- memories were the two tables whose CHECK admitted only the system set
-- (prompts_status_check from 001_baseline, memories_status_check from
-- 002_consolidated); artifacts and blueprints never had one. From here on the
-- service layer is the only gate, as it always was for those two.
ALTER TABLE prompts DROP CONSTRAINT IF EXISTS prompts_status_check;
ALTER TABLE memories DROP CONSTRAINT IF EXISTS memories_status_check;

-- Widen status to the length of team_statuses.slug. varchar(20) -> varchar(50)
-- only raises the length limit, which Postgres applies as a catalog change: no
-- table rewrite, no index rebuild, and no row trigger fires, so existing rows
-- (and their updated_at) are untouched.
ALTER TABLE prompts ALTER COLUMN status TYPE varchar(50);
ALTER TABLE artifacts ALTER COLUMN status TYPE varchar(50);
ALTER TABLE blueprints ALTER COLUMN status TYPE varchar(50);
ALTER TABLE memories ALTER COLUMN status TYPE varchar(50);
