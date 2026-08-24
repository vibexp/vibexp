-- ===========================================================================
-- Durable embedding job queue (issue #820).
--
-- The embedding dispatcher used to hold its backlog in an unbounded in-memory
-- FIFO (boundedExecutor), so a restart, a crash, or a shutdown that outlived
-- its drain window discarded every queued job: the entity was left unembedded
-- with no row, no retry and no operator-visible trace beyond a #755 "submitted"
-- log line with no terminal line to close it.
--
-- This table is the system of record for outstanding embedding work. The
-- dispatcher enqueues a row the moment an embeddable event arrives (before any
-- I/O that could be lost), workers LEASE rows out of it, and a terminal outcome
-- acks the row. A lease that expires -- which is what a dead process leaves
-- behind -- simply becomes claimable again, so recovery needs no boot-time
-- sweep: the ordinary claim query is the recovery path.
--
-- payload carries everything generation needs (the normalized title/description/
-- body extracted from the event), because a domain event is not reconstructable
-- from an entity id alone: an entity may have been updated since, and the live
-- pipeline embeds what the event carried.
--
-- There is deliberately NO team_id column. The team is resolved per job from
-- the entity (EmbeddingService.ResolveEntityTeam) and is not known at enqueue
-- time, so a column here could only ever be populated by an extra write nothing
-- reads. entity_id is polymorphic across five entity tables and therefore
-- cannot carry a foreign key either (the same constraint resource_freshness
-- lives with, #762); a job for a deleted entity fails harmlessly at resolve and
-- reaches its terminal state through the ordinary attempt bound.
-- ===========================================================================

CREATE TABLE embedding_jobs (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type      text NOT NULL,
    entity_id        text NOT NULL,
    user_id          text NOT NULL,
    payload          jsonb NOT NULL,
    state            text NOT NULL DEFAULT 'pending'
                     CHECK (state IN ('pending', 'claimed', 'done', 'failed')),
    attempts         integer NOT NULL DEFAULT 0,
    available_at     timestamptz NOT NULL DEFAULT now(),
    claimed_by       text,
    claimed_at       timestamptz,
    lease_expires_at timestamptz,
    last_error       text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE embedding_jobs IS
    'Durable, leased queue of outstanding embedding work (issue #820). Survives a restart; a claim lease that expires is reclaimable.';
COMMENT ON COLUMN embedding_jobs.payload IS
    'Normalized embedding input extracted from the originating event (title/description/body); a domain event cannot be rebuilt from an entity id alone.';
COMMENT ON COLUMN embedding_jobs.attempts IS
    'Incremented at CLAIM time, not at failure: a worker that dies mid-flight never reaches a failure path, so a claim-time counter is what bounds a poison pill.';
COMMENT ON COLUMN embedding_jobs.available_at IS
    'Earliest time this job may be claimed. Pushed forward when a retryable attempt fails, so a failing job backs off instead of hot-looping.';
COMMENT ON COLUMN embedding_jobs.lease_expires_at IS
    'When a claimed job becomes claimable again. A dead process leaves an expired lease, which is what makes restart recovery implicit.';

-- One outstanding job per entity: a re-enqueue of an entity that is still
-- pending or claimed COALESCES onto the existing row (refreshing the payload so
-- the newest content wins) rather than queueing the same work twice. Terminal
-- rows (done/failed) are excluded so the same entity can be embedded again
-- later. The dispatcher's enqueue infers this index by repeating its predicate
-- in ON CONFLICT.
CREATE UNIQUE INDEX idx_embedding_jobs_outstanding_entity
    ON embedding_jobs (entity_type, entity_id)
    WHERE state IN ('pending', 'claimed');

-- Claim-query support. Partial on the non-terminal states, because the
-- claimable set stays small while the table accumulates one row per entity ever
-- embedded; and keyed on the claim query's ORDER BY (oldest first, id as
-- tiebreaker) so the query walks the index in order and stops at its LIMIT
-- instead of sorting the whole claimable set on every poll.
CREATE INDEX idx_embedding_jobs_claimable
    ON embedding_jobs (created_at, id)
    WHERE state IN ('pending', 'claimed');
