-- Resource comments: team-visible annotations on artifacts, memories, prompts,
-- and blueprints (issue #273, epic #272 "Resource Comments").
--
-- Polymorphic table following the attachments precedent: (resource_type,
-- resource_id) identifies the commented resource. resource_id has NO foreign
-- key -- it spans four tables (artifacts/memories/prompts/blueprints), so its
-- cleanup is app-level (each resource service's delete path removes the
-- resource's comments; cf. embeddings/attachments owner_id). team_id and
-- user_id DO carry FKs (ON DELETE CASCADE) so a comment dies with its team or
-- its author. The table doubles as the comment activity log: "edited" is
-- derived (updated_at > created_at) and the homepage feed orders by latest
-- activity -- there is no separate event/audit table.

CREATE TABLE comments (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    resource_type text NOT NULL CHECK (resource_type IN ('artifact', 'memory', 'prompt', 'blueprint')),
    resource_id   uuid NOT NULL,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    content       text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Per-resource comment list (sidebar widget + "all comments" popup), newest-first.
CREATE INDEX idx_comments_resource
    ON comments (team_id, resource_type, resource_id, created_at DESC);

-- Team-wide "latest activity" ordering for the homepage recent-comments card.
-- GREATEST(created_at, updated_at) treats an edit as fresh activity; on
-- timestamptz columns it is immutable and so index-able.
CREATE INDEX idx_comments_team_activity
    ON comments (team_id, GREATEST(created_at, updated_at) DESC);
