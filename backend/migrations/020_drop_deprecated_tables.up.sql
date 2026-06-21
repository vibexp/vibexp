-- Drop deprecated tables that have been replaced by the unified artifacts system
-- These tables are no longer used after DL-74 (frontend removal) and DL-76 (backend removal)

-- Drop work_reports table
DROP TABLE IF EXISTS work_reports CASCADE;

-- Drop static_contexts table
DROP TABLE IF EXISTS static_contexts CASCADE;
