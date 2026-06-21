-- Drop webhook_events table
DROP INDEX IF EXISTS idx_webhook_events_processed_at;
DROP INDEX IF EXISTS idx_webhook_events_event_type;
DROP INDEX IF EXISTS idx_webhook_events_team_id;
DROP INDEX IF EXISTS idx_webhook_events_event_id;
DROP TABLE IF EXISTS webhook_events;
