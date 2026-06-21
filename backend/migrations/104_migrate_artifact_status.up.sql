-- Migration: Map the retired artifact status 'expired' to the new 'archived'
-- Reason: The artifact lifecycle enum changes from active|expired to
--         active|draft|archived (issue #1729). 'expired' is removed from the
--         contract; existing rows are migrated to 'archived' with no data loss.
-- Note: The status column is an unconstrained VARCHAR(20), so no type/constraint
--       change is required — this is a pure data migration.

UPDATE artifacts SET status = 'archived' WHERE status = 'expired';
