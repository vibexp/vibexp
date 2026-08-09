-- ===========================================================================
-- Resource Freshness: schema foundation (issue #729, epic #726).
--
-- Four new tables plus per-medium last-accessed columns on the four resource
-- tables. Nothing writes to any of this yet: the access-path denormalization
-- is #730, the rule engine is #732/#733, the HTTP surface is #731/#734.
--
-- Why a dedicated state table rather than the resources' JSONB `metadata`:
-- `artifacts`, `blueprints` and `memories` have a `metadata` column but
-- `prompts` does NOT. A system-owned table covers all four resource types
-- uniformly, keeps machine-written state out of user-editable metadata, and
-- makes reversal and auditing trivial.
--
-- Start clean (decision #7): every column added here begins NULL/empty. There
-- is deliberately NO backfill from `resource_access_events` -- that table is
-- pruned on a retention TTL (retention.access_event_days, default 90), so
-- seeding from it would produce a partial, silently-wrong history. Rules stay
-- quiet until post-deploy access data accrues.
--
-- Enum-ish text columns (status, action, reason, resource_types, mediums)
-- carry no CHECK constraints on purpose: the valid sets are owned by the
-- service layer (#731), so extending them later never needs a migration. The
-- constraints that ARE enforced here are true storage invariants (the 1-hour
-- interval floor, a positive threshold, a non-empty resource_types).
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- resource_freshness -- system-owned stale state, one row per resource.
--
-- A row EXISTS only while the resource is stale; clearing staleness deletes
-- it (the audit table below is what preserves the history). project_id is
-- denormalized from the resource so "list stale for a team, filtered by type
-- and/or project" is an index scan instead of a four-way union join against
-- the resource tables.
-- ---------------------------------------------------------------------------
CREATE TABLE resource_freshness (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id          uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    project_id       uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    resource_type    text NOT NULL,
    resource_id      uuid NOT NULL,
    status           text NOT NULL,
    matched_rule_ids uuid[] NOT NULL DEFAULT '{}',
    since            timestamptz NOT NULL,
    reason           text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE resource_freshness IS
    'System-owned freshness state (epic #726). A row exists only while the resource is stale; clearing deletes it.';
COMMENT ON COLUMN resource_freshness.resource_id IS
    'Polymorphic reference into prompts/artifacts/blueprints/memories. Deliberately NO foreign key -- one column cannot reference four tables; cleanup is application-level, as for comments (#272) and relations (#421).';
COMMENT ON COLUMN resource_freshness.status IS
    'Current freshness state (today only ''stale''); the valid set lives in the service layer, not the schema.';
COMMENT ON COLUMN resource_freshness.matched_rule_ids IS
    'Union of the rules currently matching this resource (#732 semantics). Plain uuid[] with no foreign key on purpose: a hard FK would cascade-delete freshness rows when a rule is deleted. Deleting a rule strips its id here instead, and a row left matching no rule is removed.';
COMMENT ON COLUMN resource_freshness.since IS
    'When the resource was FIRST marked stale; preserved across re-evaluations that keep it stale.';
COMMENT ON COLUMN resource_freshness.reason IS
    'Provenance of the current state (e.g. ''rule_run''), mirroring resource_freshness_audit.reason.';

-- One state row per resource, whatever its team. Also the upsert conflict
-- target for the rule engine.
CREATE UNIQUE INDEX idx_resource_freshness_resource
    ON resource_freshness (resource_type, resource_id);

-- The two listing shapes: "stale in this team [of this type]" and "stale in
-- this team [in this project]". Two indexes rather than one composite because
-- a single (team_id, resource_type, project_id) index cannot serve a
-- project-only filter -- that would skip its second column.
--
-- The project index leads on project_id rather than team_id deliberately.
-- Both orders serve the team+project listing equally (both columns are
-- equality predicates), but only the project-first order also serves the
-- ON DELETE CASCADE from projects: Postgres needs an index on the REFERENCING
-- column to avoid a full scan when a project is deleted.
CREATE INDEX idx_resource_freshness_team_type
    ON resource_freshness (team_id, resource_type, since DESC);
CREATE INDEX idx_resource_freshness_project_team
    ON resource_freshness (project_id, team_id, since DESC);

-- Rule deletion has to find every row referencing the rule (#731 cleanup).
CREATE INDEX idx_resource_freshness_matched_rules
    ON resource_freshness USING gin (matched_rule_ids);

-- ---------------------------------------------------------------------------
-- freshness_rules -- the team's staleness policy.
-- ---------------------------------------------------------------------------
CREATE TABLE freshness_rules (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    project_id     uuid REFERENCES projects (id) ON DELETE CASCADE,
    resource_types text[] NOT NULL,
    mediums        text[] NOT NULL DEFAULT '{}',
    threshold_days integer NOT NULL,
    enabled        boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT freshness_rules_threshold_positive
        CHECK (threshold_days > 0),
    CONSTRAINT freshness_rules_resource_types_nonempty
        CHECK (cardinality(resource_types) > 0)
);

COMMENT ON TABLE freshness_rules IS
    'Per-team freshness rules (epic #726). Managed through the API added in #731; evaluated by #732.';
COMMENT ON COLUMN freshness_rules.project_id IS
    'Scope the rule to one project; NULL means ANY project in the team.';
COMMENT ON COLUMN freshness_rules.resource_types IS
    'Resource types the rule applies to (subset of artifact/prompt/blueprint/memory), validated in the service layer.';
COMMENT ON COLUMN freshness_rules.mediums IS
    'Access mediums that count as "accessed" for this rule; an EMPTY array means ANY medium.';
COMMENT ON COLUMN freshness_rules.threshold_days IS
    'Days without a qualifying access after which the resource is stale.';

-- Rule evaluation loads a team's enabled rules; the API lists all of them.
CREATE INDEX idx_freshness_rules_team_enabled
    ON freshness_rules (team_id, enabled);

-- Serves the ON DELETE CASCADE from projects. Without an index on the
-- referencing column, deleting a project scans this table in full.
CREATE INDEX idx_freshness_rules_project
    ON freshness_rules (project_id);

-- ---------------------------------------------------------------------------
-- team_freshness_settings -- one row per team, mirroring team_search_settings.
--
-- team_id is the PRIMARY KEY so the singleton is enforced by the schema.
-- An ABSENT row means "inherit the defaults", so DELETE is the reset path.
-- ---------------------------------------------------------------------------
CREATE TABLE team_freshness_settings (
    team_id               uuid PRIMARY KEY REFERENCES teams (id) ON DELETE CASCADE,
    interval_seconds      integer NOT NULL,
    reversibility_enabled boolean NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    version               bigint NOT NULL DEFAULT 1,

    CONSTRAINT team_freshness_settings_interval_floor
        CHECK (interval_seconds >= 3600)
);

COMMENT ON TABLE team_freshness_settings IS
    'Per-team freshness evaluation settings (epic #726). No row means the team inherits the defaults, so DELETE is "reset to defaults".';
COMMENT ON COLUMN team_freshness_settings.interval_seconds IS
    'How often the team''s rules are evaluated; storage-enforced floor of 3600s (1 hour), default daily. Matches the schedules floor (#727).';
COMMENT ON COLUMN team_freshness_settings.reversibility_enabled IS
    'Whether accessing or editing a stale resource clears its stale state (decision #6).';
COMMENT ON COLUMN team_freshness_settings.version IS
    'Optimistic-lock counter, incremented on every upsert (mirrors team_search_settings).';

-- No index beyond the primary key: the only query this table serves is
-- `WHERE team_id = $1`, which the PK already covers.

-- ---------------------------------------------------------------------------
-- resource_freshness_audit -- append-only mark/clear log.
-- ---------------------------------------------------------------------------
CREATE TABLE resource_freshness_audit (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id   uuid NOT NULL,
    rule_id       uuid,
    action        text NOT NULL,
    reason        text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE resource_freshness_audit IS
    'Append-only log of freshness marks and clears (epic #726). Rows are never updated or deleted except by team cascade.';
COMMENT ON COLUMN resource_freshness_audit.rule_id IS
    'The rule that caused a mark, when there was one. Deliberately NO foreign key: the log must survive the rule being deleted. NULL for clears not attributable to a rule.';
COMMENT ON COLUMN resource_freshness_audit.action IS
    'What happened: ''marked'' or ''cleared''. Validated in the service layer.';
COMMENT ON COLUMN resource_freshness_audit.reason IS
    'Why it happened: ''rule_run'', ''accessed'' or ''edited''. Validated in the service layer.';

-- The audit list is "this team's entries, newest first". `id DESC` is a
-- deterministic tiebreaker, and it is load-bearing rather than cosmetic:
-- `now()` is transaction-start time, so a rule run marking many resources in
-- one transaction writes an identical created_at to every row -- without the
-- tiebreaker their relative order is undefined and pagination can repeat or
-- skip entries.
CREATE INDEX idx_resource_freshness_audit_team_created
    ON resource_freshness_audit (team_id, created_at DESC, id DESC);

-- Per-resource history ("why is this stale?").
CREATE INDEX idx_resource_freshness_audit_resource
    ON resource_freshness_audit (resource_type, resource_id, created_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- Per-medium last-accessed columns on the four resource tables.
--
-- Denormalized from resource_access_events so rule evaluation is an indexed
-- column compare rather than an aggregate over an event log, and so a
-- threshold longer than the event retention window still has data to read.
-- #730 keeps them current on the async access path.
--
-- Explicit per-medium columns rather than a JSONB map: "any medium" is then a
-- plain GREATEST(...) over four columns, which is indexable and needs no
-- containment operators.
--
-- All four are timestamptz even though `memories.created_at`/`updated_at` are
-- naive `timestamp` -- these values are compared against now() by the rule
-- engine and must denote the same instant across all four resource tables.
--
-- Nullable with no DEFAULT, so each ALTER is a catalog-only change: Postgres
-- does not rewrite the table, which matters because all four are hot. NULL
-- means "never accessed", which #732 treats as eligible for staleness.
-- ---------------------------------------------------------------------------
ALTER TABLE prompts
    ADD COLUMN last_accessed_web_at timestamptz,
    ADD COLUMN last_accessed_cli_at timestamptz,
    ADD COLUMN last_accessed_mcp_at timestamptz,
    ADD COLUMN last_accessed_api_at timestamptz;

ALTER TABLE artifacts
    ADD COLUMN last_accessed_web_at timestamptz,
    ADD COLUMN last_accessed_cli_at timestamptz,
    ADD COLUMN last_accessed_mcp_at timestamptz,
    ADD COLUMN last_accessed_api_at timestamptz;

ALTER TABLE blueprints
    ADD COLUMN last_accessed_web_at timestamptz,
    ADD COLUMN last_accessed_cli_at timestamptz,
    ADD COLUMN last_accessed_mcp_at timestamptz,
    ADD COLUMN last_accessed_api_at timestamptz;

ALTER TABLE memories
    ADD COLUMN last_accessed_web_at timestamptz,
    ADD COLUMN last_accessed_cli_at timestamptz,
    ADD COLUMN last_accessed_mcp_at timestamptz,
    ADD COLUMN last_accessed_api_at timestamptz;

-- One comment per column: `\d+ prompts` documenting only the web column would
-- read as though the other three meant something different.
COMMENT ON COLUMN prompts.last_accessed_web_at IS
    'Last read through the web app; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN prompts.last_accessed_cli_at IS
    'Last read through the VibeXP CLI; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN prompts.last_accessed_mcp_at IS
    'Last read through the MCP server; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN prompts.last_accessed_api_at IS
    'Last read through any other API consumer; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';

COMMENT ON COLUMN artifacts.last_accessed_web_at IS
    'Last read through the web app; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN artifacts.last_accessed_cli_at IS
    'Last read through the VibeXP CLI; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN artifacts.last_accessed_mcp_at IS
    'Last read through the MCP server; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN artifacts.last_accessed_api_at IS
    'Last read through any other API consumer; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';

COMMENT ON COLUMN blueprints.last_accessed_web_at IS
    'Last read through the web app; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN blueprints.last_accessed_cli_at IS
    'Last read through the VibeXP CLI; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN blueprints.last_accessed_mcp_at IS
    'Last read through the MCP server; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN blueprints.last_accessed_api_at IS
    'Last read through any other API consumer; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';

COMMENT ON COLUMN memories.last_accessed_web_at IS
    'Last read through the web app; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN memories.last_accessed_cli_at IS
    'Last read through the VibeXP CLI; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN memories.last_accessed_mcp_at IS
    'Last read through the MCP server; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
COMMENT ON COLUMN memories.last_accessed_api_at IS
    'Last read through any other API consumer; NULL means never. Denormalized from resource_access_events by #730; not backfilled.';
