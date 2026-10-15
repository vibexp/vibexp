-- Index a user's write events by time (#1508).
--
-- GET /auth/me reports when the user's AI tools first wrote a resource back
-- (knowledge_loop.first_write_at): MIN(created_at) over this user's
-- resource_write_events from the cli/mcp/api sources. The table's existing
-- indexes lead with team_id or the resource, so without this one that lookup
-- scans every write event on each /auth/me call. The read side already has
-- the matching idx_rae_user_created on resource_access_events.
--
-- Plain CREATE INDEX (not CONCURRENTLY): migrations run before the server
-- serves.
CREATE INDEX IF NOT EXISTS idx_rwe_user_created
    ON resource_write_events USING btree (user_id, created_at);
