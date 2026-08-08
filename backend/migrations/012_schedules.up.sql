-- ===========================================================================
-- In-process job scheduler: schedule persistence (issue #727, epic #725
-- "In-process job scheduler").
--
-- One row per (team_id, job_type) records WHEN that team's recurring job is
-- next due. The scheduler engine (#728) polls this table for due rows
-- (next_run_at <= now), runs the handler registered for job_type under a pg
-- advisory lock, and advances next_run_at. Features register job handlers in
-- code (the #728 registry owns the valid job_type set), so job_type stays a
-- plain text column -- adding a job never needs a migration.
--
-- interval_seconds carries a hard floor of 3600 (1 hour) enforced by CHECK:
-- the floor prevents abusive tight loops even if a caller above the storage
-- layer is buggy. next_run_at is advanced from the actual ran-at time, not
-- from the previous next_run_at, so a replica that was down does not unleash
-- a thundering herd of catch-up runs -- missed runs are simply skipped.
-- ===========================================================================

CREATE TABLE schedules (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id          uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    job_type         text NOT NULL,
    interval_seconds integer NOT NULL CHECK (interval_seconds >= 3600),
    last_run_at      timestamptz,
    next_run_at      timestamptz NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE schedules IS
    'Per-team recurring-job schedules for the in-process scheduler (epic #725). One row per (team_id, job_type).';
COMMENT ON COLUMN schedules.job_type IS
    'Registered job identifier (e.g. freshness_evaluate); the valid set lives in the scheduler job registry, not the schema.';
COMMENT ON COLUMN schedules.interval_seconds IS
    'Run interval; storage-enforced floor of 3600s (1 hour).';
COMMENT ON COLUMN schedules.next_run_at IS
    'Next due time; advanced from the actual ran-at time so missed runs after downtime are skipped, not caught up.';

-- One schedule per job per team: upsert (ON CONFLICT) relies on this.
CREATE UNIQUE INDEX idx_schedules_team_job
    ON schedules (team_id, job_type);

-- Due-selection: "find schedules where next_run_at <= now()" is an index
-- scan, not a seq scan.
CREATE INDEX idx_schedules_next_run_at
    ON schedules (next_run_at);
