-- ===========================================================================
-- Give every team that already has freshness rules a `freshness_evaluate`
-- schedule (issue #732, epic #726).
--
-- The scheduler's timing comes entirely from the `schedules` table (#727), and
-- #732 makes the freshness rule/settings write paths keep that row in step.
-- But a team whose rules were created by #731 BEFORE this shipped -- and which
-- never performs another rule or settings write -- would have no row at all,
-- so its rules would never be evaluated. Only the write path can create the
-- row going forward; this seeds the ones that already exist.
--
-- Scoped to teams with at least one rule (enabled or not), matching exactly
-- when the service keeps the row: a disabled rule set must still be evaluated
-- so the state it produced gets cleared.
--
-- Idempotent by construction: ON CONFLICT DO NOTHING against the
-- (team_id, job_type) unique index leaves any existing row -- and its
-- next_run_at -- untouched, so re-running the migration cannot reset a team's
-- cadence.
--
-- The interval mirrors the service's resolution: the team's stored setting, or
-- the documented default of one day when it stores none (an absent row means
-- "inherit the defaults"). next_run_at = now() makes seeded teams due on the
-- next tick rather than a day after deploy.
-- ===========================================================================

INSERT INTO schedules (team_id, job_type, interval_seconds, next_run_at)
SELECT DISTINCT
    r.team_id,
    'freshness_evaluate',
    COALESCE(s.interval_seconds, 86400),
    now()
FROM freshness_rules r
LEFT JOIN team_freshness_settings s ON s.team_id = r.team_id
ON CONFLICT (team_id, job_type) DO NOTHING;
