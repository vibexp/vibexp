-- Rollback: Reverse the expired -> archived data migration (best-effort).
-- Caveat: 'archived' rows created after this migration that were never 'expired'
--         cannot be distinguished from migrated ones, so they are all reverted
--         to 'expired'. Acceptable because the down path only runs on rollback to
--         the pre-#1729 enum, which has no 'archived' value.

UPDATE artifacts SET status = 'expired' WHERE status = 'archived';
