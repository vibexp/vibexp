-- Drops the durable embedding job queue (issue #820). Indexes die with the
-- table. Rolling this back returns the dispatcher to an in-memory-only backlog,
-- which is the data-loss window #820 exists to close.
DROP TABLE IF EXISTS embedding_jobs;
