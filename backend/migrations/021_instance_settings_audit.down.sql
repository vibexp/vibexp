-- Drops the instance settings audit log (issue #1187). The index dies with the
-- table. Rolling this back discards the record of every instance settings
-- change made while it was in place -- there is no other store to recover it
-- from.
DROP TABLE IF EXISTS instance_settings_audit;
