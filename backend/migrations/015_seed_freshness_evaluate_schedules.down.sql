-- Remove the freshness evaluation schedules (issue #732).
--
-- This deletes every `freshness_evaluate` row, not only the ones the up
-- migration inserted: they are indistinguishable afterwards (the service
-- writes identical rows), and rolling back #732 removes the handler, so any
-- such row would be a schedule the scheduler can only log as unregistered on
-- every tick.
DELETE FROM schedules WHERE job_type = 'freshness_evaluate';
