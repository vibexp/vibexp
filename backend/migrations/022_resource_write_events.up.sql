-- Record how a resource write arrived (#1392).
--
-- resource_access_events already says which channel READ a resource. Nothing
-- said which channel WROTE one: content_versions.actor_type is only
-- 'human' or 'system', so an agent writing over MCP was stored as 'human',
-- and the creation metrics count by resource type alone.
--
-- resource_write_events is the write-side twin of resource_access_events: one
-- append-only row per successful create or update of a prompt, artifact,
-- blueprint or memory. content_versions cannot carry the metric by itself. It
-- gets a row only when an update changes the content, that row holds the PRIOR
-- content, a create writes none, and rows are pruned to a retention cap.
--
-- source uses the vocabulary of resource_access_events.source. There is no
-- 'unknown' value and no backfill: a resource with no create event was written
-- before this migration, and the metrics report it as unknown. Guessing 'web'
-- for old rows would corrupt the number this table exists to make true.
--
-- assistant_name is the name an MCP client reported for itself
-- (clientInfo.name), trimmed and capped by the server. It is a label, not an
-- identity. It is NULL for every write that is not MCP and for MCP sessions
-- that reported none.
--
-- project_id and api_key_id carry no foreign key on purpose: the row is a
-- record of what happened, and it must outlive a deleted project or a revoked
-- key. team_id cascades (a deleted team takes its history with it) and user_id
-- is set to NULL, as on resource_access_events.
CREATE TABLE resource_write_events (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    project_id     uuid,
    user_id        uuid REFERENCES users (id) ON DELETE SET NULL,
    resource_type  text NOT NULL,
    resource_id    uuid NOT NULL,
    action         text NOT NULL,
    source         text NOT NULL,
    assistant_name varchar(30),
    api_key_id     uuid,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT resource_write_events_resource_type_check
        CHECK (resource_type IN ('prompt', 'artifact', 'blueprint', 'memory')),
    CONSTRAINT resource_write_events_action_check
        CHECK (action IN ('create', 'update')),
    CONSTRAINT resource_write_events_source_check
        CHECK (source IN ('web', 'cli', 'mcp', 'api'))
);

-- The per-source write totals read one team's events for a time window.
CREATE INDEX idx_resource_write_events_team_created
    ON resource_write_events (team_id, created_at DESC);

-- The creation metrics LEFT JOIN each resource to its create event. Unique, so
-- the join can never fan a resource out into two rows and the by-source
-- breakdown sums to the existing totals by construction.
CREATE UNIQUE INDEX idx_resource_write_events_create
    ON resource_write_events (resource_type, resource_id)
    WHERE action = 'create';

-- The same two facts on a version snapshot, for the version history. Both are
-- nullable with no DEFAULT, so the ALTER is a catalog-only change and every
-- version written before this migration reads NULL (unknown), never 'web'.
ALTER TABLE content_versions
    ADD COLUMN source text,
    ADD COLUMN assistant_name varchar(30),
    ADD CONSTRAINT content_versions_source_check
        CHECK (source IS NULL OR source IN ('web', 'cli', 'mcp', 'api'));
