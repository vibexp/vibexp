-- Revert the artifact type values to their original underscored slugs before
-- dropping the types table, so a rolled-back database matches the pre-106 code.
UPDATE artifacts SET type = 'work_reports'    WHERE type = 'work-reports';
UPDATE artifacts SET type = 'static_contexts' WHERE type = 'static-contexts';

DROP TABLE IF EXISTS types;
