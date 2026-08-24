-- Drops the team settings audit log (issue #828). The index dies with the
-- table. Rolling this back discards the record of every settings copy performed
-- while it was in place -- there is no other store to recover it from, which is
-- the point of the table existing (epic #827, decision 8).
DROP TABLE IF EXISTS team_settings_audit;
