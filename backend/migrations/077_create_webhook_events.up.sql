-- Create webhook_events table for idempotency tracking
-- This prevents duplicate processing of Stripe webhook events
CREATE TABLE IF NOT EXISTS webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(255) NOT NULL UNIQUE,
    event_type VARCHAR(100) NOT NULL,
    processed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    team_id VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Index for fast lookup by event_id (primary idempotency check)
CREATE INDEX IF NOT EXISTS idx_webhook_events_event_id ON webhook_events(event_id);

-- Index for querying by team_id
CREATE INDEX IF NOT EXISTS idx_webhook_events_team_id ON webhook_events(team_id);

-- Index for querying by event_type
CREATE INDEX IF NOT EXISTS idx_webhook_events_event_type ON webhook_events(event_type);

-- Index for cleanup queries (processed_at)
CREATE INDEX IF NOT EXISTS idx_webhook_events_processed_at ON webhook_events(processed_at);

-- Add comment
COMMENT ON TABLE webhook_events IS 'Tracks processed Stripe webhook events to prevent duplicate processing (idempotency)';
COMMENT ON COLUMN webhook_events.event_id IS 'Stripe event ID (evt_xxx) - unique identifier from Stripe';
COMMENT ON COLUMN webhook_events.event_type IS 'Stripe event type (e.g. checkout.session.completed)';
COMMENT ON COLUMN webhook_events.processed_at IS 'When the webhook event was successfully processed';
COMMENT ON COLUMN webhook_events.team_id IS 'Associated team ID if applicable (nullable)';
