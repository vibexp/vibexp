-- Resolve a resource's most recent write event without a per-row scan (#1418).
--
-- The list and detail reads of prompts, artifacts, blueprints and memories now
-- report last_write_source: for each resource row, the source of its newest
-- resource_write_events row. Migration 022 indexed only the create event
-- (idx_resource_write_events_create, partial on action = 'create') and the
-- team's window (team_id, created_at), so "the latest event of this resource",
-- which is as often an update, had no index and would have filtered the whole
-- team's event log once per listed row.
--
-- (resource_type, resource_id, created_at DESC) makes that lookup one index
-- descent per row that stops at the first entry. source and assistant_name are
-- INCLUDEd because they are the only two columns the reads project, so the
-- lookup is index-only and never visits the heap.
CREATE INDEX idx_resource_write_events_resource_latest
    ON resource_write_events (resource_type, resource_id, created_at DESC)
    INCLUDE (source, assistant_name);

-- The write metrics count the content changes of a window that carry no
-- channel: version snapshots with a NULL source. Partial, because those rows
-- stop being written once every write path records its channel, so the index
-- stays the size of the pre-recording history instead of growing with the
-- table.
CREATE INDEX idx_content_versions_team_created_unsourced
    ON content_versions (team_id, created_at DESC)
    WHERE source IS NULL;
