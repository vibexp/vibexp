-- Consolidated post-v0.10.0 migration.
--
-- Squashes the migrations that accumulated after the v0.10.0 release into a
-- single step, applied on top of 012, mirroring the 002/006/011
-- consolidations (#76, #399, #710). Merged here (in original order):
--   * 013_resource_freshness                        (issue #729, epic #726)
--   * 014_memories_updated_at_ignores_last_accessed (issue #730, epic #726)
--   * 015_seed_freshness_evaluate_schedules         (issue #732, epic #726)  -- up-block dropped, see below
--   * 016_team_project_search                       (issue #813, epic #811)
-- No deployed instance has applied any of them (none shipped in a release),
-- so renumbering is safe.
--
-- Unlike the 011 consolidation, these are NOT all disjoint, and one block did
-- need reconciling:
--
--   * 013 and 014 are ordered, not independent: 014 narrows the
--     `update_memories_updated_at` trigger with a WHEN clause naming the
--     `last_accessed_*` columns 013 adds. Carried verbatim in original order,
--     which preserves that dependency; the down-block reverses it.
--
--   * 015's up-block is DROPPED. It was a *repair* migration -- it back-filled
--     a `freshness_evaluate` schedule row for teams whose `freshness_rules`
--     had been created by #731 before #732 shipped, because only the service
--     write path creates that row and such a team might never write again. On
--     this chain `freshness_rules` is CREATED EMPTY by the block immediately
--     above, so its `INSERT ... SELECT FROM freshness_rules` can never match a
--     row: it is provably a no-op, and keeping dead SQL in a file operators
--     read would only mislead. The state it repaired cannot exist here.
--     Its DOWN-block is KEPT (see 013_consolidated.down.sql) and is still
--     load-bearing: the service writes `freshness_evaluate` rows at runtime,
--     so rolling back past 013 must still remove them.
--
--   * 016 is independent of the other three (indexes on teams/projects).

-- ===========================================================================
-- 013_resource_freshness (#729)
-- ===========================================================================

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

-- ===========================================================================
-- 014_memories_updated_at_ignores_last_accessed (#730)
-- ===========================================================================

-- ===========================================================================
-- Stop `memories.updated_at` from moving when only a last-accessed column is
-- written (issue #730, epic #726).
--
-- Of the four resource tables, `memories` is the ONLY one carrying a
-- BEFORE UPDATE trigger (`update_memories_updated_at`, from 001_baseline).
-- Its function sets `NEW.updated_at = CURRENT_TIMESTAMP` unconditionally, so
-- ANY update to the row moves updated_at -- including the per-medium
-- last-accessed write #730 performs on every detail read.
--
-- That would make a READ indistinguishable from an EDIT, and updated_at is
-- load-bearing in at least three places:
--   * search recency ranking weights it (`rank_weight_updated`), so merely
--     reading a memory would push it up the results;
--   * the freshness reversibility rules (#733) treat an edit as a distinct
--     signal from an access;
--   * the UI reports it as "last updated" to humans.
-- prompts/artifacts/blueprints have no such trigger and are unaffected.
--
-- The fix is a WHEN clause rather than dropping the trigger. Dropping it looks
-- tempting -- `memory.go`'s UPDATE already passes `updated_at = $7` explicitly,
-- so the column appears app-managed -- but that value is currently OVERWRITTEN
-- by this trigger on every edit, meaning the trigger, not the application, is
-- the de-facto writer. Removing it would silently promote whatever the service
-- happens to pass (possibly a zero time) to the stored value. Narrowing when
-- the trigger fires changes nothing about existing edits.
--
-- The predicate reads "fire only when no last-accessed column changed":
-- every pre-existing write path touches content columns and never these four,
-- so it behaves exactly as before; the #730 path touches only these four and
-- is therefore skipped. Enumerating the last-accessed columns rather than the
-- content columns also keeps this future-proof -- a new content column added
-- to `memories` later still bumps updated_at with no further migration.
-- ===========================================================================

DROP TRIGGER IF EXISTS update_memories_updated_at ON memories;

CREATE TRIGGER update_memories_updated_at
    BEFORE UPDATE ON memories
    FOR EACH ROW
    WHEN (
        OLD.last_accessed_web_at IS NOT DISTINCT FROM NEW.last_accessed_web_at
    AND OLD.last_accessed_cli_at IS NOT DISTINCT FROM NEW.last_accessed_cli_at
    AND OLD.last_accessed_mcp_at IS NOT DISTINCT FROM NEW.last_accessed_mcp_at
    AND OLD.last_accessed_api_at IS NOT DISTINCT FROM NEW.last_accessed_api_at
    )
    EXECUTE FUNCTION public.update_updated_at_column();

COMMENT ON TRIGGER update_memories_updated_at ON memories IS
    'Maintains updated_at on edits. Deliberately skipped when an update touches only the last_accessed_* columns (#730), so a read never looks like an edit.';

-- ===========================================================================
-- 016_team_project_search (#813)
-- ===========================================================================

-- ===========================================================================
-- Keyword search indexes for teams and projects (issue #813, epic #811).
--
-- Teams had no search path at all, and project search was `ILIKE '%term%'` on
-- name/description/slug -- a leading wildcard, so `idx_projects_name` could not
-- serve it and every search sequentially scanned the table, ordered by
-- created_at rather than relevance, with no typo tolerance.
--
-- These four indexes back the ranking ladder in postgres/entity_search.go,
-- which mirrors the one SearchKeyword already runs over the four resource
-- domains: strict FTS -> relaxed (OR-rewritten) FTS -> pg_trgm word similarity.
--
-- pg_trgm is ALREADY installed by migration 005, so there is no CREATE
-- EXTENSION here and no new operator action for self-hosters on managed
-- Postgres.
--
-- INDEX EXPRESSIONS MUST STAY BYTE-IDENTICAL TO THE EXPRESSIONS THE QUERIES
-- EMIT, or the planner silently ignores them and every pass seq-scans. The Go
-- side renders these same strings (entity_search.go: ftsMatchExpr / trgmNameExpr)
-- and team_project_search_integration_test.go asserts the plans actually use
-- them. Column qualification is the one permitted difference: the queries say
-- `t.name` / `p.name` where these say `name`, and Postgres resolves both to the
-- same column (the same convention as the 005 trigram indexes).
--
-- The FTS expression deliberately covers name + description only. `git_url` is
-- matched EXACTLY (score 1.0) instead of through FTS: the english parser turns
-- a repository URL into url/host tokens
-- ('github.com/shaharia-lab/games-for-agents', 'github.com'), which no
-- word-shaped query matches, so indexing it for FTS would cost index size and
-- buy nothing.
-- ===========================================================================

CREATE INDEX IF NOT EXISTS idx_teams_fts ON teams
    USING gin (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, '')));

CREATE INDEX IF NOT EXISTS idx_projects_fts ON projects
    USING gin (to_tsvector('english', coalesce(name, '') || ' ' || coalesce(description, '')));

-- Trigram indexes on the NAME only: a typo is tolerated against the short,
-- high-signal name, never the body (whole-document similarity dilutes to near
-- zero on long text). This is also the pass that carries slash/hyphen-heavy
-- project names -- the english parser tokenizes `shaharia-lab/games-for-agents`
-- as a file path (`/games-for-agents`), so neither FTS pass matches a query of
-- "games for agents" at all, while word_similarity scores it 1.0.
CREATE INDEX IF NOT EXISTS idx_teams_name_trgm ON teams
    USING gin ((coalesce(name, '')) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_projects_name_trgm ON projects
    USING gin ((coalesce(name, '')) gin_trgm_ops);
