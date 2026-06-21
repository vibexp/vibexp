CREATE TABLE feed_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    feed_id UUID NOT NULL,
    project_id UUID,                      -- nullable
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,                -- markdown body, max 200 KB enforced at app layer
    excerpt VARCHAR(320) NOT NULL,        -- auto-generated, ~300 chars stripped of MD
    ai_assistant_name VARCHAR(30) NOT NULL,
    posted_by_user_id UUID NOT NULL,      -- the user whose API key/JWT made the call
    archived_at TIMESTAMPTZ,              -- NULL = active, non-NULL = archived
    posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_feed_items_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    CONSTRAINT fk_feed_items_feed FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE,
    CONSTRAINT fk_feed_items_project FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL,
    CONSTRAINT fk_feed_items_user FOREIGN KEY (posted_by_user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_feed_items_team_id ON feed_items(team_id);
CREATE INDEX idx_feed_items_feed_id ON feed_items(feed_id);
CREATE INDEX idx_feed_items_project_id ON feed_items(project_id);
CREATE INDEX idx_feed_items_team_posted_at ON feed_items(team_id, posted_at DESC);
CREATE INDEX idx_feed_items_team_archived_posted ON feed_items(team_id, archived_at, posted_at DESC);
