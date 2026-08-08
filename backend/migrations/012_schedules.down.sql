-- Drops the in-process scheduler's schedule persistence (issue #727).
-- Indexes die with the table.
DROP TABLE IF EXISTS schedules;
