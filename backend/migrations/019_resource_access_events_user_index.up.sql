-- Per-user access analytics for the admin user detail page (#1136).
--
-- resource_access_events was indexed only by created_at and by team_id-leading
-- composites, so a per-user query had to filter a range scan across every
-- user's events. This index makes the per-user series and top-resources queries
-- an index range scan on (user_id, created_at).
--
-- Plain CREATE INDEX (not CONCURRENTLY): migrations run before the server
-- serves, and the table is bounded by retention.access_event_days.
CREATE INDEX IF NOT EXISTS idx_rae_user_created ON public.resource_access_events USING btree (user_id, created_at);
