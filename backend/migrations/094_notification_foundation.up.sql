CREATE TABLE notifications (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  team_id           UUID REFERENCES teams(id) ON DELETE SET NULL,
  type              VARCHAR(64) NOT NULL,
  category          VARCHAR(16) NOT NULL,
  title             TEXT NOT NULL,
  body              TEXT,
  action_url        TEXT,
  entity_ref        JSONB,
  dedupe_key        VARCHAR(128),
  read_at           TIMESTAMPTZ,
  dismissed_at      TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notifications_user_unread
  ON notifications (recipient_user_id, created_at DESC)
  WHERE read_at IS NULL AND dismissed_at IS NULL;
CREATE UNIQUE INDEX idx_notifications_dedupe
  ON notifications (recipient_user_id, dedupe_key)
  WHERE dedupe_key IS NOT NULL;
CREATE INDEX idx_notifications_created_at ON notifications (created_at);

CREATE TABLE notification_deliveries (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
  channel         VARCHAR(32) NOT NULL,
  status          VARCHAR(32) NOT NULL,
  reason          TEXT,
  attempts        INT NOT NULL DEFAULT 0,
  delivered_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON notification_deliveries (notification_id);

CREATE TABLE notification_digest_queue (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL,
  notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
  scheduled_for   TIMESTAMPTZ NOT NULL,
  sent_at         TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON notification_digest_queue (scheduled_for) WHERE sent_at IS NULL;
