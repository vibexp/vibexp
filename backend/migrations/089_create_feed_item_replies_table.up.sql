CREATE TABLE feed_item_replies (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id           UUID NOT NULL,
    feed_item_id      UUID NOT NULL,
    content           TEXT NOT NULL,
    posted_by_user_id UUID NOT NULL,
    ai_assistant_name VARCHAR(30),
    posted_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_replies_team      FOREIGN KEY (team_id)           REFERENCES teams(id)      ON DELETE CASCADE,
    CONSTRAINT fk_replies_feed_item FOREIGN KEY (feed_item_id)      REFERENCES feed_items(id) ON DELETE CASCADE,
    CONSTRAINT fk_replies_user      FOREIGN KEY (posted_by_user_id) REFERENCES users(id)      ON DELETE CASCADE
);
CREATE INDEX idx_feed_item_replies_posted_at ON feed_item_replies(feed_item_id, posted_at DESC);
